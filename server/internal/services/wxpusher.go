package services

import (
	"fmt"
	"log"
	"time"

	"github.com/imroc/req/v3"
)

const WxPusherAPIURL = "https://wxpusher.zjiecode.com/api/send/message"

// WxPusherService WxPusher推送服务
type WxPusherService struct {
	AppToken string
	Enabled  bool
}

// NewWxPusherService 创建WxPusher服务
func NewWxPusherService(appToken string) *WxPusherService {
	return &WxPusherService{
		AppToken: appToken,
		Enabled:  appToken != "",
	}
}

// PushMessage 发送消息
type PushMessage struct {
	AppToken    string   `json:"appToken"`
	Content     string   `json:"content"`
	Summary     string   `json:"summary,omitempty"`
	ContentType int      `json:"contentType"` // 1:文本 2:HTML 3:Markdown
	UIDs        []string `json:"uids,omitempty"`
	TopicIDs    []int    `json:"topicIds,omitempty"`
	URL         string   `json:"url,omitempty"`
}

// SendTextMessage 发送文本消息
func (s *WxPusherService) SendTextMessage(uid, content string) error {
	if !s.Enabled {
		log.Printf("WxPusher未配置，跳过推送")
		return nil
	}

	msg := PushMessage{
		AppToken:    s.AppToken,
		Content:     content,
		ContentType: 1, // 文本
		UIDs:        []string{uid},
	}

	return s.send(msg)
}

// SendMarkdownMessage 发送Markdown消息
func (s *WxPusherService) SendMarkdownMessage(uid, content, summary string) error {
	if !s.Enabled {
		return nil
	}

	msg := PushMessage{
		AppToken:    s.AppToken,
		Content:     content,
		Summary:     summary,
		ContentType: 3, // Markdown
		UIDs:        []string{uid},
	}

	return s.send(msg)
}

func (s *WxPusherService) send(msg PushMessage) error {
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	client := req.C().ImpersonateChrome()
	_, err := client.R().
		SetBody(msg).
		SetSuccessResult(&result).
		Post(WxPusherAPIURL)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}

	if result.Code != 1000 {
		return fmt.Errorf("推送失败: %s", result.Msg)
	}

	log.Printf("WxPusher推送成功")
	return nil
}

// NotifyUploadStart 上传开始通知
func (s *WxPusherService) NotifyUploadStart(uid, roomName, fileName string, fileSize int64) {
	content := fmt.Sprintf(`📤 上传开始
房间: %s
文件: %s
大小: %.2f GB
时间: %s`,
		roomName, fileName, float64(fileSize)/1024/1024/1024,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(uid, content)
}

// NotifyUploadSuccess 上传成功通知
func (s *WxPusherService) NotifyUploadSuccess(uid, roomName, fileName string) {
	content := fmt.Sprintf(`✅ 上传成功
房间: %s
文件: %s
时间: %s`,
		roomName, fileName,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(uid, content)
}

// NotifyUploadFailed 上传失败通知
func (s *WxPusherService) NotifyUploadFailed(uid, roomName, fileName, reason string) {
	content := fmt.Sprintf(`❌ 上传失败
房间: %s
文件: %s
原因: %s
时间: %s`,
		roomName, fileName, reason,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(uid, content)
}

// NotifyPublishSuccess 投稿成功通知
func (s *WxPusherService) NotifyPublishSuccess(uid, roomName, title, bvid string) {
	content := fmt.Sprintf(`🎉 投稿成功
房间: %s
标题: %s
BV号: %s
链接: https://www.bilibili.com/video/%s
时间: %s`,
		roomName, title, bvid, bvid,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(uid, content)
}

// NotifyLiveStart 开播通知
func (s *WxPusherService) NotifyLiveStart(uid, uname, title, areaName string) {
	content := fmt.Sprintf(`🔴 开始直播
主播: %s
标题: %s
分区: %s
时间: %s`,
		uname, title, areaName,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(uid, content)
}
