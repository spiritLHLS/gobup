package controllers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/assets"
	"github.com/gobup/server/internal/agent"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

var allowedAgentReleaseName = regexp.MustCompile(`^gobup-agent-linux-(amd64|arm64)\.tar\.gz$`)

const (
	defaultAgentGitHubRepo       = "spiritlhls/gobup"
	defaultAgentInstallerRawURL  = "https://raw.githubusercontent.com/spiritLHLS/gobup/main/scripts/install_agent.sh"
	defaultAgentReleaseBaseURL   = "https://github.com/spiritLHLS/gobup/releases/latest/download"
	defaultAgentLocalUpstreamURL = "http://127.0.0.1:12380"
)

func AgentHealth(c *gin.Context) {
	config, ok := validateAgentToken(c)
	if !ok {
		return
	}
	purpose := models.NormalizeAgentPurpose(config.AgentPurpose)
	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"msg":  "agent ok",
		"data": gin.H{
			"version":      "go-controller",
			"purpose":      purpose,
			"capabilities": models.AgentCapabilities(purpose),
			"workPath":     config.WorkPath,
			"time":         time.Now().Format(time.RFC3339),
		},
	})
}

func AgentPublish(c *gin.Context) {
	config, ok := validateAgentToken(c)
	if !ok {
		return
	}
	if !models.AgentPurposeAllows(config.AgentPurpose, models.AgentPurposeUpload) {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "Agent 当前用途不允许投稿"})
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

func AgentFilesCheck(c *gin.Context) {
	config, ok := validateAgentToken(c)
	if !ok {
		return
	}
	if !models.AgentPurposeAllows(config.AgentPurpose, models.AgentPurposeFilescan) {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "Agent 当前用途不允许检查录制文件"})
		return
	}
	limit := parsePositiveInt(c.DefaultQuery("limit", "100"), 100)
	req := agent.FileCheckRequest{Limit: limit}
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
			return
		}
		if req.Limit <= 0 {
			req.Limit = limit
		} else {
			req.Limit = parsePositiveInt(strconv.Itoa(req.Limit), 100)
		}
	}
	result, err := services.CheckRecordedFilesFromRequest(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}
	result.Purpose = models.NormalizeAgentPurpose(config.AgentPurpose)
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "文件检查完成", "data": result})
}

func DetectPublishAgent(c *gin.Context) {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}
	if strings.TrimSpace(config.PublishAgentEndpoint) == "" {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "未配置 Agent 地址"})
		return
	}
	timeout := time.Duration(config.PublishAgentTimeout) * time.Second
	client := agent.NewClient(config.PublishAgentEndpoint, config.PublishAgentToken, timeout)
	health, err := client.Health()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Agent 不可用: " + err.Error()})
		return
	}
	wantPurpose := models.NormalizeAgentPurpose(c.DefaultQuery("purpose", config.AgentPurpose))
	if !healthSupportsPurpose(health, wantPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  fmt.Sprintf("Agent 用途不匹配: 需要 %s，实际 %s", wantPurpose, strings.TrimSpace(health.Purpose)),
			"data": health,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Agent 可用", "data": health})
}

func CheckAgentFiles(c *gin.Context) {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}
	limit := parsePositiveInt(c.DefaultQuery("limit", "100"), 100)
	if models.NormalizeFileCheckMode(config.FileCheckMode) == models.FileCheckModeRemote {
		if strings.TrimSpace(config.PublishAgentEndpoint) == "" {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "未配置 Agent 地址"})
			return
		}
		client := agent.NewClient(config.PublishAgentEndpoint, config.PublishAgentToken, time.Duration(config.PublishAgentTimeout)*time.Second)
		minSize := config.FileScanMinSize
		result, err := client.CheckFiles(agent.FileCheckRequest{
			Limit:   limit,
			MinSize: &minSize,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "远程 Agent 文件检查失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "远程 Agent 文件检查完成", "data": result})
		return
	}

	result, err := services.CheckRecordedFilesFromConfig(limit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}
	result.Purpose = models.AgentPurposeFilescan
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "本地文件检查完成", "data": result})
}

func GetAgentInstallCommand(c *gin.Context) {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}
	normalizeSystemConfig(&config)
	baseURL := strings.TrimRight(strings.TrimSpace(config.AgentControllerBaseURL), "/")
	if baseURL == "" {
		baseURL = requestBaseURL(c)
	}
	purpose := models.NormalizeAgentPurpose(c.DefaultQuery("purpose", config.AgentPurpose))
	source := models.NormalizeAgentInstallerSource(c.DefaultQuery("source", config.AgentInstallerSource))
	token := strings.TrimSpace(config.PublishAgentToken)
	tokenMissing := token == ""
	if tokenMissing {
		token = "<AGENT_TOKEN>"
	}
	repo := strings.TrimSpace(config.AgentGitHubRepo)
	if repo == "" {
		repo = defaultAgentGitHubRepo
	}
	cdnBase := strings.TrimRight(strings.TrimSpace(config.AgentCDNBaseURL), "/")
	rawScriptURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/scripts/install_agent.sh", repo)
	scriptURL := baseURL + "/agent/install-agent.sh"
	if source == models.AgentInstallerSourceGitHub {
		scriptURL = rawScriptURL
	}
	if source == models.AgentInstallerSourceCDN && cdnBase != "" {
		scriptURL = cdnBase + "/" + rawScriptURL
	}

	args := []string{
		"--purpose", purpose,
		"--token", token,
		"--source", source,
		"--controller-base-url", baseURL,
		"--upstream-base-url", defaultAgentLocalUpstreamURL,
		"--upstream-token", token,
		"--repo", repo,
	}
	if strings.TrimSpace(config.WorkPath) != "" {
		args = append(args, "--work-path", config.WorkPath)
	}
	if cdnBase != "" {
		args = append(args, "--cdn-base-url", cdnBase)
	}
	command := "curl -fsSL " + shellQuote(scriptURL) + " | sh -s -- " + shellJoin(args)

	c.JSON(http.StatusOK, gin.H{
		"type":         "success",
		"msg":          "安装命令已生成",
		"command":      command,
		"scriptUrl":    scriptURL,
		"purpose":      purpose,
		"source":       source,
		"tokenMissing": tokenMissing,
	})
}

func DownloadAgentInstaller(c *gin.Context) {
	content, err := assets.ReadAgentAsset("install_agent.sh")
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, defaultAgentInstallerRawURL)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("Content-Disposition", "inline; filename=install_agent.sh")
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", content)
}

func DownloadAgentRelease(c *gin.Context) {
	name := filepath.Base(c.Param("filename"))
	if !allowedAgentReleaseName.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "invalid release filename"})
		return
	}

	content, err := assets.ReadAgentAsset(name)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, defaultAgentReleaseBaseURL+"/"+name)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("Content-Disposition", "attachment; filename="+name)
	c.Data(http.StatusOK, "application/gzip", content)
}

func validateAgentToken(c *gin.Context) (*models.SystemConfig, bool) {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "Agent 未初始化"})
		return nil, false
	}
	expected := strings.TrimSpace(config.PublishAgentToken)
	if expected == "" {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "Agent token 未配置"})
		return nil, false
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
		return nil, false
	}
	return &config, true
}

func healthSupportsPurpose(health *agent.HealthData, wantPurpose string) bool {
	if health == nil {
		return false
	}
	available := models.NormalizeAgentPurpose(health.Purpose)
	switch models.NormalizeAgentPurpose(wantPurpose) {
	case models.AgentPurposeUpload:
		return models.AgentPurposeAllows(available, models.AgentPurposeUpload) || hasCapability(health.Capabilities, models.AgentPurposeUpload)
	case models.AgentPurposeFilescan:
		return models.AgentPurposeAllows(available, models.AgentPurposeFilescan) || hasCapability(health.Capabilities, models.AgentPurposeFilescan)
	default:
		return models.AgentPurposeAllows(available, models.AgentPurposeUpload) &&
			models.AgentPurposeAllows(available, models.AgentPurposeFilescan)
	}
}

func hasCapability(capabilities []string, capability string) bool {
	for _, value := range capabilities {
		if strings.EqualFold(strings.TrimSpace(value), capability) {
			return true
		}
	}
	return false
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func requestBaseURL(c *gin.Context) string {
	proto := firstHeader(c, "X-Forwarded-Proto", "X-Forwarded-Scheme")
	if proto == "" {
		if c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := firstHeader(c, "X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func firstHeader(c *gin.Context, names ...string) string {
	for _, name := range names {
		value := strings.TrimSpace(c.GetHeader(name))
		if value != "" {
			return strings.Split(value, ",")[0]
		}
	}
	return ""
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
