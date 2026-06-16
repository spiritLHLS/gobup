package services

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

func EnsureSessionGuideComment(db *gorm.DB, client *bili.BiliClient, history *models.RecordHistory, aid int64, bvid string) {
	if db == nil || client == nil || history == nil || aid <= 0 || bvid == "" {
		return
	}
	if history.GuideCommentRPID > 0 && history.GuideCommentPinned {
		return
	}

	rpid := history.GuideCommentRPID
	if rpid <= 0 {
		message := buildSessionGuideComment(db, history, bvid)
		var err error
		rpid, err = client.AddVideoComment(aid, bvid, message)
		if err != nil {
			log.Printf("[场次索引评论] 发送失败 history_id=%d, bvid=%s: %v", history.ID, bvid, err)
			return
		}

		history.GuideCommentRPID = rpid
		if err := db.Model(history).Updates(map[string]interface{}{
			"guide_comment_rpid":   rpid,
			"guide_comment_pinned": false,
		}).Error; err != nil {
			log.Printf("[场次索引评论] 保存RPID失败 history_id=%d, rpid=%d: %v", history.ID, rpid, err)
		}
	}
	if err := client.PinVideoComment(aid, bvid, rpid); err != nil {
		log.Printf("[场次索引评论] 置顶失败 history_id=%d, rpid=%d: %v", history.ID, rpid, err)
		return
	}
	history.GuideCommentPinned = true
	if err := db.Model(history).Update("guide_comment_pinned", true).Error; err != nil {
		log.Printf("[场次索引评论] 保存置顶状态失败 history_id=%d, rpid=%d: %v", history.ID, rpid, err)
	}
	log.Printf("[场次索引评论] 已发送并置顶 history_id=%d, bvid=%s, rpid=%d", history.ID, bvid, rpid)
}

func buildSessionGuideComment(db *gorm.DB, current *models.RecordHistory, currentBvid string) string {
	lines := []string{"本场直播投稿索引："}

	var histories []models.RecordHistory
	query := db.Where("publish = ? AND bv_id != ''", true)
	if current.SessionID != "" {
		query = query.Where("session_id = ?", current.SessionID)
	} else {
		query = query.Where("id = ?", current.ID)
	}
	if err := query.Order("start_time ASC, id ASC").Find(&histories).Error; err != nil || len(histories) == 0 {
		histories = []models.RecordHistory{*current}
	}

	seen := make(map[string]bool)
	index := 1
	for _, h := range histories {
		if h.BvID == "" || seen[h.BvID] {
			continue
		}
		seen[h.BvID] = true
		marker := ""
		if h.BvID == currentBvid {
			marker = "（当前）"
		}
		lines = append(lines, fmt.Sprintf("%d. %s%s %s", index, h.BvID, marker, trimCommentText(h.Title, 42)))
		index++
	}

	if current.SessionID != "" {
		appendSpecialLiveMessages(db, current.SessionID, &lines)
	}
	lines = append(lines, fmt.Sprintf("自动生成于 %s", time.Now().Format("2006-01-02 15:04")))
	return trimCommentText(strings.Join(lines, "\n"), 900)
}

func appendSpecialLiveMessages(db *gorm.DB, sessionID string, lines *[]string) {
	var messages []models.LiveMsg
	if err := db.Where("session_id = ? AND type IN ?", sessionID, []int{2, 3, 4}).
		Order("timestamp ASC, id ASC").
		Limit(12).
		Find(&messages).Error; err != nil || len(messages) == 0 {
		return
	}

	*lines = append(*lines, "", "SC/礼物/上舰摘要：")
	for _, msg := range messages {
		name := trimCommentText(msg.UserName, 16)
		if name == "" {
			name = "观众"
		}
		*lines = append(*lines, fmt.Sprintf("- %s：%s", name, trimCommentText(msg.Message, 48)))
	}
}

func trimCommentText(text string, limit int) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit-1]) + "…"
}
