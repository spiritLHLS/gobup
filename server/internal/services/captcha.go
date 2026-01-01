package services

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// CaptchaService 验证码处理服务
type CaptchaService struct {
	retryQueue    chan CaptchaRetryTask
	maxRetries    int
	currentStatus *CaptchaStatus
	statusMu      sync.RWMutex
	waitChan      chan map[string]string
}

type CaptchaRetryTask struct {
	HistoryID  uint
	UserID     uint
	RetryCount int
	LastError  string
}

// CaptchaStatus 当前验证码状态
type CaptchaStatus struct {
	Required  bool                   `json:"required"`
	Voucher   string                 `json:"voucher"`
	Filename  string                 `json:"filename"`
	Extra     map[string]interface{} `json:"extra"`
	Timestamp int64                  `json:"timestamp"`
}

var globalCaptchaService *CaptchaService
var captchaServiceOnce sync.Once

// GetCaptchaService 获取全局验证码服务实例
func GetCaptchaService() *CaptchaService {
	captchaServiceOnce.Do(func() {
		globalCaptchaService = NewCaptchaService()
	})
	return globalCaptchaService
}

func NewCaptchaService() *CaptchaService {
	service := &CaptchaService{
		retryQueue: make(chan CaptchaRetryTask, 100),
		maxRetries: 3,
		currentStatus: &CaptchaStatus{
			Required: false,
		},
		waitChan: make(chan map[string]string, 1),
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

// SetCaptchaRequired 设置需要验证码（参考biliupforjava的CaptchaService）
func (s *CaptchaService) SetCaptchaRequired(voucher, filename string, extra map[string]interface{}) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	s.currentStatus = &CaptchaStatus{
		Required:  true,
		Voucher:   voucher,
		Filename:  filename,
		Extra:     extra,
		Timestamp: time.Now().Unix(),
	}

	log.Printf("[CAPTCHA] 验证码已设置: voucher=%s, filename=%s", voucher, filename)
}

// GetCaptchaStatus 获取当前验证码状态
func (s *CaptchaService) GetCaptchaStatus() *CaptchaStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()

	// 复制状态
	status := &CaptchaStatus{
		Required:  s.currentStatus.Required,
		Voucher:   s.currentStatus.Voucher,
		Filename:  s.currentStatus.Filename,
		Timestamp: s.currentStatus.Timestamp,
	}

	if s.currentStatus.Extra != nil {
		status.Extra = make(map[string]interface{})
		for k, v := range s.currentStatus.Extra {
			status.Extra[k] = v
		}
	}

	return status
}

// SubmitCaptchaResult 提交验证码结果（参考biliupforjava）
func (s *CaptchaService) SubmitCaptchaResult(result map[string]string) error {
	s.statusMu.Lock()
	if !s.currentStatus.Required {
		s.statusMu.Unlock()
		return fmt.Errorf("当前不需要验证码")
	}

	// 重置状态
	s.currentStatus.Required = false
	s.statusMu.Unlock()

	log.Printf("[CAPTCHA] 验证码结果已提交: %+v", result)

	// 发送到等待通道
	select {
	case s.waitChan <- result:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("发送验证码结果超时")
	}
}

// WaitForCaptcha 等待验证码输入（参考biliupforjava，带超时）
func (s *CaptchaService) WaitForCaptcha() map[string]string {
	// 等待最多10分钟
	timeout := time.After(10 * time.Minute)

	select {
	case result := <-s.waitChan:
		log.Printf("[CAPTCHA] 收到验证码结果: %+v", result)
		return result
	case <-timeout:
		log.Printf("[CAPTCHA] 等待验证码超时")
		// 清除状态
		s.statusMu.Lock()
		s.currentStatus.Required = false
		s.statusMu.Unlock()
		return nil
	}
}

// ClearCaptchaStatus 清除验证码状态
func (s *CaptchaService) ClearCaptchaStatus() {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.currentStatus.Required = false
	log.Printf("[CAPTCHA] 验证码状态已清除")
}
