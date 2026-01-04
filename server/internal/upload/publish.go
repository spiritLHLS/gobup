package upload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

func (s *Service) PublishHistory(historyID uint, userID uint) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间不存在: %w", err)
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
	// 排除已删除文件的Parts（例如被切分的原始文件）
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND upload = ? AND file_delete = ?", historyID, true, false).
		Order("start_time ASC").
		Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
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
		// 如果CID为0，说明视频还没有上传完成或上传出错，需要立即上传
		var cid int64
		if part.CID > 0 {
			cid = int64(part.CID)
		} else {
			log.Printf("检测到分P[%d]的CID为0，立即触发上传: %s", i, part.FilePath)

			// 检查文件是否存在
			if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
				return fmt.Errorf("分P[%d]文件不存在，无法上传: %s", i, part.FilePath)
			}

			// 重置上传状态，准备上传
			part.Upload = false
			part.Uploading = false
			part.FileName = ""
			part.CID = 0
			part.UploadRetryCount = 0
			part.UploadErrorMsg = ""
			db.Save(&part)

			// 立即上传该分P
			log.Printf("开始上传分P[%d]: %s", i, part.FilePath)
			if err := s.uploadPartInternal(&part, &history, &room); err != nil {
				return fmt.Errorf("分P[%d]上传失败: %w，请稍后重试投稿", i, err)
			}

			// 重新加载分P信息，获取上传后的CID
			if err := db.First(&part, part.ID).Error; err != nil {
				return fmt.Errorf("重新加载分P[%d]信息失败: %w", i, err)
			}

			if part.CID == 0 {
				return fmt.Errorf("分P[%d]上传后CID仍为0，上传可能失败", i)
			}

			cid = int64(part.CID)
			filename = part.FileName // 使用上传后获得的服务器文件名
			log.Printf("分P[%d]上传成功，CID=%d, FileName=%s", i, part.CID, filename)
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

	log.Printf("投稿成功: AV%d, BV%s", avID, bvid)

	// 兜底检测机制：使用新的API验证投稿是否真的成功
	// 等待3秒让B站后台处理完成
	time.Sleep(3 * time.Second)

	log.Printf("开始兜底检测：验证视频是否在用户投稿列表中 (mid=%d, aid=%d, bvid=%s)", user.UID, avID, bvid)
	exists, checkErr := client.CheckVideoExistsInArchive(user.UID, avID, bvid)
	if checkErr != nil {
		log.Printf("⚠️  兜底检测失败（API调用错误）: %v，但投稿API已返回成功，继续后续流程", checkErr)
	} else if !exists {
		log.Printf("⚠️  兜底检测未找到视频！投稿可能失败，但投稿API已返回成功。建议手动检查：https://space.bilibili.com/%d", user.UID)
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

	// 处理文件策略：9-投稿成功后删除, 10-投稿成功后移动
	if room.DeleteType == 9 || room.DeleteType == 10 {
		fileMoverSvc := services.NewFileMoverService()
		if err := fileMoverSvc.ProcessFilesByStrategy(historyID, room.DeleteType); err != nil {
			log.Printf("文件处理失败: %v", err)
		}
	}

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
	return client.GetSeasons(user.UID)
}
