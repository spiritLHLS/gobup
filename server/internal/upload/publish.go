package upload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

func (s *Service) PublishHistory(historyID uint, userID uint) error {
	// 进程内防重锁：阻止同一历史记录被并发投稿（API手动触发 + 自动调度器同时触发等场景）
	if _, loaded := s.publishingHistories.LoadOrStore(historyID, true); loaded {
		log.Printf("[Publish] 历史记录 %d 正在投稿中（已有另一路径持有锁），拒绝并发调用", historyID)
		return fmt.Errorf("该历史记录正在投稿中，请稍后再试")
	}
	defer s.publishingHistories.Delete(historyID)

	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	// 检查当前历史记录是否已经投稿过（防止重复投稿）
	// 关键：此检查必须在 MergeBySession 块之前，否则 MergeBySession=true 时
	// 会绕过此检查直接进入 AppendPartsToExisting，导致已追加过的历史记录被再次追加，
	// 产生 B 站视频分P重复的问题（SyncVideoInfo goroutine 与 room_auto_tasks 并发触发场景）。
	if history.Publish {
		log.Printf("[Publish] 历史记录 %d 已经投稿过，拒绝重复投稿: BvID=%s",
			historyID, history.BvID)
		return fmt.Errorf("该历史记录已经投稿过，不能重复投稿 (BvID: %s)", history.BvID)
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间不存在: %w", err)
	}

	// 核心逻辑：检查同SessionID是否已有投稿（如果启用了合并）
	log.Printf("[投稿] 开始检查SessionID合并 (history_id=%d, room_id=%s, session_id=%s, MergeBySession=%v)",
		historyID, history.RoomID, history.SessionID, room.MergeBySession)

	if room.MergeBySession && history.SessionID != "" {
		log.Printf("[投稿] SessionID合并已启用，查询同SessionID的已投稿记录 (session_id=%s)", history.SessionID)

		var existingHistory models.RecordHistory
		err := db.Where("session_id = ? AND publish = ? AND id != ?",
			history.SessionID, true, historyID).
			First(&existingHistory).Error

		if err == nil {
			// 找到同SessionID的已投稿记录，执行追加分P逻辑
			log.Printf("[投稿] 检测到同SessionID已有投稿 (session_id=%s, existing_history_id=%d, bv_id=%s, aid=%s)，执行追加分P",
				history.SessionID, existingHistory.ID, existingHistory.BvID, existingHistory.AvID)

			return s.AppendPartsToExisting(historyID, &existingHistory, userID)
		}
		// 未找到已投稿记录，继续执行新建投稿逻辑
		log.Printf("[投稿] 同SessionID无已投稿记录，执行新建投稿 (session_id=%s, query_error=%v)", history.SessionID, err)
	} else {
		if !room.MergeBySession {
			log.Printf("[投稿] SessionID合并未启用 (MergeBySession=false)")
		}
		if history.SessionID == "" {
			log.Printf("[投稿] SessionID为空，跳过合并检测")
		}
	}

	// 权限检查1：房间是否启用上传功能（总开关）
	if !room.Upload {
		return fmt.Errorf("房间未启用上传功能，无法投稿")
	}

	// 权限检查2：验证用户ID是否匹配房间配置的上传用户
	if room.UploadUserID != userID {
		log.Printf("[Publish] 用户权限不匹配: 房间配置用户ID=%d, 请求用户ID=%d",
			room.UploadUserID, userID)
		return fmt.Errorf("用户无权操作此房间的投稿，请联系管理员")
	}

	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}

	// 验证Cookie
	valid, err := bili.ValidateCookie(user.Cookies)
	if err != nil || !valid {
		user.Login = false
		db.Save(&user)
		return fmt.Errorf("用户Cookie已失效，请重新登录")
	}

	// 获取所有已上传的分P（必须按start_time ASC排序，确保投稿时分P顺序正确）
	// 如果启用了弹幕烧录，同时包含弹幕版（temp_file_type='danmaku_burn'）作为独立分P一并投稿
	// 其他临时文件类型（切分等）仍然排除
	// 注意：本地文件被删除（file_delete=true）不影响投稿，CID/FileName 仍保存在DB中
	// 查询待投稿的分P：
	// - 条件1：正常原始分P（is_temp_file=false）
	// - 条件2：大文件切分的子分P（temp_file_type='split'）—— 替代超大原始分P投稿
	// - 条件3（仅当启用弹幕烧录时）：弹幕烧录版分P（temp_file_type='danmaku_burn'）
	var parts []models.RecordHistoryPart
	var partsErr error
	if room.EnableDanmakuBurn {
		partsErr = db.Where(
			"history_id = ? AND upload = ? AND (is_temp_file = ? OR temp_file_type = ? OR temp_file_type = ?)",
			historyID, true, false, "danmaku_burn", "split",
		).Order("start_time ASC").Find(&parts).Error
	} else {
		partsErr = db.Where(
			"history_id = ? AND upload = ? AND (is_temp_file = ? OR temp_file_type = ?)",
			historyID, true, false, "split",
		).Order("start_time ASC").Find(&parts).Error
	}
	if partsErr != nil {
		return fmt.Errorf("查询分P失败: %w", partsErr)
	}

	if len(parts) == 0 {
		return fmt.Errorf("没有已上传的分P")
	}

	// 构建模板数据（优先使用历史记录中的实际数据）
	templateData := map[string]interface{}{
		"uname":     history.Uname, // 使用历史记录中实际的主播名
		"title":     history.Title, // 使用历史记录中实际的直播标题
		"roomId":    history.RoomID,
		"areaName":  history.AreaName, // 使用历史记录中实际的分区名称
		"startTime": history.StartTime,
		"uid":       user.UID,
	}

	// 使用模板服务渲染
	title := s.templateSvc.RenderTitle(room.TitleTemplate, templateData)
	desc := s.templateSvc.RenderDescription(room.DescTemplate, templateData)
	dynamic := s.templateSvc.RenderDynamic(room.DynamicTemplate, templateData) // 动态模板
	tags := s.templateSvc.BuildTags(room.Tags, templateData)
	tagsStr := strings.Join(tags, ",")

	tid := room.TID
	if tid == 0 {
		tid = 171 // 默认分区：电子竞技
	}

	// 创建客户端
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)

	// 获取封面
	coverURL := room.CoverURL
	coverType := room.CoverType

	// 处理不同类型的封面
	if coverType == "diy" && coverURL != "" {
		// 自定义封面：直接使用用户提供的URL
		log.Printf("使用自定义封面URL: %s", coverURL)
	} else if coverType == "live" && len(parts) > 0 {
		// 使用直播首帧：从录制文件查找封面图片并上传
		// 根据直播标题查找同一房间内最早录制的封面文件
		// 查找同一房间、同一直播标题的最早一次录制分P
		var oldestPart models.RecordHistoryPart
		err := db.Where("room_id = ? AND live_title = ?", history.RoomID, history.Title).
			Order("start_time ASC").
			First(&oldestPart).Error

		if err == nil && oldestPart.FilePath != "" {
			// 使用最早录制的分P文件路径查找封面
			basePath := strings.TrimSuffix(oldestPart.FilePath, filepath.Ext(oldestPart.FilePath))
			log.Printf("找到同标题最早录制: %s (开始时间: %s)", oldestPart.FilePath, oldestPart.StartTime)

			// 尝试多种封面文件格式
			coverPaths := []string{
				basePath + ".cover.jpg",
				basePath + ".jpg",
				basePath + ".cover.png",
				basePath + ".png",
			}

			for _, coverPath := range coverPaths {
				if _, err := os.Stat(coverPath); err == nil {
					// 找到封面文件，上传到B站
					coverData, err := os.ReadFile(coverPath)
					if err == nil {
						log.Printf("找到封面文件: %s", coverPath)
						uploadedURL, err := client.UploadCover(coverData)
						if err == nil {
							coverURL = uploadedURL
							log.Printf("封面上传成功: %s", coverURL)
							break
						} else {
							log.Printf("封面上传失败: %v", err)
						}
					}
				}
			}
		} else {
			log.Printf("未找到同标题的历史录制，尝试使用当前录制的封面")
			// 如果没有找到同标题的历史录制，使用当前录制的第一个分P
			firstPartPath := parts[0].FilePath
			basePath := strings.TrimSuffix(firstPartPath, filepath.Ext(firstPartPath))

			coverPaths := []string{
				basePath + ".cover.jpg",
				basePath + ".jpg",
				basePath + ".cover.png",
				basePath + ".png",
			}

			for _, coverPath := range coverPaths {
				if _, err := os.Stat(coverPath); err == nil {
					coverData, err := os.ReadFile(coverPath)
					if err == nil {
						log.Printf("找到封面文件: %s", coverPath)
						uploadedURL, err := client.UploadCover(coverData)
						if err == nil {
							coverURL = uploadedURL
							log.Printf("封面上传成功: %s", coverURL)
							break
						} else {
							log.Printf("封面上传失败: %v", err)
						}
					}
				}
			}
		}

		if coverURL == "live" {
			// 如果没找到封面文件，使用默认或从视频截取
			coverURL = ""
			log.Printf("未找到封面文件，将使用默认封面或从视频截取")
		}
	} else {
		// 默认：不使用封面或从视频截取
		coverURL = ""
	}

	// 构建分P信息（parts已按start_time ASC排序，循环按时间顺序处理）
	var videoParts []bili.PublishVideoPartRequest
	log.Printf("开始构建%d个分P的投稿信息（按录制时间顺序）", len(parts))
	for i, part := range parts {
		// 为分P标题模板构建数据，包含所有可用变量
		partTemplateData := map[string]interface{}{
			"index":     i + 1,
			"startTime": part.StartTime,
			"areaName":  part.AreaName,
			"uname":     history.Uname,  // 主播名
			"title":     history.Title,  // 直播标题
			"roomId":    history.RoomID, // 房间号
			"fileName":  part.FileName,  // 文件名
		}
		partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)
		// 弹幕烧录版分P：在标题末尾附加「弹幕版」标识，明确区分 P1(原始) / P2(弹幕)
		if part.IsTempFile && part.TempFileType == "danmaku_burn" {
			partTitle = partTitle + "（弹幕版）"
		}

		// 获取文件名：优先使用数据库中的 FileName（从上传响应获取），如果为空则从 FilePath 提取
		filename := part.FileName
		if filename == "" {
			// 兼容旧数据：从文件路径提取文件名（不含扩展名）
			baseName := filepath.Base(part.FilePath)
			if ext := filepath.Ext(baseName); ext != "" {
				filename = baseName[:len(baseName)-len(ext)]
			} else {
				filename = baseName
			}
			log.Printf("警告: 分P[%d]的FileName为空，从FilePath提取: %s", i, filename)
		}

		// 调试日志：检查关键参数
		log.Printf("构建分P[%d]: filename=%s, cid=%d", i, filename, part.CID)

		// 检查CID是否为0（参考biliupforjava实现）
		// 原来CID=0时会在当前goroutine中同步执行完整上传流程（可能耗时数小时），
		// 导致整个投稿调用链阻塞，进而阻塞上传队列中所有其他任务。
		// 现在改为: 重置分P上传状态后返回错误，让自动上传调度器在下一轮（10分钟内）重新上传，
		// 上传完成后 checkAndPublish 会再次触发投稿。
		var cid int64
		if part.CID > 0 {
			cid = int64(part.CID)
		} else {
			log.Printf("检测到分P[%d]的CID为0（数据异常），重置上传状态等待自动重传: part_id=%d, file=%s", i, part.ID, part.FilePath)

			// 重置上传状态，让自动上传调度器在下次轮询时重新上传
			db.Model(&part).Updates(map[string]interface{}{
				"upload":             false,
				"uploading":          false,
				"file_name":          "",
				"c_id":               0,
				"upload_retry_count": 0,
				"upload_error_msg":   "CID为0，数据异常，已重置等待自动重传",
			})
			return fmt.Errorf("分P[%d](part_id=%d)的CID为0，已重置上传状态，自动上传调度器将在10分钟内重传，请稍后重试投稿", i, part.ID)
		}

		videoParts = append(videoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: filename,
			Cid:      cid,
		})
	}

	// 打印最终的分P列表，确认顺序正确
	log.Printf("投稿分P列表（共%d个，按录制时间顺序）:", len(videoParts))
	for i, vp := range videoParts {
		log.Printf("  分P[%d]: %s (CID=%d)", i, vp.Title, vp.Cid)
	}

	// 处理转载来源
	source := ""
	if room.Copyright == 2 {
		// 使用模板生成转载来源
		sourceTemplate := room.SourceTemplate
		if sourceTemplate == "" {
			sourceTemplate = "直播间: https://live.bilibili.com/${roomId}  稿件直播源"
		}
		source = s.templateSvc.RenderTitle(sourceTemplate, templateData)
	}

	// 投稿，同时获取AID和BV号
	avID, bvid, err := client.PublishVideo(title, desc, tagsStr, tid, room.Copyright, coverURL, videoParts, source)
	if err != nil {
		// 检查是否是验证码错误
		captchaService := services.NewCaptchaService()
		if captchaService.IsCaptchaError(err.Error()) {
			log.Printf("检测到验证码错误: %v", err)
			history.Message = "投稿失败: 需要验证码验证"
			db.Save(&history)

			// 加入重试队列
			captchaService.HandleCaptchaError(historyID, userID, err.Error())
			return fmt.Errorf("需要验证码验证，已加入重试队列")
		}

		history.Message = fmt.Sprintf("投稿失败: %v", err)
		db.Save(&history)
		return fmt.Errorf("投稿失败: %w", err)
	}

	// 检查返回的AID和BVID是否有效
	if avID == 0 || bvid == "" {
		log.Printf("警告: 投稿API返回的AID或BVID为空 (AID=%d, BVID=%s)", avID, bvid)
		history.Message = "投稿失败: 返回数据无效"
		db.Save(&history)
		return fmt.Errorf("投稿失败: API返回的AID或BVID为空")
	}

	// 更新历史记录
	history.AvID = fmt.Sprintf("%d", avID)

	// 检查BV号格式，如果格式错误则通过aid从B站API获取正确的BV号
	if !strings.HasPrefix(bvid, "BV") || len(bvid) != 12 {
		log.Printf("警告: API返回的BV号格式错误: %s, 使用AID=%d从视频信息接口获取正确BV号", bvid, avID)

		// 等待一下，让B站处理完投稿
		time.Sleep(2 * time.Second)

		// 通过aid获取视频信息来获取正确的BV号
		videoInfo, err := client.GetVideoInfoByAid(avID)
		if err != nil {
			log.Printf("警告: 从视频信息接口获取BV号失败: %v, 尝试使用算法转换", err)
			// 如果API调用失败，使用算法转换作为后备方案
			bvid = Av2Bv(avID)
		} else {
			bvid = videoInfo.Bvid
			log.Printf("✓ 从视频信息接口获取到正确的BV号: %s", bvid)
		}
	}

	history.BvID = bvid
	history.Publish = true
	history.Message = "投稿成功"
	// 注意：投稿后不修改UploadStatus，保持为2（已上传）
	db.Save(&history)

	// 如果弹幕烧录功能已开启，标记此次一起投稿的弹幕版分P 为 appended_to_video=true。
	// 防止 AppendDanmakuBurnedPartsToApprovedVideos 定时任务在审核通过后再次追加已经包含在初始投稿里的弹幕版分P。
	if room.EnableDanmakuBurn {
		affected := db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ?",
			historyID, true, "danmaku_burn", true,
		).Update("appended_to_video", true)
		if affected.RowsAffected > 0 {
			log.Printf("[投稿成功] 已标记 %d 个弹幕版分P 为 appended_to_video=true (history_id=%d)", affected.RowsAffected, historyID)
		}
	}

	log.Printf("投稿成功: AV%d, BV%s", avID, bvid)

	// 兜底检测机制：使用新的API验证投稿是否真的成功
	// 等待3秒让B站后台处理完成
	time.Sleep(3 * time.Second)

	log.Printf("开始兜底检测：验证视频是否在用户投稿列表中 (mid=%d, aid=%d, bvid=%s)", user.UID, avID, bvid)
	exists, checkErr := client.CheckVideoExistsInArchive(user.UID, avID, bvid)
	if checkErr != nil {
		log.Printf("兜底检测失败（API调用错误）: %v，但投稿API已返回成功，继续后续流程", checkErr)
	} else if !exists {
		log.Printf("兜底检测未找到视频！投稿可能失败，但投稿API已返回成功。建议手动检查：https://space.bilibili.com/%d", user.UID)
		// 不返回错误，只记录日志，避免误报
		// 因为新投稿可能需要更长时间才能在列表中显示
	} else {
		log.Printf("✓ 兜底检测通过：视频已在用户投稿列表中")
	}

	// 加入合集
	if room.SeasonID > 0 && len(videoParts) > 0 {
		// 使用第一个分P的CID
		cid := videoParts[0].Cid
		if err := client.AddToSeason(room.SeasonID, avID, cid, title); err != nil {
			log.Printf("加入合集失败: %v", err)
		} else {
			log.Printf("加入合集成功: SeasonID=%d, AID=%d", room.SeasonID, avID)
		}
	}

	// 创建视频同步任务
	syncService := services.NewVideoSyncService()
	if err := syncService.CreateSyncTask(historyID); err != nil {
		log.Printf("创建同步任务失败: %v", err)
	}

	// 推送通知（使用历史记录中实际的主播名）
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "投稿") {
		s.wxPusher.NotifyPublishSuccess(room.UploadUserID, room.Wxuid, history.Uname, title, history.BvID)
	}

	// 发送动态
	if dynamic != "" {
		// 替换动态中的bvid变量
		dynamicWithBv := strings.ReplaceAll(dynamic, "${bvid}", history.BvID)
		if err := client.SendDynamic(dynamicWithBv); err != nil {
			log.Printf("发送动态失败: %v", err)
		} else {
			log.Printf("发送动态成功: %s", dynamicWithBv)
		}
	}

	// 触发“投稿成功后”文件操作
	fileMoverSvc := services.NewFileMoverService()
	if err := fileMoverSvc.TriggerFileOp(historyID, &room, services.FileOpTriggerAfterPublish); err != nil {
		log.Printf("文件处理失败: %v", err)
	}

	// 注意：临时文件（切分文件、弹幕烧录文件等）的清理已移至审核通过后执行
	// 见 videosync.go 中的审核通过逻辑

	// 如果启用高能剪辑，创建高能剪辑任务
	if room.HighEnergyCut {
		go func() {
			log.Printf("开始高能剪辑: history_id=%d", historyID)
			highEnergySvc := services.NewHighEnergyCutService()
			outputFile, err := highEnergySvc.CutHighEnergySegments(historyID)
			if err != nil {
				log.Printf("高能剪辑失败: %v", err)
				return
			}
			log.Printf("高能剪辑完成: %s", outputFile)
			// TODO: 自动上传高能剪辑版本
		}()
	}

	return nil
}

// bv2avLocal 将 BV 号转换为 AV 号（upload 包内部使用，避免跨包循环导入）
func bv2avLocal(bv string) int64 {
	const (
		xorCode  = int64(23442827791579)
		maskCode = int64(2251799813685247)
		base     = 58
		alphabet = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	)
	if len(bv) != 12 || bv[:2] != "BV" {
		return 0
	}
	charMap := make(map[byte]int64)
	for i, c := range alphabet {
		charMap[byte(c)] = int64(i)
	}
	bytes := []byte(bv)
	bytes[3], bytes[9] = bytes[9], bytes[3]
	bytes[4], bytes[7] = bytes[7], bytes[4]
	var tmp int64
	for i := 2; i < len(bytes); i++ {
		tmp = tmp*base + charMap[bytes[i]]
	}
	return (tmp ^ xorCode) & maskCode
}

// Av2Bv 将AV号转换为BV号
// 算法参考: https://github.com/SocialSisterYi/bilibili-API-collect
func Av2Bv(av int64) string {
	const (
		xorCode  = int64(23442827791579)
		maskCode = int64(2251799813685247)
		maxAid   = int64(1) << 51
		base     = 58
		alphabet = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	)

	bytes := []byte{'B', 'V', '1', '0', '0', '0', '0', '0', '0', '0', '0', '0'}
	bvIndex := len(bytes) - 1
	tmp := (maxAid | av) ^ xorCode

	for tmp > 0 {
		bytes[bvIndex] = alphabet[tmp%base]
		tmp /= base
		bvIndex--
	}

	// 交换特定位置的字符
	bytes[3], bytes[9] = bytes[9], bytes[3]
	bytes[4], bytes[7] = bytes[7], bytes[4]

	return string(bytes)
}

// GetSeasons 获取合集列表
func (s *Service) GetSeasons(userID uint) ([]bili.Season, error) {
	db := database.GetDB()

	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return nil, fmt.Errorf("用户未登录")
	}

	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	return client.GetSeasons()
}

// AppendPartsToExisting 追加分P到已有投稿（同SessionID合并）
func (s *Service) AppendPartsToExisting(newHistoryID uint, existingHistory *models.RecordHistory, userID uint) error {
	db := database.GetDB()

	log.Printf("[追加分P] 开始追加: new_history=%d -> existing_aid=%s", newHistoryID, existingHistory.AvID)

	// 获取新历史记录
	var newHistory models.RecordHistory
	if err := db.First(&newHistory, newHistoryID).Error; err != nil {
		return fmt.Errorf("新历史记录不存在: %w", err)
	}

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", newHistory.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间不存在: %w", err)
	}

	// 获取用户信息
	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}

	// 验证Cookie
	valid, err := bili.ValidateCookie(user.Cookies)
	if err != nil || !valid {
		user.Login = false
		db.Save(&user)
		return fmt.Errorf("用户Cookie已失效，请重新登录")
	}

	// 获取已存在投稿的所有原始分P（仅包含 publish=true 且非临时文件的分P）
	// 获取已投稿视频的所有分P（非临时文件 OR 切分子分P，排除弹幕版以免对B站选P重复）
	// - record_histories.publish=true：只统计已成功投稿/追加的历史记录分P，与B站视频当前状态保持一致
	// - file_delete 不过滤：本地文件删除不影响CID/FileName，已上传记录仍有效
	var existingParts []models.RecordHistoryPart
	if err := db.Joins("JOIN record_histories ON record_history_parts.history_id = record_histories.id").
		Where("record_histories.session_id = ? AND record_histories.publish = ? AND record_history_parts.upload = ? AND (record_history_parts.is_temp_file = ? OR record_history_parts.temp_file_type = ?)",
			existingHistory.SessionID, true, true, false, "split").
		Order("record_history_parts.start_time ASC").
		Find(&existingParts).Error; err != nil {
		return fmt.Errorf("查询已存在分P失败: %w", err)
	}

	// 获取新历史记录的已上传分P（原始分P + 切分子分P，排除弹幕烧录版临时文件避免重复）
	// file_delete 不过滤：本地文件删除后CID/FileName仍有效，投稿时使用DB中保存的服务端文件名
	var newParts []models.RecordHistoryPart
	if err := db.Where(
		"history_id = ? AND upload = ? AND (is_temp_file = ? OR temp_file_type = ?)",
		newHistoryID, true, false, "split").
		Order("start_time ASC").
		Find(&newParts).Error; err != nil {
		return fmt.Errorf("查询新分P失败: %w", err)
	}

	if len(newParts) == 0 {
		return fmt.Errorf("没有可追加的分P")
	}

	log.Printf("[追加分P] 已存在分P数: %d, 新增分P数: %d", len(existingParts), len(newParts))

	// 构建模板数据
	templateData := map[string]interface{}{
		"uname":     existingHistory.Uname,
		"title":     existingHistory.Title,
		"roomId":    existingHistory.RoomID,
		"areaName":  existingHistory.AreaName,
		"startTime": existingHistory.StartTime,
		"uid":       user.UID,
	}

	// 获取原投稿信息
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)

	// 从已存在的History获取AID；若 AvID 为空（旧数据兼容），尝试从 BvID 转换
	aidInt, parseErr := strconv.ParseInt(existingHistory.AvID, 10, 64)
	if parseErr != nil || aidInt == 0 {
		if existingHistory.BvID != "" {
			aidInt = bv2avLocal(existingHistory.BvID)
		}
		if aidInt == 0 {
			return fmt.Errorf("解析AID失败，AvID=%q BvID=%q 均无法解析", existingHistory.AvID, existingHistory.BvID)
		}
		// 顺手修复数据库中的 AvID
		db.Model(existingHistory).Update("av_id", fmt.Sprintf("%d", aidInt))
		log.Printf("[追加分P] 补充 AvID: history_id=%d, avID=%d", existingHistory.ID, aidInt)
	}

	// 获取原视频稿件详细信息（包含desc, tag, copyright, source）
	archiveDetail, err := client.GetArchiveDetailByAid(aidInt)
	if err != nil {
		return fmt.Errorf("获取原视频信息失败: %w", err)
	}

	// 合并分P列表
	var allVideoParts []bili.PublishVideoPartRequest

	// 1. 添加已存在的分P
	for i, part := range existingParts {
		partTemplateData := map[string]interface{}{
			"index":     i + 1,
			"startTime": part.StartTime,
			"areaName":  part.AreaName,
			"uname":     existingHistory.Uname,
			"title":     existingHistory.Title,
			"roomId":    existingHistory.RoomID,
			"fileName":  part.FileName,
		}
		partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)

		allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: part.FileName,
			Cid:      int64(part.CID),
		})
	}

	// 2. 添加新分P
	startIndex := len(existingParts)
	for i, part := range newParts {
		partTemplateData := map[string]interface{}{
			"index":     startIndex + i + 1,
			"startTime": part.StartTime,
			"areaName":  part.AreaName,
			"uname":     newHistory.Uname,
			"title":     newHistory.Title,
			"roomId":    newHistory.RoomID,
			"fileName":  part.FileName,
		}
		partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)

		allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: part.FileName,
			Cid:      int64(part.CID),
		})
	}

	log.Printf("[追加分P] 合并后总分P数: %d", len(allVideoParts))

	// 使用原视频的信息进行编辑
	title := archiveDetail.Archive.Title
	desc := archiveDetail.Archive.Desc
	tags := strings.Join(archiveDetail.Archive.Tag, ",")
	tid := archiveDetail.Archive.Tid
	copyright := archiveDetail.Archive.Copyright
	cover := archiveDetail.Archive.Pic

	// 处理转载来源
	source := archiveDetail.Archive.Source
	if source == "" && copyright == 2 {
		sourceTemplate := room.SourceTemplate
		if sourceTemplate == "" {
			sourceTemplate = "直播间: https://live.bilibili.com/${roomId}  稿件直播源"
		}
		source = s.templateSvc.RenderTitle(sourceTemplate, templateData)
	}

	// 调用编辑API追加分P
	log.Printf("[追加分P] 调用EditVideo API: aid=%d, 总分P=%d", aidInt, len(allVideoParts))
	if err := client.EditVideo(aidInt, title, desc, tags, tid, copyright, cover, allVideoParts, source); err != nil {
		return fmt.Errorf("追加分P失败: %w", err)
	}

	log.Printf("[追加分P] 追加成功: aid=%d, bvid=%s", aidInt, existingHistory.BvID)

	// 更新新历史记录的投稿状态，指向同一个投稿
	newHistory.Publish = true
	newHistory.BvID = existingHistory.BvID
	newHistory.AvID = existingHistory.AvID
	newHistory.Message = fmt.Sprintf("已追加到 %s", existingHistory.BvID)
	db.Save(&newHistory)

	// 清理临时文件
	burnService := services.NewDanmakuBurnService()
	if err := burnService.CleanTempFiles(newHistoryID); err != nil {
		log.Printf("[临时文件清理] 清理失败: %v", err)
	}

	// 推送通知
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "投稿") {
		s.wxPusher.NotifyPublishSuccess(room.UploadUserID, room.Wxuid, newHistory.Uname,
			fmt.Sprintf("%s (追加%d个分P)", title, len(newParts)), existingHistory.BvID)
	}

	log.Printf("[追加分P] 完成: new_history=%d 已追加到 %s (%d个新分P)",
		newHistoryID, existingHistory.BvID, len(newParts))

	// 自动解析新追加分P的弹幕（如果房间启用了自动解析弹幕）
	if room.AutoParseDanmaku {
		log.Printf("[追加分P] 房间启用了自动解析弹幕，开始解析新分P的弹幕: %d个分P", len(newParts))
		danmakuParser := services.NewDanmakuXMLParser()

		for _, part := range newParts {
			// 检查是否有对应的XML文件
			xmlPath := strings.TrimSuffix(part.FilePath, filepath.Ext(part.FilePath)) + ".xml"
			if _, err := os.Stat(xmlPath); err == nil {
				log.Printf("[追加分P] 开始解析弹幕: part_id=%d, xml=%s", part.ID, xmlPath)
				if count, err := danmakuParser.ParseDanmakuFile(xmlPath, newHistory.SessionID, &room, part.ID); err != nil {
					log.Printf("[追加分P] 弹幕解析失败: %v", err)
				} else {
					log.Printf("[追加分P] 弹幕解析成功: part_id=%d, 解析到%d条弹幕", part.ID, count)

					// 更新历史记录的弹幕数量
					var totalDanmakuCount int64
					db.Model(&models.LiveMsg{}).Where("session_id = ?", newHistory.SessionID).Count(&totalDanmakuCount)

					// 更新所有同SessionID的历史记录的弹幕数
					db.Model(&models.RecordHistory{}).Where("session_id = ?", newHistory.SessionID).Update("danmaku_count", totalDanmakuCount)
					log.Printf("[追加分P] 已更新SessionID %s 的弹幕总数: %d", newHistory.SessionID, totalDanmakuCount)
				}
			} else {
				log.Printf("[追加分P] 未找到弹幕文件，跳过: part_id=%d, xml=%s", part.ID, xmlPath)
			}
		}
	}

	return nil
}

// UpdatePublishedVideoWithBurnedParts 回补更新已投稿视频，追加弹幕版分P
// 当弹幕版分P上传完成后，检查对应的历史记录是否已投稿，如果已投稿且没有弹幕版，则追加弹幕版分P
func (s *Service) UpdatePublishedVideoWithBurnedParts(burnedPartID uint) error {
	// 内存级防重入：防止 Path A（上传完成立即调用）与 Path B/C（定时任务发现 appended_to_video=false）并发调用
	// 导致同一弹幕版分P 在 B 站视频里出现两次
	if _, loaded := s.appendingBurnedParts.LoadOrStore(burnedPartID, true); loaded {
		log.Printf("[回补弹幕版] 烧录版分P %d 正在追加中，跳过并发调用", burnedPartID)
		return nil
	}
	defer s.appendingBurnedParts.Delete(burnedPartID)

	db := database.GetDB()

	// 获取弹幕版分P
	var burnedPart models.RecordHistoryPart
	if err := db.First(&burnedPart, burnedPartID).Error; err != nil {
		return fmt.Errorf("弹幕版分P不存在: %w", err)
	}

	// 检查是否为弹幕烧录产生的临时文件
	if !burnedPart.IsTempFile || burnedPart.TempFileType != "danmaku_burn" {
		log.Printf("[回补弹幕版] 跳过：不是弹幕烧录分P (part_id=%d)", burnedPartID)
		return nil
	}

	// 检查是否已上传
	if !burnedPart.Upload {
		log.Printf("[回补弹幕版] 跳过：弹幕版尚未上传完成 (part_id=%d)", burnedPartID)
		return nil
	}

	// 获取对应的历史记录
	var history models.RecordHistory
	if err := db.First(&history, burnedPart.HistoryID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	// 检查是否已投稿
	if !history.Publish || history.AvID == "" {
		log.Printf("[回补弹幕版] 跳过：历史记录尚未投稿 (history_id=%d)", history.ID)
		return nil
	}

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间不存在: %w", err)
	}

	// 获取用户信息
	var user models.BiliBiliUser
	if err := db.First(&user, room.UploadUserID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}

	log.Printf("[回补弹幕版] 开始处理: history_id=%d, aid=%s, burned_part_id=%d",
		history.ID, history.AvID, burnedPartID)

	// 先从B站API获取视频当前状态，以准确判断弹幕版是否已追加过
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	aidInt, err := strconv.ParseInt(history.AvID, 10, 64)
	if err != nil {
		return fmt.Errorf("解析AID失败: %w", err)
	}

	archiveDetail, err := client.GetArchiveDetailByAid(aidInt)
	if err != nil {
		return fmt.Errorf("获取原视频信息失败: %w", err)
	}

	// 使用B站API返回的实际分P列表判断弹幕版CID是否已在视频中
	// 不能依赖DB中 upload=true 的记录来判断，因为 upload=true 只表示文件已上传到CDN，不代表已提交到视频
	for _, v := range archiveDetail.Videos {
		if v.CID == burnedPart.CID {
			log.Printf("[回补弹幕版] 跳过：弹幕版CID=%d 已在B站视频中 (part_id=%d)", burnedPart.CID, burnedPartID)
			return nil
		}
	}

	// 构建更新后的分P列表：以B站API返回的当前分P列表为基础，追加弹幕版
	// 这样可以确保与B站实际状态严格一致，避免遗漏或重复
	var allVideoParts []bili.PublishVideoPartRequest
	for i, v := range archiveDetail.Videos {
		// 尝试从DB查找对应记录以获取 PartTitle 模板所需元数据，找不到则使用B站返回的 part 名
		partTitle := v.Part
		if room.PartTitleTemplate != "" {
			var dbPart models.RecordHistoryPart
			if dbErr := db.Where("c_id = ? AND file_delete = ?", v.CID, false).First(&dbPart).Error; dbErr == nil {
				partTemplateData := map[string]interface{}{
					"index":     i + 1,
					"startTime": dbPart.StartTime,
					"areaName":  dbPart.AreaName,
					"uname":     history.Uname,
					"title":     history.Title,
					"roomId":    history.RoomID,
					"fileName":  dbPart.FileName,
				}
				partTitle = s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)
			}
		}
		allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: v.Filename,
			Cid:      v.CID,
		})
	}

	// 追加弹幕版分P
	partTemplateData := map[string]interface{}{
		"index":     len(allVideoParts) + 1,
		"startTime": burnedPart.StartTime,
		"areaName":  burnedPart.AreaName,
		"uname":     history.Uname,
		"title":     history.Title,
		"roomId":    history.RoomID,
		"fileName":  burnedPart.FileName,
	}
	partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData) + "（弹幕版）"
	allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
		Title:    partTitle,
		Desc:     "",
		Filename: burnedPart.FileName,
		Cid:      int64(burnedPart.CID),
	})

	log.Printf("[回补弹幕版] 更新后总分P数: %d (B站现有%d + 弹幕版1)",
		len(allVideoParts), len(archiveDetail.Videos))

	// 使用原视频的信息进行编辑
	title := archiveDetail.Archive.Title
	desc := archiveDetail.Archive.Desc
	tags := strings.Join(archiveDetail.Archive.Tag, ",")
	tid := archiveDetail.Archive.Tid
	copyright := archiveDetail.Archive.Copyright
	cover := archiveDetail.Archive.Pic
	source := archiveDetail.Archive.Source

	// 调用EditVideo API追加弹幕版分P
	log.Printf("[回补弹幕版] 调用EditVideo API: aid=%d, 总分P=%d", aidInt, len(allVideoParts))
	if err := client.EditVideo(aidInt, title, desc, tags, tid, copyright, cover, allVideoParts, source); err != nil {
		errStr := err.Error()
		// code=21588: 该文件内容无法被识别（Bilibili CDN上的文件损坏/无效）
		// code=21054: 视频文件不存在（CDN上已被清除）
		// 这类错误不可通过重试恢复，需要删除损坏的烧录记录并重新触发一次完整的烧录+上传流程
		if strings.Contains(errStr, "code=21588") || strings.Contains(errStr, "code=21054") {
			log.Printf("[回补弹幕版] 检测到不可恢复的CDN文件错误 (burned_part_id=%d): %v", burnedPartID, err)
			log.Printf("[回补弹幕版] 将删除损坏的烧录记录，下次回补检查周期将重新触发烧录 (source_part_id=%d)", burnedPart.SourcePartID)
			// 删除本地烧录文件（如果还在磁盘上）
			if burnedPart.FilePath != "" {
				if removeErr := os.Remove(burnedPart.FilePath); removeErr != nil && !os.IsNotExist(removeErr) {
					log.Printf("[回补弹幕版] 删除损坏烧录文件失败: %v", removeErr)
				} else {
					log.Printf("[回补弹幕版] 已删除损坏烧录文件: %s", burnedPart.FilePath)
				}
			}
			// 硬删除DB记录（不用软删除，避免 anyBurnedCount 仍能查到）
			if dbErr := db.Unscoped().Delete(&burnedPart).Error; dbErr != nil {
				log.Printf("[回补弹幕版] 删除损坏烧录DB记录失败: burned_part_id=%d, err=%v", burnedPartID, dbErr)
			} else {
				log.Printf("[回补弹幕版] 已删除损坏烧录DB记录: burned_part_id=%d", burnedPartID)
			}
			return fmt.Errorf("追加弹幕版分P失败(CDN文件损坏，已重置等待重新烧录): %w", err)
		}
		return fmt.Errorf("追加弹幕版分P失败: %w", err)
	}

	log.Printf("[回补弹幕版] 更新成功: aid=%d, bvid=%s, 已追加弹幕版分P", aidInt, history.BvID)

	// 标记弹幕版分P为已追加到视频
	burnedPart.AppendedToVideo = true
	db.Save(&burnedPart)
	log.Printf("[回补弹幕版] 已标记 AppendedToVideo=true: burned_part_id=%d", burnedPart.ID)

	// 清理已追加的弹幕烧录视频文件（物理文件已无需保留）
	if burnedPart.SessionID != "" {
		burnService := services.NewDanmakuBurnService()
		_ = burnService.CleanAppendedBurnedPartsBySessionID(burnedPart.SessionID)
	}

	// 更新历史记录的消息
	history.Message = fmt.Sprintf("投稿成功（已追加弹幕版）")
	db.Save(&history)

	// 推送通知
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "投稿") {
		s.wxPusher.NotifyPublishSuccess(room.UploadUserID, room.Wxuid, history.Uname,
			fmt.Sprintf("%s (已追加弹幕版)", title), history.BvID)
	}

	return nil
}

// AppendDanmakuBurnedPartsToApprovedVideos 定时任务入口：
// 对所有已审核通过（video_state=1）且 approved_at 距今超过 1 小时的投稿，
// 检查各原始分P是否还有未追加的弹幕烧录版。如果原始录制视频文件和对应的
// XML 弹幕文件仍然存在，则触发烧录 → 上传 → 追加到 B 站视频的完整流程。
func (s *Service) AppendDanmakuBurnedPartsToApprovedVideos() error {
	db := database.GetDB()

	// 只处理启用了弹幕烧录功能的房间
	var rooms []models.RecordRoom
	if err := db.Where("enable_danmaku_burn = ? AND upload = ?", true, true).Find(&rooms).Error; err != nil {
		return fmt.Errorf("[弹幕回补] 查询房间失败: %w", err)
	}

	if len(rooms) == 0 {
		return nil
	}

	oneHourAgo := time.Now().Add(-1 * time.Hour)

	for _, room := range rooms {
		if room.UploadUserID == 0 {
			continue
		}

		// 查找该房间已投稿、已审核通过且 approved_at 至少 1 小时前的历史记录
		var histories []models.RecordHistory
		if err := db.Where(
			"room_id = ? AND publish = ? AND video_state = ? AND approved_at IS NOT NULL AND approved_at <= ?",
			room.RoomID, true, 1, oneHourAgo,
		).Find(&histories).Error; err != nil {
			log.Printf("[弹幕回补] 查询房间 %s 的历史记录失败: %v", room.RoomID, err)
			continue
		}

		if len(histories) == 0 {
			continue
		}

		log.Printf("[弹幕回补] 房间 %s 发现 %d 条符合弹幕回补条件的历史记录", room.RoomID, len(histories))

		for _, history := range histories {
			s.appendBurnedPartsForApprovedHistory(&history, &room)
		}
	}

	return nil
}

// appendBurnedPartsForApprovedHistory 对单条已审核通过的历史记录执行弹幕回补逻辑。
// 遍历该历史记录下的每个原始分P，若满足以下所有条件则异步触发烧录+上传+追加：
//  1. 原始视频文件还在磁盘上
//  2. 同名 .xml 弹幕文件存在
//  3. 该分P尚未有对应的已追加弹幕版（appended_to_video=false 或无记录）
func (s *Service) appendBurnedPartsForApprovedHistory(history *models.RecordHistory, room *models.RecordRoom) {
	db := database.GetDB()

	// 获取该历史记录的所有已上传原始分P（非临时文件）
	var originalParts []models.RecordHistoryPart
	if err := db.Where(
		"history_id = ? AND upload = ? AND is_temp_file = ? AND recording = ?",
		history.ID, true, false, false,
	).Find(&originalParts).Error; err != nil {
		log.Printf("[弹幕回补] 查询历史记录 %d 的原始分P失败: %v", history.ID, err)
		return
	}

	if len(originalParts) == 0 {
		return
	}

	burnService := services.NewDanmakuBurnService()

	for _, part := range originalParts {
		// 1. 检查是否已有追加成功的弹幕烧录版
		var appendedCount int64
		db.Model(&models.RecordHistoryPart{}).Where(
			"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ? AND appended_to_video = ?",
			part.ID, true, "danmaku_burn", true,
		).Count(&appendedCount)
		if appendedCount > 0 {
			continue // 已追加，跳过
		}

		// 2. 检查是否已有上传完成但 appended_to_video=false 的烧录版（可能 EditVideo 失败需重试）
		var pendingAppend models.RecordHistoryPart
		if err := db.Where(
			"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ? AND c_id > 0 AND appended_to_video = ?",
			part.ID, true, "danmaku_burn", true, false,
		).First(&pendingAppend).Error; err == nil {
			// 已上传但未成功追加，直接重新触发 UpdatePublishedVideoWithBurnedParts
			log.Printf("[弹幕回补] 发现已上传但未追加的弹幕版，重新触发追加: burned_part_id=%d", pendingAppend.ID)
			go func(pid uint) {
				s.appendBurnedSem <- struct{}{}
				defer func() { <-s.appendBurnedSem }()
				if err := s.UpdatePublishedVideoWithBurnedParts(pid); err != nil {
					log.Printf("[弹幕回补] 重新追加失败: burned_part_id=%d, err=%v", pid, err)
				}
			}(pendingAppend.ID)
			continue
		}

		// 3. 检查是否已有正在烧录/上传中的记录（任意状态，避免重复触发）
		var anyBurnedCount int64
		db.Model(&models.RecordHistoryPart{}).Where(
			"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ?",
			part.ID, true, "danmaku_burn",
		).Count(&anyBurnedCount)
		if anyBurnedCount > 0 {
			// 已有烧录记录（upload=false：正在上传中，或文件不存在待重试等），跳过避免重复
			continue
		}

		// 4. 检查原始视频文件是否存在
		if part.FilePath == "" {
			continue
		}
		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			log.Printf("[弹幕回补] 原始视频文件不存在，跳过: history_id=%d, part_id=%d, file=%s",
				history.ID, part.ID, part.FilePath)
			continue
		}

		// 5. 检查 XML 弹幕文件是否存在
		xmlPath := burnService.FindDanmakuXML(part.FilePath)
		if xmlPath == "" {
			log.Printf("[弹幕回补] XML弹幕文件不存在，跳过: history_id=%d, part_id=%d, video=%s",
				history.ID, part.ID, part.FilePath)
			continue
		}

		log.Printf("[弹幕回补] 开始异步烧录并追加弹幕版: history_id=%d, part_id=%d, video=%s, xml=%s",
			history.ID, part.ID, part.FilePath, xmlPath)

		// 异步执行烧录 → 上传 → 追加（避免阻塞定时任务）
		go func(p models.RecordHistoryPart, h models.RecordHistory, r models.RecordRoom) {
			bs := services.NewDanmakuBurnService()
			burnedPath, err := bs.BurnDanmakuToVideo(&p, &h, &r)
			if err != nil {
				log.Printf("[弹幕回补] 烧录失败: part_id=%d, err=%v", p.ID, err)
				// 创建失败标记，防止定时任务对同一损坏/无效XML无限重试
				failedMarker := &models.RecordHistoryPart{
					HistoryID:    p.HistoryID,
					SourcePartID: p.ID,
					IsTempFile:   true,
					TempFileType: "danmaku_burn",
					Upload:       false,
				}
				if createErr := database.GetDB().Create(failedMarker).Error; createErr != nil {
					log.Printf("[弹幕回补] 创建失败标记出错: part_id=%d, err=%v", p.ID, createErr)
				}
				return
			}

			// 查询新生成的烧录版 Part 记录
			var burnedPart models.RecordHistoryPart
			if dbErr := database.GetDB().Where(
				"file_path = ? AND is_temp_file = ? AND temp_file_type = ?",
				burnedPath, true, "danmaku_burn",
			).First(&burnedPart).Error; dbErr != nil {
				log.Printf("[弹幕回补] 查询烧录版分P记录失败: part_id=%d, err=%v", p.ID, dbErr)
				return
			}

			log.Printf("[弹幕回补] 烧录完成，加入上传队列: burned_part_id=%d", burnedPart.ID)
			if uploadErr := s.UploadPart(&burnedPart, &h, &r); uploadErr != nil {
				log.Printf("[弹幕回补] 烧录版入队失败: burned_part_id=%d, err=%v", burnedPart.ID, uploadErr)
			}
		}(part, *history, *room)
	}
}
