package controllers

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

// BatchDeleteWithFiles 批量删除记录和文件
func BatchDeleteWithFiles(c *gin.Context) {
	type BatchDeleteReq struct {
		HistoryIDs         []uint `json:"historyIds" binding:"required"`
		ConfirmDeleteFiles bool   `json:"confirmDeleteFiles"`
		ConfirmText        string `json:"confirmText"`
	}

	var req BatchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}
	if !req.ConfirmDeleteFiles || req.ConfirmText != "DELETE_FILES" {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "删除文件需要二次确认"})
		return
	}

	db := database.GetDB()
	successCount := 0

	for _, historyID := range req.HistoryIDs {
		var history models.RecordHistory
		if err := db.First(&history, historyID).Error; err != nil {
			continue
		}

		moverService := services.NewFileMoverService()
		if history.FilePath != "" {
			if _, err := os.Stat(history.FilePath); err == nil {
				if err := os.Remove(history.FilePath); err != nil {
					log.Printf("[批量删除] 删除文件失败: %s, error: %v", history.FilePath, err)
				} else {
					log.Printf("[批量删除] 已删除主文件: %s", history.FilePath)
				}
				moverService.DeleteRelatedFiles(history.FilePath)
			}
		}

		var parts []models.RecordHistoryPart
		db.Where("history_id = ?", historyID).Find(&parts)
		for _, part := range parts {
			if part.FilePath == "" {
				continue
			}
			if _, err := os.Stat(part.FilePath); err != nil {
				continue
			}
			if err := os.Remove(part.FilePath); err != nil {
				log.Printf("[批量删除] 删除分P文件失败: %s, error: %v", part.FilePath, err)
			} else {
				log.Printf("[批量删除] 已删除分P主文件: %s", part.FilePath)
			}
			moverService.DeleteRelatedFiles(part.FilePath)
		}

		db.Delete(&models.RecordHistoryPart{}, "history_id = ?", historyID)
		db.Delete(&history)
		successCount++
	}

	log.Printf("[批量删除] 删除完成 %d/%d", successCount, len(req.HistoryIDs))

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"msg":     fmt.Sprintf("删除完成：成功%d个", successCount),
		"success": successCount,
		"total":   len(req.HistoryIDs),
	})
}
