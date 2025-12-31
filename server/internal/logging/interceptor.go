package logging

import (
	"io"
	"log"
	"strings"

	"github.com/gobup/server/internal/websocket"
)

// LogInterceptor 日志拦截器，将日志推送到WebSocket
type LogInterceptor struct {
	originalWriter io.Writer
	hub            *websocket.Hub
}

// NewLogInterceptor 创建日志拦截器
func NewLogInterceptor(originalWriter io.Writer) *LogInterceptor {
	return &LogInterceptor{
		originalWriter: originalWriter,
		hub:            websocket.GetHub(),
	}
}

// Write 实现io.Writer接口
func (l *LogInterceptor) Write(p []byte) (n int, err error) {
	// 写入原始输出
	n, err = l.originalWriter.Write(p)

	// 同时推送到WebSocket
	message := string(p)
	message = strings.TrimSuffix(message, "\n")

	// 解析日志级别
	level := "INFO"
	if strings.Contains(message, "[ERROR]") || strings.Contains(message, "错误") || strings.Contains(message, "失败") {
		level = "ERROR"
	} else if strings.Contains(message, "[WARN]") || strings.Contains(message, "警告") {
		level = "WARN"
	} else if strings.Contains(message, "[DEBUG]") || strings.Contains(message, "调试") {
		level = "DEBUG"
	}

	// 广播到WebSocket
	if l.hub != nil {
		l.hub.BroadcastLog(level, message)
	}

	return n, err
}

// SetupLogInterceptor 设置日志拦截器
func SetupLogInterceptor() {
	interceptor := NewLogInterceptor(log.Writer())
	log.SetOutput(interceptor)
}
