package middleware

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/config"
)

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
			log.Printf("[INFO] Basic认证失败 - 未提供认证信息, IP: %s, Path: %s", c.ClientIP(), c.Request.URL.Path)
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 验证用户名密码
		if username != config.AppConfig.InitUsername || password != config.AppConfig.InitPassword {
			log.Printf("[INFO] Basic认证失败 - 用户名或密码错误, IP: %s, Path: %s", c.ClientIP(), c.Request.URL.Path)
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}
