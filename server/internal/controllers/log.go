package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/websocket"
)

// GetLogs 获取历史日志
func GetLogs(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "1000")
	limit, _ := strconv.Atoi(limitStr)

	hub := websocket.GetHub()
	logs := hub.GetLogHistory(limit)

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// ClearLogs 清空日志
func ClearLogs(c *gin.Context) {
	hub := websocket.GetHub()
	hub.ClearLogHistory()

	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"msg":  "日志已清空",
	})
}
