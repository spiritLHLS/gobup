package services

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// CaptchaService 验证码处理服务
type CaptchaService struct {
	retryQueue chan CaptchaRetryTask
	maxRetries int
}

type CaptchaRetryTask struct {
	HistoryID  uint
	UserID     uint
	RetryCount int
	LastError  string
}

func NewCaptchaService() *CaptchaService {
	service := &CaptchaService{
		retryQueue: make(chan CaptchaRetryTask, 100),
		maxRetries: 3,
	}
	go service.processRetryQueue()
	return service
}

// HandleCaptchaError 处理验证码错误
func (s *CaptchaService) HandleCaptchaError(historyID, userID uint, errorMsg string) error {
	// 检查是否是验证码错误
	if !s.isCaptchaError(errorMsg) {
		return fmt.Errorf("非验证码错误: %s", errorMsg)
	}

	log.Printf("检测到验证码错误，历史记录ID: %d", historyID)

	// 添加到重试队列
	task := CaptchaRetryTask{
		HistoryID:  historyID,
		UserID:     userID,
		RetryCount: 0,
		LastError:  errorMsg,
	}

	select {
	case s.retryQueue <- task:
		log.Printf("验证码重试任务已加入队列")
		return nil
	default:
		return fmt.Errorf("重试队列已满")
	}
}

// IsCaptchaError 判断是否是验证码错误（公开方法）
func (s *CaptchaService) IsCaptchaError(errorMsg string) bool {
	return s.isCaptchaError(errorMsg)
}

// isCaptchaError 判断是否是验证码错误
func (s *CaptchaService) isCaptchaError(errorMsg string) bool {
	captchaKeywords := []string{
		"验证码",
		"captcha",
		"geetest",
		"请完成验证",
		"-105", // B站验证码错误码
		"风控",
	}

	lowerMsg := strings.ToLower(errorMsg)
	for _, keyword := range captchaKeywords {
		if strings.Contains(lowerMsg, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// processRetryQueue 处理重试队列
func (s *CaptchaService) processRetryQueue() {
	for task := range s.retryQueue {
		// 等待一段时间再重试（让用户有时间手动验证）
		waitTime := time.Duration(120+task.RetryCount*60) * time.Second
		log.Printf("验证码任务将在 %v 后重试 (第 %d 次)", waitTime, task.RetryCount+1)

		time.Sleep(waitTime)

		// 这里暂时不实际重试，只是记录日志
		// 实际重试需要调用upload service的PublishHistory方法
		// 但为了避免循环依赖，这里只做记录
		log.Printf("验证码重试时间到达，历史记录ID: %d", task.HistoryID)

		// 如果重试次数达到上限，不再重试
		if task.RetryCount >= s.maxRetries {
			log.Printf("验证码重试次数达到上限，放弃重试: 历史记录ID %d", task.HistoryID)
			continue
		}

		// 重新加入队列进行下一次重试
		task.RetryCount++
		select {
		case s.retryQueue <- task:
		default:
			log.Printf("重试队列已满，任务丢失: 历史记录ID %d", task.HistoryID)
		}
	}
}

// GetRetryQueueSize 获取重试队列大小
func (s *CaptchaService) GetRetryQueueSize() int {
	return len(s.retryQueue)
}
