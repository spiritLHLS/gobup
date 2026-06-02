package upload

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

func (s *Service) selectUploadUserID(room *models.RecordRoom) (uint, error) {
	strategy := strings.TrimSpace(room.UploadUserStrategy)
	if strategy == "" {
		strategy = "fixed"
	}

	if strategy == "fixed" {
		if room.UploadUserID == 0 {
			return 0, fmt.Errorf("房间未配置上传用户")
		}
		return room.UploadUserID, nil
	}

	db := database.GetDB()
	var users []models.BiliBiliUser
	if err := db.Where("login = ? AND uid != ?", true, -1).Order("id ASC").Find(&users).Error; err != nil {
		return 0, fmt.Errorf("查询可用上传用户失败: %w", err)
	}
	if len(users) == 0 {
		if room.UploadUserID != 0 {
			return room.UploadUserID, nil
		}
		return 0, fmt.Errorf("没有可用的已登录上传用户")
	}

	switch strategy {
	case "round_robin":
		next := atomic.AddUint64(&s.roundRobinCursor, 1)
		return users[int((next-1)%uint64(len(users)))].ID, nil
	case "least_queue":
		selected := users[0].ID
		minLen := s.queueManager.GetQueueLength(selected)
		for _, user := range users[1:] {
			queueLen := s.queueManager.GetQueueLength(user.ID)
			if queueLen < minLen {
				selected = user.ID
				minLen = queueLen
			}
		}
		return selected, nil
	default:
		if room.UploadUserID == 0 {
			return 0, fmt.Errorf("未知上传账号策略 %q，且未配置固定上传用户", strategy)
		}
		return room.UploadUserID, nil
	}
}

func roomHasUploadUserOrStrategy(room *models.RecordRoom) bool {
	if room == nil {
		return false
	}
	if room.UploadUserID != 0 {
		return true
	}
	switch strings.TrimSpace(room.UploadUserStrategy) {
	case "round_robin", "least_queue":
		return true
	default:
		return false
	}
}

func isWithinUploadWindow(room *models.RecordRoom, now time.Time) (bool, string) {
	if room == nil || !room.UploadWindowEnabled {
		return true, ""
	}

	start, startText := parseUploadClock(room.UploadWindowStart, "00:00")
	end, endText := parseUploadClock(room.UploadWindowEnd, "23:59")
	window := startText + "-" + endText

	if start == end {
		return true, window
	}

	current := now.Hour()*60 + now.Minute()
	if start < end {
		return current >= start && current <= end, window
	}
	return current >= start || current <= end, window
}

func parseUploadClock(value, fallback string) (int, string) {
	text := strings.TrimSpace(value)
	if text == "" {
		text = fallback
	}

	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		text = fallback
		parts = strings.Split(text, ":")
	}

	hour, errHour := strconv.Atoi(parts[0])
	minute, errMinute := strconv.Atoi(parts[1])
	if errHour != nil || errMinute != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		text = fallback
		parts = strings.Split(text, ":")
		hour, _ = strconv.Atoi(parts[0])
		minute, _ = strconv.Atoi(parts[1])
	}

	return hour*60 + minute, fmt.Sprintf("%02d:%02d", hour, minute)
}

// containsTag 检查标签列表中是否包含指定标签
func containsTag(tags, target string) bool {
	tagList := strings.Split(tags, ",")
	for _, tag := range tagList {
		normalized := strings.TrimSpace(tag)
		if normalized == target || (target == "分P上传" && normalized == "上传") {
			return true
		}
	}
	return false
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
