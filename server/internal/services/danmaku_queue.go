package services

import (
	"fmt"
	"log"
	"sync"
)

// DanmakuTask 弹幕发送任务
type DanmakuTask struct {
	HistoryID uint
}

// DanmakuQueueManager 弹幕队列管理器（全局单视频处理）
type DanmakuQueueManager struct {
	tasks      chan *DanmakuTask
	processing bool
	mu         sync.Mutex
	service    *DanmakuService
}

// NewDanmakuQueueManager 创建弹幕队列管理器
func NewDanmakuQueueManager(service *DanmakuService) *DanmakuQueueManager {
	qm := &DanmakuQueueManager{
		tasks:   make(chan *DanmakuTask, 100), // 全局队列，最多缓存100个任务
		service: service,
	}
	// 启动全局队列处理器
	go qm.process()
	return qm
}

// AddTask 添加弹幕发送任务到全局队列
func (m *DanmakuQueueManager) AddTask(historyID uint) error {
	select {
	case m.tasks <- &DanmakuTask{HistoryID: historyID}:
		log.Printf("[弹幕队列] ➕ 添加任务到全局队列 (history_id=%d, 队列长度=%d)",
			historyID, len(m.tasks))
		return nil
	default:
		return fmt.Errorf("全局弹幕发送队列已满，无法添加新任务")
	}
}

// process 全局队列处理器（确保同一时间只处理一个视频）
func (m *DanmakuQueueManager) process() {
	log.Printf("[弹幕队列] 🚀 全局队列处理器已启动")

	for task := range m.tasks {
		m.mu.Lock()
		m.processing = true
		m.mu.Unlock()

		log.Printf("[弹幕队列] 🎬 开始处理视频的弹幕发送任务 (history_id=%d, 剩余队列=%d)",
			task.HistoryID, len(m.tasks))

		// 执行弹幕发送（用户串行发送）
		if err := m.service.sendDanmakuForHistoryWithSerialUsers(task.HistoryID); err != nil {
			log.Printf("[弹幕队列] ❌ 视频%d的弹幕发送任务失败: %v",
				task.HistoryID, err)
		} else {
			log.Printf("[弹幕队列] ✅ 视频%d的弹幕发送任务成功",
				task.HistoryID)
		}

		m.mu.Lock()
		m.processing = false
		m.mu.Unlock()
	}
}

// GetQueueLength 获取全局队列长度
func (m *DanmakuQueueManager) GetQueueLength(historyID uint) int {
	return len(m.tasks)
}

// IsProcessing 检查是否有任务正在处理
func (m *DanmakuQueueManager) IsProcessing(historyID uint) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processing
}

// GetAllQueuesStatus 获取全局队列状态
func (m *DanmakuQueueManager) GetAllQueuesStatus() map[uint]int {
	return map[uint]int{
		0: len(m.tasks), // 使用0表示全局队列
	}
}
