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

// VideoDanmakuQueue 视频弹幕发送队列（支持多用户并行发送）
type VideoDanmakuQueue struct {
	historyID  uint
	tasks      chan *DanmakuTask
	processing bool
	mu         sync.Mutex
	service    *DanmakuService
}

// NewVideoDanmakuQueue 创建视频弹幕发送队列
func NewVideoDanmakuQueue(historyID uint, service *DanmakuService) *VideoDanmakuQueue {
	return &VideoDanmakuQueue{
		historyID: historyID,
		tasks:     make(chan *DanmakuTask, 10), // 缓存最多10个弹幕发送任务
		service:   service,
	}
}

// Add 添加弹幕发送任务到队列
func (q *VideoDanmakuQueue) Add(task *DanmakuTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	select {
	case q.tasks <- task:
		log.Printf("[弹幕队列] ➕ 添加任务到视频%d的队列 (队列长度: %d)",
			q.historyID, len(q.tasks))

		// 如果没有正在处理，启动处理
		if !q.processing {
			q.processing = true
			go q.process()
		}
		return nil
	default:
		return fmt.Errorf("视频%d的弹幕发送队列已满，无法添加新任务", q.historyID)
	}
}

// process 处理队列中的任务
func (q *VideoDanmakuQueue) process() {
	defer func() {
		q.mu.Lock()
		q.processing = false
		q.mu.Unlock()
		log.Printf("[弹幕队列] 🏁 视频%d的队列处理完毕", q.historyID)
	}()

	for {
		select {
		case task := <-q.tasks:
			log.Printf("[弹幕队列] 🎬 开始处理视频%d的弹幕发送任务 (剩余队列: %d)",
				q.historyID, len(q.tasks))

			// 执行弹幕发送（使用多用户并行）
			if err := q.service.sendDanmakuForHistoryWithMultipleUsers(task.HistoryID); err != nil {
				log.Printf("[弹幕队列] ❌ 视频%d的弹幕发送任务失败: error=%v",
					q.historyID, err)
			} else {
				log.Printf("[弹幕队列] ✅ 视频%d的弹幕发送任务成功",
					q.historyID)
			}

			// 队列为空时退出
			if len(q.tasks) == 0 {
				log.Printf("[弹幕队列] ℹ️  视频%d的队列已空，准备退出处理循环", q.historyID)
				return
			}
		default:
			// 如果没有任务了，退出
			log.Printf("[弹幕队列] ℹ️  视频%d的队列已空，准备退出处理循环", q.historyID)
			return
		}
	}
}

// DanmakuQueueManager 弹幕队列管理器
type DanmakuQueueManager struct {
	queues  sync.Map // historyID -> *VideoDanmakuQueue
	service *DanmakuService
}

// NewDanmakuQueueManager 创建弹幕队列管理器
func NewDanmakuQueueManager(service *DanmakuService) *DanmakuQueueManager {
	return &DanmakuQueueManager{
		service: service,
	}
}

// GetQueue 获取或创建视频的弹幕发送队列
func (m *DanmakuQueueManager) GetQueue(historyID uint) *VideoDanmakuQueue {
	if queue, ok := m.queues.Load(historyID); ok {
		return queue.(*VideoDanmakuQueue)
	}

	// 创建新队列
	queue := NewVideoDanmakuQueue(historyID, m.service)
	actual, loaded := m.queues.LoadOrStore(historyID, queue)
	if loaded {
		return actual.(*VideoDanmakuQueue)
	}
	return queue
}

// AddTask 添加弹幕发送任务
func (m *DanmakuQueueManager) AddTask(historyID uint) error {
	queue := m.GetQueue(historyID)
	return queue.Add(&DanmakuTask{
		HistoryID: historyID,
	})
}

// GetQueueLength 获取指定视频的队列长度
func (m *DanmakuQueueManager) GetQueueLength(historyID uint) int {
	if queue, ok := m.queues.Load(historyID); ok {
		return len(queue.(*VideoDanmakuQueue).tasks)
	}
	return 0
}

// GetAllQueuesStatus 获取所有队列的状态
func (m *DanmakuQueueManager) GetAllQueuesStatus() map[uint]int {
	status := make(map[uint]int)
	m.queues.Range(func(key, value interface{}) bool {
		historyID := key.(uint)
		queue := value.(*VideoDanmakuQueue)
		status[historyID] = len(queue.tasks)
		return true
	})
	return status
}

// IsProcessing 检查视频是否有正在处理的弹幕任务
func (m *DanmakuQueueManager) IsProcessing(historyID uint) bool {
	if queue, ok := m.queues.Load(historyID); ok {
		q := queue.(*VideoDanmakuQueue)
		q.mu.Lock()
		defer q.mu.Unlock()
		return q.processing
	}
	return false
}
