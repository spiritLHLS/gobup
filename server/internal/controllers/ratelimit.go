package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/upload"
)

// GetRateLimitConfig 获取上传限速配置
func GetRateLimitConfig(c *gin.Context) {
	speedMBps, enabled := upload.GetGlobalRateLimit()

	c.JSON(http.StatusOK, gin.H{
		"enabled":   enabled,
		"speedMBps": speedMBps,
	})
}

// SetRateLimitConfig 设置上传限速
func SetRateLimitConfig(c *gin.Context) {
	type RateLimitReq struct {
		Enabled   bool    `json:"enabled"`
		SpeedMBps float64 `json:"speedMBps"` // MB/s
	}

	var req RateLimitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Enabled {
		if req.SpeedMBps <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "限速值必须大于0"})
			return
		}
		upload.SetGlobalRateLimit(req.SpeedMBps)
	} else {
		upload.SetGlobalRateLimit(0) // 禁用限速
	}

	c.JSON(http.StatusOK, gin.H{
		"type":      "success",
		"msg":       "限速配置已更新",
		"enabled":   req.Enabled,
		"speedMBps": req.SpeedMBps,
	})
}
