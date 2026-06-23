package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

func TestUpdateSystemConfigPreservesAgentRoutingFields(t *testing.T) {
	oldDB := database.DB
	t.Cleanup(func() {
		database.DB = oldDB
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	database.DB = db

	config := models.SystemConfig{
		AutoFileScan:           true,
		EnableFileWatcher:      true,
		FileScanInterval:       60,
		FileScanMinAge:         12,
		FileScanMinSize:        1048576,
		FileScanMaxAge:         720,
		WorkPath:               "/rec",
		PublishMode:            "remote",
		PublishAgentEndpoint:   "http://127.0.0.1:12381",
		PublishAgentToken:      "shared-token",
		PublishAgentTimeout:    45,
		AgentPurpose:           models.AgentPurposeBoth,
		AgentInstallerSource:   models.AgentInstallerSourceController,
		AgentControllerBaseURL: "http://controller",
		AgentGitHubRepo:        "spiritlhls/gobup",
		FileCheckMode:          models.FileCheckModeRemote,
		DanmakuBurnStyle:       "default",
		DanmakuScrollArea:      0.75,
		DanmakuDisplayArea:     0.8,
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatal(err)
	}

	payload := map[string]interface{}{
		"autoFileScan":          false,
		"enableFileWatcher":     false,
		"fileScanInterval":      120,
		"fileScanMinAge":        6,
		"fileScanMinSize":       2048,
		"fileScanMaxAge":        240,
		"workPath":              "/new-rec",
		"uploadSpeedLimitMbps":  2.5,
		"uploadWhileRecording":  true,
		"publishWhileRecording": true,
		"danmakuBurnStyle":      "compact",
		"danmakuScrollArea":     0.6,
		"danmakuDisplayArea":    0.7,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/config/system", UpdateSystemConfig)
	req := httptest.NewRequest(http.MethodPut, "/config/system", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var saved models.SystemConfig
	if err := db.First(&saved, config.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.PublishMode != "remote" {
		t.Fatalf("PublishMode=%q, want remote", saved.PublishMode)
	}
	if saved.PublishAgentEndpoint != "http://127.0.0.1:12381" {
		t.Fatalf("PublishAgentEndpoint=%q, want preserved endpoint", saved.PublishAgentEndpoint)
	}
	if saved.PublishAgentToken != "shared-token" {
		t.Fatalf("PublishAgentToken=%q, want preserved token", saved.PublishAgentToken)
	}
	if saved.FileCheckMode != models.FileCheckModeRemote {
		t.Fatalf("FileCheckMode=%q, want remote", saved.FileCheckMode)
	}
	if saved.WorkPath != "/new-rec" {
		t.Fatalf("WorkPath=%q, want updated system setting", saved.WorkPath)
	}
}
