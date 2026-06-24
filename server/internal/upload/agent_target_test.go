package upload

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func newUploadPartTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RecordHistoryPart{}); err != nil {
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

func TestBuildAgentProxyURL(t *testing.T) {
	proxyURL, err := buildAgentProxyURL("http://127.0.0.1:12381/agent/v1", "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL != "http://secret-token@127.0.0.1:12381" {
		t.Fatalf("proxyURL=%q", proxyURL)
	}
}

func TestPersistUploadFailureBackoffPreservesControlState(t *testing.T) {
	db := newUploadPartTestDB(t)
	part := models.RecordHistoryPart{
		FileName:         "a.flv",
		Uploading:        true,
		UploadPaused:     true,
		UploadRetryCount: 2,
	}
	if err := db.Create(&part).Error; err != nil {
		t.Fatal(err)
	}

	cooldownText := persistUploadFailure(db, &part, UploadErrorTypeNetwork, errors.New("unexpected EOF"))
	if cooldownText == "" {
		t.Fatal("expected network failure to set a cooldown")
	}

	var got models.RecordHistoryPart
	if err := db.First(&got, part.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !got.UploadPaused || !got.Uploading {
		t.Fatalf("control state changed: paused=%v uploading=%v", got.UploadPaused, got.Uploading)
	}
	if got.RateLimitCooldownAt == nil || time.Until(*got.RateLimitCooldownAt) <= 0 {
		t.Fatalf("cooldown not set in future: %v", got.RateLimitCooldownAt)
	}
	if got.RateLimitRetryCount != 1 {
		t.Fatalf("RateLimitRetryCount=%d, want 1", got.RateLimitRetryCount)
	}
}

func TestPersistUploadFailureUserAbortDoesNotRetry(t *testing.T) {
	db := newUploadPartTestDB(t)
	part := models.RecordHistoryPart{
		FileName:         "a.flv",
		UploadRetryCount: 2,
	}
	if err := db.Create(&part).Error; err != nil {
		t.Fatal(err)
	}

	persistUploadFailure(db, &part, UploadErrorTypeUser, errors.New("用户暂停上传"))

	var got models.RecordHistoryPart
	if err := db.First(&got, part.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.UploadRetryCount != 2 {
		t.Fatalf("UploadRetryCount=%d, want unchanged 2", got.UploadRetryCount)
	}
	if got.RateLimitCooldownAt != nil || got.RateLimitRetryCount != 0 {
		t.Fatalf("user abort should not set cooldown: cooldown=%v retry=%d", got.RateLimitCooldownAt, got.RateLimitRetryCount)
	}
}

func TestRefreshPartUploadCooldownStateReturnsTypedCooldown(t *testing.T) {
	db := newUploadPartTestDB(t)
	cooldownUntil := time.Now().Add(10 * time.Minute)
	part := models.RecordHistoryPart{
		FileName:            "a.flv",
		UploadErrorType:     UploadErrorTypeNetwork,
		RateLimitCooldownAt: &cooldownUntil,
		RateLimitRetryCount: 2,
	}
	if err := db.Create(&part).Error; err != nil {
		t.Fatal(err)
	}

	err := refreshPartUploadCooldownState(db, &part, time.Now())
	var cooldownErr *UploadCooldownActiveError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("err=%v, want UploadCooldownActiveError", err)
	}
	if classifyUploadError(err) != UploadErrorTypeNetwork {
		t.Fatalf("classifyUploadError=%s, want network", classifyUploadError(err))
	}
	if cooldownErr.ErrorType != UploadErrorTypeNetwork {
		t.Fatalf("cooldown error type=%s, want network", cooldownErr.ErrorType)
	}
}

func TestRefreshPartUploadCooldownStateClearsExpiredWithoutControlState(t *testing.T) {
	db := newUploadPartTestDB(t)
	expiredAt := time.Now().Add(-time.Minute)
	part := models.RecordHistoryPart{
		FileName:            "a.flv",
		Uploading:           true,
		UploadPaused:        true,
		UploadErrorMsg:      "temporary failure",
		UploadErrorType:     UploadErrorTypeUnknown,
		RateLimitCooldownAt: &expiredAt,
		RateLimitRetryCount: 3,
	}
	if err := db.Create(&part).Error; err != nil {
		t.Fatal(err)
	}

	if err := refreshPartUploadCooldownState(db, &part, time.Now()); err != nil {
		t.Fatalf("refreshPartUploadCooldownState returned error: %v", err)
	}

	var got models.RecordHistoryPart
	if err := db.First(&got, part.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !got.UploadPaused || !got.Uploading {
		t.Fatalf("control state changed: paused=%v uploading=%v", got.UploadPaused, got.Uploading)
	}
	if got.RateLimitCooldownAt != nil || got.RateLimitRetryCount != 0 {
		t.Fatalf("cooldown not cleared: cooldown=%v retry=%d", got.RateLimitCooldownAt, got.RateLimitRetryCount)
	}
	if got.UploadErrorMsg != "" || got.UploadErrorType != "" {
		t.Fatalf("error fields not cleared: msg=%q type=%q", got.UploadErrorMsg, got.UploadErrorType)
	}
}
