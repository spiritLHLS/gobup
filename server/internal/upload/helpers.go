package upload

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

// UploadWindowClosedError 表示当前不在上传时间窗口内，队列可按 RetryAfter 延迟重试。
type UploadWindowClosedError struct {
	Window     string
	RetryAfter time.Duration
}

func (e *UploadWindowClosedError) Error() string {
	return fmt.Sprintf("当前不在上传时间窗口(%s)，将在 %s 后重试", e.Window, formatDurationForLog(e.RetryAfter))
}

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
	if err := db.Where("login = ? AND enabled = ? AND uid != ?", true, true, -1).Order("id ASC").Find(&users).Error; err != nil {
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
	case "daily_quota":
		selected, ok := s.selectDailyQuotaUser(users, time.Now())
		if !ok {
			return 0, fmt.Errorf("所有可用上传账号的每日配额已用尽")
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
	case "round_robin", "least_queue", "daily_quota":
		return true
	default:
		return false
	}
}

type dailyQuotaCandidate struct {
	ID       uint
	Quota    int
	Used     int
	QueueLen int
}

func (s *Service) selectDailyQuotaUser(users []models.BiliBiliUser, now time.Time) (uint, bool) {
	db := database.GetDB()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	candidates := make([]dailyQuotaCandidate, 0, len(users))

	for _, user := range users {
		var used int64
		db.Model(&models.RecordHistoryPart{}).
			Where("upload_user_id = ? AND upload = ? AND uploaded_at >= ?", user.ID, true, dayStart).
			Count(&used)

		candidates = append(candidates, dailyQuotaCandidate{
			ID:       user.ID,
			Quota:    user.DailyUploadQuota,
			Used:     int(used),
			QueueLen: s.queueManager.GetQueueLength(user.ID),
		})
	}

	return chooseDailyQuotaUser(candidates)
}

func chooseDailyQuotaUser(candidates []dailyQuotaCandidate) (uint, bool) {
	var selected dailyQuotaCandidate
	hasSelected := false
	selectedUnlimited := false
	selectedRemaining := 0

	for _, candidate := range candidates {
		unlimited := candidate.Quota <= 0
		remaining := 0
		if !unlimited {
			remaining = candidate.Quota - candidate.Used - candidate.QueueLen
			if remaining <= 0 {
				continue
			}
		}

		if !hasSelected {
			selected = candidate
			selectedUnlimited = unlimited
			selectedRemaining = remaining
			hasSelected = true
			continue
		}

		switch {
		case unlimited && !selectedUnlimited:
			selected = candidate
			selectedUnlimited = true
			selectedRemaining = remaining
		case unlimited && selectedUnlimited:
			if candidate.QueueLen < selected.QueueLen || (candidate.QueueLen == selected.QueueLen && candidate.ID < selected.ID) {
				selected = candidate
			}
		case !unlimited && !selectedUnlimited:
			if remaining > selectedRemaining ||
				(remaining == selectedRemaining && candidate.QueueLen < selected.QueueLen) ||
				(remaining == selectedRemaining && candidate.QueueLen == selected.QueueLen && candidate.ID < selected.ID) {
				selected = candidate
				selectedRemaining = remaining
			}
		}
	}

	if !hasSelected {
		return 0, false
	}
	return selected.ID, true
}

func validateUploadVideoPath(filePath string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".flv", ".mp4", ".m4v", ".mov", ".mkv", ".ts", ".webm":
		return nil
	default:
		return fmt.Errorf("不支持的上传文件类型 %q，仅允许常见视频文件", ext)
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

func newUploadWindowClosedError(room *models.RecordRoom, now time.Time) *UploadWindowClosedError {
	ok, window := isWithinUploadWindow(room, now)
	if ok {
		return nil
	}
	return &UploadWindowClosedError{
		Window:     window,
		RetryAfter: timeUntilNextUploadWindow(room, now),
	}
}

func timeUntilNextUploadWindow(room *models.RecordRoom, now time.Time) time.Duration {
	if room == nil || !room.UploadWindowEnabled {
		return 0
	}

	start, _ := parseUploadClock(room.UploadWindowStart, "00:00")
	end, _ := parseUploadClock(room.UploadWindowEnd, "23:59")
	if start == end {
		return 0
	}

	current := now.Hour()*60 + now.Minute()
	targetDay := now
	if start < end && current > end {
		targetDay = now.Add(24 * time.Hour)
	}
	target := time.Date(
		targetDay.Year(), targetDay.Month(), targetDay.Day(),
		start/60, start%60, 0, 0, now.Location(),
	)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}

func formatDurationForLog(duration time.Duration) string {
	if duration <= 0 {
		return "0分钟"
	}
	minutes := int(duration.Round(time.Minute).Minutes())
	if minutes < 1 {
		minutes = 1
	}
	if minutes < 60 {
		return fmt.Sprintf("%d分钟", minutes)
	}
	return fmt.Sprintf("%d小时%d分钟", minutes/60, minutes%60)
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
