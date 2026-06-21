package upload

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

const (
	biliPublishTitleMaxRunes = 80
	biliPartTitleMaxRunes    = 80
)

func normalizeBiliPublishTitle(label, title string) string {
	normalized := strings.TrimSpace(title)
	if normalized == "" {
		normalized = "直播回放"
	}
	truncated, before, changed := truncateRunes(normalized, biliPublishTitleMaxRunes)
	if changed {
		log.Printf("[投稿] %s超过%d字符，已自动截断: 原长度=%d, 新长度=%d",
			label, biliPublishTitleMaxRunes, before, len([]rune(truncated)))
	}
	return truncated
}

func normalizeBiliPartTitle(index int, title string) string {
	normalized := strings.TrimSpace(title)
	if normalized == "" {
		normalized = fmt.Sprintf("P%d", index)
	}
	truncated, before, changed := truncateRunes(normalized, biliPartTitleMaxRunes)
	if changed {
		log.Printf("[投稿] 分P标题超过%d字符，已自动截断: index=%d, 原长度=%d, 新长度=%d",
			biliPartTitleMaxRunes, index, before, len([]rune(truncated)))
	}
	return truncated
}

func truncateRunes(text string, max int) (string, int, bool) {
	if max <= 0 {
		return "", len([]rune(text)), text != ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text, len(runes), false
	}
	return string(runes[:max]), len(runes), true
}

func shouldSkipTooShortPublishPart(part models.RecordHistoryPart) (bool, string) {
	duration := estimatedPartDurationSeconds(part)
	if duration > 0 && duration < 1 {
		return true, fmt.Sprintf("分P时长 %.3f 秒，不足 1 秒，已从投稿分P中过滤", duration)
	}
	if part.Duration <= 0 && !part.StartTime.IsZero() && !part.EndTime.IsZero() && !part.EndTime.After(part.StartTime) {
		return true, "分P时长无效或不足 1 秒，已从投稿分P中过滤"
	}
	return false, ""
}

func estimatedPartDurationSeconds(part models.RecordHistoryPart) float64 {
	if part.Duration > 0 {
		return float64(part.Duration)
	}
	if !part.StartTime.IsZero() && !part.EndTime.IsZero() && part.EndTime.After(part.StartTime) {
		return part.EndTime.Sub(part.StartTime).Seconds()
	}
	return 0
}

func markPublishPartSkipped(db *gorm.DB, part *models.RecordHistoryPart, reason string) {
	if db == nil || part == nil || part.ID == 0 {
		return
	}
	updates := map[string]interface{}{
		"upload_cancelled":       true,
		"uploading":              false,
		"upload_error_msg":       reason,
		"upload_error_type":      UploadErrorTypePermanent,
		"rate_limit_cooldown_at": nil,
	}
	if err := db.Model(part).Updates(updates).Error; err != nil {
		log.Printf("[投稿] 标记无效分P失败: part_id=%d, err=%v", part.ID, err)
	}
	part.UploadCancelled = true
	part.Uploading = false
	part.UploadErrorMsg = reason
	part.UploadErrorType = UploadErrorTypePermanent
	part.RateLimitCooldownAt = nil
}

func publishPartFilename(part models.RecordHistoryPart, index int) string {
	filename := strings.TrimSpace(part.FileName)
	if filename != "" {
		return filename
	}
	baseName := filepath.Base(part.FilePath)
	if ext := filepath.Ext(baseName); ext != "" {
		filename = baseName[:len(baseName)-len(ext)]
	} else {
		filename = baseName
	}
	if filename == "." || filename == string(filepath.Separator) {
		filename = ""
	}
	if filename == "" {
		filename = fmt.Sprintf("part_%d", index)
	}
	log.Printf("警告: 分P[%d]的FileName为空，从FilePath提取: %s", index, filename)
	return filename
}
