package logging

import (
	"io"
	"log"
	"regexp"
	"strings"

	"github.com/gobup/server/internal/websocket"
)

var zeroFailureSummaryPattern = regexp.MustCompile(`失败\s*[=:：]\s*0(\D|$)`)

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

	level := inferLogLevel(message)

	// 广播到WebSocket
	if l.hub != nil {
		l.hub.BroadcastLog(level, message)
	}

	return n, err
}

func inferLogLevel(message string) string {
	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(message, "[ERROR]") || strings.Contains(normalized, " error "):
		return "ERROR"
	case strings.Contains(message, "[WARN]") || strings.Contains(message, "警告") || strings.Contains(normalized, " warn "):
		return "WARN"
	case strings.Contains(message, "[DEBUG]") || strings.Contains(message, "调试") || strings.Contains(normalized, "[debug]"):
		return "DEBUG"
	}

	withoutZeroSummaries := zeroFailureSummaryPattern.ReplaceAllString(message, "")
	if strings.Contains(withoutZeroSummaries, "失败") ||
		strings.Contains(withoutZeroSummaries, "错误") ||
		strings.Contains(withoutZeroSummaries, "异常") {
		return "ERROR"
	}
	return "INFO"
}

// SetupLogInterceptor 设置日志拦截器
func SetupLogInterceptor() {
	interceptor := NewLogInterceptor(log.Writer())
	log.SetOutput(interceptor)
}
