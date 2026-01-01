package upload

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

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
	// 防止重复上传
	if _, loaded := s.uploadingParts.LoadOrStore(part.ID, true); loaded {
		return fmt.Errorf("分P %d 正在上传中", part.ID)
	}
	defer s.uploadingParts.Delete(part.ID)

	db := database.GetDB()

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

	// 计算总分片数
	fileInfo, err := os.Stat(part.FilePath)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}
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

	// 执行上传，支持重试
	var uploadResult *bili.UploadResult
	var uploadErr error

	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		uploadResult, uploadErr = uploader.Upload(part.FilePath)
		if uploadErr == nil {
			break
		}
		log.Printf("上传失败 (重试 %d/%d): %v", retry+1, maxRetries, uploadErr)
	}

	if uploadErr != nil {
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

	// 如果所有分P都上传完成且未投稿，则自动投稿
	if totalCount > 0 && totalCount == uploadedCount && !history.Publish && room.AutoUpload {
		log.Printf("所有分P上传完成，开始投稿: history_id=%d", history.ID)

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
