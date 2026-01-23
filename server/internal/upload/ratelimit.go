package upload

import (
	"github.com/gobup/server/internal/ratelimit"
)

// 为了保持向后兼容，重新导出 ratelimit 包的函数

// SetGlobalRateLimit 设置全局上传速率限制
func SetGlobalRateLimit(speedMBps float64) {
	ratelimit.SetGlobalRateLimit(speedMBps)
}

// GetGlobalRateLimit 获取当前限速设置
func GetGlobalRateLimit() (speedMBps float64, enabled bool) {
	return ratelimit.GetGlobalRateLimit()
}

// GetGlobalLimiter 获取全局限速器
func GetGlobalLimiter() *ratelimit.RateLimiter {
	return ratelimit.GetGlobalLimiter()
}

// NewRateLimitedReader 创建限速Reader
func NewRateLimitedReader(reader interface{ Read([]byte) (int, error) }, limiter *ratelimit.RateLimiter) *ratelimit.RateLimitedReader {
	return ratelimit.NewRateLimitedReader(reader, limiter)
}
