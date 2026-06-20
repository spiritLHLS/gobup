package upload

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

const danmakuBurnTempType = "danmaku_burn"

func (s *Service) markDanmakuBurnSkipped(part *models.RecordHistoryPart, message, errorType string) {
	if part == nil || part.ID == 0 {
		return
	}
	if strings.TrimSpace(errorType) == "" {
		errorType = UploadErrorTypeUnknown
	}
	db := database.GetDB()
	updates := map[string]interface{}{
		"upload":                 false,
		"uploading":              false,
		"upload_cancelled":       true,
		"upload_error_msg":       message,
		"upload_error_type":      errorType,
		"is_temp_file":           true,
		"temp_file_type":         danmakuBurnTempType,
		"source_part_id":         part.ID,
		"appended_to_video":      false,
		"rate_limit_cooldown_at": nil,
	}

	var existing models.RecordHistoryPart
	if err := db.Where(
		"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ?",
		part.ID, true, danmakuBurnTempType,
	).First(&existing).Error; err == nil {
		if err := db.Model(&existing).Updates(updates).Error; err != nil {
			log.Printf("[弹幕烧录] 更新失败标记失败: source_part_id=%d, err=%v", part.ID, err)
		}
		return
	}

	fileName := fmt.Sprintf("danmaku_burn_failed_%d", part.ID)
	if trimmed := strings.TrimSpace(part.FileName); trimmed != "" {
		fileName = filepath.Base(trimmed) + ".danmaku_failed"
	}
	marker := &models.RecordHistoryPart{
		HistoryID:       part.HistoryID,
		RoomID:          part.RoomID,
		SessionID:       part.SessionID,
		Title:           part.Title + " (弹幕版失败)",
		LiveTitle:       part.LiveTitle,
		AreaName:        part.AreaName,
		FilePath:        fmt.Sprintf("__gobup_danmaku_burn_failed_%d", part.ID),
		FileName:        fileName,
		FileSize:        0,
		Duration:        part.Duration,
		StartTime:       part.StartTime,
		EndTime:         part.EndTime,
		Recording:       false,
		Upload:          false,
		Uploading:       false,
		UploadCancelled: true,
		UploadErrorMsg:  message,
		UploadErrorType: errorType,
		IsTempFile:      true,
		SourcePartID:    part.ID,
		TempFileType:    danmakuBurnTempType,
		AppendedToVideo: false,
	}
	if err := db.Create(marker).Error; err != nil {
		log.Printf("[弹幕烧录] 创建失败标记失败: source_part_id=%d, err=%v", part.ID, err)
	}
}
