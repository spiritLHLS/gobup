package controllers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

// SendDanmaku 发送弹幕到视频
func SendDanmaku(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	type SendDanmakuReq struct {
		UserID uint `json:"userId" binding:"required"`
	}

	var req SendDanmakuReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	danmakuService := services.NewDanmakuService()

	log.Printf("=== 开始启动弹幕发送任务 (history_id=%d, user_id=%d) ===", historyID, req.UserID)

	// 添加到队列（队列会自动异步处理）
	if err := danmakuService.SendDanmakuForHistory(uint(historyID), req.UserID); err != nil {
		log.Printf("[弹幕发送] ❌ 加入队列失败 (history_id=%d): %v", historyID, err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	queueLength := danmakuService.GetQueueManager().GetQueueLength(req.UserID)
	log.Printf("[弹幕发送] ✅ 任务已加入队列 (history_id=%d, 队列长度=%d)", historyID, queueLength)

	c.JSON(http.StatusOK, gin.H{
		"type":        "success",
		"msg":         "弹幕发送任务已加入队列",
		"queueLength": queueLength,
	})
}

// BatchSendDanmaku 批量发送弹幕
func BatchSendDanmaku(c *gin.Context) {
	type BatchSendReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
		UserID     uint   `json:"userId" binding:"required"`
	}

	var req BatchSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	danmakuService := services.NewDanmakuService()
	addedCount := 0

	for _, historyID := range req.HistoryIDs {
		if err := danmakuService.SendDanmakuForHistory(historyID, req.UserID); err != nil {
			log.Printf("[批量弹幕发送] ⚠️  添加任务失败 history_id=%d: %v", historyID, err)
			continue
		}
		addedCount++
	}

	queueLength := danmakuService.GetQueueManager().GetQueueLength(req.UserID)
	log.Printf("[批量弹幕发送] ✅ 已添加 %d/%d 个任务到队列 (队列长度=%d)",
		addedCount, len(req.HistoryIDs), queueLength)

	c.JSON(http.StatusOK, gin.H{
		"type":        "success",
		"msg":         fmt.Sprintf("已添加%d个发送任务到队列", addedCount),
		"added":       addedCount,
		"total":       len(req.HistoryIDs),
		"queueLength": queueLength,
	})
}

// MoveFiles 移动历史记录的文件
func MoveFiles(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	moverService := services.NewFileMoverService()
	if err := moverService.MoveFilesForHistory(uint(historyID)); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "文件移动成功"})
}

// BatchMoveFiles 批量移动文件
func BatchMoveFiles(c *gin.Context) {
	type BatchMoveReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
	}

	var req BatchMoveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	moverService := services.NewFileMoverService()
	successCount := 0
	failedCount := 0

	for _, historyID := range req.HistoryIDs {
		if err := moverService.MoveFilesForHistory(historyID); err != nil {
			log.Printf("[批量移动文件] ⚠️  移动失败 history_id=%d: %v", historyID, err)
			failedCount++
			continue
		}
		successCount++
	}

	log.Printf("[批量移动文件] ✅ 完成 %d/%d (失败 %d)",
		successCount, len(req.HistoryIDs), failedCount)

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"msg":     fmt.Sprintf("移动完成：成功%d个，失败%d个", successCount, failedCount),
		"success": successCount,
		"failed":  failedCount,
		"total":   len(req.HistoryIDs),
	})
}

// SyncVideoInfo 手动同步视频信息
func SyncVideoInfo(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	syncService := services.NewVideoSyncService()
	if err := syncService.SyncVideoInfo(uint(historyID)); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "视频信息同步成功"})
}

// BatchSyncVideo 批量同步视频信息
func BatchSyncVideo(c *gin.Context) {
	type BatchSyncReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
	}

	var req BatchSyncReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	syncService := services.NewVideoSyncService()
	successCount := 0
	failedCount := 0

	for _, historyID := range req.HistoryIDs {
		if err := syncService.SyncVideoInfo(historyID); err != nil {
			log.Printf("[批量同步视频] ⚠️  同步失败 history_id=%d: %v", historyID, err)
			failedCount++
			continue
		}
		successCount++
	}

	log.Printf("[批量同步视频] ✅ 完成 %d/%d (失败 %d)",
		successCount, len(req.HistoryIDs), failedCount)

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"msg":     fmt.Sprintf("同步完成：成功%d个，失败%d个", successCount, failedCount),
		"success": successCount,
		"failed":  failedCount,
		"total":   len(req.HistoryIDs),
	})
}

// CreateSyncTask 创建视频同步任务
func CreateSyncTask(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	syncService := services.NewVideoSyncService()
	if err := syncService.CreateSyncTask(uint(historyID)); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "同步任务已创建"})
}

// ListSyncTasks 获取同步任务列表
func ListSyncTasks(c *gin.Context) {
	db := database.GetDB()

	var tasks []models.VideoSyncTask
	db.Order("created_at DESC").Limit(100).Find(&tasks)

	c.JSON(http.StatusOK, gin.H{
		"list":  tasks,
		"total": len(tasks),
	})
}

// RetryFailedSyncTasks 重试失败的同步任务
func RetryFailedSyncTasks(c *gin.Context) {
	syncService := services.NewVideoSyncService()
	if err := syncService.RetryFailedTasks(); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "失败任务已重置"})
}

// ResetHistoryStatus 重置历史记录状态
func ResetHistoryStatus(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	type ResetOptions struct {
		Upload  bool `json:"upload"`
		Publish bool `json:"publish"`
		Danmaku bool `json:"danmaku"`
		Files   bool `json:"files"`
	}

	var options ResetOptions
	if err := c.ShouldBindJSON(&options); err != nil {
		// 如果没有传递选项，默认重置所有
		options = ResetOptions{
			Upload:  true,
			Publish: true,
			Danmaku: true,
			Files:   true,
		}
	}

	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "历史记录不存在"})
		return
	}

	resetItems := []string{}

	// 根据选项重置相应状态
	if options.Publish {
		history.Publish = false
		history.BvID = ""
		history.AvID = ""
		history.Code = -1
		history.Message = ""
		history.VideoState = -1
		history.VideoStateDesc = ""
		resetItems = append(resetItems, "投稿状态")
	}

	if options.Danmaku {
		history.DanmakuSent = false
		history.DanmakuCount = 0
		// 同时重置所有弹幕的sent状态
		db.Model(&models.LiveMsg{}).Where("session_id = ?", history.SessionID).Update("sent", false)
		resetItems = append(resetItems, "弹幕状态")
	}

	if options.Files {
		history.FilesMoved = false
		resetItems = append(resetItems, "文件状态")
	}

	// 重置上传状态时，设置UploadStatus为0
	if options.Upload {
		history.UploadStatus = 0
		history.UploadRetryCount = 0
	}

	db.Save(&history)

	// 重置分P的上传状态
	if options.Upload {
		partUpdates := map[string]interface{}{
			"upload":             false,
			"uploading":          false,
			"file_name":          "", // 清空服务器文件名，重新上传时会重新获取
			"c_id":               0,
			"page":               0,
			"xcode_state":        0,
			"upload_retry_count": 0,  // 清空重试次数
			"upload_error_msg":   "", // 清空错误信息
			"upload_line":        "", // 清空上传线路
		}
		if options.Files {
			partUpdates["file_delete"] = false
			partUpdates["file_moved"] = false
		}
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ?", historyID).Updates(partUpdates)
		resetItems = append(resetItems, "上传状态")
	} else if options.Files {
		// 如果只重置文件状态而不重置上传状态
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ?", historyID).Updates(map[string]interface{}{
			"file_delete": false,
			"file_moved":  false,
		})
	}

	msg := "状态已重置"
	if len(resetItems) > 0 {
		msg = "已重置: " + strings.Join(resetItems, "、")
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": msg})
}

// DeleteHistoryWithFiles 删除记录和文件
func DeleteHistoryWithFiles(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "历史记录不存在"})
		return
	}

	// 获取所有分P
	var parts []models.RecordHistoryPart
	db.Where("history_id = ?", historyID).Find(&parts)

	// 删除文件
	for _, part := range parts {
		if part.FilePath != "" {
			if err := os.Remove(part.FilePath); err != nil && !os.IsNotExist(err) {
				log.Printf("删除文件失败: %s, %v", part.FilePath, err)
			}
		}
	}

	// 删除数据库记录
	db.Delete(&models.RecordHistoryPart{}, "history_id = ?", historyID)
	db.Delete(&models.RecordHistory{}, historyID)

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "记录和文件已删除"})
}

// GetDanmakuStats 获取弹幕统计
func GetDanmakuStats(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "历史记录不存在"})
		return
	}

	var totalCount int64
	var sentCount int64

	db.Model(&models.LiveMsg{}).Where("session_id = ?", history.SessionID).Count(&totalCount)
	db.Model(&models.LiveMsg{}).Where("session_id = ? AND sent = ?", history.SessionID, true).Count(&sentCount)

	c.JSON(http.StatusOK, gin.H{
		"total":              totalCount,
		"sent":               sentCount,
		"historyDanmakuSent": history.DanmakuSent,
	})
}

// ParseDanmaku 解析弹幕XML文件（使用队列）
func ParseDanmaku(c *gin.Context) {
	historyID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	log.Printf("[弹幕解析] 收到解析请求: history_id=%d", historyID)

	// 添加到解析队列
	queue := services.NewDanmakuParserQueue()
	task := &services.DanmakuParseTask{
		HistoryID: uint(historyID),
	}

	if err := queue.Add(task); err != nil {
		log.Printf("[弹幕解析] ❌ 添加到队列失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	queueLength := queue.GetQueueLength()
	log.Printf("[弹幕解析] ✅ 任务已加入队列 (队列长度=%d)", queueLength)

	c.JSON(http.StatusOK, gin.H{
		"type":        "success",
		"msg":         "弹幕解析任务已加入队列",
		"queueLength": queueLength,
	})
}

// BatchParseDanmaku 批量解析弹幕
func BatchParseDanmaku(c *gin.Context) {
	type BatchParseReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
	}

	var req BatchParseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	queue := services.NewDanmakuParserQueue()
	addedCount := 0

	for _, historyID := range req.HistoryIDs {
		task := &services.DanmakuParseTask{
			HistoryID: historyID,
		}
		if err := queue.Add(task); err != nil {
			log.Printf("[批量弹幕解析] ⚠️  添加任务失败 history_id=%d: %v", historyID, err)
			continue
		}
		addedCount++
	}

	queueLength := queue.GetQueueLength()
	log.Printf("[批量弹幕解析] ✅ 已添加 %d/%d 个任务到队列 (队列长度=%d)",
		addedCount, len(req.HistoryIDs), queueLength)

	c.JSON(http.StatusOK, gin.H{
		"type":        "success",
		"msg":         fmt.Sprintf("已添加%d个解析任务到队列", addedCount),
		"added":       addedCount,
		"total":       len(req.HistoryIDs),
		"queueLength": queueLength,
	})
}

// GetParseQueueStatus 获取解析队列状态
func GetParseQueueStatus(c *gin.Context) {
	queue := services.NewDanmakuParserQueue()

	c.JSON(http.StatusOK, gin.H{
		"queueLength": queue.GetQueueLength(),
		"processing":  queue.IsProcessing(),
	})
}
