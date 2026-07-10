package upload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gobup/server/internal/agent"
	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

func (s *Service) PublishHistory(historyID uint, userID uint) error {
	return s.publishHistory(historyID, userID, true)
}

func (s *Service) PublishHistoryLocal(historyID uint, userID uint) error {
	return s.publishHistory(historyID, userID, false)
}

func (s *Service) publishHistory(historyID uint, userID uint, allowRemote bool) error {
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
	if history.PublishCooldownAt != nil && history.PublishCooldownAt.After(time.Now()) {
		return fmt.Errorf("投稿风控冷却中，将在 %s 后自动重试", history.PublishCooldownAt.Format("2006-01-02 15:04:05"))
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间不存在: %w", err)
	}

	// 核心逻辑：检查同SessionID是否已有投稿（如果启用了合并）
	log.Printf("[投稿] 开始检查SessionID合并 (history_id=%d, room_id=%s, session_id=%s, MergeBySession=%v)",
		historyID, history.RoomID, history.SessionID, room.MergeBySession)

	if room.MergeBySession && history.SessionID != "" {
		if dayStart, dayEnd, ok := models.LiveSessionDayRange(history.StartTime); ok {
			log.Printf("[投稿] SessionID合并已启用，查询同日同SessionID的已投稿记录 (session_id=%s, day=%s)",
				history.SessionID, models.LiveSessionDayKey(history.StartTime))

			var existingHistory models.RecordHistory
			err := db.Where(
				"session_id = ? AND publish = ? AND id != ? AND room_id = ? AND start_time >= ? AND start_time < ?",
				history.SessionID, true, historyID, history.RoomID, dayStart, dayEnd,
			).First(&existingHistory).Error

			if err == nil {
				// 找到同日同SessionID的已投稿记录，执行追加分P逻辑
				log.Printf("[投稿] 检测到同日同SessionID已有投稿 (session_id=%s, existing_history_id=%d, bv_id=%s, aid=%s)，执行追加分P",
					history.SessionID, existingHistory.ID, existingHistory.BvID, existingHistory.AvID)

				return s.AppendPartsToExisting(historyID, &existingHistory, userID)
			}
			// 未找到已投稿记录，继续执行新建投稿逻辑
			log.Printf("[投稿] 同日同SessionID无已投稿记录，执行新建投稿 (session_id=%s, query_error=%v)", history.SessionID, err)
		} else {
			log.Printf("[投稿] 历史记录缺少有效开始时间，跳过SessionID合并: history_id=%d", history.ID)
		}
	} else {
		if !room.MergeBySession {
			log.Printf("[投稿] SessionID合并未启用 (MergeBySession=false)")
		}
		if history.SessionID == "" {
			log.Printf("[投稿] SessionID为空，跳过合并检测")
		}
	}

	if room.MergeBySession {
		normalizedTitle := normalizePublishTitle(history.Title)
		if normalizedTitle != "" {
			if dayStart, dayEnd, ok := models.LiveSessionDayRange(history.StartTime); ok {
				log.Printf("[投稿] 查询同日同标题已投稿记录 (room_id=%s, title=%s, day=%s)",
					history.RoomID, normalizedTitle, models.LiveSessionDayKey(history.StartTime))
				var existingHistory models.RecordHistory
				err := db.Where(
					"room_id = ? AND title = ? AND publish = ? AND id != ? AND is_highlight = ? AND start_time >= ? AND start_time < ?",
					history.RoomID, normalizedTitle, true, historyID, false, dayStart, dayEnd,
				).Order("start_time ASC").First(&existingHistory).Error
				if err == nil {
					log.Printf("[投稿] 检测到同日同标题已有投稿 (existing_history_id=%d, bv_id=%s)，执行追加分P",
						existingHistory.ID, existingHistory.BvID)
					return s.AppendPartsToExisting(historyID, &existingHistory, userID)
				}
				log.Printf("[投稿] 同日同标题无已投稿记录，继续新建投稿 (query_error=%v)", err)
			} else {
				log.Printf("[投稿] 历史记录缺少有效开始时间，跳过同标题合并: history_id=%d", history.ID)
			}
		}
	}

	// 权限检查1：房间是否启用上传功能（总开关）
	if !room.Upload {
		return fmt.Errorf("房间未启用上传功能，无法投稿")
	}

	if room.UploadUserID == 0 {
		return fmt.Errorf("房间未配置投稿用户")
	}
	if userID != room.UploadUserID {
		log.Printf("[Publish] 请求用户ID=%d 与房间配置用户ID=%d 不一致，使用房间配置用户投稿",
			userID, room.UploadUserID)
		userID = room.UploadUserID
	}

	if allowRemote {
		var sysConfig models.SystemConfig
		if err := db.First(&sysConfig).Error; err == nil && strings.EqualFold(strings.TrimSpace(sysConfig.PublishMode), "remote") {
			if !models.AgentPurposeAllows(sysConfig.AgentPurpose, models.AgentPurposeUpload) {
				return fmt.Errorf("当前 Agent 用途为 %s，不允许远程投稿", models.NormalizeAgentPurpose(sysConfig.AgentPurpose))
			}
			endpoint := models.NormalizeAgentEndpoint(sysConfig.PublishAgentEndpoint)
			if endpoint == "" {
				if selectedEndpoint, ok := selectPreferredPublishAgentEndpoint(db); ok {
					endpoint = selectedEndpoint
					sysConfig.PublishAgentEndpoint = selectedEndpoint
					_ = db.Model(&sysConfig).Update("publish_agent_endpoint", selectedEndpoint).Error
				}
			}
			if endpoint == "" {
				return fmt.Errorf("已选择远程投稿模式，但未配置远程 Agent 地址")
			}
			if err := validatePublishAgentEndpointForUpload(db, endpoint); err != nil {
				markPublishFailure(db, &history, err)
				return err
			}
			if endpoint != sysConfig.PublishAgentEndpoint {
				db.Model(&sysConfig).Update("publish_agent_endpoint", endpoint)
			}
			token := strings.TrimSpace(sysConfig.PublishAgentToken)
			if token == "" {
				token = models.NewAgentToken()
				db.Model(&sysConfig).Update("publish_agent_token", token)
			}
			timeoutSeconds := sysConfig.PublishAgentTimeout
			if timeoutSeconds < 3 {
				timeoutSeconds = 30
			}
			if timeoutSeconds > 600 {
				timeoutSeconds = 600
			}
			timeout := time.Duration(timeoutSeconds) * time.Second
			client := agent.NewClient(endpoint, token, timeout)
			result, err := client.PublishHistory(historyID, userID)
			if err != nil {
				wrappedErr := fmt.Errorf("远程 Agent 投稿失败: %w", err)
				markPublishAgentEndpointError(endpoint, wrappedErr)
				markPublishFailure(db, &history, wrappedErr)
				return wrappedErr
			}
			if result != nil {
				updates := map[string]interface{}{
					"publish": result.Publish,
				}
				if result.BvID != "" {
					updates["bv_id"] = result.BvID
				}
				if result.AvID != "" {
					updates["av_id"] = result.AvID
				}
				if result.Message != "" {
					updates["message"] = result.Message
				}
				db.Model(&history).Updates(updates)
			}
			markPublishAgentEndpointSuccess(endpoint)
			log.Printf("[Agent] 已通过远程 Agent 完成投稿: history_id=%d, endpoint=%s", historyID, endpoint)
			return nil
		}
	}

	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}
	if !user.Enabled {
		return fmt.Errorf("用户已禁用")
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
			"history_id = ? AND upload = ? AND upload_cancelled = ? AND (is_temp_file = ? OR temp_file_type = ? OR temp_file_type = ?)",
			historyID, true, false, false, "danmaku_burn", "split",
		).Order("start_time ASC").Find(&parts).Error
	} else {
		partsErr = db.Where(
			"history_id = ? AND upload = ? AND upload_cancelled = ? AND (is_temp_file = ? OR temp_file_type = ?)",
			historyID, true, false, false, "split",
		).Order("start_time ASC").Find(&parts).Error
	}
	if partsErr != nil {
		return fmt.Errorf("查询分P失败: %w", partsErr)
	}

	if len(parts) == 0 {
		return fmt.Errorf("没有已上传的分P")
	}

	titleSequence := 1
	if strings.TrimSpace(history.Title) != "" {
		if dayStart, dayEnd, ok := models.LiveSessionDayRange(history.StartTime); ok {
			var previousCount int64
			db.Model(&models.RecordHistory{}).Where(
				"room_id = ? AND title = ? AND id != ? AND start_time >= ? AND start_time < ? AND start_time <= ?",
				history.RoomID, history.Title, history.ID, dayStart, dayEnd, history.StartTime,
			).Count(&previousCount)
			titleSequence = int(previousCount) + 1
		}
	}

	// 构建模板数据（优先使用历史记录中的实际数据）
	templateData := map[string]interface{}{
		"uname":     history.Uname, // 使用历史记录中实际的主播名
		"title":     history.Title, // 使用历史记录中实际的直播标题
		"roomId":    history.RoomID,
		"areaName":  history.AreaName, // 使用历史记录中实际的分区名称
		"startTime": history.StartTime,
		"uid":       user.UID,
		"sequence":  titleSequence,
		"seq":       titleSequence,
	}

	// 使用模板服务渲染
	title := s.templateSvc.RenderTitle(room.TitleTemplate, templateData)
	title = normalizeBiliPublishTitle("稿件标题", title)
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
	} else if coverType == "frame" && len(parts) > 0 {
		coverPart := parts[0]
		coverPath, err := services.NewVideoProcessingService().EnsureFrameCover(&coverPart, &room)
		if err != nil {
			log.Printf("自动截取封面失败: %v", err)
			coverURL = ""
		} else if coverData, readErr := os.ReadFile(coverPath); readErr == nil {
			if uploadedURL, uploadErr := client.UploadCover(coverData); uploadErr == nil {
				coverURL = uploadedURL
				log.Printf("自动截取封面上传成功: %s", coverURL)
			} else {
				log.Printf("自动截取封面上传失败: %v", uploadErr)
				coverURL = ""
			}
		} else {
			log.Printf("读取自动截取封面失败: %v", readErr)
			coverURL = ""
		}
	} else if coverType == "live" && len(parts) > 0 {
		// 使用直播首帧：从录制文件查找封面图片并上传
		// 根据直播标题查找同一房间内最早录制的封面文件
		// 查找同一房间、同一直播标题的最早一次录制分P
		var oldestPart models.RecordHistoryPart
		oldestPartQuery := db.Where("room_id = ? AND live_title = ?", history.RoomID, history.Title)
		if dayStart, dayEnd, ok := models.LiveSessionDayRange(history.StartTime); ok {
			oldestPartQuery = oldestPartQuery.Where("start_time >= ? AND start_time < ?", dayStart, dayEnd)
		}
		err := oldestPartQuery.Order("start_time ASC").First(&oldestPart).Error

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

		if coverURL == "" && room.AutoExtractCover && len(parts) > 0 {
			coverPart := parts[0]
			coverPath, err := services.NewVideoProcessingService().EnsureFrameCover(&coverPart, &room)
			if err != nil {
				log.Printf("未找到现有封面，自动截取封面失败: %v", err)
			} else if coverData, readErr := os.ReadFile(coverPath); readErr == nil {
				if uploadedURL, uploadErr := client.UploadCover(coverData); uploadErr == nil {
					coverURL = uploadedURL
					log.Printf("自动截取封面上传成功: %s", coverURL)
				} else {
					log.Printf("自动截取封面上传失败: %v", uploadErr)
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
	for _, part := range parts {
		if skip, reason := shouldSkipTooShortPublishPart(part); skip {
			log.Printf("[投稿] 跳过不可投稿分P: history_id=%d, part_id=%d, reason=%s", history.ID, part.ID, reason)
			markPublishPartSkipped(db, &part, reason)
			continue
		}
		partIndex := len(videoParts) + 1
		// 为分P标题模板构建数据，包含所有可用变量
		partTemplateData := map[string]interface{}{
			"index":     partIndex,
			"startTime": part.StartTime,
			"areaName":  part.AreaName,
			"uname":     history.Uname,  // 主播名
			"title":     history.Title,  // 直播标题
			"roomId":    history.RoomID, // 房间号
			"fileName":  part.FileName,  // 文件名
			"sequence":  titleSequence,
			"seq":       titleSequence,
		}
		partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)
		// 弹幕烧录版分P：在标题末尾附加「弹幕版」标识，明确区分 P1(原始) / P2(弹幕)
		if part.IsTempFile && part.TempFileType == "danmaku_burn" {
			partTitle = partTitle + "（弹幕版）"
		}
		partTitle = normalizeBiliPartTitle(partIndex, partTitle)

		// 获取文件名：优先使用数据库中的 FileName（从上传响应获取），如果为空则从 FilePath 提取
		filename := publishPartFilename(part, partIndex)

		// 调试日志：检查关键参数
		log.Printf("构建分P[%d]: filename=%s, cid=%d", partIndex, filename, part.CID)

		// 检查CID是否为0（参考biliupforjava实现）
		// 原来CID=0时会在当前goroutine中同步执行完整上传流程（可能耗时数小时），
		// 导致整个投稿调用链阻塞，进而阻塞上传队列中所有其他任务。
		// 现在改为: 重置分P上传状态后返回错误，让自动上传调度器在下一轮（10分钟内）重新上传，
		// 上传完成后 checkAndPublish 会再次触发投稿。
		var cid int64
		if part.CID > 0 {
			cid = int64(part.CID)
		} else {
			log.Printf("检测到分P[%d]的CID为0（数据异常），重置上传状态等待自动重传: part_id=%d, file=%s", partIndex, part.ID, part.FilePath)

			// 重置上传状态，让自动上传调度器在下次轮询时重新上传
			db.Model(&part).Updates(map[string]interface{}{
				"upload":             false,
				"uploading":          false,
				"file_name":          "",
				"c_id":               0,
				"upload_retry_count": 0,
				"upload_error_msg":   "CID为0，数据异常，已重置等待自动重传",
				"upload_error_type":  UploadErrorTypeFile,
			})
			return fmt.Errorf("分P[%d](part_id=%d)的CID为0，已重置上传状态，自动上传调度器将在10分钟内重传，请稍后重试投稿", partIndex, part.ID)
		}

		videoParts = append(videoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: filename,
			Cid:      cid,
		})
	}
	if len(videoParts) == 0 {
		return fmt.Errorf("没有可投稿的有效分P，已过滤时长不足或无效的分P")
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

		markPublishFailure(db, &history, err)
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
		sectionID := room.SeasonID
		// 使用第一个分P的CID
		cid := videoParts[0].Cid
		seasonTitle := normalizeBiliPublishTitle("合集视频标题", title)
		if err := client.AddToSeason(sectionID, avID, cid, seasonTitle); err != nil {
			log.Printf("加入合集失败，尝试解析已保存ID=%d 是否为合集ID: %v", room.SeasonID, err)
			if resolvedSectionID, resolveErr := client.ResolveSeasonSectionID(room.SeasonID); resolveErr != nil {
				log.Printf("解析合集小节ID失败，已保存ID=%d: %v", room.SeasonID, resolveErr)
			} else if resolvedSectionID <= 0 {
				log.Printf("跳过加入合集：已保存ID=%d 未解析到可用小节ID", room.SeasonID)
			} else if resolvedSectionID == sectionID {
				log.Printf("加入合集失败：已保存ID=%d 已是解析后的小节ID", room.SeasonID)
			} else if retryErr := client.AddToSeason(resolvedSectionID, avID, cid, seasonTitle); retryErr != nil {
				log.Printf("使用解析后小节ID加入合集仍失败: SectionID=%d, AID=%d, err=%v", resolvedSectionID, avID, retryErr)
			} else {
				log.Printf("加入合集成功: SectionID=%d, AID=%d", resolvedSectionID, avID)
			}
		} else {
			log.Printf("加入合集成功: SectionID=%d, AID=%d", sectionID, avID)
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

	services.EnsureSessionGuideComment(db, client, &history, avID, history.BvID)

	// 如果启用高能剪辑，先生成高光稿件再执行投稿后文件操作。
	// 这样可以避免 DeleteType/文件移动策略先处理源视频，导致高能剪辑找不到源文件。
	if room.HighEnergyCut && !history.IsHighlight {
		s.createAndQueueHighEnergyClip(history.ID)
	}

	// 触发“投稿成功后”文件操作
	fileMoverSvc := services.NewFileMoverService()
	if err := fileMoverSvc.TriggerFileOp(historyID, &room, services.FileOpTriggerAfterPublish); err != nil {
		log.Printf("文件处理失败: %v", err)
	}

	// 注意：临时文件（切分文件、弹幕烧录文件等）的清理已移至审核通过后执行
	// 见 videosync.go 中的审核通过逻辑

	return nil
}

func normalizePublishTitle(title string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
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
	if !user.Enabled {
		return nil, fmt.Errorf("用户已禁用")
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
	if !user.Enabled {
		return fmt.Errorf("用户已禁用")
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
	normalizedExistingTitle := normalizePublishTitle(existingHistory.Title)
	existingPartsQuery := db.Joins("JOIN record_histories ON record_history_parts.history_id = record_histories.id").
		Where("record_histories.room_id = ? AND record_histories.publish = ? AND record_history_parts.upload = ? AND record_history_parts.upload_cancelled = ? AND (record_history_parts.is_temp_file = ? OR record_history_parts.temp_file_type = ?)",
			existingHistory.RoomID, true, true, false, false, "split")
	if normalizedExistingTitle != "" && existingHistory.SessionID != "" {
		if dayStart, dayEnd, ok := models.LiveSessionDayRange(existingHistory.StartTime); ok {
			existingPartsQuery = existingPartsQuery.Where(
				"(record_histories.session_id = ? OR record_histories.title = ?) AND record_histories.start_time >= ? AND record_histories.start_time < ?",
				existingHistory.SessionID, normalizedExistingTitle, dayStart, dayEnd,
			)
		} else {
			existingPartsQuery = existingPartsQuery.Where("record_histories.session_id = ?", existingHistory.SessionID)
		}
	} else if existingHistory.SessionID != "" {
		if dayStart, dayEnd, ok := models.LiveSessionDayRange(existingHistory.StartTime); ok {
			existingPartsQuery = existingPartsQuery.Where(
				"record_histories.session_id = ? AND record_histories.start_time >= ? AND record_histories.start_time < ?",
				existingHistory.SessionID, dayStart, dayEnd,
			)
		} else {
			existingPartsQuery = existingPartsQuery.Where("record_histories.session_id = ?", existingHistory.SessionID)
		}
	} else {
		dayStart, dayEnd, ok := models.LiveSessionDayRange(existingHistory.StartTime)
		if !ok {
			return fmt.Errorf("已有投稿缺少有效开始时间，无法安全按标题追加: history_id=%d", existingHistory.ID)
		}
		existingPartsQuery = existingPartsQuery.Where(
			"record_histories.title = ? AND record_histories.start_time >= ? AND record_histories.start_time < ?",
			normalizedExistingTitle, dayStart, dayEnd,
		)
	}
	if err := existingPartsQuery.
		Order("record_history_parts.start_time ASC").
		Find(&existingParts).Error; err != nil {
		return fmt.Errorf("查询已存在分P失败: %w", err)
	}

	// 获取新历史记录的已上传分P（原始分P + 切分子分P，排除弹幕烧录版临时文件避免重复）
	// file_delete 不过滤：本地文件删除后CID/FileName仍有效，投稿时使用DB中保存的服务端文件名
	var newParts []models.RecordHistoryPart
	if err := db.Where(
		"history_id = ? AND upload = ? AND upload_cancelled = ? AND (is_temp_file = ? OR temp_file_type = ?)",
		newHistoryID, true, false, false, "split").
		Order("start_time ASC").
		Find(&newParts).Error; err != nil {
		return fmt.Errorf("查询新分P失败: %w", err)
	}

	if len(newParts) == 0 {
		return fmt.Errorf("没有可追加的分P")
	}

	log.Printf("[追加分P] 已存在分P数: %d, 新增分P数: %d", len(existingParts), len(newParts))

	titleSequence := 1
	if strings.TrimSpace(existingHistory.Title) != "" {
		if dayStart, dayEnd, ok := models.LiveSessionDayRange(existingHistory.StartTime); ok {
			var previousCount int64
			db.Model(&models.RecordHistory{}).Where(
				"room_id = ? AND title = ? AND id != ? AND start_time >= ? AND start_time < ? AND start_time <= ?",
				existingHistory.RoomID, existingHistory.Title, existingHistory.ID, dayStart, dayEnd, existingHistory.StartTime,
			).Count(&previousCount)
			titleSequence = int(previousCount) + 1
		}
	}

	// 构建模板数据
	templateData := map[string]interface{}{
		"uname":     existingHistory.Uname,
		"title":     existingHistory.Title,
		"roomId":    existingHistory.RoomID,
		"areaName":  existingHistory.AreaName,
		"startTime": existingHistory.StartTime,
		"uid":       user.UID,
		"sequence":  titleSequence,
		"seq":       titleSequence,
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
		if part.CID <= 0 {
			return fmt.Errorf("已存在分PCID无效，无法安全编辑原稿: part_id=%d", part.ID)
		}
		partTemplateData := map[string]interface{}{
			"index":     i + 1,
			"startTime": part.StartTime,
			"areaName":  part.AreaName,
			"uname":     existingHistory.Uname,
			"title":     existingHistory.Title,
			"roomId":    existingHistory.RoomID,
			"fileName":  part.FileName,
			"sequence":  titleSequence,
			"seq":       titleSequence,
		}
		partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)
		partTitle = normalizeBiliPartTitle(i+1, partTitle)

		allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: publishPartFilename(part, i+1),
			Cid:      int64(part.CID),
		})
	}

	// 2. 添加新分P
	addedNewParts := 0
	for _, part := range newParts {
		if skip, reason := shouldSkipTooShortPublishPart(part); skip {
			log.Printf("[追加分P] 跳过不可追加分P: new_history=%d, part_id=%d, reason=%s", newHistoryID, part.ID, reason)
			markPublishPartSkipped(db, &part, reason)
			continue
		}
		if part.CID <= 0 {
			return fmt.Errorf("新增分PCID无效，无法追加: part_id=%d", part.ID)
		}
		partIndex := len(allVideoParts) + 1
		partTemplateData := map[string]interface{}{
			"index":     partIndex,
			"startTime": part.StartTime,
			"areaName":  part.AreaName,
			"uname":     newHistory.Uname,
			"title":     newHistory.Title,
			"roomId":    newHistory.RoomID,
			"fileName":  part.FileName,
			"sequence":  titleSequence,
			"seq":       titleSequence,
		}
		partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)
		partTitle = normalizeBiliPartTitle(partIndex, partTitle)

		allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: publishPartFilename(part, partIndex),
			Cid:      int64(part.CID),
		})
		addedNewParts++
	}
	if addedNewParts == 0 {
		return fmt.Errorf("没有可追加的有效分P，已过滤时长不足或无效的分P")
	}

	log.Printf("[追加分P] 合并后总分P数: %d", len(allVideoParts))

	// 使用原视频的信息进行编辑
	title := normalizeBiliPublishTitle("稿件标题", archiveDetail.Archive.Title)
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

	services.EnsureSessionGuideComment(db, client, existingHistory, aidInt, existingHistory.BvID)

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
