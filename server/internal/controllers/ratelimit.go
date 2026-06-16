package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/ratelimit"
)

// GetRateLimitConfig 获取上传限速配置
func GetRateLimitConfig(c *gin.Context) {
	speedMBps, enabled := ratelimit.GetGlobalRateLimit()

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
		ratelimit.SetGlobalRateLimit(req.SpeedMBps)
	} else {
		ratelimit.SetGlobalRateLimit(0) // 禁用限速
		req.SpeedMBps = 0
	}

	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err == nil {
		config.UploadSpeedLimitMBps = req.SpeedMBps
		_ = db.Save(&config).Error
	}

	c.JSON(http.StatusOK, gin.H{
		"type":      "success",
		"msg":       "限速配置已更新",
		"enabled":   req.Enabled,
		"speedMBps": req.SpeedMBps,
	})
}
