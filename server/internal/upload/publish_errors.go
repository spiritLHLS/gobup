package upload

import (
	"fmt"
	"time"

	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

func markPublishFailure(db *gorm.DB, history *models.RecordHistory, err error) {
	if db == nil || history == nil || err == nil {
		return
	}

	errorType := classifyUploadError(err)
	message := fmt.Sprintf("投稿失败: %v", err)
	updates := map[string]interface{}{
		"message":             message,
		"publish_error_type":  errorType,
		"publish_retry_count": gorm.Expr("publish_retry_count + ?", 1),
	}

	history.Message = message
	history.PublishErrorType = errorType
	history.PublishRetryCount++

	if errorType == UploadErrorTypeRateLimit {
		cooldown := time.Now().Add(24 * time.Hour)
		updates["publish_cooldown_at"] = &cooldown
		history.PublishCooldownAt = &cooldown
	} else {
		updates["publish_cooldown_at"] = nil
		history.PublishCooldownAt = nil
	}

	_ = db.Model(history).Updates(updates).Error
}
