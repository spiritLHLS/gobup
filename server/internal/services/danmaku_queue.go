package services

import (
	"fmt"
	"log"
	"sync"
)

// DanmakuTask 弹幕发送任务
type DanmakuTask struct {
	HistoryID uint
	UserID    uint
}

// UserDanmakuQueue 用户弹幕发送队列
type UserDanmakuQueue struct {
	userID     uint
	tasks      chan *DanmakuTask
	processing bool
	mu         sync.Mutex
	service    *DanmakuService
}

// NewUserDanmakuQueue 创建用户弹幕发送队列
func NewUserDanmakuQueue(userID uint, service *DanmakuService) *UserDanmakuQueue {
	return &UserDanmakuQueue{
		userID:  userID,
		tasks:   make(chan *DanmakuTask, 50), // 缓存最多50个弹幕发送任务
		service: service,
	}
}

// Add 添加弹幕发送任务到队列
func (q *UserDanmakuQueue) Add(task *DanmakuTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	select {
	case q.tasks <- task:
		log.Printf("[弹幕队列] ➕ 添加任务到用户%d的队列: history_id=%d (队列长度: %d)",
			q.userID, task.HistoryID, len(q.tasks))

		// 如果没有正在处理，启动处理
		if !q.processing {
			q.processing = true
			go q.process()
		}
		return nil
	default:
		return fmt.Errorf("用户%d的弹幕发送队列已满，无法添加新任务", q.userID)
	}
}

// process 处理队列中的任务
func (q *UserDanmakuQueue) process() {
	defer func() {
		q.mu.Lock()
		q.processing = false
		q.mu.Unlock()
		log.Printf("[弹幕队列] 🏁 用户%d的队列处理完毕", q.userID)
	}()

	for task := range q.tasks {
		log.Printf("[弹幕队列] 🎬 开始处理用户%d的弹幕发送任务: history_id=%d (剩余队列: %d)",
			q.userID, task.HistoryID, len(q.tasks))

		// 执行弹幕发送
		if err := q.service.sendDanmakuForHistoryInternal(task.HistoryID, task.UserID); err != nil {
			log.Printf("[弹幕队列] ❌ 用户%d的弹幕发送任务失败: history_id=%d, error=%v",
				q.userID, task.HistoryID, err)
		} else {
			log.Printf("[弹幕队列] ✅ 用户%d的弹幕发送任务成功: history_id=%d",
				q.userID, task.HistoryID)
		}

		// 队列为空时退出
		if len(q.tasks) == 0 {
			log.Printf("[弹幕队列] ℹ️  用户%d的队列已空，准备退出处理循环", q.userID)
			break
		}
	}
}

// DanmakuQueueManager 弹幕队列管理器
type DanmakuQueueManager struct {
	queues  sync.Map // userID -> *UserDanmakuQueue
	service *DanmakuService
}

// NewDanmakuQueueManager 创建弹幕队列管理器
func NewDanmakuQueueManager(service *DanmakuService) *DanmakuQueueManager {
	return &DanmakuQueueManager{
		service: service,
	}
}

// GetQueue 获取或创建用户的弹幕发送队列
func (m *DanmakuQueueManager) GetQueue(userID uint) *UserDanmakuQueue {
	if queue, ok := m.queues.Load(userID); ok {
		return queue.(*UserDanmakuQueue)
	}

	// 创建新队列
	queue := NewUserDanmakuQueue(userID, m.service)
	actual, loaded := m.queues.LoadOrStore(userID, queue)
	if loaded {
		return actual.(*UserDanmakuQueue)
	}
	return queue
}

// AddTask 添加弹幕发送任务
func (m *DanmakuQueueManager) AddTask(userID uint, historyID uint) error {
	queue := m.GetQueue(userID)
	return queue.Add(&DanmakuTask{
		HistoryID: historyID,
		UserID:    userID,
	})
}

// GetQueueLength 获取指定用户的队列长度
func (m *DanmakuQueueManager) GetQueueLength(userID uint) int {
	if queue, ok := m.queues.Load(userID); ok {
		return len(queue.(*UserDanmakuQueue).tasks)
	}
	return 0
}

// GetAllQueuesStatus 获取所有队列的状态
func (m *DanmakuQueueManager) GetAllQueuesStatus() map[uint]int {
	status := make(map[uint]int)
	m.queues.Range(func(key, value interface{}) bool {
		userID := key.(uint)
		queue := value.(*UserDanmakuQueue)
		status[userID] = len(queue.tasks)
		return true
	})
	return status
}

// IsProcessing 检查用户是否有正在处理的弹幕任务
func (m *DanmakuQueueManager) IsProcessing(userID uint) bool {
	if queue, ok := m.queues.Load(userID); ok {
		q := queue.(*UserDanmakuQueue)
		q.mu.Lock()
		defer q.mu.Unlock()
		return q.processing
	}
	return false
}
