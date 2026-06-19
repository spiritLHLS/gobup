package controllers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/agent"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

func AgentHealth(c *gin.Context) {
	if !validateAgentToken(c) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"msg":  "agent ok",
		"time": time.Now().Format(time.RFC3339),
	})
}

func AgentPublish(c *gin.Context) {
	if !validateAgentToken(c) {
		return
	}
	var req agent.PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.HistoryID == 0 || req.UserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}
	if historyUploadService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}
	if err := historyUploadService.PublishHistoryLocal(req.HistoryID, req.UserID); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}
	var history models.RecordHistory
	data := agent.PublishResult{Publish: true, Message: "投稿成功"}
	if err := database.GetDB().First(&history, req.HistoryID).Error; err == nil {
		data.Publish = history.Publish
		data.BvID = history.BvID
		data.AvID = history.AvID
		data.Message = history.Message
	}
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "远程 Agent 投稿完成", "data": data})
}

func DetectPublishAgent(c *gin.Context) {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}
	if strings.TrimSpace(config.PublishAgentEndpoint) == "" {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "未配置远程 Agent 地址"})
		return
	}
	timeout := time.Duration(config.PublishAgentTimeout) * time.Second
	client := agent.NewClient(config.PublishAgentEndpoint, config.PublishAgentToken, timeout)
	if err := client.Health(); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "远程 Agent 不可用: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "远程 Agent 可用"})
}

func validateAgentToken(c *gin.Context) bool {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "Agent 未初始化"})
		return false
	}
	expected := strings.TrimSpace(config.PublishAgentToken)
	if expected == "" {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "Agent token 未配置"})
		return false
	}

	token := strings.TrimSpace(c.GetHeader("X-Agent-Token"))
	if token == "" {
		auth := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token != expected {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "Agent token 无效"})
		return false
	}
	return true
}
