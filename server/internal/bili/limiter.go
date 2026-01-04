package bili

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// APILimiter B站API调用限流器 - 全局防风控
type APILimiter struct {
	// 预上传API限流器 - 每秒最多1次
	preUploadLimiter *rate.Limiter

	// 分片上传限流器 - 每秒最多3次
	chunkUploadLimiter *rate.Limiter

	// 投稿API限流器 - 每分钟最多5次
	publishLimiter *rate.Limiter

	// 弹幕发送限流器 - 全局限流，防止风控
	danmakuLimiter *rate.Limiter

	// 其他API限流器 - 每秒最多2次
	generalLimiter *rate.Limiter

	mu sync.Mutex
}

var (
	globalLimiter *APILimiter
	once          sync.Once
)

// GetAPILimiter 获取全局API限流器（单例）
func GetAPILimiter() *APILimiter {
	once.Do(func() {
		globalLimiter = &APILimiter{
			preUploadLimiter:   rate.NewLimiter(rate.Every(1*time.Second), 1),        // 预上传：1次/秒
			chunkUploadLimiter: rate.NewLimiter(rate.Every(350*time.Millisecond), 3), // 分片上传：3次/秒，间隔350ms
			publishLimiter:     rate.NewLimiter(rate.Every(12*time.Second), 5),       // 投稿：5次/分钟
			danmakuLimiter:     rate.NewLimiter(rate.Every(22*time.Second), 1),       // 弹幕：22秒1条，参考biliupforjava的25秒策略
			generalLimiter:     rate.NewLimiter(rate.Every(500*time.Millisecond), 2), // 通用：2次/秒
		}
	})
	return globalLimiter
}

// WaitPreUpload 等待预上传API调用
func (l *APILimiter) WaitPreUpload() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.preUploadLimiter.Wait(context.Background())
}

// WaitChunkUpload 等待分片上传API调用
func (l *APILimiter) WaitChunkUpload() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.chunkUploadLimiter.Wait(context.Background())
}

// WaitPublish 等待投稿API调用
func (l *APILimiter) WaitPublish() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.publishLimiter.Wait(context.Background())
}

// WaitDanmaku 等待弹幕发送API调用（全局限流）
func (l *APILimiter) WaitDanmaku() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.danmakuLimiter.Wait(context.Background())
}

// WaitGeneral 等待通用API调用
func (l *APILimiter) WaitGeneral() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.generalLimiter.Wait(context.Background())
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries      int           // 最大重试次数
	InitialDelay    time.Duration // 初始延迟
	MaxDelay        time.Duration // 最大延迟
	BackoffFactor   float64       // 退避因子
	RetryableErrors []string      // 可重试的错误关键字
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries:    3,
	InitialDelay:  2 * time.Second,
	MaxDelay:      30 * time.Second,
	BackoffFactor: 2.0,
	RetryableErrors: []string{
		"timeout",
		"connection",
		"EOF",
		"reset",
		"temporary",
		"429",    // Too Many Requests
		"503",    // Service Unavailable
		"502",    // Bad Gateway
		"406",    // B站限流错误
		"601",    // B站限流错误码
		"上传视频过快", // B站限流提示
	},
}

// RateLimitRetryConfig B站限流专用重试配置（更长的等待时间）
var RateLimitRetryConfig = RetryConfig{
	MaxRetries:    5,
	InitialDelay:  15 * time.Second,  // 首次等待15秒
	MaxDelay:      120 * time.Second, // 最多等待2分钟
	BackoffFactor: 1.5,
	RetryableErrors: []string{
		"timeout",
		"connection",
		"EOF",
		"reset",
		"temporary",
		"429",
		"503",
		"502",
		"406",
		"601",
		"上传视频过快",
	},
}

// WithRetry 带重试的执行函数
func WithRetry(config RetryConfig, fn func() error) error {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// 等待一段时间后重试
			time.Sleep(delay)

			// 指数退避
			delay = time.Duration(float64(delay) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否为可重试错误
		isRetryable := false
		errMsg := err.Error()
		for _, keyword := range config.RetryableErrors {
			if contains(errMsg, keyword) {
				isRetryable = true
				break
			}
		}

		if !isRetryable {
			// 不可重试的错误，直接返回
			return err
		}
	}

	return lastErr
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
