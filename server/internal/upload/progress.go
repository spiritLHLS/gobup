package upload

import (
	"sync"
	"time"
)

// ProgressState 上传进度状态
type ProgressState string

const (
	StateUploading ProgressState = "UPLOADING"
	StateRetryWait ProgressState = "RETRY_WAIT"
	StateSuccess   ProgressState = "SUCCESS"
	StateFailed    ProgressState = "FAILED"
)

// Progress 上传进度
type Progress struct {
	PartID     int64         `json:"partId"`
	HistoryID  int64         `json:"historyId"`
	Page       int           `json:"page"`
	ChunkDone  int           `json:"chunkDone"`
	ChunkTotal int           `json:"chunkTotal"`
	Percent    int           `json:"percent"`
	State      ProgressState `json:"state"`
	StateMsg   string        `json:"stateMsg,omitempty"`
	UpdateAtMs int64         `json:"updateAtMs"`
}

// IsActive 是否正在活跃上传
func (p *Progress) IsActive() bool {
	return p.State == StateUploading || p.State == StateRetryWait
}

// ProgressTracker 上传进度追踪器
type ProgressTracker struct {
	mu        sync.RWMutex
	byPartID  map[int64]*Progress
	expireDur time.Duration
}

// NewProgressTracker 创建进度追踪器
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		byPartID:  make(map[int64]*Progress),
		expireDur: 10 * time.Minute,
	}
}

// Start 开始上传任务
func (t *ProgressTracker) Start(partID, historyID int64, page, chunkTotal int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UnixMilli()
	p := t.byPartID[partID]
	if p == nil {
		p = &Progress{
			PartID:    partID,
			HistoryID: historyID,
		}
	}

	p.Page = page
	p.ChunkTotal = max(chunkTotal, 0)
	if p.ChunkDone < 0 {
		p.ChunkDone = 0
	}
	p.Percent = calcPercent(p.ChunkDone, p.ChunkTotal)
	p.State = StateUploading
	p.StateMsg = ""
	p.UpdateAtMs = now

	t.byPartID[partID] = p
	t.cleanupExpired(now)
}

// UpdateChunkDone 更新已完成的块数
func (t *ProgressTracker) UpdateChunkDone(partID, historyID int64, page, chunkDone, chunkTotal int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UnixMilli()
	p := t.byPartID[partID]
	if p == nil {
		p = &Progress{
			PartID:    partID,
			HistoryID: historyID,
		}
	}

	p.Page = page
	p.ChunkDone = max(chunkDone, 0)
	p.ChunkTotal = max(chunkTotal, 0)
	p.Percent = calcPercent(p.ChunkDone, p.ChunkTotal)
	p.State = StateUploading
	p.StateMsg = ""
	p.UpdateAtMs = now

	t.byPartID[partID] = p
	t.cleanupExpired(now)
}

// MarkRetryWait 标记为等待重试
func (t *ProgressTracker) MarkRetryWait(partID int64, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if p, ok := t.byPartID[partID]; ok {
		p.State = StateRetryWait
		p.StateMsg = msg
		p.UpdateAtMs = time.Now().UnixMilli()
	}
}

// MarkFailed 标记为失败
func (t *ProgressTracker) MarkFailed(partID int64, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if p, ok := t.byPartID[partID]; ok {
		p.State = StateFailed
		p.StateMsg = msg
		p.UpdateAtMs = time.Now().UnixMilli()
	}
}

// MarkSuccessAndRemove 标记为成功并移除
func (t *ProgressTracker) MarkSuccessAndRemove(partID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if p, ok := t.byPartID[partID]; ok {
		p.State = StateSuccess
		p.Percent = 100
		p.UpdateAtMs = time.Now().UnixMilli()
	}
	// 成功后延迟1秒删除，让前端有时间看到成功状态
	go func() {
		time.Sleep(1 * time.Second)
		t.Remove(partID)
	}()
}

// Remove 移除进度
func (t *ProgressTracker) Remove(partID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byPartID, partID)
}

// GetByPartID 根据分P ID获取进度
func (t *ProgressTracker) GetByPartID(partID int64) *Progress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if p, ok := t.byPartID[partID]; ok {
		return copyProgress(p)
	}
	return nil
}

// ListByHistoryID 获取历史记录的所有分P进度
func (t *ProgressTracker) ListByHistoryID(historyID int64) []*Progress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*Progress
	for _, p := range t.byPartID {
		if p.HistoryID == historyID {
			result = append(result, copyProgress(p))
		}
	}
	return result
}

// SnapshotAll 获取所有进度快照
func (t *ProgressTracker) SnapshotAll() map[int64]*Progress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[int64]*Progress, len(t.byPartID))
	for k, v := range t.byPartID {
		result[k] = copyProgress(v)
	}
	return result
}

// 计算百分比
func calcPercent(done, total int) int {
	if total <= 0 {
		return 0
	}
	d := min(max(done, 0), total)
	return int(float64(d) * 100.0 / float64(total))
}

// 清理过期的进度
func (t *ProgressTracker) cleanupExpired(nowMs int64) {
	expireBefore := nowMs - t.expireDur.Milliseconds()
	for partID, p := range t.byPartID {
		if p.UpdateAtMs < expireBefore {
			delete(t.byPartID, partID)
		}
	}
}

// 复制进度对象
func copyProgress(src *Progress) *Progress {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
