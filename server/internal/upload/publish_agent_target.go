package upload

import (
	"fmt"
	"strings"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

func selectPreferredPublishAgentEndpoint(db *gorm.DB) (string, bool) {
	if db == nil {
		return "", false
	}
	var node models.AgentNode
	err := db.Where("enabled = ? AND blocked = ?", true, false).
		Where("purpose IN ?", []string{models.AgentPurposeUpload, models.AgentPurposeBoth, ""}).
		Order("CASE WHEN last_health_status = 'success' THEN 0 ELSE 1 END, priority DESC, updated_at DESC").
		First(&node).Error
	if err != nil {
		return "", false
	}
	endpoint := models.NormalizeAgentEndpoint(node.Endpoint)
	return endpoint, endpoint != ""
}

func validatePublishAgentEndpointForUpload(db *gorm.DB, endpoint string) error {
	if db == nil {
		return nil
	}
	endpoint = models.NormalizeAgentEndpoint(endpoint)
	var node models.AgentNode
	if err := db.Where("endpoint = ?", endpoint).First(&node).Error; err != nil {
		return nil
	}
	if node.Blocked {
		if strings.TrimSpace(node.BlockReason) != "" {
			return fmt.Errorf("当前远程 Agent 已屏蔽: %s", strings.TrimSpace(node.BlockReason))
		}
		return fmt.Errorf("当前远程 Agent 已屏蔽")
	}
	if !node.Enabled {
		return fmt.Errorf("当前远程 Agent 已停用")
	}
	if !models.AgentPurposeAllows(node.Purpose, models.AgentPurposeUpload) {
		return fmt.Errorf("当前远程 Agent 用途为 %s，不允许上传投稿", models.NormalizeAgentPurpose(node.Purpose))
	}
	return nil
}

func markPublishAgentEndpointError(endpoint string, err error) {
	if err == nil {
		return
	}
	endpoint = models.NormalizeAgentEndpoint(endpoint)
	if endpoint == "" {
		return
	}
	_ = database.GetDB().Model(&models.AgentNode{}).
		Where("endpoint = ?", endpoint).
		Updates(map[string]interface{}{
			"last_health_status":  "error",
			"last_health_message": err.Error(),
		}).Error
}

func markPublishAgentEndpointSuccess(endpoint string) {
	endpoint = models.NormalizeAgentEndpoint(endpoint)
	if endpoint == "" {
		return
	}
	now := time.Now()
	_ = database.GetDB().Model(&models.AgentNode{}).
		Where("endpoint = ?", endpoint).
		Updates(map[string]interface{}{
			"last_seen_at":        &now,
			"last_health_status":  "success",
			"last_health_message": "最近投稿请求成功",
		}).Error
}
