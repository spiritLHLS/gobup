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
	retryCount := history.PublishRetryCount + 1
	message := fmt.Sprintf("投稿失败: %v", err)
	updates := map[string]interface{}{
		"message":             message,
		"publish_error_type":  errorType,
		"publish_retry_count": gorm.Expr("publish_retry_count + ?", 1),
	}

	history.Message = message
	history.PublishErrorType = errorType
	history.PublishRetryCount = retryCount

	if shouldAutoStopErrorType(errorType, retryCount) {
		updates["upload"] = false
		updates["publish_cooldown_at"] = nil
		history.Upload = false
		history.PublishCooldownAt = nil
		history.Message = message + "；已自动停止该历史记录的自动投稿，请修正后手动重试"
		updates["message"] = history.Message
	} else if cooldownDelay, ok := autoTaskCooldownDuration(errorType, retryCount); ok {
		cooldown := time.Now().Add(cooldownDelay)
		updates["publish_cooldown_at"] = &cooldown
		history.PublishCooldownAt = &cooldown
	} else {
		updates["publish_cooldown_at"] = nil
		history.PublishCooldownAt = nil
	}

	_ = db.Model(history).Updates(updates).Error
}
