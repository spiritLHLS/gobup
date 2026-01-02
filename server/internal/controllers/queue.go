package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/services"
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

// GetDanmakuQueueStatus 获取弹幕发送队列状态
func GetDanmakuQueueStatus(c *gin.Context) {
	danmakuService := services.NewDanmakuService()
	queueManager := danmakuService.GetQueueManager()
	status := queueManager.GetAllQueuesStatus()

	c.JSON(http.StatusOK, gin.H{
		"queues": status,
	})
}
