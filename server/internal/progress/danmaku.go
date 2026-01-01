package progress

import (
	"sync"
)

// DanmakuProgress 弹幕发送进度
type DanmakuProgress struct {
	HistoryID int64 `json:"historyId"`
	Current   int   `json:"current"`
	Total     int   `json:"total"`
	Sending   bool  `json:"sending"`
	Completed bool  `json:"completed"`
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

	return progress
}

// SetDanmakuProgress 设置弹幕发送进度
func SetDanmakuProgress(historyID int64, current, total int, sending, completed bool) {
	danmakuProgressMu.Lock()
	defer danmakuProgressMu.Unlock()

	danmakuProgressMap[historyID] = &DanmakuProgress{
		HistoryID: historyID,
		Current:   current,
		Total:     total,
		Sending:   sending,
		Completed: completed,
	}
}

// ClearDanmakuProgress 清除弹幕发送进度
func ClearDanmakuProgress(historyID int64) {
	danmakuProgressMu.Lock()
	defer danmakuProgressMu.Unlock()

	delete(danmakuProgressMap, historyID)
}
