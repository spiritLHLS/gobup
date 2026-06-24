package upload

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

type uploadRoute struct {
	Mode     string
	Endpoint string
	ProxyURL string
}

func resolveUploadRoute(db *gorm.DB) (uploadRoute, error) {
	route := uploadRoute{Mode: "local"}
	if db == nil {
		return route, nil
	}

	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		return route, nil
	}
	if !strings.EqualFold(strings.TrimSpace(config.PublishMode), "remote") {
		return route, nil
	}
	if !models.AgentPurposeAllows(config.AgentPurpose, models.AgentPurposeUpload) {
		return route, fmt.Errorf("当前 Agent 用途为 %s，不允许作为上传出口", models.NormalizeAgentPurpose(config.AgentPurpose))
	}

	endpoint := models.NormalizeAgentEndpoint(config.PublishAgentEndpoint)
	if endpoint == "" {
		if selectedEndpoint, ok := selectPreferredPublishAgentEndpoint(db); ok {
			endpoint = selectedEndpoint
			_ = db.Model(&config).Update("publish_agent_endpoint", selectedEndpoint).Error
		}
	}
	if endpoint == "" {
		return route, fmt.Errorf("已选择 Agent 上传，但未配置可用 Agent 地址")
	}
	if err := validatePublishAgentEndpointForUpload(db, endpoint); err != nil {
		return route, err
	}

	token := strings.TrimSpace(config.PublishAgentToken)
	if token == "" {
		token = models.NewAgentToken()
		_ = db.Model(&config).Update("publish_agent_token", token).Error
	}

	proxyURL, err := buildAgentProxyURL(endpoint, token)
	if err != nil {
		return route, err
	}
	return uploadRoute{
		Mode:     "agent",
		Endpoint: endpoint,
		ProxyURL: proxyURL,
	}, nil
}

func buildAgentProxyURL(endpoint, token string) (string, error) {
	parsed, err := url.Parse(models.NormalizeAgentEndpoint(endpoint))
	if err != nil {
		return "", fmt.Errorf("解析 Agent 地址失败: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("Agent 地址无效: %s", endpoint)
	}
	proxy := &url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
	}
	if strings.TrimSpace(token) != "" {
		proxy.User = url.User(strings.TrimSpace(token))
	}
	return proxy.String(), nil
}
