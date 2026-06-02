package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/websocket"
	ws "github.com/gorilla/websocket"
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境应该限制
	},
}

// WSLog WebSocket日志连接处理
func WSLog(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WebSocket升级失败"})
		return
	}

	hub := websocket.GetHub()
	websocket.NewClient(hub, conn)
}

// WSProgress 推送上传进度快照。
func WSProgress(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WebSocket升级失败"})
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		snapshot := map[string]interface{}{
			"type":       "uploadProgress",
			"updateAtMs": time.Now().UnixMilli(),
			"items":      map[int64]interface{}{},
		}
		if uploadService != nil {
			snapshot["items"] = uploadService.GetProgressTracker().SnapshotAll()
		}

		if err := conn.WriteJSON(snapshot); err != nil {
			return
		}

		<-ticker.C
	}
}
