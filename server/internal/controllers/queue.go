package controllers

import (
	"net/http"
	"strconv"
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
	LiveTitle           string     `json:"liveTitle"`
	AreaName            string     `json:"areaName"`
	FileName            string     `json:"fileName"`
	FilePath            string     `json:"filePath"`
	FileSize            int64      `json:"fileSize"`
	Duration            int        `json:"duration"`
	CreatedAt           time.Time  `json:"createdAt"`
	StartTime           time.Time  `json:"startTime"`
	EndTime             time.Time  `json:"endTime"`
	Uploading           bool       `json:"uploading"`
	Upload              bool       `json:"upload"`
	UploadRetryCount    int        `json:"uploadRetryCount"`
	UploadErrorMsg      string     `json:"uploadErrorMsg"`
	UploadErrorType     string     `json:"uploadErrorType"`
	UploadLine          string     `json:"uploadLine"`
	UploadPaused        bool       `json:"uploadPaused"`
	UploadCancelled     bool       `json:"uploadCancelled"`
	FileDelete          bool       `json:"fileDelete"`
	FileMoved           bool       `json:"fileMoved"`
	Page                int        `json:"page"`
	CID                 int64      `json:"cid"`
	Status              string     `json:"status"`
	RateLimitCooldownAt *time.Time `json:"rateLimitCooldownAt"`
	IsTempFile          bool       `json:"isTempFile"`
	SourcePartID        uint       `json:"sourcePartId"`
	TempFileType        string     `json:"tempFileType"`
	AppendedToVideo     bool       `json:"appendedToVideo"`
}

// GetUploadQueueStatus 获取上传队列状态。
//
// @Summary Get upload queue status
// @Description Returns queue snapshots, counts, and representative pending/running/paused/cancelled/completed tasks.
// @Tags queue
// @Security BasicAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /queue/upload/status [get]
func GetUploadQueueStatus(c *gin.Context) {
	if historyUploadService == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}

	c.JSON(http.StatusOK, buildUploadQueueSnapshot())
}

// GetTaskManagerStatus 获取所有长任务的统一快照。
func GetTaskManagerStatus(c *gin.Context) {
	db := database.GetDB()

	danmakuService := services.NewDanmakuService()
	parseQueue := services.NewDanmakuParserQueue()

	var syncTasks []models.VideoSyncTask
	db.Order("updated_at DESC").Limit(50).Find(&syncTasks)

	now := time.Now()
	var publishCooldown []models.RecordHistory
	db.Where("publish = ? AND publish_cooldown_at > ?", false, now).
		Order("publish_cooldown_at ASC").
		Limit(50).
		Find(&publishCooldown)

	var publishCooldownCount int64
	var publishFailedCount int64
	db.Model(&models.RecordHistory{}).Where("publish = ? AND publish_cooldown_at > ?", false, now).Count(&publishCooldownCount)
	db.Model(&models.RecordHistory{}).Where("publish = ? AND publish_error_type != ''", false).Count(&publishFailedCount)

	var syncCounts []struct {
		Status string
		Count  int64
	}
	db.Model(&models.VideoSyncTask{}).Select("status, COUNT(*) as count").Group("status").Scan(&syncCounts)
	syncCountMap := map[string]int64{
		"pending":   0,
		"running":   0,
		"completed": 0,
		"failed":    0,
	}
	for _, row := range syncCounts {
		syncCountMap[row.Status] = row.Count
	}

	c.JSON(http.StatusOK, gin.H{
		"upload": buildUploadQueueSnapshot(),
		"danmaku": gin.H{
			"queues": danmakuService.GetQueueManager().GetAllQueuesStatus(),
		},
		"parse": gin.H{
			"queueLength": parseQueue.GetQueueLength(),
			"processing":  parseQueue.IsProcessing(),
		},
		"publish": gin.H{
			"counts": gin.H{
				"cooldown": publishCooldownCount,
				"failed":   publishFailedCount,
			},
			"cooldown": publishCooldown,
		},
		"sync": gin.H{
			"counts": syncCountMap,
			"tasks":  syncTasks,
		},
	})
}

func buildUploadQueueSnapshot() gin.H {
	if historyUploadService == nil {
		return gin.H{"type": "error", "msg": "上传服务未初始化"}
	}
	queueManager := historyUploadService.GetQueueManager()
	status := queueManager.GetAllQueuesStatus()

	db := database.GetDB()
	pending := listQueueParts(db.Where("upload = ? AND recording = ? AND uploading = ? AND file_delete = ? AND upload_paused = ? AND upload_cancelled = ?", false, false, false, false, false, false).
		Order("start_time ASC").Limit(50))
	running := listQueueParts(db.Where("upload = ? AND uploading = ? AND upload_cancelled = ?", false, true, false).
		Order("created_at ASC").Limit(50))
	paused := listQueueParts(db.Where("upload = ? AND upload_paused = ? AND upload_cancelled = ?", false, true, false).
		Order("created_at DESC").Limit(50))
	cancelled := listQueueParts(db.Where("upload = ? AND upload_cancelled = ?", false, true).
		Order("created_at DESC").Limit(50))
	completed := listQueueParts(db.Where("upload = ?", true).
		Order("created_at DESC").Limit(50))

	var pendingCount int64
	var runningCount int64
	var pausedCount int64
	var cancelledCount int64
	var completedCount int64
	db.Model(&models.RecordHistoryPart{}).Where("upload = ? AND recording = ? AND uploading = ? AND file_delete = ? AND upload_paused = ? AND upload_cancelled = ?", false, false, false, false, false, false).Count(&pendingCount)
	db.Model(&models.RecordHistoryPart{}).Where("upload = ? AND uploading = ? AND upload_cancelled = ?", false, true, false).Count(&runningCount)
	db.Model(&models.RecordHistoryPart{}).Where("upload = ? AND upload_paused = ? AND upload_cancelled = ?", false, true, false).Count(&pausedCount)
	db.Model(&models.RecordHistoryPart{}).Where("upload = ? AND upload_cancelled = ?", false, true).Count(&cancelledCount)
	db.Model(&models.RecordHistoryPart{}).Where("upload = ?", true).Count(&completedCount)

	return gin.H{
		"queues": status,
		"counts": gin.H{
			"pending":   pendingCount,
			"running":   runningCount,
			"paused":    pausedCount,
			"cancelled": cancelledCount,
			"completed": completedCount,
		},
		"pending":   pending,
		"running":   running,
		"paused":    paused,
		"cancelled": cancelled,
		"completed": completed,
	}
}

// PauseUploadPart 暂停单个待上传分P。
func PauseUploadPart(c *gin.Context) {
	part, ok := loadQueuePart(c)
	if !ok {
		return
	}
	if part.Upload {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "已完成任务不能暂停"})
		return
	}
	if part.Uploading {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "正在上传的任务暂不支持立即暂停，请等待当前分片结束"})
		return
	}

	db := database.GetDB()
	if err := db.Model(&part).Updates(map[string]interface{}{
		"upload_paused":     true,
		"upload_cancelled":  false,
		"upload_error_msg":  "用户暂停上传",
		"upload_error_type": "user",
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "暂停失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "任务已暂停"})
}

// ResumeUploadPart 恢复单个分P，并在条件允许时立即重新入队。
func ResumeUploadPart(c *gin.Context) {
	part, ok := loadQueuePart(c)
	if !ok {
		return
	}
	if part.Upload {
		c.JSON(http.StatusOK, gin.H{"type": "warning", "msg": "任务已完成，无需恢复"})
		return
	}

	db := database.GetDB()
	if err := db.Model(&part).Updates(map[string]interface{}{
		"upload_paused":     false,
		"upload_cancelled":  false,
		"uploading":         false,
		"upload_error_msg":  "",
		"upload_error_type": "",
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "恢复失败"})
		return
	}

	part.UploadPaused = false
	part.UploadCancelled = false
	part.Uploading = false
	msg := "任务已恢复"
	if err := enqueuePartIfReady(&part); err != nil {
		msg = msg + "，等待自动调度重新入队: " + err.Error()
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": msg})
}

// CancelUploadPart 取消单个待上传分P。
func CancelUploadPart(c *gin.Context) {
	part, ok := loadQueuePart(c)
	if !ok {
		return
	}
	if part.Upload {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "已完成任务不能取消"})
		return
	}
	if part.Uploading {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "正在上传的任务暂不支持立即取消"})
		return
	}

	db := database.GetDB()
	if err := db.Model(&part).Updates(map[string]interface{}{
		"upload_paused":     false,
		"upload_cancelled":  true,
		"uploading":         false,
		"upload_error_msg":  "用户取消上传",
		"upload_error_type": "user",
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "取消失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "任务已取消"})
}

// RetryUploadPart 清除失败/取消状态并重新入队。
func RetryUploadPart(c *gin.Context) {
	part, ok := loadQueuePart(c)
	if !ok {
		return
	}
	if part.Upload {
		c.JSON(http.StatusOK, gin.H{"type": "warning", "msg": "任务已完成，无需重试"})
		return
	}

	db := database.GetDB()
	if err := db.Model(&part).Updates(map[string]interface{}{
		"upload_paused":          false,
		"upload_cancelled":       false,
		"uploading":              false,
		"upload_retry_count":     0,
		"upload_error_msg":       "",
		"upload_error_type":      "",
		"rate_limit_retry_count": 0,
		"rate_limit_cooldown_at": nil,
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "重试失败"})
		return
	}

	part.UploadPaused = false
	part.UploadCancelled = false
	part.Uploading = false
	part.UploadRetryCount = 0
	part.UploadErrorMsg = ""
	part.UploadErrorType = ""
	part.RateLimitCooldownAt = nil
	part.RateLimitRetryCount = 0
	if err := enqueuePartIfReady(&part); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "warning", "msg": "状态已重置，等待自动调度重新入队: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "任务已重新入队"})
}

// PauseAllPendingUploads 暂停所有未开始的待上传分P。
func PauseAllPendingUploads(c *gin.Context) {
	db := database.GetDB()
	result := db.Model(&models.RecordHistoryPart{}).
		Where("upload = ? AND uploading = ? AND recording = ? AND file_delete = ? AND upload_cancelled = ?", false, false, false, false, false).
		Updates(map[string]interface{}{"upload_paused": true, "upload_error_msg": "用户批量暂停上传", "upload_error_type": "user"})
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "已批量暂停待上传任务", "count": result.RowsAffected})
}

// ResumeAllPausedUploads 恢复所有暂停的分P。
func ResumeAllPausedUploads(c *gin.Context) {
	db := database.GetDB()
	result := db.Model(&models.RecordHistoryPart{}).
		Where("upload = ? AND upload_paused = ? AND upload_cancelled = ?", false, true, false).
		Updates(map[string]interface{}{"upload_paused": false, "upload_error_msg": "", "upload_error_type": ""})
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "已批量恢复暂停任务，自动调度会重新入队", "count": result.RowsAffected})
}

// CancelAllPendingUploads 取消所有未开始或已暂停的待上传分P。
func CancelAllPendingUploads(c *gin.Context) {
	db := database.GetDB()
	result := db.Model(&models.RecordHistoryPart{}).
		Where("upload = ? AND uploading = ? AND recording = ? AND file_delete = ?", false, false, false, false).
		Updates(map[string]interface{}{"upload_paused": false, "upload_cancelled": true, "upload_error_msg": "用户批量取消上传", "upload_error_type": "user"})
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "已批量取消待上传任务", "count": result.RowsAffected})
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
			LiveTitle:           part.LiveTitle,
			AreaName:            part.AreaName,
			FileName:            part.FileName,
			FilePath:            part.FilePath,
			FileSize:            part.FileSize,
			Duration:            part.Duration,
			CreatedAt:           part.CreatedAt,
			StartTime:           part.StartTime,
			EndTime:             part.EndTime,
			Uploading:           part.Uploading,
			Upload:              part.Upload,
			UploadRetryCount:    part.UploadRetryCount,
			UploadErrorMsg:      part.UploadErrorMsg,
			UploadErrorType:     part.UploadErrorType,
			UploadLine:          part.UploadLine,
			UploadPaused:        part.UploadPaused,
			UploadCancelled:     part.UploadCancelled,
			FileDelete:          part.FileDelete,
			FileMoved:           part.FileMoved,
			Page:                part.Page,
			CID:                 part.CID,
			Status:              queuePartStatus(part),
			RateLimitCooldownAt: part.RateLimitCooldownAt,
			IsTempFile:          part.IsTempFile,
			SourcePartID:        part.SourcePartID,
			TempFileType:        part.TempFileType,
			AppendedToVideo:     part.AppendedToVideo,
		})
	}
	return result
}

func loadQueuePart(c *gin.Context) (models.RecordHistoryPart, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "无效的任务ID"})
		return models.RecordHistoryPart{}, false
	}

	var part models.RecordHistoryPart
	if err := database.GetDB().First(&part, uint(id)).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "任务不存在"})
		return models.RecordHistoryPart{}, false
	}
	return part, true
}

func enqueuePartIfReady(part *models.RecordHistoryPart) error {
	if historyUploadService == nil {
		return nil
	}
	if part == nil || part.Recording || part.FileDelete || part.FilePath == "" {
		return nil
	}

	db := database.GetDB()
	var history models.RecordHistory
	if err := db.First(&history, part.HistoryID).Error; err != nil {
		return err
	}
	if !history.Upload {
		return nil
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", part.RoomID).First(&room).Error; err != nil {
		return err
	}
	if !room.Upload {
		return nil
	}
	return historyUploadService.UploadPart(part, &history, &room)
}

func queuePartStatus(part models.RecordHistoryPart) string {
	switch {
	case part.Upload:
		return "completed"
	case part.UploadCancelled:
		return "cancelled"
	case part.UploadPaused:
		return "paused"
	case part.Uploading:
		return "running"
	case part.RateLimitCooldownAt != nil && part.RateLimitCooldownAt.After(time.Now()):
		return "cooldown"
	default:
		return "pending"
	}
}
