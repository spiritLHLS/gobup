package upload

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gobup/server/internal/models"
)

// UploadTask 上传任务
type UploadTask struct {
	Part    *models.RecordHistoryPart
	History *models.RecordHistory
	Room    *models.RecordRoom
}

// UserUploadQueue 用户上传队列
type UserUploadQueue struct {
	userID     uint
	tasks      chan *UploadTask
	processing bool
	mu         sync.Mutex
	service    *Service
}

// NewUserUploadQueue 创建用户上传队列
func NewUserUploadQueue(userID uint, service *Service) *UserUploadQueue {
	return &UserUploadQueue{
		userID:  userID,
		tasks:   make(chan *UploadTask, 100), // 缓存最多100个上传任务
		service: service,
	}
}

// Add 添加上传任务到队列
func (q *UserUploadQueue) Add(task *UploadTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	select {
	case q.tasks <- task:
		log.Printf("[队列] 添加上传任务到用户%d的队列: part_id=%d, file=%s (队列长度: %d)",
			q.userID, task.Part.ID, task.Part.FileName, len(q.tasks))

		// 如果没有正在处理，启动处理
		if !q.processing {
			q.processing = true
			go q.process()
		}
		return nil
	default:
		return fmt.Errorf("用户%d的上传队列已满，无法添加新任务", q.userID)
	}
}

// process 处理队列中的任务
func (q *UserUploadQueue) process() {
	defer func() {
		//捕获 panic，重置 processing 标志，并在有剩余任务时自动重启 goroutine
		r := recover()
		q.mu.Lock()
		if r != nil {
			log.Printf("[队列] 用户%d的上传goroutine发生panic: %v，尝试重启", q.userID, r)
			if len(q.tasks) > 0 {
				// 还有待处理任务，1秒后重启
				go func() {
					time.Sleep(time.Second)
					q.process()
				}()
			} else {
				q.processing = false
			}
		} else {
			// 正常退出（channel 关闭），重置标志
			q.processing = false
		}
		q.mu.Unlock()
	}()

	for task := range q.tasks {
		log.Printf("[队列] 开始处理用户%d的上传任务: part_id=%d, file=%s (剩余队列: %d)",
			q.userID, task.Part.ID, task.Part.FileName, len(q.tasks))

		// 速率限制冷却期内直接丢弃任务，由10分钟调度器重新入队
		// 原来的做法是将任务放回队列尾部并立即 continue，导致忙等死循环（CPU 100%）
		if task.Part.RateLimitCooldownAt != nil && time.Now().Before(*task.Part.RateLimitCooldownAt) {
			remainingTime := time.Until(*task.Part.RateLimitCooldownAt)
			log.Printf("[队列] 用户%d的任务part_id=%d处于速率限制冷却期，剩余%.0f分钟，丢弃任务（调度器将在冷却期结束后重新入队）",
				q.userID, task.Part.ID, remainingTime.Minutes())
			continue
		}

		// 执行上传
		if err := q.service.uploadPartInternal(task.Part, task.History, task.Room); err != nil {
			log.Printf("[队列] 用户%d的上传任务失败: part_id=%d, error=%v",
				q.userID, task.Part.ID, err)
		} else {
			log.Printf("[队列] 用户%d的上传任务成功: part_id=%d",
				q.userID, task.Part.ID)
		}
	}
}

// QueueManager 队列管理器
type QueueManager struct {
	queues  sync.Map // userID -> *UserUploadQueue
	service *Service
}

// NewQueueManager 创建队列管理器
func NewQueueManager(service *Service) *QueueManager {
	return &QueueManager{
		service: service,
	}
}

// GetQueue 获取或创建用户的上传队列
func (m *QueueManager) GetQueue(userID uint) *UserUploadQueue {
	if queue, ok := m.queues.Load(userID); ok {
		return queue.(*UserUploadQueue)
	}

	// 创建新队列
	queue := NewUserUploadQueue(userID, m.service)
	actual, loaded := m.queues.LoadOrStore(userID, queue)
	if loaded {
		return actual.(*UserUploadQueue)
	}
	return queue
}

// AddTask 添加上传任务
func (m *QueueManager) AddTask(userID uint, part *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) error {
	queue := m.GetQueue(userID)
	return queue.Add(&UploadTask{
		Part:    part,
		History: history,
		Room:    room,
	})
}

// GetQueueLength 获取指定用户的队列长度
func (m *QueueManager) GetQueueLength(userID uint) int {
	if queue, ok := m.queues.Load(userID); ok {
		return len(queue.(*UserUploadQueue).tasks)
	}
	return 0
}

// GetAllQueuesStatus 获取所有队列的状态
func (m *QueueManager) GetAllQueuesStatus() map[uint]int {
	status := make(map[uint]int)
	m.queues.Range(func(key, value interface{}) bool {
		userID := key.(uint)
		queue := value.(*UserUploadQueue)
		status[userID] = len(queue.tasks)
		return true
	})
	return status
}
