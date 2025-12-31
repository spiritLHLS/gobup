package controllers

import (
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
	if err := danmakuService.SendDanmakuForHistory(uint(historyID), req.UserID); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "弹幕发送成功"})
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
		resetItems = append(resetItems, "弹幕状态")
	}

	if options.Files {
		history.FilesMoved = false
		resetItems = append(resetItems, "文件状态")
	}

	db.Save(&history)

	// 重置分P的上传状态
	if options.Upload {
		partUpdates := map[string]interface{}{
			"upload":      false,
			"uploading":   false,
			"cid":         0,
			"page":        0,
			"xcode_state": 0,
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
