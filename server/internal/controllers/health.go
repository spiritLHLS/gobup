package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health 健康检查。
//
// @Summary Health check
// @Description Returns a lightweight service health status.
// @Tags system
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
