package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/models"
)

func TestBuildAgentInstallCommandUsesControllerBaseAsUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://ignored.local/api/agents/1/install-command", nil)

	config := &models.SystemConfig{
		PublishAgentToken:      "shared-token",
		AgentControllerBaseURL: "https://controller.example:12380",
		AgentGitHubRepo:        "spiritlhls/gobup",
		WorkPath:               "/rec",
	}

	install := buildAgentInstallCommand(ctx, config, models.AgentPurposeUpload, models.AgentInstallerSourceController)
	if !strings.Contains(install.Command, "'--controller-base-url' 'https://controller.example:12380'") {
		t.Fatalf("install command missing controller base URL: %s", install.Command)
	}
	if !strings.Contains(install.Command, "'--upstream-base-url' 'https://controller.example:12380'") {
		t.Fatalf("install command should use controller base URL as upstream: %s", install.Command)
	}
	if strings.Contains(install.Command, "'--upstream-base-url' 'http://127.0.0.1:12380'") {
		t.Fatalf("install command must not point remote agent upstream to localhost: %s", install.Command)
	}
	if strings.Count(install.Command, "shared-token") != 2 {
		t.Fatalf("install command should use the shared controller token for agent and upstream: %s", install.Command)
	}
}
