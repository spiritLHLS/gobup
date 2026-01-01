package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/services"
)

// GetCaptchaStatus 获取验证码状态（参考biliupforjava /captcha/status）
func GetCaptchaStatus(c *gin.Context) {
	captchaService := services.GetCaptchaService()
	status := captchaService.GetCaptchaStatus()

	c.JSON(http.StatusOK, status)
}

// SubmitCaptchaResult 提交验证码结果（参考biliupforjava /captcha/submit）
func SubmitCaptchaResult(c *gin.Context) {
	var result map[string]string
	if err := c.ShouldBindJSON(&result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	captchaService := services.GetCaptchaService()
	if err := captchaService.SubmitCaptchaResult(result); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "验证码已提交"})
}

// ClearCaptcha 清除验证码状态
func ClearCaptcha(c *gin.Context) {
	captchaService := services.GetCaptchaService()
	captchaService.ClearCaptchaStatus()

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "验证码状态已清除"})
}
