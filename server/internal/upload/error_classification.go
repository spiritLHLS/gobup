package upload

import (
	"errors"
	"strings"
)

const (
	UploadErrorTypeNetwork   = "network"
	UploadErrorTypeRateLimit = "rate_limit"
	UploadErrorTypeAuth      = "auth"
	UploadErrorTypeFile      = "file"
	UploadErrorTypeTranscode = "transcode"
	UploadErrorTypeWindow    = "window"
	UploadErrorTypeUser      = "user"
	UploadErrorTypeUnknown   = "unknown"
)

func classifyUploadError(err error) string {
	if err == nil {
		return ""
	}
	var windowErr *UploadWindowClosedError
	if errors.As(err, &windowErr) {
		return UploadErrorTypeWindow
	}
	return classifyUploadErrorText(err.Error())
}

func classifyUploadErrorText(text string) string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return UploadErrorTypeUnknown
	}

	switch {
	case containsAny(normalized, []string{
		"速率限制", "限流", "rate limit", "retry-after", "http 429", "http 406", "http 601", "上传视频过快",
	}):
		return UploadErrorTypeRateLimit
	case containsAny(normalized, []string{
		"cookie", "用户未登录", "用户已禁用", "鉴权", "未授权", "unauthorized", "forbidden", "csrf", "access_key",
	}):
		return UploadErrorTypeAuth
	case containsAny(normalized, []string{
		"转码", "ffmpeg", "ffprobe", "xcode",
	}):
		return UploadErrorTypeTranscode
	case containsAny(normalized, []string{
		"文件不存在", "源视频不可用", "获取文件信息失败", "格式不支持", "不允许上传", "file not found", "no such file", "invalid file", "disk", "no space",
	}):
		return UploadErrorTypeFile
	case containsAny(normalized, []string{
		"用户暂停", "用户取消", "paused", "cancelled", "canceled",
	}):
		return UploadErrorTypeUser
	case containsAny(normalized, []string{
		"timeout", "超时", "connection", "connect", "network", "eof", "reset", "broken pipe", "tls", "i/o timeout",
	}):
		return UploadErrorTypeNetwork
	default:
		return UploadErrorTypeUnknown
	}
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
