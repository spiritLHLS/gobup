package services

import (
	"log"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

// AutoUploadService 自动上传服务
type AutoUploadService struct{}

// NewAutoUploadService 创建自动上传服务
func NewAutoUploadService() *AutoUploadService {
	return &AutoUploadService{}
}

// ProcessPendingUploads 处理待上传的分P
// 根据房间的Upload和AutoUpload设置，自动将录制完成的分P加入上传队列
func (s *AutoUploadService) ProcessPendingUploads() error {
	db := database.GetDB()

	// 查询所有启用了上传功能的房间
	var rooms []models.RecordRoom
	if err := db.Where("upload = ?", true).Find(&rooms).Error; err != nil {
		return err
	}

	if len(rooms) == 0 {
		return nil
	}

	log.Printf("[自动上传] 开始检查 %d 个房间的待上传分P", len(rooms))

	totalProcessed := 0
	totalQueued := 0

	for _, room := range rooms {
		// 只处理启用了"自动上传分P"的房间
		if !room.AutoUpload {
			continue
		}

		// 检查房间是否配置了上传用户
		if room.UploadUserID == 0 {
			continue
		}

		// 查询该房间所有录制完成但未上传的分P
		// 条件：recording=false（录制完成）, upload=false（未上传）, uploading=false（未在上传中）
		var parts []models.RecordHistoryPart
		if err := db.Where(
			"room_id = ? AND recording = ? AND upload = ? AND uploading = ?",
			room.RoomID, false, false, false,
		).Order("start_time ASC").Find(&parts).Error; err != nil {
			log.Printf("[自动上传] 查询房间 %s 的待上传分P失败: %v", room.RoomID, err)
			continue
		}

		if len(parts) == 0 {
			continue
		}

		log.Printf("[自动上传] 房间 %s (%s) 有 %d 个待上传分P", room.RoomID, room.Uname, len(parts))

		// 为每个分P加入上传队列
		for _, part := range parts {
			// 获取对应的历史记录
			var history models.RecordHistory
			if err := db.First(&history, part.HistoryID).Error; err != nil {
				log.Printf("[自动上传] 获取历史记录失败: part_id=%d, history_id=%d, error=%v",
					part.ID, part.HistoryID, err)
				continue
			}

			// 检查文件是否存在
			if part.FilePath == "" {
				log.Printf("[自动上传] 跳过没有文件路径的分P: part_id=%d", part.ID)
				continue
			}

			// 跳过速率限制冷却期中的分P
			if part.RateLimitCooldownAt != nil && time.Now().Before(*part.RateLimitCooldownAt) {
				remainingTime := time.Until(*part.RateLimitCooldownAt)
				log.Printf("[自动上传] 跳过速率限制冷却期中的分P: part_id=%d, 剩余%.0f分钟",
					part.ID, remainingTime.Minutes())
				continue
			}

			// 将分P加入上传队列
			// 注意：这里需要通过upload服务来加入队列，不能直接操作
			// 由于循环依赖问题，我们返回需要上传的分P列表，由调用方处理
			totalProcessed++
		}

		if len(parts) > 0 {
			totalQueued += len(parts)
		}
	}

	if totalProcessed > 0 {
		log.Printf("[自动上传] 检查完成，发现 %d 个待上传分P", totalProcessed)
	}

	return nil
}

// GetPendingUploadParts 获取所有待上传的分P（供upload服务调用）
func (s *AutoUploadService) GetPendingUploadParts() ([]PendingUploadTask, error) {
	db := database.GetDB()

	// 查询所有启用了上传功能和自动上传的房间
	var rooms []models.RecordRoom
	if err := db.Where("upload = ? AND auto_upload = ?", true, true).Find(&rooms).Error; err != nil {
		return nil, err
	}

	var tasks []PendingUploadTask

	for _, room := range rooms {
		// 检查房间是否配置了上传用户
		if room.UploadUserID == 0 {
			continue
		}

		// 查询该房间所有录制完成但未上传的分P
		var parts []models.RecordHistoryPart
		if err := db.Where(
			"room_id = ? AND recording = ? AND upload = ? AND uploading = ?",
			room.RoomID, false, false, false,
		).Order("start_time ASC").Find(&parts).Error; err != nil {
			log.Printf("[自动上传] 查询房间 %s 的待上传分P失败: %v", room.RoomID, err)
			continue
		}

		for _, part := range parts {
			// 跳过没有文件路径的分P
			if part.FilePath == "" {
				continue
			}

			// 跳过速率限制冷却期中的分P
			if part.RateLimitCooldownAt != nil && time.Now().Before(*part.RateLimitCooldownAt) {
				continue
			}

			// 获取对应的历史记录
			var history models.RecordHistory
			if err := db.First(&history, part.HistoryID).Error; err != nil {
				log.Printf("[自动上传] 获取历史记录失败: part_id=%d, history_id=%d, error=%v",
					part.ID, part.HistoryID, err)
				continue
			}

			tasks = append(tasks, PendingUploadTask{
				Part:    part,
				History: history,
				Room:    room,
			})
		}
	}

	return tasks, nil
}

// PendingUploadTask 待上传任务
type PendingUploadTask struct {
	Part    models.RecordHistoryPart
	History models.RecordHistory
	Room    models.RecordRoom
}
