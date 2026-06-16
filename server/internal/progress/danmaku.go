package progress

import (
	"sync"
	"time"
)

// DanmakuProgress 弹幕发送进度
type DanmakuProgress struct {
	HistoryID  int64  `json:"historyId"`
	Current    int    `json:"current"`
	Total      int    `json:"total"`
	Sending    bool   `json:"sending"`
	Completed  bool   `json:"completed"`
	Failed     bool   `json:"failed,omitempty"`
	Stage      string `json:"stage,omitempty"`
	Message    string `json:"message,omitempty"`
	UpdateAtMs int64  `json:"updateAtMs,omitempty"`
}

var (
	danmakuProgressMap = make(map[int64]*DanmakuProgress)
	danmakuProgressMu  sync.RWMutex
)

// GetDanmakuProgress 获取弹幕发送进度
func GetDanmakuProgress(historyID int64) *DanmakuProgress {
	danmakuProgressMu.RLock()
	defer danmakuProgressMu.RUnlock()

	progress, exists := danmakuProgressMap[historyID]
	if !exists {
		return &DanmakuProgress{
			HistoryID: historyID,
			Current:   0,
			Total:     0,
			Sending:   false,
			Completed: false,
		}
	}

	copied := *progress
	return &copied
}

// SetDanmakuProgress 设置弹幕发送进度
func SetDanmakuProgress(historyID int64, current, total int, sending, completed bool) {
	SetDanmakuTaskProgress(historyID, current, total, sending, completed, false, "sending", "")
}

// SetDanmakuTaskProgress 设置弹幕任务进度，可用于发送、烧录等阶段。
func SetDanmakuTaskProgress(historyID int64, current, total int, sending, completed, failed bool, stage, message string) {
	danmakuProgressMu.Lock()
	defer danmakuProgressMu.Unlock()

	danmakuProgressMap[historyID] = &DanmakuProgress{
		HistoryID:  historyID,
		Current:    current,
		Total:      total,
		Sending:    sending,
		Completed:  completed,
		Failed:     failed,
		Stage:      stage,
		Message:    message,
		UpdateAtMs: nowMs(),
	}
}

// ClearDanmakuProgress 清除弹幕发送进度
func ClearDanmakuProgress(historyID int64) {
	danmakuProgressMu.Lock()
	defer danmakuProgressMu.Unlock()

	delete(danmakuProgressMap, historyID)
}

func nowMs() int64 {
	return timeNow().UnixMilli()
}

var timeNow = func() time.Time {
	return time.Now()
}
