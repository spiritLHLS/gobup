package controllers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

type syncLiveSessionsRequest struct {
	HistoryIDs []uint `json:"historyIds"`
}

func SyncLiveSessions(c *gin.Context) {
	var req syncLiveSessionsRequest
	_ = c.ShouldBindJSON(&req)

	db := database.GetDB()
	query := db.Where("is_highlight = ?", false)
	if len(req.HistoryIDs) > 0 {
		query = query.Where("id IN ?", req.HistoryIDs)
	}

	var histories []models.RecordHistory
	if err := query.Order("room_id ASC, title ASC, start_time ASC").Find(&histories).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "查询历史记录失败"})
		return
	}

	groups := make(map[string][]models.RecordHistory)
	for _, history := range histories {
		title := normalizeHistorySyncTitle(history.Title)
		dayKey := models.LiveSessionDayKey(history.StartTime)
		if history.RoomID == "" || title == "" || dayKey == "" {
			continue
		}
		key := history.RoomID + "\x00" + title + "\x00" + dayKey
		groups[key] = append(groups[key], history)
	}

	mergedHistories := 0
	movedParts := int64(0)
	taggedForAppend := 0
	checkedGroups := 0
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		checkedGroups++
		sort.Slice(group, func(i, j int) bool {
			return group[i].StartTime.Before(group[j].StartTime)
		})

		published := make([]models.RecordHistory, 0)
		unpublished := make([]models.RecordHistory, 0)
		for _, history := range group {
			if history.Publish {
				published = append(published, history)
			} else {
				unpublished = append(unpublished, history)
			}
		}

		if len(unpublished) > 1 {
			target := unpublished[0]
			for _, source := range unpublished[1:] {
				parts, err := mergeHistoryInto(db, &target, &source)
				if err != nil {
					continue
				}
				movedParts += parts
				mergedHistories++
				if source.StartTime.Before(target.StartTime) {
					target.StartTime = source.StartTime
				}
				if source.EndTime.After(target.EndTime) {
					target.EndTime = source.EndTime
				}
			}
		}

		if len(published) > 0 && len(unpublished) > 0 {
			sort.Slice(published, func(i, j int) bool {
				return published[i].StartTime.Before(published[j].StartTime)
			})
			target := published[0]
			for _, pending := range unpublished {
				msg := "同步完成：同日同标题已有投稿"
				if target.BvID != "" {
					msg = fmt.Sprintf("同步完成：同日同标题已有投稿 %s，投稿时将自动追加为新分P", target.BvID)
				}
				if err := db.Model(&models.RecordHistory{}).Where("id = ? AND publish = ?", pending.ID, false).
					Update("message", msg).Error; err == nil {
					taggedForAppend++
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"type":            "success",
		"msg":             fmt.Sprintf("同步完成：检查%d组，合并%d条历史，移动%d个分P，标记%d条待追加", checkedGroups, mergedHistories, movedParts, taggedForAppend),
		"checkedGroups":   checkedGroups,
		"mergedHistories": mergedHistories,
		"movedParts":      movedParts,
		"taggedForAppend": taggedForAppend,
	})
}

func mergeHistoryInto(db *gorm.DB, target, source *models.RecordHistory) (int64, error) {
	if target.ID == source.ID || source.Publish {
		return 0, nil
	}
	var movedParts int64
	err := db.Transaction(func(tx *gorm.DB) error {
		partUpdate := tx.Model(&models.RecordHistoryPart{}).
			Where("history_id = ?", source.ID).
			Updates(map[string]interface{}{
				"history_id": target.ID,
				"session_id": target.SessionID,
			})
		if partUpdate.Error != nil {
			return partUpdate.Error
		}
		movedParts = partUpdate.RowsAffected

		if source.SessionID != "" && target.SessionID != "" && source.SessionID != target.SessionID {
			if err := tx.Model(&models.LiveMsg{}).
				Where("session_id = ?", source.SessionID).
				Update("session_id", target.SessionID).Error; err != nil {
				return err
			}
		}

		startTime := target.StartTime
		if source.StartTime.Before(startTime) {
			startTime = source.StartTime
		}
		endTime := target.EndTime
		if source.EndTime.After(endTime) {
			endTime = source.EndTime
		}
		if err := tx.Model(&models.RecordHistory{}).Where("id = ?", target.ID).
			Updates(map[string]interface{}{
				"start_time": startTime,
				"end_time":   endTime,
				"message":    "同步完成：已按同日同标题合并为同一场直播",
			}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.RecordHistory{}, source.ID).Error; err != nil {
			return err
		}
		return nil
	})
	return movedParts, err
}

func normalizeHistorySyncTitle(title string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
}
