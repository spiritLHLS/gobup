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

// uploadPartInternal 实际执行上传分P（内部方法，由队列调用）
func (s *Service) uploadPartInternal(part *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) error {
	db := database.GetDB()

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

	// 推送上传开始通知
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
		s.wxPusher.NotifyUploadStart(room.UploadUserID, room.Wxuid, room.Uname, part.FileName, part.FileSize)
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

		// 推送失败通知
		if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
			s.wxPusher.NotifyUploadFailed(room.UploadUserID, room.Wxuid, room.Uname, part.FileName, uploadErr.Error())
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

	// 处理文件策略：3-上传后删除, 4-上传后移动, 6-上传后复制, 7-上传完成后立即删除
	if room.DeleteType == 3 || room.DeleteType == 4 || room.DeleteType == 6 || room.DeleteType == 7 {
		fileMoverSvc := services.NewFileMoverService()
		if err := fileMoverSvc.ProcessFilesByStrategy(history.ID, room.DeleteType); err != nil {
			log.Printf("文件处理失败: %v", err)
		}
	}

	// 推送成功通知
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "分P上传") {
		s.wxPusher.NotifyUploadSuccess(room.UploadUserID, room.Wxuid, room.Uname, part.FileName)
	}

	// 检查是否可以投稿
	s.checkAndPublish(history, room)

	return nil
}

func (s *Service) checkAndPublish(history *models.RecordHistory, room *models.RecordRoom) {
	db := database.GetDB()

	// 查询所有分P
	var totalCount int64
	var uploadedCount int64

	db.Model(&models.RecordHistoryPart{}).Where("history_id = ?", history.ID).Count(&totalCount)
	db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND upload = ?", history.ID, true).Count(&uploadedCount)

	// 如果所有分P都上传完成且未投稿，根据房间的AutoPublish设置决定是否自动投稿
	if totalCount > 0 && totalCount == uploadedCount && !history.Publish && room.AutoPublish {
		log.Printf("所有分P上传完成，房间设置允许自动投稿，开始投稿: history_id=%d", history.ID)

		if room.UploadUserID > 0 {
			if err := s.PublishHistory(history.ID, room.UploadUserID); err != nil {
				log.Printf("自动投稿失败: %v", err)
			}
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
		}

		if err := db.Create(newPart).Error; err != nil {
			return fmt.Errorf("创建新Part记录失败: %w", err)
		}

		newParts = append(newParts, newPart)
		log.Printf("[自动分P] 创建Part %d/%d 成功: id=%d, size=%d, duration=%d", i+1, numParts, newPart.ID, newPart.FileSize, newPart.Duration)
	}

	// 标记原始Part为已上传（实际上是被分割了）
	originalPart.Upload = true
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
