package upload

import (
	"fmt"
	"time"
)

const maxAutoPublishRetries = 6

type UploadCooldownActiveError struct {
	PartID    uint
	ErrorType string
	RetryAt   time.Time
	Remaining time.Duration
}

func (e *UploadCooldownActiveError) Error() string {
	if e == nil {
		return "upload cooldown active"
	}
	return fmt.Sprintf("upload cooldown active: part_id=%d, error_type=%s, retry_at=%s, remaining=%s",
		e.PartID, e.ErrorType, e.RetryAt.Format("2006-01-02 15:04:05"), formatDurationForLog(e.Remaining))
}

func retryBackoffDuration(retryCount int, base, max time.Duration) time.Duration {
	if retryCount < 1 {
		retryCount = 1
	}
	if base <= 0 {
		base = time.Minute
	}
	if max <= 0 {
		max = base
	}
	delay := base
	for i := 1; i < retryCount; i++ {
		if delay >= max/2 {
			return max
		}
		delay *= 2
		if delay > max {
			return max
		}
	}
	return delay
}

func autoTaskCooldownDuration(errorType string, retryCount int) (time.Duration, bool) {
	switch errorType {
	case UploadErrorTypeRateLimit:
		return retryBackoffDuration(retryCount, 30*time.Minute, 24*time.Hour), true
	case UploadErrorTypeNetwork:
		return retryBackoffDuration(retryCount, 10*time.Minute, 6*time.Hour), true
	case UploadErrorTypeUnknown:
		return retryBackoffDuration(retryCount, 15*time.Minute, 6*time.Hour), true
	case UploadErrorTypeAuth:
		return retryBackoffDuration(retryCount, 6*time.Hour, 24*time.Hour), true
	default:
		return 0, false
	}
}

func shouldAutoStopErrorType(errorType string, retryCount int) bool {
	switch errorType {
	case UploadErrorTypePermanent, UploadErrorTypeFile, UploadErrorTypeTranscode, UploadErrorTypeUser:
		return true
	case UploadErrorTypeUnknown:
		return retryCount >= maxAutoPublishRetries
	default:
		return false
	}
}
