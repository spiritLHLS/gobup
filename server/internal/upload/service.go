package upload

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

const (
	ChunkSize = 5 * 1024 * 1024 // 5MB per chunk
)

type Service struct {
	uploadingParts  sync.Map
	wxPusher        *services.WxPusherService
	templateSvc     *services.TemplateService
	progressTracker *ProgressTracker
	queueManager    *QueueManager
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
	// 将任务添加到用户的上传队列
	if room.UploadUserID == 0 {
		return fmt.Errorf("房间未配置上传用户")
	}

	return s.queueManager.AddTask(room.UploadUserID, part, history, room)
}

// RequeueStuckTempParts 将服务重启后滞留在DB中的临时分P重新加入上传队列
// 触发场景：弹幕烧录成功 → 临时Part入库 → 服务崩溃/重启 → 内存队列丢失
// 由调度器在启动后调用一次，确保这些分P不丢失
// 通过立即将 Uploading 置为 true 防止10分钟周期调用时重复入队
func (s *Service) RequeueStuckTempParts() {
	db := database.GetDB()

	var stuckParts []models.RecordHistoryPart
	if err := db.Where(
		"is_temp_file = ? AND upload = ? AND uploading = ? AND file_delete = ?",
		true, false, false, false,
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

		if room.UploadUserID == 0 {
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
		Where("uploading = ? AND upload = ?", true, false).
		Updates(map[string]interface{}{"uploading": false})
	if result.Error != nil {
		log.Printf("[启动恢复] 重置滞留 uploading 状态失败: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("[启动恢复] 重置了 %d 个上次崩溃时卡在 uploading=true 的分P", result.RowsAffected)
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
		db.Save(part)
	}

	// 防止重复上传
	if _, loaded := s.uploadingParts.LoadOrStore(part.ID, true); loaded {
		return fmt.Errorf("分P %d 正在上传中", part.ID)
	}
	defer s.uploadingParts.Delete(part.ID)

	// 检查是否已经上传过（防止重复上传）
	if part.Upload && part.CID > 0 {
		log.Printf("[Upload] 分P %d 已经上传过，跳过: CID=%d, FileName=%s", part.ID, part.CID, part.FileName)
		return nil
	}

	// 标记为上传中
	part.Uploading = true
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

	log.Printf("开始上传: room=%s, file=%s, line=%s", room.RoomID, part.FilePath, room.Line)

	// 推送上传开始通知（使用历史记录中实际的主播名）
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
		s.wxPusher.NotifyUploadStart(room.UploadUserID, room.Wxuid, history.Uname, part.FileName, part.FileSize)
	}

	// 创建客户端
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	client.Line = room.Line // 设置上传线路

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
	page := 1 // 默认第一页
	s.progressTracker.Start(int64(part.ID), int64(history.ID), page, chunkTotal)

	// 设置进度回调
	uploader.SetProgressCallback(func(chunkDone, chunkTotal int) {
		s.progressTracker.UpdateChunkDone(int64(part.ID), int64(history.ID), page, chunkDone, chunkTotal)
	})

	// 执行上传（upload_upos.go内部已经有断点续传和重试机制）
	var uploadResult *bili.UploadResult
	var uploadErr error
	var is406RateLimit bool

	uploadResult, uploadErr = uploader.Upload(part.FilePath)

	if uploadErr != nil {
		// 检测是否为真正的406/601速率限制错误
		// 只有明确的HTTP状态码才判定为速率限制，避免误判网络错误
		errMsg := uploadErr.Error()
		if contains(errMsg, "HTTP 406") || contains(errMsg, "HTTP 601") || contains(errMsg, "上传视频过快") {
			is406RateLimit = true
			log.Printf("检测到速率限制错误: %v", uploadErr)
		} else {
			log.Printf("上传失败: %v", uploadErr)
		}
	}

	if uploadErr != nil {
		// 如果是406速率限制，并且所有重试都失败，设置24小时冷却期
		if is406RateLimit {
			cooldownTime := time.Now().Add(24 * time.Hour)
			part.RateLimitCooldownAt = &cooldownTime
			part.RateLimitRetryCount++
			part.UploadErrorMsg = fmt.Sprintf("速率限制(406)，已设置24小时冷却期至 %s", cooldownTime.Format("2006-01-02 15:04:05"))
			db.Save(part)
			log.Printf("[速率限制] 分P %d 触发406限制，设置24小时冷却期至: %s", part.ID, cooldownTime.Format("2006-01-02 15:04:05"))
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
			s.wxPusher.NotifyUploadFailed(room.UploadUserID, room.Wxuid, history.Uname, part.FileName, uploadErr.Error())
		}
		return fmt.Errorf("上传失败: %w", uploadErr)
	}

	// 更新分P信息
	part.Upload = true
	part.FileName = uploadResult.FileName
	part.CID = uploadResult.BizID
	db.Save(part)

	log.Printf("上传完成: part_id=%d, cid=%d", part.ID, part.CID)

	// 检查是否所有分P都已上传，更新History的UploadStatus
	var totalCount int64
	var uploadedCount int64
	db.Model(&models.RecordHistoryPart{}).Where("history_id = ?", history.ID).Count(&totalCount)
	db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND upload = ?", history.ID, true).Count(&uploadedCount)

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

	// 处理文件策略：3-上传后删除, 4-上传后移动, 6-上传后复制, 7-上传完成后立即删除
	// 注意：弹幕烧录在前面已执行，此时原始文件已不再需要可以安全删除
	if room.DeleteType == 3 || room.DeleteType == 4 || room.DeleteType == 6 || room.DeleteType == 7 {
		fileMoverSvc := services.NewFileMoverService()
		if err := fileMoverSvc.ProcessFilesByStrategy(history.ID, room.DeleteType); err != nil {
			log.Printf("文件处理失败: %v", err)
		}
	}

	// 推送成功通知（使用历史记录中实际的主播名）
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
		s.wxPusher.NotifyUploadSuccess(room.UploadUserID, room.Wxuid, history.Uname, part.FileName)
	}

	// 回补检测：如果是弹幕版分P上传完成且房间启用了自动更新投稿，检查是否需要更新已投稿的视频
	if part.IsTempFile && part.TempFileType == "danmaku_burn" && part.Upload && room.AutoUpdatePublished {
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

func (s *Service) checkAndPublish(history *models.RecordHistory, room *models.RecordRoom) {
	db := database.GetDB()

	// 查询分P数统计（仅统计原始非临时分P）
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
	if totalCount > 0 && totalCount == uploadedCount && recordingCount == 0 &&
		!history.Publish && !history.Recording && !history.Streaming &&
		room.AutoPublish {

		// 额外验证：检查最后一个分P的结束时间，确保距离现在已超过10分钟
		// 这是为了应对：同场直播的最后一个分P已上传，但可能马上又有新的分P产生的情况
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

		log.Printf("[自动投稿] 所有条件满足，开始自动投稿: history_id=%d, 总分P=%d, 已上传=%d",
			history.ID, totalCount, uploadedCount)

		if room.UploadUserID > 0 {
			if err := s.PublishHistory(history.ID, room.UploadUserID); err != nil {
				log.Printf("[自动投稿] 投稿失败: %v", err)
			} else {
				log.Printf("[自动投稿] 投稿成功: history_id=%d", history.ID)
				
				// 投稿成功后，检查同SessionID是否还有其他已上传完成但未投稿的历史记录
				// 如果有，应该将它们追加到刚才投稿的视频上
				if room.MergeBySession && history.SessionID != "" {
					log.Printf("[自动投稿] 投稿成功后检查同SessionID是否有待追加记录: session_id=%s", history.SessionID)
					go s.checkAndAppendPendingHistories(history, room)
				}
			}
		} else {
			log.Printf("[自动投稿] 房间未配置上传用户，无法投稿: history_id=%d", history.ID)
		}
	} else if totalCount > 0 && uploadedCount < totalCount {
		log.Printf("[自动投稿] 等待所有分P上传完成: history_id=%d, 总分P=%d, 已上传=%d, 正在录制=%d",
			history.ID, totalCount, uploadedCount, recordingCount)
	} else if recordingCount > 0 {
		log.Printf("[自动投稿] 仍有分P正在录制，等待录制完成: history_id=%d, 正在录制=%d",
			history.ID, recordingCount)
	}
}

// checkAndAppendPendingHistories 检查并追加同SessionID的待投稿历史记录（延迟执行避免冲突）
func (s *Service) checkAndAppendPendingHistories(publishedHistory *models.RecordHistory, room *models.RecordRoom) {
	// 延迟30秒，等待视频状态稳定
	time.Sleep(30 * time.Second)
	
	db := database.GetDB()
	
	log.Printf("[投稿后检查] 开始检查同SessionID待追加记录: session_id=%s", publishedHistory.SessionID)
	
	// 查询同SessionID的其他历史记录（未投稿但有已上传分P的）
	var pendingHistories []models.RecordHistory
	if err := db.Where("session_id = ? AND publish = ? AND room_id = ? AND id != ?", 
		publishedHistory.SessionID, false, room.RoomID, publishedHistory.ID).Find(&pendingHistories).Error; err != nil {
		log.Printf("[投稿后检查] 查询失败: %v", err)
		return
	}
	
	if len(pendingHistories) == 0 {
		log.Printf("[投稿后检查] 未发现待追加记录")
		return
	}
	
	log.Printf("[投稿后检查] 发现 %d 个可能需要追加的历史记录", len(pendingHistories))
	
	for _, pendingHistory := range pendingHistories {
		// 检查是否有已上传的分P
		var uploadedCount int64
		var totalCount int64
		var recordingCount int64
		
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ? AND upload = ? AND file_delete = ?", 
			pendingHistory.ID, true, false).Count(&uploadedCount)
		db.Model(&models.RecordHistoryPart{}).Where(
			"history_id = ?", pendingHistory.ID).Count(&totalCount)
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

// containsTag 检查标签列表中是否包含指定标签
func containsTag(tags, target string) bool {
	tagList := strings.Split(tags, ",")
	for _, tag := range tagList {
		if strings.TrimSpace(tag) == target {
			return true
		}
	}
	return false
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// splitLargeFile 将大文件分割成多个Part（当分片数超过10000时）
func (s *Service) splitLargeFile(originalPart *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) error {
	db := database.GetDB()

	// 获取文件信息
	fileInfo, err := os.Stat(originalPart.FilePath)
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

	totalChunks := bili.CalculateChunkCount(fileInfo.Size(), chunkSize)

	// 计算需要分成多少个Part（每个Part最多9000个分片，留一些余量）
	maxChunksPerPart := int64(9000)
	numParts := (totalChunks + maxChunksPerPart - 1) / maxChunksPerPart

	// 计算每个Part的时长（假设视频时长均匀分布）
	totalDuration := originalPart.Duration
	if totalDuration == 0 {
		// 如果没有时长信息，使用文件大小比例来估算
		log.Printf("[自动分P] 警告：原始Part没有时长信息，将平均分割")
		totalDuration = int(numParts) * 3600 // 假设每个Part 1小时
	}

	durationPerPart := totalDuration / int(numParts)

	log.Printf("[自动分P] 将文件分割成 %d 个Part，每个Part约 %d 秒", numParts, durationPerPart)

	// 使用ffmpeg分割文件
	baseDir := filepath.Dir(originalPart.FilePath)
	baseNameWithoutExt := strings.TrimSuffix(filepath.Base(originalPart.FilePath), filepath.Ext(originalPart.FilePath))
	ext := filepath.Ext(originalPart.FilePath)

	// 创建新的Part记录
	var newParts []*models.RecordHistoryPart
	for i := int64(0); i < numParts; i++ {
		startTime := int(i) * durationPerPart
		duration := durationPerPart
		if i == numParts-1 {
			// 最后一个Part包含剩余的所有时间
			duration = totalDuration - startTime
		}

		// 生成输出文件名
		outputFileName := fmt.Sprintf("%s_part%d%s", baseNameWithoutExt, i+1, ext)
		outputPath := filepath.Join(baseDir, outputFileName)

		// 使用ffmpeg切割视频
		// -ss: 开始时间 -t: 持续时间 -c copy: 不重新编码
		var ffmpegArgs []string
		if startTime > 0 {
			ffmpegArgs = append(ffmpegArgs, "-ss", fmt.Sprintf("%d", startTime))
		}
		ffmpegArgs = append(ffmpegArgs,
			"-i", originalPart.FilePath,
			"-t", fmt.Sprintf("%d", duration),
			"-c", "copy",
			"-avoid_negative_ts", "1",
			outputPath,
		)

		log.Printf("[自动分P] 正在切割Part %d/%d: %s (时长: %ds)", i+1, numParts, outputFileName, duration)

		cmd := exec.Command("ffmpeg", ffmpegArgs...)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[自动分P] ffmpeg输出: %s", string(output))
			return fmt.Errorf("ffmpeg切割失败 (Part %d): %w", i+1, err)
		}

		// 获取切割后的文件大小
		splitFileInfo, err := os.Stat(outputPath)
		if err != nil {
			return fmt.Errorf("获取切割文件信息失败: %w", err)
		}

		// 创建新的Part记录
		newPart := &models.RecordHistoryPart{
			HistoryID:  originalPart.HistoryID,
			RoomID:     originalPart.RoomID,
			SessionID:  originalPart.SessionID,
			Title:      originalPart.Title,
			LiveTitle:  originalPart.LiveTitle,
			AreaName:   originalPart.AreaName,
			FilePath:   outputPath,
			FileName:   outputFileName,
			FileSize:   splitFileInfo.Size(),
			Duration:   duration,
			StartTime:  originalPart.StartTime.Add(time.Duration(startTime) * time.Second),
			EndTime:    originalPart.StartTime.Add(time.Duration(startTime+duration) * time.Second),
			Recording:  false,
			Upload:     false,
			Uploading:  false,
			Page:       0,
			XcodeState: 0,
			IsTempFile:   true,                 // 标记为临时文件
			SourcePartID: originalPart.ID,      // 记录源Part ID
			TempFileType: "split",              // 切分文件类型
		}

		if err := db.Create(newPart).Error; err != nil {
			return fmt.Errorf("创建新Part记录失败: %w", err)
		}

		newParts = append(newParts, newPart)
		log.Printf("[自动分P] 创建Part %d/%d 成功: id=%d, size=%d, duration=%d", i+1, numParts, newPart.ID, newPart.FileSize, newPart.Duration)
	}

	// 删除原始文件，避免被识别为cid=0的文件
	if err := os.Remove(originalPart.FilePath); err != nil {
		log.Printf("[自动分P] 删除原始文件失败: %v (文件: %s)", err, originalPart.FilePath)
	} else {
		log.Printf("[自动分P] 原始文件已删除: %s", originalPart.FilePath)
	}

	// 标记原始Part为已上传（实际上是被分割了），并标记文件已删除
	originalPart.Upload = true
	originalPart.FileDelete = true
	originalPart.UploadErrorMsg = fmt.Sprintf("文件过大(分片数%d>10000)，已自动分割成%d个Part", totalChunks, numParts)
	db.Save(originalPart)

	log.Printf("[自动分P] 文件分割完成，已创建 %d 个新Part，原Part(id=%d)标记为已处理", len(newParts), originalPart.ID)

	// 将新创建的Part添加到上传队列
	for _, newPart := range newParts {
		if err := s.UploadPart(newPart, history, room); err != nil {
			log.Printf("[自动分P] 将新Part(id=%d)加入上传队列失败: %v", newPart.ID, err)
		} else {
			log.Printf("[自动分P] 新Part(id=%d)已加入上传队列", newPart.ID)
		}
	}

	return nil
}
