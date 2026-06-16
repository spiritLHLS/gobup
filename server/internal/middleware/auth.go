package middleware

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/config"
)

const (
	basicAuthFailureWindow = 15 * time.Minute
	basicAuthLockDuration  = 15 * time.Minute
	basicAuthMaxFailures   = 5
)

type basicAuthAttempt struct {
	Failures      int
	FirstFailedAt time.Time
	LockedUntil   time.Time
}

var basicAuthAttempts = struct {
	sync.Mutex
	records map[string]basicAuthAttempt
}{
	records: make(map[string]basicAuthAttempt),
}

// BasicAuth 拦截器，每次请求都需要验证Basic Auth
// 参考biliupforjava的LoginInterceptor实现
func BasicAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 如果没有配置用户名密码，则跳过认证（有安全风险）
		if config.AppConfig.InitUsername == "" || config.AppConfig.InitPassword == "" {
			log.Println("[WARN] Basic认证未启用，未配置用户名或密码（存在安全风险）")
			c.Next()
			return
		}

		// 每次请求都必须提供Basic Auth
		username, password, ok := c.Request.BasicAuth()
		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"type": "error", "msg": "请先登录"})
			return
		}

		if retryAfter, locked := isBasicAuthLocked(c.ClientIP(), time.Now()); locked {
			c.Header("Retry-After", retryAfterHeader(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"type":       "error",
				"msg":        "登录失败次数过多，请稍后再试",
				"retryAfter": int(retryAfter.Seconds()),
			})
			return
		}

		// 验证用户名密码
		if username != config.AppConfig.InitUsername || password != config.AppConfig.InitPassword {
			log.Printf("[WARN] Basic认证失败 - 密码错误, IP: %s, Path: %s", c.ClientIP(), c.Request.URL.Path)
			if retryAfter, locked := recordBasicAuthFailure(c.ClientIP(), time.Now()); locked {
				c.Header("Retry-After", retryAfterHeader(retryAfter))
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"type":       "error",
					"msg":        "登录失败次数过多，请稍后再试",
					"retryAfter": int(retryAfter.Seconds()),
				})
				return
			}
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"type": "error", "msg": "用户名或密码错误"})
			return
		}

		clearBasicAuthFailures(c.ClientIP())
		c.Next()
	}
}

func isBasicAuthLocked(ip string, now time.Time) (time.Duration, bool) {
	basicAuthAttempts.Lock()
	defer basicAuthAttempts.Unlock()

	record, ok := basicAuthAttempts.records[ip]
	if !ok {
		return 0, false
	}
	if !record.LockedUntil.IsZero() {
		if record.LockedUntil.After(now) {
			return record.LockedUntil.Sub(now).Round(time.Second), true
		}
		delete(basicAuthAttempts.records, ip)
	}
	return 0, false
}

func recordBasicAuthFailure(ip string, now time.Time) (time.Duration, bool) {
	basicAuthAttempts.Lock()
	defer basicAuthAttempts.Unlock()

	record := basicAuthAttempts.records[ip]
	if record.FirstFailedAt.IsZero() || now.Sub(record.FirstFailedAt) > basicAuthFailureWindow {
		record = basicAuthAttempt{FirstFailedAt: now}
	}
	record.Failures++
	if record.Failures >= basicAuthMaxFailures {
		record.LockedUntil = now.Add(basicAuthLockDuration)
	}
	basicAuthAttempts.records[ip] = record

	if record.LockedUntil.After(now) {
		return record.LockedUntil.Sub(now).Round(time.Second), true
	}
	return 0, false
}

func clearBasicAuthFailures(ip string) {
	basicAuthAttempts.Lock()
	delete(basicAuthAttempts.records, ip)
	basicAuthAttempts.Unlock()
}

func retryAfterHeader(duration time.Duration) string {
	seconds := int(duration.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
