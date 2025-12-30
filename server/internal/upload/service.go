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
}

func NewService() *Service {
	return &Service{
		wxPusher:        services.NewWxPusherService(),
		templateSvc:     services.NewTemplateService(),
		progressTracker: NewProgressTracker(),
	}
}

// GetProgressTracker 获取进度追踪器
func (s *Service) GetProgressTracker() *ProgressTracker {
	return s.progressTracker
}

// UploadPart 上传分P
func (s *Service) UploadPart(part *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) error {
	// 防止重复上传
	if _, loaded := s.uploadingParts.LoadOrStore(part.ID, true); loaded {
		return fmt.Errorf("分P %d 正在上传中", part.ID)
	}
	defer s.uploadingParts.Delete(part.ID)

	db := database.GetDB()

	// 标记为上传中
	part.Uploading = true
	db.Save(part)
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

	// 根据线路选择上传器
	var uploader interface {
		Upload(string) (*bili.UploadResult, error)
	}

	switch room.Line {
	case "kodo":
		uploader = bili.NewKodoUploader(client)
	case "app":
		uploader = bili.NewAppUploader(client)
	default: // upos
		uploader = bili.NewUposUploader(client)
	}

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
