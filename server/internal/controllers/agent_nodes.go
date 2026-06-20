package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/agent"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

type agentNodeRequest struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Purpose     string `json:"purpose"`
	Enabled     *bool  `json:"enabled"`
	Blocked     *bool  `json:"blocked"`
	BlockReason string `json:"blockReason"`
}

func ListAgentNodes(c *gin.Context) {
	db := database.GetDB()
	var config models.SystemConfig
	_ = db.First(&config).Error

	var nodes []models.AgentNode
	if err := db.Order("blocked ASC, enabled DESC, updated_at DESC").Find(&nodes).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "查询 Agent 失败"})
		return
	}
	markPrimaryAgentNodes(nodes, config.PublishAgentEndpoint)
	c.JSON(http.StatusOK, gin.H{"type": "success", "data": nodes, "total": len(nodes)})
}

func CreateAgentNode(c *gin.Context) {
	var req agentNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}
	node, err := buildAgentNodeFromRequest(req, nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}
	db := database.GetDB()
	var existing models.AgentNode
	if err := db.Unscoped().Where("endpoint = ?", node.Endpoint).First(&existing).Error; err == nil {
		if existing.DeletedAt.Valid {
			node.ID = existing.ID
			node.CreatedAt = existing.CreatedAt
			node.DeletedAt = gorm.DeletedAt{}
			if err := db.Unscoped().Save(&node).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "恢复 Agent 失败: " + err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Agent 已恢复", "data": node})
			return
		}
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 地址已存在"})
		return
	}
	if err := db.Create(&node).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "创建 Agent 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Agent 已创建", "data": node})
}

func UpdateAgentNode(c *gin.Context) {
	db := database.GetDB()
	id, ok := parseAgentNodeID(c)
	if !ok {
		return
	}
	var node models.AgentNode
	if err := db.First(&node, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
		return
	}
	oldEndpoint := node.Endpoint

	var req agentNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}
	updated, err := buildAgentNodeFromRequest(req, &node)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}
	if err := db.Save(&updated).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存 Agent 失败: " + err.Error()})
		return
	}
	syncPrimaryAgentAfterNodeChange(db, oldEndpoint, &updated)
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Agent 已更新", "data": updated})
}

func DeleteAgentNode(c *gin.Context) {
	db := database.GetDB()
	id, ok := parseAgentNodeID(c)
	if !ok {
		return
	}
	var node models.AgentNode
	if err := db.First(&node, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
		return
	}
	clearPrimaryAgentIfEndpoint(db, node.Endpoint)

	force := c.Query("force") == "true"
	query := db
	if force {
		query = db.Unscoped()
	}
	if err := query.Delete(&node).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "删除 Agent 失败: " + err.Error()})
		return
	}
	msg := "Agent 已删除"
	if force {
		msg = "Agent 已强制删除"
	}
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": msg})
}

func BlockAgentNode(c *gin.Context) {
	db := database.GetDB()
	id, ok := parseAgentNodeID(c)
	if !ok {
		return
	}
	var req agentNodeRequest
	_ = c.ShouldBindJSON(&req)
	var node models.AgentNode
	if err := db.First(&node, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
		return
	}
	node.Blocked = true
	node.Enabled = false
	node.BlockReason = strings.TrimSpace(req.BlockReason)
	if err := db.Save(&node).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "屏蔽 Agent 失败"})
		return
	}
	clearPrimaryAgentIfEndpoint(db, node.Endpoint)
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Agent 已屏蔽", "data": node})
}

func UnblockAgentNode(c *gin.Context) {
	db := database.GetDB()
	id, ok := parseAgentNodeID(c)
	if !ok {
		return
	}
	var node models.AgentNode
	if err := db.First(&node, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
		return
	}
	node.Blocked = false
	node.Enabled = true
	node.BlockReason = ""
	if err := db.Save(&node).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "解除屏蔽失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Agent 已启用", "data": node})
}

func UseAgentNode(c *gin.Context) {
	db := database.GetDB()
	id, ok := parseAgentNodeID(c)
	if !ok {
		return
	}
	var node models.AgentNode
	if err := db.First(&node, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
		return
	}
	if node.Blocked || !node.Enabled {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 已禁用或被屏蔽，不能设为当前"})
		return
	}
	config, err := loadWritableSystemConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}
	config.PublishAgentEndpoint = node.Endpoint
	config.AgentPurpose = models.NormalizeAgentPurpose(node.Purpose)
	if models.AgentPurposeAllows(node.Purpose, models.AgentPurposeUpload) {
		config.PublishMode = "remote"
	} else {
		config.PublishMode = "local"
	}
	if models.AgentPurposeAllows(node.Purpose, models.AgentPurposeFilescan) {
		config.FileCheckMode = models.FileCheckModeRemote
	} else {
		config.FileCheckMode = models.FileCheckModeLocal
	}
	normalizeSystemConfig(&config)
	if err := db.Save(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存当前 Agent 失败"})
		return
	}
	node.IsPrimary = true
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "已设为当前 Agent", "data": gin.H{"agent": node, "config": config}})
}

func DetectAgentNode(c *gin.Context) {
	db := database.GetDB()
	id, ok := parseAgentNodeID(c)
	if !ok {
		return
	}
	var node models.AgentNode
	if err := db.First(&node, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
		return
	}
	config, err := loadWritableSystemConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}
	client := agent.NewClient(node.Endpoint, config.PublishAgentToken, time.Duration(config.PublishAgentTimeout)*time.Second)
	health, err := client.Health()
	now := time.Now()
	if err != nil {
		node.LastHealthStatus = "error"
		node.LastHealthMessage = err.Error()
		db.Save(&node)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 检测失败: " + err.Error(), "data": node})
		return
	}
	wantPurpose := models.NormalizeAgentPurpose(node.Purpose)
	if !healthSupportsPurpose(health, wantPurpose) {
		msg := fmt.Sprintf("Agent 用途不匹配: 需要 %s，实际 %s", wantPurpose, strings.TrimSpace(health.Purpose))
		node.LastHealthStatus = "error"
		node.LastHealthMessage = msg
		node.LastPurpose = models.NormalizeAgentPurpose(health.Purpose)
		node.LastCapabilities = strings.Join(health.Capabilities, ",")
		db.Save(&node)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": msg, "data": gin.H{"agent": node, "health": health}})
		return
	}
	node.LastSeenAt = &now
	node.LastHealthStatus = "success"
	node.LastHealthMessage = "Agent 可用"
	node.LastVersion = strings.TrimSpace(health.Version)
	node.LastPurpose = models.NormalizeAgentPurpose(health.Purpose)
	node.LastCapabilities = strings.Join(health.Capabilities, ",")
	db.Save(&node)
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Agent 可用", "data": gin.H{"agent": node, "health": health}})
}

func GetAgentNodeInstallCommand(c *gin.Context) {
	db := database.GetDB()
	id, ok := parseAgentNodeID(c)
	if !ok {
		return
	}
	var node models.AgentNode
	if err := db.First(&node, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
		return
	}
	config, err := loadWritableSystemConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}
	source := models.NormalizeAgentInstallerSource(c.DefaultQuery("source", config.AgentInstallerSource))
	purpose := models.NormalizeAgentPurpose(c.DefaultQuery("purpose", node.Purpose))
	install := buildAgentInstallCommand(c, &config, purpose, source)
	c.JSON(http.StatusOK, gin.H{
		"type":         "success",
		"msg":          "安装命令已生成",
		"command":      install.Command,
		"scriptUrl":    install.ScriptURL,
		"endpoint":     node.Endpoint,
		"purpose":      purpose,
		"source":       source,
		"tokenMissing": false,
	})
}

func buildAgentNodeFromRequest(req agentNodeRequest, existing *models.AgentNode) (models.AgentNode, error) {
	node := models.AgentNode{Enabled: true, Purpose: models.AgentPurposeBoth}
	if existing != nil {
		node = *existing
	}
	endpoint := models.NormalizeAgentEndpoint(req.Endpoint)
	if endpoint == "" {
		if existing != nil {
			endpoint = existing.Endpoint
		}
		if endpoint == "" {
			return node, fmt.Errorf("Agent 地址不能为空")
		}
	}
	node.Endpoint = endpoint
	node.Name = strings.TrimSpace(req.Name)
	if node.Name == "" {
		node.Name = endpoint
	}
	if strings.TrimSpace(req.Purpose) != "" || existing == nil {
		node.Purpose = models.NormalizeAgentPurpose(req.Purpose)
	}
	if node.Purpose == "" {
		node.Purpose = models.AgentPurposeBoth
	}
	if req.Enabled != nil {
		node.Enabled = *req.Enabled
	}
	if req.Blocked != nil {
		node.Blocked = *req.Blocked
	}
	if node.Blocked {
		node.Enabled = false
	}
	node.BlockReason = strings.TrimSpace(req.BlockReason)
	return node, nil
}

func parseAgentNodeID(c *gin.Context) (uint, bool) {
	raw := strings.TrimSpace(c.Param("id"))
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "Agent ID 无效"})
		return 0, false
	}
	return uint(value), true
}

func markPrimaryAgentNodes(nodes []models.AgentNode, endpoint string) {
	primaryEndpoint := models.NormalizeAgentEndpoint(endpoint)
	for i := range nodes {
		nodes[i].Purpose = models.NormalizeAgentPurpose(nodes[i].Purpose)
		nodes[i].Endpoint = models.NormalizeAgentEndpoint(nodes[i].Endpoint)
		nodes[i].IsPrimary = primaryEndpoint != "" && nodes[i].Endpoint == primaryEndpoint
	}
}

func loadWritableSystemConfig(db *gorm.DB) (models.SystemConfig, error) {
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		return config, err
	}
	normalizeSystemConfig(&config)
	if err := db.Save(&config).Error; err != nil {
		return config, err
	}
	return config, nil
}

func syncPrimaryAgentAfterNodeChange(db *gorm.DB, oldEndpoint string, node *models.AgentNode) {
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		return
	}
	primaryEndpoint := models.NormalizeAgentEndpoint(config.PublishAgentEndpoint)
	if primaryEndpoint == "" || primaryEndpoint != models.NormalizeAgentEndpoint(oldEndpoint) {
		return
	}
	if node != nil && node.Enabled && !node.Blocked {
		config.PublishAgentEndpoint = node.Endpoint
	} else {
		config.PublishAgentEndpoint = ""
		if config.PublishMode == "remote" {
			config.PublishMode = "local"
		}
		if config.FileCheckMode == models.FileCheckModeRemote {
			config.FileCheckMode = models.FileCheckModeLocal
		}
	}
	normalizeSystemConfig(&config)
	db.Save(&config)
}

func clearPrimaryAgentIfEndpoint(db *gorm.DB, endpoint string) {
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		return
	}
	if models.NormalizeAgentEndpoint(config.PublishAgentEndpoint) != models.NormalizeAgentEndpoint(endpoint) {
		return
	}
	config.PublishAgentEndpoint = ""
	if config.PublishMode == "remote" {
		config.PublishMode = "local"
	}
	if config.FileCheckMode == models.FileCheckModeRemote {
		config.FileCheckMode = models.FileCheckModeLocal
	}
	normalizeSystemConfig(&config)
	db.Save(&config)
}
