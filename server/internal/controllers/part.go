package controllers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

func ListParts(c *gin.Context) {
	historyID := c.Param("id")

	db := database.GetDB()
	var parts []models.RecordHistoryPart
	db.Where("history_id = ?", historyID).Order("start_time ASC").Find(&parts)

	c.JSON(http.StatusOK, parts)
}

// ListPartsById 兼容 GET /history/part/:id 前端调用
func ListPartsById(c *gin.Context) {
	ListParts(c)
}

func UploadToEditor(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"type": "info", "msg": "功能开发中"})
}

// ForceArchiveHistory 强制将历史记录标记为归档（不再追踪录制状态）
func ForceArchiveHistory(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}

	// 强制关闭录制状态，防止卡在进行中
	updates := map[string]interface{}{
		"recording": false,
		"end_time":  time.Now(),
	}
	// 同时将所有未完成的分P标记为停止录制
	db.Model(&models.RecordHistoryPart{}).
		Where("history_id = ? AND recording = ?", id, true).
		Updates(map[string]interface{}{"recording": false})

	db.Model(&history).Updates(updates)

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "已强制归档"})
}

// GetCandidateFiles 获取本地可绑定到该历史记录的候选文件
func GetCandidateFiles(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	// 获取已绑定文件路径集合（从现有分P中）
	var parts []models.RecordHistoryPart
	db.Where("history_id = ?", id).Order("start_time ASC").Find(&parts)
	bound := make(map[string]bool)
	var scanDir string
	for _, p := range parts {
		if p.FilePath != "" {
			bound[p.FilePath] = true
			if scanDir == "" {
				scanDir = filepath.Dir(p.FilePath)
			}
		}
	}

	var candidates []map[string]interface{}
	if scanDir == "" {
		c.JSON(http.StatusOK, candidates)
		return
	}

	videoExts := map[string]bool{
		".flv": true, ".ts": true, ".mp4": true, ".mkv": true,
	}

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		c.JSON(http.StatusOK, []map[string]interface{}{})
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !videoExts[ext] {
			continue
		}
		fullPath := filepath.Join(scanDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, map[string]interface{}{
			"path":  fullPath,
			"name":  entry.Name(),
			"size":  info.Size(),
			"bound": bound[fullPath],
		})
	}

	if candidates == nil {
		candidates = []map[string]interface{}{}
	}
	c.JSON(http.StatusOK, candidates)
}
