package controllers

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/config"
	"github.com/gobup/server/internal/websocket"
	ws "github.com/gorilla/websocket"
)

const (
	progressSnapshotInterval = 2 * time.Second
	websocketWriteWait       = 10 * time.Second
	websocketPongWait        = 60 * time.Second
	websocketPingInterval    = (websocketPongWait * 9) / 10
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedWebSocketOrigin(r)
	},
}

func isAllowedWebSocketOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		log.Printf("[WebSocket] 拒绝无效 Origin: origin=%q host=%q", origin, r.Host)
		return false
	}

	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}

	normalizedOrigin := strings.TrimRight(origin, "/")
	for _, allowed := range strings.Split(os.Getenv("GOBUP_ALLOWED_ORIGINS"), ",") {
		allowed = strings.TrimRight(strings.TrimSpace(allowed), "/")
		if allowed == "" || allowed == "*" {
			continue
		}
		if strings.EqualFold(normalizedOrigin, allowed) {
			return true
		}
	}

	log.Printf("[WebSocket] 拒绝跨源连接: origin=%q host=%q", origin, r.Host)
	return false
}

// WSLog WebSocket日志连接处理
func WSLog(c *gin.Context) {
	if !isAuthorizedWebSocketRequest(c.Request) {
		rejectUnauthorizedWebSocket(c)
		return
	}

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
	if !isAuthorizedWebSocketRequest(c.Request) {
		rejectUnauthorizedWebSocket(c)
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WebSocket升级失败"})
		return
	}
	defer conn.Close()

	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(websocketPongWait))
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	snapshotTicker := time.NewTicker(progressSnapshotInterval)
	defer snapshotTicker.Stop()

	pingTicker := time.NewTicker(websocketPingInterval)
	defer pingTicker.Stop()

	if err := writeProgressSnapshot(conn); err != nil {
		return
	}

	for {
		select {
		case <-done:
			return
		case <-snapshotTicker.C:
			if err := writeProgressSnapshot(conn); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := conn.WriteControl(ws.PingMessage, []byte("ping"), time.Now().Add(websocketWriteWait)); err != nil {
				return
			}
		}
	}
}

func isAuthorizedWebSocketRequest(r *http.Request) bool {
	if config.AppConfig.InitUsername == "" || config.AppConfig.InitPassword == "" {
		return true
	}

	username, password, ok := r.BasicAuth()
	if ok && username == config.AppConfig.InitUsername && password == config.AppConfig.InitPassword {
		return true
	}

	return validateWebSocketToken(strings.TrimSpace(r.URL.Query().Get("token")))
}

func rejectUnauthorizedWebSocket(c *gin.Context) {
	c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "WebSocket认证失败"})
}

func writeProgressSnapshot(conn *ws.Conn) error {
	if err := conn.SetWriteDeadline(time.Now().Add(websocketWriteWait)); err != nil {
		return err
	}
	return conn.WriteJSON(uploadProgressSnapshot())
}

func uploadProgressSnapshot() map[string]interface{} {
	snapshot := map[string]interface{}{
		"type":       "uploadProgress",
		"updateAtMs": time.Now().UnixMilli(),
		"items":      map[int64]interface{}{},
	}
	if uploadService != nil {
		snapshot["items"] = uploadService.GetProgressTracker().SnapshotAll()
	}
	return snapshot
}
