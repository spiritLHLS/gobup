package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
	"gorm.io/gorm"
)

type QueuePartResponse struct {
	ID                  uint       `json:"id"`
	HistoryID           uint       `json:"historyId"`
	RoomID              string     `json:"roomId"`
	SessionID           string     `json:"sessionId"`
	Title               string     `json:"title"`
	FileName            string     `json:"fileName"`
	FilePath            string     `json:"filePath"`
	FileSize            int64      `json:"fileSize"`
	CreatedAt           time.Time  `json:"createdAt"`
	Uploading           bool       `json:"uploading"`
	Upload              bool       `json:"upload"`
	UploadRetryCount    int        `json:"uploadRetryCount"`
	UploadErrorMsg      string     `json:"uploadErrorMsg"`
	UploadLine          string     `json:"uploadLine"`
	RateLimitCooldownAt *time.Time `json:"rateLimitCooldownAt"`
}

// GetUploadQueueStatus 获取上传队列状态
func GetUploadQueueStatus(c *gin.Context) {
	if historyUploadService == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}

	queueManager := historyUploadService.GetQueueManager()
	status := queueManager.GetAllQueuesStatus()

	db := database.GetDB()
	pending := listQueueParts(db.Where("upload = ? AND recording = ? AND uploading = ? AND file_delete = ?", false, false, false, false).
		Order("start_time ASC").Limit(50))
	running := listQueueParts(db.Where("upload = ? AND uploading = ?", false, true).
		Order("created_at ASC").Limit(50))
	completed := listQueueParts(db.Where("upload = ?", true).
		Order("created_at DESC").Limit(50))

	var pendingCount int64
	var runningCount int64
	var completedCount int64
	db.Model(&models.RecordHistoryPart{}).Where("upload = ? AND recording = ? AND uploading = ? AND file_delete = ?", false, false, false, false).Count(&pendingCount)
	db.Model(&models.RecordHistoryPart{}).Where("upload = ? AND uploading = ?", false, true).Count(&runningCount)
	db.Model(&models.RecordHistoryPart{}).Where("upload = ?", true).Count(&completedCount)

	c.JSON(http.StatusOK, gin.H{
		"queues": status,
		"counts": gin.H{
			"pending":   pendingCount,
			"running":   runningCount,
			"completed": completedCount,
		},
		"pending":   pending,
		"running":   running,
		"completed": completed,
	})
}

// GetDanmakuQueueStatus 获取弹幕发送队列状态
func GetDanmakuQueueStatus(c *gin.Context) {
	danmakuService := services.NewDanmakuService()
	queueManager := danmakuService.GetQueueManager()
	status := queueManager.GetAllQueuesStatus()

	c.JSON(http.StatusOK, gin.H{
		"queues": status,
	})
}

func listQueueParts(query *gorm.DB) []QueuePartResponse {
	var parts []models.RecordHistoryPart
	if err := query.Find(&parts).Error; err != nil {
		return []QueuePartResponse{}
	}

	result := make([]QueuePartResponse, 0, len(parts))
	for _, part := range parts {
		result = append(result, QueuePartResponse{
			ID:                  part.ID,
			HistoryID:           part.HistoryID,
			RoomID:              part.RoomID,
			SessionID:           part.SessionID,
			Title:               part.Title,
			FileName:            part.FileName,
			FilePath:            part.FilePath,
			FileSize:            part.FileSize,
			CreatedAt:           part.CreatedAt,
			Uploading:           part.Uploading,
			Upload:              part.Upload,
			UploadRetryCount:    part.UploadRetryCount,
			UploadErrorMsg:      part.UploadErrorMsg,
			UploadLine:          part.UploadLine,
			RateLimitCooldownAt: part.RateLimitCooldownAt,
		})
	}
	return result
}
