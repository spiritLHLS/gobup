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
	Priority    *int   `json:"priority"`
	Enabled     *bool  `json:"enabled"`
	Blocked     *bool  `json:"blocked"`
	BlockReason string `json:"blockReason"`
}

type uploadTargetRequest struct {
	TargetType string `json:"targetType"`
	AgentID    uint   `json:"agentId"`
}

func ListAgentNodes(c *gin.Context) {
	db := database.GetDB()
	var config models.SystemConfig
	_ = db.First(&config).Error

	var nodes []models.AgentNode
	if err := db.Order("blocked ASC, enabled DESC, priority DESC, updated_at DESC").Find(&nodes).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "查询 Agent 失败"})
		return
	}
	markPrimaryAgentNodes(nodes, config.PublishAgentEndpoint)
	c.JSON(http.StatusOK, gin.H{"type": "success", "data": nodes, "total": len(nodes)})
}

func ListUploadTargets(c *gin.Context) {
	db := database.GetDB()
	config, err := loadWritableSystemConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}

	var nodes []models.AgentNode
	if err := db.Order("blocked ASC, enabled DESC, priority DESC, updated_at DESC").Find(&nodes).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "查询上传目标失败"})
		return
	}

	refresh := c.Query("refresh") == "true"
	targets := make([]gin.H, 0, len(nodes)+1)
	currentEndpoint := models.NormalizeAgentEndpoint(config.PublishAgentEndpoint)
	localCurrent := config.PublishMode != "remote" || currentEndpoint == ""
	targets = append(targets, gin.H{
		"targetType": "local",
		"id":         0,
		"name":       "本地上传",
		"endpoint":   "",
		"purpose":    models.AgentPurposeUpload,
		"priority":   0,
		"available":  true,
		"current":    localCurrent,
		"status":     "success",
		"message":    "本机主控执行投稿请求",
	})

	for i := range nodes {
		nodes[i].Endpoint = models.NormalizeAgentEndpoint(nodes[i].Endpoint)
		nodes[i].Purpose = models.NormalizeAgentPurpose(nodes[i].Purpose)
		if refresh && nodes[i].Enabled && !nodes[i].Blocked && models.AgentPurposeAllows(nodes[i].Purpose, models.AgentPurposeUpload) {
			_, _ = refreshAgentNodeHealth(db, &nodes[i], &config, models.AgentPurposeUpload)
		}
		available, reason := agentUploadTargetAvailability(&nodes[i])
		message := strings.TrimSpace(nodes[i].LastHealthMessage)
		if reason != "" {
			message = reason
		}
		targets = append(targets, gin.H{
			"targetType":        "agent",
			"id":                nodes[i].ID,
			"name":              nodes[i].Name,
			"endpoint":          nodes[i].Endpoint,
			"purpose":           nodes[i].Purpose,
			"priority":          nodes[i].Priority,
			"available":         available,
			"disabledReason":    reason,
			"current":           config.PublishMode == "remote" && currentEndpoint != "" && nodes[i].Endpoint == currentEndpoint,
			"status":            nodes[i].LastHealthStatus,
			"message":           message,
			"lastSeenAt":        nodes[i].LastSeenAt,
			"lastHealthMessage": nodes[i].LastHealthMessage,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"data": gin.H{
			"targets": targets,
			"config":  config,
		},
	})
}

func SelectUploadTarget(c *gin.Context) {
	var req uploadTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	db := database.GetDB()
	config, err := loadWritableSystemConfig(db)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}

	targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
	switch targetType {
	case "local":
		config.PublishMode = "local"
		normalizeSystemConfig(&config)
		if err := db.Save(&config).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存上传目标失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "已切换为本地上传", "data": gin.H{"config": config}})
	case "agent":
		if req.AgentID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "Agent ID 无效"})
			return
		}
		var node models.AgentNode
		if err := db.First(&node, req.AgentID).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不存在"})
			return
		}
		if node.Blocked || !node.Enabled {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 已禁用或被屏蔽，不能作为上传目标"})
			return
		}
		if !models.AgentPurposeAllows(node.Purpose, models.AgentPurposeUpload) {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "该 Agent 用途不包含上传投稿"})
			return
		}
		if _, err := refreshAgentNodeHealth(db, &node, &config, models.AgentPurposeUpload); err != nil {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不可用，不能作为上传目标: " + err.Error(), "data": node})
			return
		}
		config.PublishAgentEndpoint = node.Endpoint
		config.AgentPurpose = models.NormalizeAgentPurpose(node.Purpose)
		config.PublishMode = "remote"
		if config.FileCheckMode == models.FileCheckModeRemote && !models.AgentPurposeAllows(node.Purpose, models.AgentPurposeFilescan) {
			config.FileCheckMode = models.FileCheckModeLocal
		}
		normalizeSystemConfig(&config)
		if err := db.Save(&config).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存上传目标失败"})
			return
		}
		node.IsPrimary = true
		c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "已切换为 Agent 上传", "data": gin.H{"agent": node, "config": config}})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "未知上传目标类型"})
	}
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
	if err := db.Select("*").Create(&node).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "创建 Agent 失败: " + err.Error()})
		return
	}
	if !node.Enabled || node.Blocked {
		_ = db.Model(&node).Updates(map[string]interface{}{
			"enabled": node.Enabled,
			"blocked": node.Blocked,
		}).Error
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
	if _, err := refreshAgentNodeHealth(db, &node, &config, models.NormalizeAgentPurpose(node.Purpose)); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不可用，不能设为当前: " + err.Error(), "data": node})
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
	health, err := refreshAgentNodeHealth(db, &node, &config, models.NormalizeAgentPurpose(node.Purpose))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 检测失败: " + err.Error(), "data": node})
		return
	}
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
	normalizeSystemConfig(&config)
	db.Save(&config)
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
	node := models.AgentNode{Enabled: true, Purpose: models.AgentPurposeBoth, Priority: 50}
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
	if req.Priority != nil {
		node.Priority = clampAgentPriority(*req.Priority)
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

func clampAgentPriority(priority int) int {
	if priority < 0 {
		return 0
	}
	if priority > 100 {
		return 100
	}
	return priority
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

func agentUploadTargetAvailability(node *models.AgentNode) (bool, string) {
	if node == nil {
		return false, "Agent 不存在"
	}
	if node.Blocked {
		if strings.TrimSpace(node.BlockReason) != "" {
			return false, "已屏蔽: " + strings.TrimSpace(node.BlockReason)
		}
		return false, "已屏蔽"
	}
	if !node.Enabled {
		return false, "已停用"
	}
	if !models.AgentPurposeAllows(node.Purpose, models.AgentPurposeUpload) {
		return false, "用途不包含上传投稿"
	}
	if node.LastHealthStatus != "success" {
		if strings.TrimSpace(node.LastHealthMessage) != "" {
			return false, node.LastHealthMessage
		}
		return false, "尚未检测为可用"
	}
	return true, ""
}

func refreshAgentNodeHealth(db *gorm.DB, node *models.AgentNode, config *models.SystemConfig, wantPurpose string) (*agent.HealthData, error) {
	if db == nil || node == nil || config == nil {
		return nil, fmt.Errorf("Agent 检测上下文为空")
	}
	node.Endpoint = models.NormalizeAgentEndpoint(node.Endpoint)
	node.Purpose = models.NormalizeAgentPurpose(node.Purpose)
	client := agent.NewClient(node.Endpoint, strings.TrimSpace(config.PublishAgentToken), boundedAgentProbeTimeout(config.PublishAgentTimeout))
	health, err := client.Health()
	now := time.Now()
	if err != nil {
		node.LastHealthStatus = "error"
		node.LastHealthMessage = err.Error()
		_ = db.Save(node).Error
		return nil, err
	}
	wantPurpose = models.NormalizeAgentPurpose(wantPurpose)
	if !healthSupportsPurpose(health, wantPurpose) {
		msg := fmt.Sprintf("Agent 用途不匹配: 需要 %s，实际 %s", wantPurpose, strings.TrimSpace(health.Purpose))
		node.LastHealthStatus = "error"
		node.LastHealthMessage = msg
		node.LastPurpose = models.NormalizeAgentPurpose(health.Purpose)
		node.LastCapabilities = strings.Join(health.Capabilities, ",")
		_ = db.Save(node).Error
		return health, fmt.Errorf("%s", msg)
	}
	node.LastSeenAt = &now
	node.LastHealthStatus = "success"
	node.LastHealthMessage = "Agent 可用"
	node.LastVersion = strings.TrimSpace(health.Version)
	node.LastPurpose = models.NormalizeAgentPurpose(health.Purpose)
	node.LastCapabilities = strings.Join(health.Capabilities, ",")
	_ = db.Save(node).Error
	return health, nil
}

func boundedAgentProbeTimeout(timeoutSeconds int) time.Duration {
	if timeoutSeconds < 3 {
		timeoutSeconds = 3
	}
	if timeoutSeconds > 10 {
		timeoutSeconds = 10
	}
	return time.Duration(timeoutSeconds) * time.Second
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
