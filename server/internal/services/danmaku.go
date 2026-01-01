package services

import (
	"fmt"
	"log"
	"strings"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	danmakuprogress "github.com/gobup/server/internal/progress"
)

type DanmakuService struct{}

func NewDanmakuService() *DanmakuService {
	return &DanmakuService{}
}

// SendDanmakuForHistory 为历史记录发送弹幕
func (s *DanmakuService) SendDanmakuForHistory(historyID uint, userID uint) error {
	db := database.GetDB()

	// 获取历史记录
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	if history.BvID == "" {
		return fmt.Errorf("视频尚未投稿")
	}

	// 检查BV号格式
	if !strings.HasPrefix(history.BvID, "BV") {
		return fmt.Errorf("无效的BV号格式")
	}

	if history.DanmakuSent {
		return fmt.Errorf("弹幕已发送，请勿重复操作")
	}

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间配置不存在: %w", err)
	}

	// 获取用户
	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}

	// 获取弹幕列表（应用过滤规则）
	var danmakus []models.LiveMsg
	query := db.Where("session_id = ? AND sent = ?", history.SessionID, false).
		Order("timestamp ASC")

	// 应用弹幕过滤规则
	if room.DmUlLevel > 0 {
		// 用户等级过滤（佩戴勋章的不受影响）
		query = query.Where("ulevel >= ? OR medal_level > 0", room.DmUlLevel)
	}

	if room.DmMedalLevel == 1 {
		// 必须佩戴粉丝勋章
		query = query.Where("medal_level > 0")
	} else if room.DmMedalLevel == 2 {
		// 必须佩戴主播粉丝勋章（需要额外逻辑判断）
		// 这里简化处理，只要有勋章名称匹配即可
		if room.Uname != "" {
			query = query.Where("medal_name = ?", room.Uname)
		}
	}

	// 关键词屏蔽
	if room.DmKeywordBlacklist != "" {
		keywords := strings.Split(room.DmKeywordBlacklist, "\n")
		for _, keyword := range keywords {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" {
				query = query.Where("message NOT LIKE ?", "%"+keyword+"%")
			}
		}
	}

	if err := query.Find(&danmakus).Error; err != nil {
		return fmt.Errorf("查询弹幕失败: %w", err)
	}

	// 应用去重逻辑
	if room.DmDistinct && len(danmakus) > 0 {
		danmakus = s.deduplicateDanmakus(danmakus)
	}

	if len(danmakus) == 0 {
		log.Printf("历史记录 %d 没有可发送的弹幕", historyID)
		history.DanmakuSent = true
		history.DanmakuCount = 0
		db.Save(&history)
		return nil
	}

	// 初始化进度
	danmakuprogress.SetDanmakuProgress(int64(historyID), 0, len(danmakus), true, false)

	// 获取视频分P信息
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	videoInfo, err := client.GetVideoInfo(history.BvID)
	if err != nil {
		return fmt.Errorf("获取视频信息失败: %w", err)
	}

	// 获取所有分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND upload = ?", historyID, true).
		Order("start_time ASC").
		Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	if len(parts) == 0 {
		return fmt.Errorf("没有已上传的分P")
	}

	// 构建分P时间映射（毫秒）
	partTimeMap := make(map[int]struct {
		startMs int64
		endMs   int64
		cid     int64
	})

	for i, part := range parts {
		startMs := part.StartTime.UnixMilli() - history.StartTime.UnixMilli()
		endMs := part.EndTime.UnixMilli() - history.StartTime.UnixMilli()

		// 查找对应的CID
		cid := part.CID
		if cid == 0 && i < len(videoInfo.Pages) {
			cid = videoInfo.Pages[i].CID
		}

		partTimeMap[i] = struct {
			startMs int64
			endMs   int64
			cid     int64
		}{startMs, endMs, cid}
	}

	// 准备发送的弹幕
	var danmakuItems []bili.DanmakuItem
	sentCount := 0

	for _, dm := range danmakus {
		// 找到弹幕所属的分P
		found := false
		for partIdx, timeRange := range partTimeMap {
			if dm.Timestamp >= timeRange.startMs && dm.Timestamp < timeRange.endMs {
				// 计算相对于分P的时间
				relativeProgress := int(dm.Timestamp - timeRange.startMs)

				danmakuItems = append(danmakuItems, bili.DanmakuItem{
					CID:      timeRange.cid,
					BvID:     history.BvID,
					Progress: relativeProgress,
					Message:  dm.Message,
					Mode:     dm.Mode,
					FontSize: dm.FontSize,
					Color:    dm.Color,
				})

				// 更新弹幕记录
				dm.Sent = true
				dm.CID = timeRange.cid
				dm.Progress = relativeProgress
				dm.BvID = history.BvID
				db.Save(&dm)

				found = true
				sentCount++
				break
			}

			// 如果超出最后一个分P，归到最后一个分P
			if !found && partIdx == len(partTimeMap)-1 {
				relativeProgress := int(dm.Timestamp - timeRange.startMs)
				if relativeProgress < 0 {
					relativeProgress = 0
				}

				danmakuItems = append(danmakuItems, bili.DanmakuItem{
					CID:      timeRange.cid,
					BvID:     history.BvID,
					Progress: relativeProgress,
					Message:  dm.Message,
					Mode:     dm.Mode,
					FontSize: dm.FontSize,
					Color:    dm.Color,
				})

				dm.Sent = true
				dm.CID = timeRange.cid
				dm.Progress = relativeProgress
				dm.BvID = history.BvID
				db.Save(&dm)
				sentCount++
			}
		}
	}

	// 批量发送弹幕
	if len(danmakuItems) > 0 {
		log.Printf("开始发送 %d 条弹幕到视频 %s", len(danmakuItems), history.BvID)

		// 发送弹幕并更新进度
		successCount := 0
		for i, dm := range danmakuItems {
			err := client.SendDanmaku(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color)
			if err != nil {
				log.Printf("发送第%d条弹幕失败: %v", i+1, err)
			} else {
				successCount++
			}

			// 更新进度
			danmakuprogress.SetDanmakuProgress(int64(historyID), i+1, len(danmakuItems), true, false)
		}

		log.Printf("弹幕发送完成: %d/%d", successCount, len(danmakuItems))

		// 更新历史记录
		history.DanmakuSent = true
		history.DanmakuCount = sentCount
		db.Save(&history)

		// 完成进度
		danmakuprogress.SetDanmakuProgress(int64(historyID), len(danmakuItems), len(danmakuItems), false, true)

		return nil
	}

	history.DanmakuSent = true
	history.DanmakuCount = 0
	db.Save(&history)

	// 完成进度
	danmakuprogress.ClearDanmakuProgress(int64(historyID))

	return nil
}

// deduplicateDanmakus 弹幕去重
func (s *DanmakuService) deduplicateDanmakus(danmakus []models.LiveMsg) []models.LiveMsg {
	seen := make(map[string]bool)
	result := make([]models.LiveMsg, 0, len(danmakus))

	for _, dm := range danmakus {
		// 使用"用户ID+内容"作为去重key
		key := fmt.Sprintf("%d:%s", dm.UID, dm.Message)
		if !seen[key] {
			seen[key] = true
			result = append(result, dm)
		}
	}

	return result
}
