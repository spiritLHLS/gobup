package services

import (
	"fmt"
	"log"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
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

	if history.DanmakuSent {
		return fmt.Errorf("弹幕已发送，请勿重复操作")
	}

	// 获取用户
	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}

	// 获取弹幕列表
	var danmakus []models.LiveMsg
	if err := db.Where("session_id = ? AND sent = ?", history.SessionID, false).
		Order("timestamp ASC").
		Find(&danmakus).Error; err != nil {
		return fmt.Errorf("查询弹幕失败: %w", err)
	}

	if len(danmakus) == 0 {
		log.Printf("历史记录 %d 没有可发送的弹幕", historyID)
		history.DanmakuSent = true
		history.DanmakuCount = 0
		db.Save(&history)
		return nil
	}

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
		successCount, err := client.BatchSendDanmaku(danmakuItems)
		if err != nil {
			log.Printf("弹幕发送部分失败: %v (成功 %d/%d)", err, successCount, len(danmakuItems))
		} else {
			log.Printf("弹幕发送完成: %d/%d", successCount, len(danmakuItems))
		}

		// 更新历史记录
		history.DanmakuSent = true
		history.DanmakuCount = sentCount
		db.Save(&history)

		return nil
	}

	history.DanmakuSent = true
	history.DanmakuCount = 0
	db.Save(&history)

	return nil
}
