package middleware

import (
"net/http"

"github.com/gin-gonic/gin"
"github.com/gobup/server/internal/config"
)

func BasicAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.AppConfig.Username == "" || config.AppConfig.Password == "" {
			c.Next()
			return
		}

		username, password, ok := c.Request.BasicAuth()
		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		if username != config.AppConfig.Username || password != config.AppConfig.Password {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}
