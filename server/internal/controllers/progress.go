package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/upload"
)

var uploadService *upload.Service

// SetUploadService 设置上传服务
func SetUploadService(svc *upload.Service) {
	uploadService = svc
}

// HistoryProgressResponse 历史记录进度响应
type HistoryProgressResponse struct {
	HistoryID      int64              `json:"historyId"`
	ActiveCount    int                `json:"activeCount"`
	OverallPercent int                `json:"overallPercent"`
	Items          []*upload.Progress `json:"items"`
}

// GetPartProgress 获取分P上传进度
func GetPartProgress(c *gin.Context) {
	partIDStr := c.Param("partId")
	partID, err := strconv.ParseInt(partIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分P ID"})
		return
	}

	if uploadService == nil {
		c.JSON(http.StatusOK, gin.H{"found": false, "progress": nil})
		return
	}

	tracker := uploadService.GetProgressTracker()
	progress := tracker.GetByPartID(partID)

	c.JSON(http.StatusOK, gin.H{
		"found":    progress != nil,
		"progress": progress,
	})
}

// GetHistoryProgress 获取历史记录所有分P进度
func GetHistoryProgress(c *gin.Context) {
	historyIDStr := c.Param("historyId")
	historyID, err := strconv.ParseInt(historyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的历史ID"})
		return
	}

	if uploadService == nil {
		c.JSON(http.StatusOK, HistoryProgressResponse{
			HistoryID:      historyID,
			ActiveCount:    0,
			OverallPercent: 0,
			Items:          []*upload.Progress{},
		})
		return
	}

	tracker := uploadService.GetProgressTracker()
	items := tracker.ListByHistoryID(historyID)

	activeCount := 0
	sumPercent := 0
	for _, item := range items {
		if item.IsActive() {
			activeCount++
			sumPercent += item.Percent
		}
	}

	overallPercent := 0
	if activeCount > 0 {
		overallPercent = sumPercent / activeCount
	}

	c.JSON(http.StatusOK, HistoryProgressResponse{
		HistoryID:      historyID,
		ActiveCount:    activeCount,
		OverallPercent: overallPercent,
		Items:          items,
	})
}
