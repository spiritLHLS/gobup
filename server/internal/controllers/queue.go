package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetUploadQueueStatus 获取上传队列状态
func GetUploadQueueStatus(c *gin.Context) {
	if historyUploadService == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}

	queueManager := historyUploadService.GetQueueManager()
	status := queueManager.GetAllQueuesStatus()

	c.JSON(http.StatusOK, gin.H{
		"queues": status,
	})
}
