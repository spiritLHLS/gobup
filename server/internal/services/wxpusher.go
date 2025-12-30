package services

import (
	"fmt"
	"log"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/imroc/req/v3"
)

const WxPusherAPIURL = "https://wxpusher.zjiecode.com/api/send/message"

// WxPusherService WxPusher推送服务
type WxPusherService struct{}

// NewWxPusherService 创建WxPusher服务
func NewWxPusherService() *WxPusherService {
	return &WxPusherService{}
}

// getUserToken 获取用户的WxPushToken
func (s *WxPusherService) getUserToken(userID uint) (string, error) {
	db := database.GetDB()
	var user models.BiliBiliUser

	if err := db.First(&user, userID).Error; err != nil {
		return "", fmt.Errorf("获取用户信息失败: %w", err)
	}

	return user.WxPushToken, nil
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
func (s *WxPusherService) SendTextMessage(userID uint, wxuid, content string) error {
	appToken, err := s.getUserToken(userID)
	if err != nil {
		log.Printf("获取用户Token失败: %v", err)
		return err
	}

	if appToken == "" {
		log.Printf("用户%d未配置WxPusher token，跳过推送", userID)
		return nil
	}

	msg := PushMessage{
		AppToken:    appToken,
		Content:     content,
		ContentType: 1, // 文本
		UIDs:        []string{wxuid},
	}

	return s.send(msg)
}

// SendMarkdownMessage 发送Markdown消息
func (s *WxPusherService) SendMarkdownMessage(userID uint, wxuid, content, summary string) error {
	appToken, err := s.getUserToken(userID)
	if err != nil {
		return err
	}

	if appToken == "" {
		return nil
	}

	msg := PushMessage{
		AppToken:    appToken,
		Content:     content,
		Summary:     summary,
		ContentType: 3, // Markdown
		UIDs:        []string{wxuid},
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
func (s *WxPusherService) NotifyUploadStart(userID uint, wxuid, roomName, fileName string, fileSize int64) {
	content := fmt.Sprintf(`📤 上传开始
房间: %s
文件: %s
大小: %.2f GB
时间: %s`,
		roomName, fileName, float64(fileSize)/1024/1024/1024,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(userID, wxuid, content)
}

// NotifyUploadSuccess 上传成功通知
func (s *WxPusherService) NotifyUploadSuccess(userID uint, wxuid, roomName, fileName string) {
	content := fmt.Sprintf(`✅ 上传成功
房间: %s
文件: %s
时间: %s`,
		roomName, fileName,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(userID, wxuid, content)
}

// NotifyUploadFailed 上传失败通知
func (s *WxPusherService) NotifyUploadFailed(userID uint, wxuid, roomName, fileName, reason string) {
	content := fmt.Sprintf(`❌ 上传失败
房间: %s
文件: %s
原因: %s
时间: %s`,
		roomName, fileName, reason,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(userID, wxuid, content)
}

// NotifyPublishSuccess 投稿成功通知
func (s *WxPusherService) NotifyPublishSuccess(userID uint, wxuid, roomName, title, bvid string) {
	content := fmt.Sprintf(`🎉 投稿成功
房间: %s
标题: %s
BV号: %s
链接: https://www.bilibili.com/video/%s
时间: %s`,
		roomName, title, bvid, bvid,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(userID, wxuid, content)
}

// NotifyLiveStart 开播通知
func (s *WxPusherService) NotifyLiveStart(userID uint, wxuid, uname, title, areaName string) {
	content := fmt.Sprintf(`🔴 开始直播
主播: %s
标题: %s
分区: %s
时间: %s`,
		uname, title, areaName,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendTextMessage(userID, wxuid, content)
}
