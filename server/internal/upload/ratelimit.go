package upload

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter 上传速率限制器
type RateLimiter struct {
	limiter *rate.Limiter
	enabled bool
	mu      sync.RWMutex
}

var globalRateLimiter = &RateLimiter{
	limiter: rate.NewLimiter(rate.Inf, 0), // 默认无限制
	enabled: false,
}

// SetGlobalRateLimit 设置全局上传速率限制
// speedMBps: 速度限制（MB/s），0表示无限制
func SetGlobalRateLimit(speedMBps float64) {
	globalRateLimiter.mu.Lock()
	defer globalRateLimiter.mu.Unlock()

	if speedMBps <= 0 {
		// 禁用限速
		globalRateLimiter.enabled = false
		globalRateLimiter.limiter = rate.NewLimiter(rate.Inf, 0)
	} else {
		// 启用限速：转换为每秒字节数
		bytesPerSecond := speedMBps * 1024 * 1024
		globalRateLimiter.enabled = true
		// burst设置为1秒的数据量，允许短时突发
		globalRateLimiter.limiter = rate.NewLimiter(rate.Limit(bytesPerSecond), int(bytesPerSecond))
	}
}

// GetGlobalRateLimit 获取当前限速设置
func GetGlobalRateLimit() (speedMBps float64, enabled bool) {
	globalRateLimiter.mu.RLock()
	defer globalRateLimiter.mu.RUnlock()

	if !globalRateLimiter.enabled {
		return 0, false
	}

	bytesPerSecond := float64(globalRateLimiter.limiter.Limit())
	speedMBps = bytesPerSecond / 1024 / 1024
	return speedMBps, true
}

// WaitN 等待N个字节的配额
func (rl *RateLimiter) WaitN(ctx context.Context, n int) error {
	rl.mu.RLock()
	enabled := rl.enabled
	limiter := rl.limiter
	rl.mu.RUnlock()

	if !enabled {
		return nil
	}

	return limiter.WaitN(ctx, n)
}

// RateLimitedReader 支持限速的Reader
type RateLimitedReader struct {
	reader  interface{ Read([]byte) (int, error) }
	limiter *RateLimiter
	ctx     context.Context
}

// NewRateLimitedReader 创建限速Reader
func NewRateLimitedReader(reader interface{ Read([]byte) (int, error) }, limiter *RateLimiter) *RateLimitedReader {
	return &RateLimitedReader{
		reader:  reader,
		limiter: limiter,
		ctx:     context.Background(),
	}
}

// Read 实现io.Reader接口，带限速
func (r *RateLimitedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.limiter != nil {
		// 等待令牌桶分配足够的令牌
		_ = r.limiter.WaitN(r.ctx, n)
	}
	return n, err
}

// GetGlobalLimiter 获取全局限速器
func GetGlobalLimiter() *RateLimiter {
	return globalRateLimiter
}
