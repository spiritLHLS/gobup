package upload

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

// RequeueStuckTempParts 将服务重启后滞留在DB中的临时分P重新加入上传队列
// 触发场景：弹幕烧录成功 → 临时Part入库 → 服务崩溃/重启 → 内存队列丢失
// 由调度器在启动后调用一次，确保这些分P不丢失
// 通过立即将 Uploading 置为 true 防止10分钟周期调用时重复入队
func (s *Service) RequeueStuckTempParts() {
	db := database.GetDB()

	var stuckParts []models.RecordHistoryPart
	if err := db.Where(
		"is_temp_file = ? AND upload = ? AND uploading = ? AND file_delete = ? AND upload_paused = ? AND upload_cancelled = ?",
		true, false, false, false, false, false,
	).Find(&stuckParts).Error; err != nil {
		log.Printf("[重启恢复] 查询滞留临时分P失败: %v", err)
		return
	}

	if len(stuckParts) == 0 {
		return
	}

	log.Printf("[重启恢复] 发现 %d 个滞留临时分P，准备重新入队", len(stuckParts))

	for _, part := range stuckParts {
		// 检查物理文件是否存在
		if part.FilePath == "" {
			continue
		}
		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			// 文件不存在，标记为已删除
			log.Printf("[重启恢复] 临时分P文件不存在，标记为已删除: part_id=%d, file=%s", part.ID, part.FilePath)
			part.FileDelete = true
			part.UploadErrorMsg = "临时文件不存在，已自动清理"
			part.UploadErrorType = UploadErrorTypeFile
			db.Save(&part)
			continue
		}

		// 先标记 Uploading=true 防止下次10分钟周期调用重复入队
		// uploadPartInternal 内部也会设置 Uploading=true（幂等），defer 会在完成后重置为 false
		part.Uploading = true
		db.Save(&part)

		// 重新获取关联的历史记录和房间配置
		var history models.RecordHistory
		if err := db.First(&history, part.HistoryID).Error; err != nil {
			log.Printf("[重启恢复] 获取历史记录失败: part_id=%d, err=%v", part.ID, err)
			part.Uploading = false
			db.Save(&part)
			continue
		}

		var room models.RecordRoom
		if err := db.Where("room_id = ?", part.RoomID).First(&room).Error; err != nil {
			log.Printf("[重启恢复] 获取房间配置失败: part_id=%d, err=%v", part.ID, err)
			part.Uploading = false
			db.Save(&part)
			continue
		}

		if !roomHasUploadUserOrStrategy(&room) {
			part.Uploading = false
			db.Save(&part)
			continue
		}

		if err := s.UploadPart(&part, &history, &room); err != nil {
			// 入队失败才重置，让下次定时任务重试
			part.Uploading = false
			db.Save(&part)
			log.Printf("[重启恢复] 临时分P重新入队失败: part_id=%d, err=%v", part.ID, err)
		} else {
			log.Printf("[重启恢复] 临时分P重新入队成功: part_id=%d, file=%s", part.ID, part.FilePath)
		}
	}
}

// ResetStuckUploadingParts 重置服务崩溃时卡在 uploading=true 的分P（启动时调用一次）
// 防止因上次崩溃留下的 uploading=true 状态导致分P永远不被重试
func (s *Service) ResetStuckUploadingParts() {
	db := database.GetDB()
	result := db.Model(&models.RecordHistoryPart{}).
		Where("uploading = ? AND upload = ? AND upload_cancelled = ?", true, false, false).
		Updates(map[string]interface{}{"uploading": false})
	if result.Error != nil {
		log.Printf("[启动恢复] 重置滞留 uploading 状态失败: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("[启动恢复] 重置了 %d 个上次崩溃时卡在 uploading=true 的分P", result.RowsAffected)
	}
}

// RequeueInterruptedBurns 启动恢复：检测 flv 已上传但弹幕烧录被中断（容器重启导致 ffmpeg 被 kill）
// 场景：flv upload=true，但没有对应的 danmaku_burn 临时分P 记录 → ffmpeg 从未完成
// 对每个这样的分P重新异步触发烧录并将烧录版入队
func (s *Service) RequeueInterruptedBurns() {
	db := database.GetDB()
	factoryAvailable := true
	if _, err := services.CheckDanmakuFactoryAvailable(); err != nil {
		factoryAvailable = false
		log.Printf("[启动恢复-烧录] DanmakuFactory 不可用，跳过需要重新编码的烧录恢复: %v", err)
	}
	missingSourceCount := 0
	requeuedCount := 0
	factorySkippedCount := 0

	// 只处理启用了弹幕烧录的房间
	var rooms []models.RecordRoom
	if err := db.Where("enable_danmaku_burn = ? AND upload = ?", true, true).Find(&rooms).Error; err != nil {
		log.Printf("[启动恢复-烧录] 查询房间失败: %v", err)
		return
	}

	for _, room := range rooms {
		if !roomHasUploadUserOrStrategy(&room) {
			continue
		}

		// 找到该房间所有已上传的原始分P（非临时文件）
		var uploadedParts []models.RecordHistoryPart
		if err := db.Where(
			"room_id = ? AND upload = ? AND is_temp_file = ? AND recording = ?",
			room.RoomID, true, false, false,
		).Find(&uploadedParts).Error; err != nil {
			log.Printf("[启动恢复-烧录] 查询房间 %s 的上传分P失败: %v", room.RoomID, err)
			continue
		}

		for _, part := range uploadedParts {
			// 检查是否已存在对应的烧录 Part 记录（任意状态均算，包括上传失败待重试）
			var burnedCount int64
			db.Model(&models.RecordHistoryPart{}).Where(
				"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ?",
				part.ID, true, "danmaku_burn",
			).Count(&burnedCount)
			if burnedCount > 0 {
				continue // 烧录记录已存在，由 RequeueStuckTempParts 负责重传
			}

			// 检查对应历史记录是否已投稿（已投稿则通过 UpdatePublishedVideoWithBurnedParts 回补）
			var history models.RecordHistory
			if err := db.First(&history, part.HistoryID).Error; err != nil {
				continue
			}

			if strings.TrimSpace(part.FilePath) == "" {
				s.markDanmakuBurnSkipped(&part, "源视频文件路径为空，已停止该分P弹幕烧录恢复", UploadErrorTypeFile)
				missingSourceCount++
				continue
			}
			if _, err := os.Stat(part.FilePath); err != nil {
				if os.IsNotExist(err) {
					s.markDanmakuBurnSkipped(&part, "源视频文件不存在，已停止该分P弹幕烧录恢复: "+part.FilePath, UploadErrorTypeFile)
					missingSourceCount++
					continue
				}
				log.Printf("[启动恢复-烧录] 检查源视频失败，暂缓恢复: part_id=%d, err=%v", part.ID, err)
				continue
			}
			if !factoryAvailable {
				factorySkippedCount++
				continue
			}

			log.Printf("[启动恢复-烧录] 发现未烧录的已上传分P，重新触发烧录: part_id=%d, file=%s", part.ID, part.FilePath)
			requeuedCount++

			// 异步烧录，避免阻塞启动流程
			go func(p models.RecordHistoryPart, h models.RecordHistory, r models.RecordRoom) {
				// 在重新运行 ffmpeg 之前，先检查磁盘上是否已有符合命名规则的弹幕烧录文件
				// 场景: ffmpeg 写完了文件，但 db.Create(burnedPart) 还没来得及执行容器就被重启
				baseNoExt := strings.TrimSuffix(filepath.Base(p.FilePath), filepath.Ext(p.FilePath))
				globPattern := filepath.Join(filepath.Dir(p.FilePath), baseNoExt+"_danmaku_*.mp4")
				if matches, _ := filepath.Glob(globPattern); len(matches) > 0 {
					existingPath := matches[0] // 取第一个匹配
					fInfo, statErr := os.Stat(existingPath)
					if statErr == nil {
						log.Printf("[启动恢复-烧录] 磁盘上已有弹幕烧录文件，重新入库代替重新编码: part_id=%d, file=%s", p.ID, existingPath)
						burnedPart := &models.RecordHistoryPart{
							HistoryID:    p.HistoryID,
							RoomID:       p.RoomID,
							SessionID:    p.SessionID,
							Title:        p.Title + " (弹幕版)",
							LiveTitle:    p.LiveTitle,
							AreaName:     p.AreaName,
							FilePath:     existingPath,
							FileName:     filepath.Base(existingPath),
							FileSize:     fInfo.Size(),
							Duration:     p.Duration,
							StartTime:    p.StartTime,
							EndTime:      p.EndTime,
							Recording:    false,
							Upload:       false,
							Uploading:    false,
							IsTempFile:   true,
							SourcePartID: p.ID,
							TempFileType: "danmaku_burn",
						}
						if err := database.GetDB().Create(burnedPart).Error; err != nil {
							log.Printf("[启动恢复-烧录] 入库孤儿烧录文件失败: %v", err)
						} else if err := s.UploadPart(burnedPart, &h, &r); err != nil {
							log.Printf("[启动恢复-烧录] 孤儿烧录版入队失败: part_id=%d, err=%v", burnedPart.ID, err)
						}
						return // 不重新运行 ffmpeg
					}
				}

				// 磁盘上没有现成的弹幕烧录文件，重新运行 ffmpeg
				burnService := services.NewDanmakuBurnService()
				burnedPath, err := burnService.BurnDanmakuToVideo(&p, &h, &r)
				if err != nil {
					errorType := classifyUploadError(err)
					if shouldAutoStopErrorType(errorType, 1) {
						s.markDanmakuBurnSkipped(&p, "弹幕烧录失败，已停止该分P自动恢复: "+err.Error(), errorType)
					} else {
						log.Printf("[启动恢复-烧录] 烧录失败: part_id=%d, err=%v", p.ID, err)
					}
					return
				}
				log.Printf("[启动恢复-烧录] 烧录完成，准备入队: part_id=%d, burned=%s", p.ID, burnedPath)

				// 查询新生成的烧录版 Part 并入队
				var burnedPart models.RecordHistoryPart
				if err := database.GetDB().Where("file_path = ?", burnedPath).First(&burnedPart).Error; err != nil {
					log.Printf("[启动恢复-烧录] 查询烧录版 Part 失败: %v", err)
					return
				}
				if err := s.UploadPart(&burnedPart, &h, &r); err != nil {
					log.Printf("[启动恢复-烧录] 烧录版入队失败: part_id=%d, err=%v", burnedPart.ID, err)
				}
			}(part, history, room)
		}
	}
	if missingSourceCount > 0 || factorySkippedCount > 0 || requeuedCount > 0 {
		log.Printf("[启动恢复-烧录] 恢复扫描完成: 已触发=%d, 源文件缺失已标记=%d, DanmakuFactory不可用跳过=%d",
			requeuedCount, missingSourceCount, factorySkippedCount)
	}
}

// RecoverUnpublishedHistories 启动恢复：检测所有分P已上传完毕但历史记录尚未投稿的情况
// 场景：checkAndPublish 被容器重启打断，或烧录版上传完成后 checkAndPublish 从未触发
func (s *Service) RecoverUnpublishedHistories() {
	db := database.GetDB()
	allowPublishWhileRecording := false
	var sysConfig models.SystemConfig
	if err := db.First(&sysConfig).Error; err == nil {
		allowPublishWhileRecording = sysConfig.PublishWhileRecording
	}

	// 只处理启用了自动投稿的房间
	var rooms []models.RecordRoom
	if err := db.Where("upload = ? AND auto_publish = ?", true, true).Find(&rooms).Error; err != nil {
		log.Printf("[启动恢复-投稿] 查询房间失败: %v", err)
		return
	}

	for _, room := range rooms {
		if room.UploadUserID == 0 {
			continue
		}

		// 查找该房间未投稿的历史记录；默认仅处理已结束历史，开启边录制投稿时允许活动场次进入检查。
		var histories []models.RecordHistory
		query := db.Where("room_id = ? AND publish = ? AND upload = ?", room.RoomID, false, true)
		if !allowPublishWhileRecording {
			query = query.Where("recording = ? AND streaming = ?", false, false)
		}
		if err := query.Find(&histories).Error; err != nil {
			log.Printf("[启动恢复-投稿] 查询房间 %s 的历史记录失败: %v", room.RoomID, err)
			continue
		}

		for _, hist := range histories {
			if !hist.Upload {
				continue
			}
			// 检查是否有已上传的原始分P
			var uploadedCount, totalCount int64
			db.Model(&models.RecordHistoryPart{}).Where(
				"history_id = ? AND is_temp_file = ? AND NOT (file_delete = true AND upload = false)",
				hist.ID, false,
			).Count(&totalCount)
			db.Model(&models.RecordHistoryPart{}).Where(
				"history_id = ? AND upload = ? AND is_temp_file = ?",
				hist.ID, true, false,
			).Count(&uploadedCount)
			// 大文件切分兼容：原始分P退役后 totalCount==0，改用切分子分P统计
			if totalCount == 0 {
				db.Model(&models.RecordHistoryPart{}).Where(
					"history_id = ? AND is_temp_file = ? AND temp_file_type = ?",
					hist.ID, true, "split").Count(&totalCount)
				db.Model(&models.RecordHistoryPart{}).Where(
					"history_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ?",
					hist.ID, true, "split", true).Count(&uploadedCount)
			}

			if totalCount == 0 || uploadedCount < totalCount {
				continue // 还有未上传的原始/子分P，不尝试恢复投稿
			}

			log.Printf("[启动恢复-投稿] 发现已全部上传但未投稿的历史记录，重新检查: history_id=%d, 总分P=%d, 已上传=%d",
				hist.ID, totalCount, uploadedCount)
			// 复用 checkAndPublish 逻辑（含10分钟冷却检查），在单独 goroutine 中执行
			go func(h models.RecordHistory, r models.RecordRoom) {
				s.checkAndPublish(&h, &r)
			}(hist, room)
		}
	}
}
