package services

import (
	"fmt"
	"log"
	"sync"
)

// DanmakuParseTask 弹幕解析任务
type DanmakuParseTask struct {
	HistoryID uint
}

// DanmakuParserQueue 弹幕解析队列（全局单例，确保同一时间只处理一个）
type DanmakuParserQueue struct {
	tasks      chan *DanmakuParseTask
	processing bool
	mu         sync.Mutex
	parser     *DanmakuXMLParser
}

var (
	parserQueueInstance *DanmakuParserQueue
	parserQueueOnce     sync.Once
)

// NewDanmakuParserQueue 获取弹幕解析队列单例
func NewDanmakuParserQueue() *DanmakuParserQueue {
	parserQueueOnce.Do(func() {
		parserQueueInstance = &DanmakuParserQueue{
			tasks:  make(chan *DanmakuParseTask, 50), // 缓存最多50个解析任务
			parser: NewDanmakuXMLParser(),
		}
	})
	return parserQueueInstance
}

// Add 添加弹幕解析任务到队列
func (q *DanmakuParserQueue) Add(task *DanmakuParseTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	select {
	case q.tasks <- task:
		log.Printf("[弹幕解析队列] ➕ 添加任务: history_id=%d (队列长度: %d)",
			task.HistoryID, len(q.tasks))

		// 如果没有正在处理，启动处理
		if !q.processing {
			q.processing = true
			go q.process()
		}
		return nil
	default:
		return fmt.Errorf("弹幕解析队列已满，无法添加新任务")
	}
}

// process 处理队列中的任务
func (q *DanmakuParserQueue) process() {
	defer func() {
		q.mu.Lock()
		q.processing = false
		q.mu.Unlock()
		log.Printf("[弹幕解析队列] 🏁 队列处理完毕")
	}()

	for task := range q.tasks {
		log.Printf("[弹幕解析队列] 🎬 开始处理解析任务: history_id=%d (剩余队列: %d)",
			task.HistoryID, len(q.tasks))

		// 执行弹幕解析
		count, err := q.parser.ParseDanmakuForHistory(task.HistoryID)
		if err != nil {
			log.Printf("[弹幕解析队列] ❌ 解析任务失败: history_id=%d, error=%v",
				task.HistoryID, err)
		} else {
			log.Printf("[弹幕解析队列] ✅ 解析任务成功: history_id=%d, count=%d",
				task.HistoryID, count)
		}

		// 队列为空时退出
		if len(q.tasks) == 0 {
			log.Printf("[弹幕解析队列] ℹ️  队列已空，准备退出处理循环")
			break
		}
	}
}

// GetQueueLength 获取队列长度
func (q *DanmakuParserQueue) GetQueueLength() int {
	return len(q.tasks)
}

// IsProcessing 检查是否有正在处理的任务
func (q *DanmakuParserQueue) IsProcessing() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.processing
}
