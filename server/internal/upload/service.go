package upload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/ratelimit"
	"github.com/gobup/server/internal/services"
)

const (
	ChunkSize = 5 * 1024 * 1024 // 5MB per chunk
)

type Service struct {
	uploadingParts       sync.Map // partID -> true，防止同一分P并发上传
	publishingHistories  sync.Map // historyID -> true，防止同一历史记录并发投稿
	appendingBurnedParts sync.Map // burnedPartID -> true，防止同一烧录版分P并发调用 EditVideo
	wxPusher             *services.WxPusherService
	templateSvc          *services.TemplateService
	progressTracker      *ProgressTracker
	queueManager         *QueueManager
	roundRobinCursor     uint64
}

func NewService() *Service {
	svc := &Service{
		wxPusher:        services.NewWxPusherService(),
		templateSvc:     services.NewTemplateService(),
		progressTracker: NewProgressTracker(),
	}
	svc.queueManager = NewQueueManager(svc)
	return svc
}

// GetProgressTracker 获取进度追踪器
func (s *Service) GetProgressTracker() *ProgressTracker {
	return s.progressTracker
}

// GetQueueManager 获取队列管理器
func (s *Service) GetQueueManager() *QueueManager {
	return s.queueManager
}

// UploadPart 上传分P（通过队列）
func (s *Service) UploadPart(part *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) error {
	if room == nil {
		return fmt.Errorf("房间配置为空")
	}
	if part == nil {
		return fmt.Errorf("分P为空")
	}
	if part.UploadPaused {
		return fmt.Errorf("分P %d 已暂停上传", part.ID)
	}
	if part.UploadCancelled {
		return fmt.Errorf("分P %d 已取消上传", part.ID)
	}

	selectedUserID, err := s.selectUploadUserID(room)
	if err != nil {
		return err
	}

	taskRoom := *room
	taskRoom.UploadUserID = selectedUserID
	if windowErr := newUploadWindowClosedError(&taskRoom, time.Now()); windowErr != nil {
		log.Printf("[队列] 分P %d 当前不在上传时间窗口(%s)，已入队等待约 %s 后重试",
			part.ID, windowErr.Window, formatDurationForLog(windowErr.RetryAfter))
	}

	return s.queueManager.AddTask(selectedUserID, part, history, &taskRoom)
}

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

// uploadPartInternal 实际执行上传分P（内部方法，由队列调用）
func (s *Service) uploadPartInternal(part *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) error {
	db := database.GetDB()

	// 权限检查1：房间是否启用上传功能
	if !room.Upload {
		log.Printf("[Upload] 房间未启用上传功能，拒绝上传: room_id=%s, part_id=%d", room.RoomID, part.ID)
		return fmt.Errorf("房间未启用上传功能")
	}

	// 权限检查2：历史记录是否允许上传
	if !history.Upload {
		log.Printf("[Upload] 历史记录禁止上传，拒绝上传: history_id=%d, part_id=%d", history.ID, part.ID)
		return fmt.Errorf("历史记录禁止上传")
	}

	// 权限检查3：房间是否配置了上传用户
	if room.UploadUserID == 0 {
		log.Printf("[Upload] 房间未配置上传用户，拒绝上传: room_id=%s, part_id=%d", room.RoomID, part.ID)
		return fmt.Errorf("房间未配置上传用户")
	}

	// 检查是否在速率限制冷却期内
	if part.RateLimitCooldownAt != nil && time.Now().Before(*part.RateLimitCooldownAt) {
		remainingTime := time.Until(*part.RateLimitCooldownAt)
		log.Printf("[速率限制] 分P %d 仍在冷却期内，剩余时间: %.0f分钟", part.ID, remainingTime.Minutes())
		return fmt.Errorf("速率限制冷却期中，剩余时间: %.0f分钟", remainingTime.Minutes())
	}

	// 如果冷却期已过，重置相关字段
	if part.RateLimitCooldownAt != nil && time.Now().After(*part.RateLimitCooldownAt) {
		log.Printf("[速率限制] 分P %d 冷却期已过，重置限制状态", part.ID)
		part.RateLimitCooldownAt = nil
		part.RateLimitRetryCount = 0
		part.UploadErrorMsg = ""
		part.UploadErrorType = ""
		db.Save(part)
	}

	// 防止重复上传（内存级锁，防止同一进程内并发）
	if _, loaded := s.uploadingParts.LoadOrStore(part.ID, true); loaded {
		return fmt.Errorf("分P %d 正在上传中", part.ID)
	}
	defer s.uploadingParts.Delete(part.ID)

	// 从数据库重新读取最新状态，防止使用调度器传入的过期结构体导致重复上传
	var freshPart models.RecordHistoryPart
	if err := db.First(&freshPart, part.ID).Error; err == nil {
		if freshPart.Upload && freshPart.CID > 0 {
			log.Printf("[Upload] 分P %d 已经上传过（DB最新状态），跳过: CID=%d, FileName=%s", part.ID, freshPart.CID, freshPart.FileName)
			return nil
		}
		if freshPart.UploadCancelled {
			log.Printf("[Upload] 分P %d 已取消上传，跳过: FileName=%s", part.ID, freshPart.FileName)
			return fmt.Errorf("分P %d 已取消上传", part.ID)
		}
		if freshPart.UploadPaused {
			log.Printf("[Upload] 分P %d 已暂停上传，跳过: FileName=%s", part.ID, freshPart.FileName)
			return fmt.Errorf("分P %d 已暂停上传", part.ID)
		}
		// 注意：不在此处因 freshPart.Uploading==true 而跳过。
		// RequeueStuckTempParts 会在入队前将 Uploading 置为 true 作为防重入队标记，
		// 但入队后由此处（已持有 uploadingParts 内存锁）负责实际上传。
		// 跳过逻辑由 uploadingParts.LoadOrStore 唯一保证——持有锁则继续，否则返回 "正在上传中"。
		// 用DB最新数据更新内存中的part，确保后续逻辑基于最新状态
		*part = freshPart
	} else {
		log.Printf("[Upload] 无法从DB读取分P %d 的最新状态，使用传入数据继续: %v", part.ID, err)
	}

	if windowErr := newUploadWindowClosedError(room, time.Now()); windowErr != nil {
		return windowErr
	}

	// 标记为上传中
	part.Uploading = true
	part.UploadUserID = room.UploadUserID
	db.Save(part)

	// 更新历史记录的上传状态为“上传中”
	if history.UploadStatus == 0 {
		history.UploadStatus = 1
		db.Save(history)
	}

	defer func() {
		part.Uploading = false
		db.Save(part)
	}()

	// 获取用户信息
	var user models.BiliBiliUser
	if err := db.First(&user, room.UploadUserID).Error; err != nil {
		log.Printf("上传用户未配置，跳过上传")
		return nil
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
		return fmt.Errorf("用户Cookie已失效")
	}

	// 检查文件是否存在
	if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", part.FilePath)
	}
	if err := validateUploadVideoPath(part.FilePath); err != nil {
		return err
	}

	// 检查文件是否需要分割（分片数超过10000）
	fileInfo, err := os.Stat(part.FilePath)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	var chunkSize int64
	switch room.Line {
	case "app":
		chunkSize = 2 * 1024 * 1024 // 2MB
	default: // upos
		chunkSize = 5 * 1024 * 1024 // 5MB
	}

	// 如果分片数超过10000，需要将文件分割成多个Part
	if bili.ShouldSplitFile(fileInfo.Size(), chunkSize) {
		chunkCount := bili.CalculateChunkCount(fileInfo.Size(), chunkSize)
		log.Printf("[自动分P] 文件 %s 分片数为 %d，超过10000限制，将自动分割成多个Part", part.FileName, chunkCount)

		// 调用分割函数
		if err := s.splitLargeFile(part, history, room); err != nil {
			return fmt.Errorf("文件分割失败: %w", err)
		}

		log.Printf("[自动分P] 文件分割完成，已创建多个Part，原Part标记为已上传")
		return nil
	}

	page := part.Page
	if page <= 0 {
		page = 1
	}

	uploadPath := part.FilePath
	cleanupUploadFile := func() {}
	if room.EnablePreTranscode && !part.IsTempFile {
		s.progressTracker.MarkTranscoding(int64(part.ID), int64(history.ID), page, 0, "准备转码")
		processedPath, cleanup, err := services.NewVideoProcessingService().TranscodeForUploadWithProgress(part.FilePath, room, func(percent int, msg string) {
			s.progressTracker.MarkTranscoding(int64(part.ID), int64(history.ID), page, percent, msg)
		})
		if err != nil {
			part.UploadErrorMsg = fmt.Sprintf("上传前转码失败: %v", err)
			part.UploadErrorType = UploadErrorTypeTranscode
			s.progressTracker.MarkFailed(int64(part.ID), part.UploadErrorMsg)
			db.Save(part)
			return fmt.Errorf("上传前转码失败: %w", err)
		}
		uploadPath = processedPath
		cleanupUploadFile = cleanup
		defer cleanupUploadFile()

		fileInfo, err = os.Stat(uploadPath)
		if err != nil {
			return fmt.Errorf("获取转码后文件信息失败: %w", err)
		}
	}

	log.Printf("开始上传: room=%s, file=%s, upload_file=%s, line=%s", room.RoomID, part.FilePath, uploadPath, room.Line)

	// 推送上传开始通知（使用历史记录中实际的主播名）
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
		s.wxPusher.NotifyUploadStart(room.UploadUserID, room.Wxuid, history.Uname, part.FileName, fileInfo.Size())
	}

	// 创建客户端
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	client.Line = room.Line // 设置上传线路
	if room.UploadSpeedLimitMBps > 0 {
		client.UploadRateLimiter = ratelimit.NewRateLimiter(room.UploadSpeedLimitMBps)
	}

	// 根据线路选择上传器
	var uploader interface {
		Upload(string) (*bili.UploadResult, error)
		SetProgressCallback(bili.ProgressCallback)
	}

	// 计算总分片数（复用前面已获取的fileInfo）
	var chunkTotal int
	switch room.Line {
	case "app":
		chunkTotal = int((fileInfo.Size() + 2*1024*1024 - 1) / (2 * 1024 * 1024))
		uploader = bili.NewAppUploader(client)
	default: // upos (包含所有upos线路)
		chunkTotal = int((fileInfo.Size() + 5*1024*1024 - 1) / (5 * 1024 * 1024))
		uploader = bili.NewUposUploader(client)
	}

	// 开始进度跟踪
	s.progressTracker.Start(int64(part.ID), int64(history.ID), page, chunkTotal)

	// 设置进度回调
	uploader.SetProgressCallback(func(chunkDone, chunkTotal int) {
		s.progressTracker.UpdateChunkDone(int64(part.ID), int64(history.ID), page, chunkDone, chunkTotal)
	})
	if retryAware, ok := uploader.(interface{ SetRetryCallback(bili.RetryCallback) }); ok {
		retryAware.SetRetryCallback(func(attempt, maxAttempts int, delay time.Duration, chunkDone, chunkTotal int) {
			s.progressTracker.MarkRetryWait(
				int64(part.ID),
				formatUploadRetryWaitMessage(attempt, maxAttempts, delay, chunkDone, chunkTotal),
			)
		})
	}

	// 执行上传（upload_upos.go内部已经有断点续传和重试机制）
	var uploadResult *bili.UploadResult
	var uploadErr error
	var uploadErrorType string

	uploadResult, uploadErr = uploader.Upload(uploadPath)

	if uploadErr != nil {
		uploadErrorType = classifyUploadError(uploadErr)
		if uploadErrorType == UploadErrorTypeRateLimit {
			log.Printf("检测到速率限制错误: %v", uploadErr)
		} else {
			log.Printf("上传失败: %v", uploadErr)
		}
	}

	if uploadErr != nil {
		// 如果是速率限制，并且所有重试都失败，设置指数退避冷却期
		rateLimitCooldownText := ""
		if uploadErrorType == UploadErrorTypeRateLimit {
			retryCount := part.RateLimitRetryCount + 1
			cooldownDelay, ok := autoTaskCooldownDuration(uploadErrorType, retryCount)
			if !ok {
				cooldownDelay = 24 * time.Hour
			}
			cooldownTime := time.Now().Add(cooldownDelay)
			part.RateLimitCooldownAt = &cooldownTime
			part.RateLimitRetryCount = retryCount
			rateLimitCooldownText = cooldownTime.Format("2006-01-02 15:04:05")
			part.UploadErrorMsg = fmt.Sprintf("速率限制，已设置%s冷却期至 %s", formatDurationForLog(cooldownDelay), rateLimitCooldownText)
			part.UploadErrorType = UploadErrorTypeRateLimit
			db.Save(part)
			log.Printf("[速率限制] 分P %d 触发限制，第%d次失败，冷却至: %s", part.ID, retryCount, rateLimitCooldownText)
		} else {
			part.UploadRetryCount++
			part.UploadErrorMsg = uploadErr.Error()
			part.UploadErrorType = uploadErrorType
			db.Save(part)
		}

		// 标记上传失败
		s.progressTracker.MarkFailed(int64(part.ID), uploadErr.Error())

		// 检查是否还有其他分P在上传
		var uploadingCount int64
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND uploading = ?", history.ID, true).Count(&uploadingCount)

		// 如果没有其他分P在上传了，根据已上传数量更新状态
		if uploadingCount <= 1 { // <=1 因为当前分P还在uploading中，defer还没执行
			var uploadedCount int64
			db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND upload = ?", history.ID, true).Count(&uploadedCount)
			if uploadedCount > 0 {
				history.UploadStatus = 2 // 有已上传的，设为已上传
			} else {
				history.UploadStatus = 0 // 没有已上传的，设为未上传
			}
			db.Save(history)
		}

		// 推送失败通知（使用历史记录中实际的主播名）
		if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
			if uploadErrorType == UploadErrorTypeRateLimit {
				s.wxPusher.NotifyRateLimit(room.UploadUserID, room.Wxuid, history.Uname, part.FileName, rateLimitCooldownText)
			} else {
				s.wxPusher.NotifyUploadFailed(room.UploadUserID, room.Wxuid, history.Uname, part.FileName, uploadErr.Error())
			}
		}
		return fmt.Errorf("上传失败: %w", uploadErr)
	}

	// 更新分P信息
	uploadedAt := time.Now()
	part.Upload = true
	part.UploadedAt = &uploadedAt
	part.UploadUserID = room.UploadUserID
	part.FileName = uploadResult.FileName
	part.CID = uploadResult.BizID
	part.UploadErrorMsg = ""
	part.UploadErrorType = ""
	db.Save(part)

	log.Printf("上传完成: part_id=%d, cid=%d", part.ID, part.CID)

	// 检查是否所有分P都已上传，更新History的UploadStatus
	// 使用与 checkAndPublish 完全一致的计数逻辑：
	// - 排除"已标记删除且未上传"的孤立/退役原始分P（file_delete=true AND upload=false）
	// - 大文件切分兼容：原始分P退役后 totalCount==0，改用 split 子分P统计
	var totalCount int64
	var uploadedCount int64
	db.Model(&models.RecordHistoryPart{}).Where(
		"history_id = ? AND is_temp_file = ? AND NOT (file_delete = ? AND upload = ?)",
		history.ID, false, true, false).Count(&totalCount)
	db.Model(&models.RecordHistoryPart{}).Where(
		"history_id = ? AND upload = ? AND is_temp_file = ?",
		history.ID, true, false).Count(&uploadedCount)
	if totalCount == 0 {
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND is_temp_file = ? AND temp_file_type = ?",
			history.ID, true, "split").Count(&totalCount)
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ?",
			history.ID, true, "split", true).Count(&uploadedCount)
	}

	if totalCount > 0 && uploadedCount == totalCount {
		// 所有分P已上传完成
		history.UploadStatus = 2
		db.Save(history)
	} else if uploadedCount > 0 {
		// 部分已上传
		history.UploadStatus = 2
		db.Save(history)
	}

	// 标记上传成功并移除进度
	s.progressTracker.MarkSuccessAndRemove(int64(part.ID))

	// 弹幕烧录：如果启用且不是临时文件，则生成带弹幕版本并上传
	// 注意：必须在删除策略(DeleteType 3/7)之前执行，否则原始文件被先删除导致烧录失败
	if room.EnableDanmakuBurn && !part.IsTempFile {
		log.Printf("[弹幕烧录] 检测到启用弹幕烧录功能，开始处理 part_id=%d", part.ID)
		burnService := services.NewDanmakuBurnService()

		// 生成带弹幕的视频文件
		burnedVideoPath, err := burnService.BurnDanmakuToVideo(part, history, room)
		if err != nil {
			log.Printf("[弹幕烧录] 烧录失败（将继续后续流程）: %v", err)
		} else {
			log.Printf("[弹幕烧录] 烧录成功，开始上传弹幕版: %s", burnedVideoPath)

			// 查询生成的弹幕版Part
			var burnedPart models.RecordHistoryPart
			if err := db.Where("file_path = ?", burnedVideoPath).First(&burnedPart).Error; err == nil {
				// 自动上传弹幕版
				if err := s.UploadPart(&burnedPart, history, room); err != nil {
					log.Printf("[弹幕烧录] 弹幕版上传失败: %v", err)
				} else {
					log.Printf("[弹幕烧录] 弹幕版已加入上传队列")
				}
			}
		}
	}

	// 触发“分P上传完成后”文件操作
	// 注意：弹幕烧录在前面已执行，此时原始文件已不再需要可以安全删除
	{
		fileMoverSvc := services.NewFileMoverService()
		if err := fileMoverSvc.TriggerFileOp(history.ID, room, services.FileOpTriggerAfterPart); err != nil {
			log.Printf("文件处理失败: %v", err)
		}
	}

	// 推送成功通知（使用历史记录中实际的主播名）
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
		s.wxPusher.NotifyUploadSuccess(room.UploadUserID, room.Wxuid, history.Uname, part.FileName)
	}

	// 回补检测：如果是弹幕版分P上传完成，检查是否需要更新已投稿的视频
	// EnableDanmakuBurn=true 时始终尝试回补（弹幕版是该功能的标准产物）
	// AutoUpdatePublished 作为兜底开关保留，任一为真即触发
	if part.IsTempFile && part.TempFileType == "danmaku_burn" && part.Upload && (room.EnableDanmakuBurn || room.AutoUpdatePublished) {
		log.Printf("[回补检测] 弹幕版上传完成，检查是否需要更新投稿: part_id=%d", part.ID)
		if err := s.UpdatePublishedVideoWithBurnedParts(part.ID); err != nil {
			log.Printf("[回补检测] 回补更新失败: %v", err)
		} else {
			log.Printf("[回补检测] 回补更新完成")
		}
	}

	// 检查是否可以投稿
	s.checkAndPublish(history, room)

	return nil
}

func formatUploadRetryWaitMessage(attempt, maxAttempts int, delay time.Duration, chunkDone, chunkTotal int) string {
	if attempt < 1 {
		attempt = 1
	}
	if maxAttempts < attempt {
		maxAttempts = attempt
	}
	base := fmt.Sprintf("等待 %s 后重试 (%d/%d)", formatDurationForLog(delay), attempt, maxAttempts)
	if chunkTotal > 0 {
		if chunkDone < 0 {
			chunkDone = 0
		}
		return fmt.Sprintf("%s，已完成分片 %d/%d", base, chunkDone, chunkTotal)
	}
	return base
}

func (s *Service) checkAndPublish(history *models.RecordHistory, room *models.RecordRoom) {
	db := database.GetDB()

	// 从DB重新读取最新的历史记录状态，避免使用upload开始时传入的过期结构体
	// 关键：history.Publish 可能在多个goroutine并发场景下已被另一路径更新为true
	var freshHistory models.RecordHistory
	if err := db.First(&freshHistory, history.ID).Error; err != nil {
		log.Printf("[自动投稿] 无法读取最新历史记录状态，终止自动投稿: history_id=%d, err=%v", history.ID, err)
		return
	}
	// 使用freshHistory进行后续判断（替代stale的传入指针）
	history = &freshHistory
	if !history.Upload {
		return
	}
	if history.PublishCooldownAt != nil && history.PublishCooldownAt.After(time.Now()) {
		log.Printf("[自动投稿] 历史记录处于投稿冷却期，暂缓投稿: history_id=%d, cooldown_until=%s, error_type=%s",
			history.ID, history.PublishCooldownAt.Format("2006-01-02 15:04:05"), history.PublishErrorType)
		return
	}
	var sysConfig models.SystemConfig
	allowPublishWhileRecording := false
	if err := db.First(&sysConfig).Error; err == nil {
		allowPublishWhileRecording = sysConfig.PublishWhileRecording
	}
	// totalCount 只统计非临时文件（is_temp_file=false），并排除"已标记删除且未上传"的孤立记录：
	//   - 烧录/切分产生的临时分P（is_temp_file=true）不应阻塞投稿条件
	//   - file_delete=true 且 upload=false ：孤立临时分P（烧录失败后清理标记），不能阻塞投稿
	//   - file_delete=true 且 upload=true  ：正常已上传并补删（应计入 uploadedCount）
	var totalCount int64
	var uploadedCount int64
	var recordingCount int64

	db.Model(&models.RecordHistoryPart{}).Where(
		"history_id = ? AND is_temp_file = ? AND NOT (file_delete = true AND upload = false)",
		history.ID, false).Count(&totalCount)
	db.Model(&models.RecordHistoryPart{}).Where(
		"history_id = ? AND upload = ? AND is_temp_file = ?",
		history.ID, true, false).Count(&uploadedCount)
	// 大文件切分兼容：原始分P被切分退役后（file_delete=true, upload=false），totalCount==0。
	// 此时改用切分子分P（is_temp_file=true, temp_file_type='split'）的数量作为代替，
	// 确保所有子分P上传完成后投稿条件能正常触发。
	if totalCount == 0 {
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND is_temp_file = ? AND temp_file_type = ?",
			history.ID, true, "split").Count(&totalCount)
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ?",
			history.ID, true, "split", true).Count(&uploadedCount)
	}
	db.Model(&models.RecordHistoryPart{}).Where(
		"history_id = ? AND recording = ?",
		history.ID, true).Count(&recordingCount)

	// 关键判断：只有在以下所有条件满足时才自动投稿
	// 1. 有分P存在（totalCount > 0）
	// 2. 所有分P都已上传（totalCount == uploadedCount）
	// 3. 没有正在录制的分P（recordingCount == 0）- 确保这场直播已完全结束
	// 4. 历史记录未投稿（!history.Publish）
	// 5. 历史记录未标记为正在录制/直播（!history.Recording && !history.Streaming）
	// 6. 房间启用了自动投稿（room.AutoPublish）

	// 如果启用了弹幕烧录，需等待烧录版分P也上传完才一起投稿，确保弹幕版也包含在提交中
	if room.EnableDanmakuBurn {
		var pendingBurnedParts int64
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ? AND file_delete = ?",
			history.ID, true, "danmaku_burn", false, false,
		).Count(&pendingBurnedParts)
		if pendingBurnedParts > 0 {
			log.Printf("[自动投稿] 等待 %d 个弹幕烧录版分P上传完成后再投稿: history_id=%d", pendingBurnedParts, history.ID)
			return
		}
	}

	noActiveRecording := recordingCount == 0 && !history.Recording && !history.Streaming
	if totalCount > 0 && totalCount == uploadedCount &&
		!history.Publish && room.AutoPublish &&
		(noActiveRecording || allowPublishWhileRecording) {

		// 额外验证：检查最后一个分P的结束时间，确保距离现在已超过10分钟
		// 这是为了应对：同场直播的最后一个分P已上传，但可能马上又有新的分P产生的情况
		if !allowPublishWhileRecording {
			var lastPart models.RecordHistoryPart
			err := db.Where("history_id = ?", history.ID).
				Order("end_time DESC").
				First(&lastPart).Error

			if err == nil {
				timeSinceLastPart := time.Since(lastPart.EndTime)
				if timeSinceLastPart < 10*time.Minute {
					log.Printf("[自动投稿] 最后分P结束时间过近(%.1f分钟)，暂缓投稿等待确认直播结束: history_id=%d",
						timeSinceLastPart.Minutes(), history.ID)
					return
				}
			}
		} else if !noActiveRecording {
			log.Printf("[自动投稿] 已开启边录制边投稿，允许直播仍在进行时提交已完成分P: history_id=%d", history.ID)
		}

		log.Printf("[自动投稿] 所有条件满足，开始自动投稿: history_id=%d, 总分P=%d, 已上传=%d",
			history.ID, totalCount, uploadedCount)

		if room.UploadUserID > 0 {
			// 触发“全部分P上传完成、投稿前”文件操作
			{
				fileMoverSvc := services.NewFileMoverService()
				if err := fileMoverSvc.TriggerFileOp(history.ID, room, services.FileOpTriggerBeforePublish); err != nil {
					log.Printf("[自动投稿] 投稿前文件处理失败: %v", err)
				}
			}
			if err := s.PublishHistory(history.ID, room.UploadUserID); err != nil {
				log.Printf("[自动投稿] 投稿失败: %v", err)
			} else {
				log.Printf("[自动投稿] 投稿成功: history_id=%d", history.ID)

				// 投稿成功后，检查同SessionID是否还有其他已上传完成但未投稿的历史记录
				// 如果有，应该将它们追加到刚才投稿的视频上
				if room.MergeBySession && (history.SessionID != "" || normalizePublishTitle(history.Title) != "") {
					log.Printf("[自动投稿] 投稿成功后检查同SessionID/同标题是否有待追加记录: session_id=%s title=%s",
						history.SessionID, history.Title)
					go s.checkAndAppendPendingHistories(history, room)
				}
			}
		} else {
			log.Printf("[自动投稿] 房间未配置上传用户，无法投稿: history_id=%d", history.ID)
		}
	} else if totalCount > 0 && uploadedCount < totalCount {
		log.Printf("[自动投稿] 等待所有分P上传完成: history_id=%d, 总分P=%d, 已上传=%d, 正在录制=%d",
			history.ID, totalCount, uploadedCount, recordingCount)
	} else if !noActiveRecording && !allowPublishWhileRecording {
		log.Printf("[自动投稿] 直播仍在进行，等待录制完成: history_id=%d, 正在录制分P=%d",
			history.ID, recordingCount)
	}
}

// checkAndAppendPendingHistories 检查并追加同SessionID的待投稿历史记录（延迟执行避免冲突）
func (s *Service) checkAndAppendPendingHistories(publishedHistory *models.RecordHistory, room *models.RecordRoom) {
	// 延迟30秒，等待视频状态稳定
	time.Sleep(30 * time.Second)

	db := database.GetDB()

	log.Printf("[投稿后检查] 开始检查同SessionID/同标题待追加记录: session_id=%s title=%s",
		publishedHistory.SessionID, publishedHistory.Title)

	// 查询同SessionID或同标题的其他历史记录（未投稿但有已上传分P的）
	var pendingHistories []models.RecordHistory
	pendingQuery := db.Where("publish = ? AND room_id = ? AND id != ?", false, room.RoomID, publishedHistory.ID)
	normalizedTitle := normalizePublishTitle(publishedHistory.Title)
	if publishedHistory.SessionID != "" && normalizedTitle != "" {
		pendingQuery = pendingQuery.Where("(session_id = ? OR title = ?)", publishedHistory.SessionID, normalizedTitle)
	} else if publishedHistory.SessionID != "" {
		pendingQuery = pendingQuery.Where("session_id = ?", publishedHistory.SessionID)
	} else if normalizedTitle != "" {
		pendingQuery = pendingQuery.Where("title = ?", normalizedTitle)
	} else {
		log.Printf("[投稿后检查] SessionID和标题均为空，跳过")
		return
	}
	if err := pendingQuery.Find(&pendingHistories).Error; err != nil {
		log.Printf("[投稿后检查] 查询失败: %v", err)
		return
	}

	if len(pendingHistories) == 0 {
		log.Printf("[投稿后检查] 未发现待追加记录")
		return
	}

	log.Printf("[投稿后检查] 发现 %d 个可能需要追加的历史记录", len(pendingHistories))

	for _, pendingHistory := range pendingHistories {
		// 检查是否有已上传的分P（与 checkAndPublish 保持完全一致的计数逻辑）
		var uploadedCount int64
		var totalCount int64
		var recordingCount int64

		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND upload = ? AND is_temp_file = ?",
			pendingHistory.ID, true, false).Count(&uploadedCount)
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND is_temp_file = ? AND NOT (file_delete = true AND upload = false)",
			pendingHistory.ID, false).Count(&totalCount)
		// 大文件切分兼容：原始分P退役后 totalCount==0，改用切分子分P统计
		if totalCount == 0 {
			db.Model(&models.RecordHistoryPart{}).Where(
				"history_id = ? AND is_temp_file = ? AND temp_file_type = ?",
				pendingHistory.ID, true, "split").Count(&totalCount)
			db.Model(&models.RecordHistoryPart{}).Where(
				"history_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ?",
				pendingHistory.ID, true, "split", true).Count(&uploadedCount)
		}
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND recording = ?", pendingHistory.ID, true).Count(&recordingCount)

		// 只追加已全部上传完成且没有正在录制的历史记录
		if uploadedCount > 0 && totalCount == uploadedCount && recordingCount == 0 {
			log.Printf("[投稿后检查] 发现可追加记录: history_id=%d, 已上传分P=%d, 将触发追加",
				pendingHistory.ID, uploadedCount)

			// 触发投稿（会自动检测到同SessionID已有投稿并追加）
			if err := s.PublishHistory(pendingHistory.ID, room.UploadUserID); err != nil {
				log.Printf("[投稿后检查] 追加投稿失败: history_id=%d, error=%v", pendingHistory.ID, err)
			} else {
				log.Printf("[投稿后检查] 追加投稿成功: history_id=%d", pendingHistory.ID)
			}

			// 避免请求过快，间隔5秒
			time.Sleep(5 * time.Second)
		}
	}
}
