package upload

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

func newAgentTargetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AgentNode{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSelectPreferredPublishAgentEndpoint(t *testing.T) {
	db := newAgentTargetTestDB(t)
	nodes := []models.AgentNode{
		{Name: "low", Endpoint: "http://127.0.0.1:12381", Purpose: models.AgentPurposeUpload, Enabled: true, Priority: 10, LastHealthStatus: "success"},
		{Name: "high", Endpoint: "http://127.0.0.2:12381", Purpose: models.AgentPurposeBoth, Enabled: true, Priority: 90, LastHealthStatus: "success"},
		{Name: "disabled", Endpoint: "http://127.0.0.3:12381", Purpose: models.AgentPurposeUpload, Enabled: false, Priority: 100, LastHealthStatus: "success"},
	}
	if err := db.Select("*").Create(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AgentNode{}).Where("name = ?", "disabled").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	endpoint, ok := selectPreferredPublishAgentEndpoint(db)
	if !ok {
		t.Fatal("expected a preferred endpoint")
	}
	if endpoint != "http://127.0.0.2:12381" {
		t.Fatalf("endpoint=%q, want high priority node", endpoint)
	}
}

func TestValidatePublishAgentEndpointForUpload(t *testing.T) {
	db := newAgentTargetTestDB(t)
	node := models.AgentNode{
		Endpoint: "http://127.0.0.1:12381",
		Purpose:  models.AgentPurposeFilescan,
		Enabled:  true,
	}
	if err := db.Select("*").Create(&node).Error; err != nil {
		t.Fatal(err)
	}

	err := validatePublishAgentEndpointForUpload(db, "http://127.0.0.1:12381")
	if err == nil || !strings.Contains(err.Error(), "不允许上传投稿") {
		t.Fatalf("err=%v, want upload purpose rejection", err)
	}

	if err := db.Model(&node).Updates(map[string]interface{}{"purpose": models.AgentPurposeUpload, "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	err = validatePublishAgentEndpointForUpload(db, "http://127.0.0.1:12381")
	if err == nil || !strings.Contains(err.Error(), "停用") {
		t.Fatalf("err=%v, want disabled rejection", err)
	}
}
