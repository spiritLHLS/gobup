package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/config"
)

func BasicAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.AppConfig.InitUsername == "" || config.AppConfig.InitPassword == "" {
			c.Next()
			return
		}

		username, password, ok := c.Request.BasicAuth()
		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		if username != config.AppConfig.InitUsername || password != config.AppConfig.InitPassword {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}
