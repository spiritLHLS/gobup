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
	UploadErrorTypePermanent = "permanent"
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
	var cooldownErr *UploadCooldownActiveError
	if errors.As(err, &cooldownErr) {
		if cooldownErr.ErrorType != "" {
			return cooldownErr.ErrorType
		}
		return UploadErrorTypeRateLimit
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
		"速率限制", "限流", "rate limit", "retry-after", "http 429", "http 406", "http 601", "上传视频过快", "请求频率过高", "请求过于频繁", "code=-702",
	}):
		return UploadErrorTypeRateLimit
	case containsAny(normalized, []string{
		"稿件标题过长", "超过80个字符", "时长不足 1 秒", "视频时长不足", "该视频时长不足",
		"不能小于1秒", "不能小于 1 秒", "小于1秒", "小于 1 秒", "少于1秒", "少于 1 秒",
		"分区不存在", "标题不能为空",
	}):
		return UploadErrorTypePermanent
	case containsAny(normalized, []string{
		"cookie", "用户未登录", "用户已禁用", "鉴权", "未授权", "unauthorized", "forbidden", "csrf", "access_key",
	}):
		return UploadErrorTypeAuth
	case containsAny(normalized, []string{
		"转码", "ffmpeg", "ffprobe", "xcode", "danmakufactory", "ass文件", "ass 文件",
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
