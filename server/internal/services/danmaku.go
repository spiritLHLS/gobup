package services

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	danmakuprogress "github.com/gobup/server/internal/progress"
)

type DanmakuService struct {
	queueManager *DanmakuQueueManager
}

var (
	danmakuServiceInstance *DanmakuService
	danmakuServiceOnce     sync.Once
)

func NewDanmakuService() *DanmakuService {
	danmakuServiceOnce.Do(func() {
		danmakuServiceInstance = &DanmakuService{}
		danmakuServiceInstance.queueManager = NewDanmakuQueueManager(danmakuServiceInstance)
	})
	return danmakuServiceInstance
}

// GetQueueManager 获取队列管理器
func (s *DanmakuService) GetQueueManager() *DanmakuQueueManager {
	return s.queueManager
}

// SendDanmakuForHistory 为历史记录发送弹幕（通过队列）
func (s *DanmakuService) SendDanmakuForHistory(historyID uint, userID uint) error {
	// 添加到队列
	if err := s.queueManager.AddTask(userID, historyID); err != nil {
		log.Printf("[弹幕发送] ❌ 添加到队列失败 (history_id=%d, user_id=%d): %v", historyID, userID, err)
		return err
	}

	log.Printf("[弹幕发送] ✅ 任务已加入队列 (history_id=%d, user_id=%d, 队列长度=%d)",
		historyID, userID, s.queueManager.GetQueueLength(userID))
	return nil
}

// sendDanmakuForHistoryInternal 为历史记录发送弹幕（内部方法，由队列调用）
func (s *DanmakuService) sendDanmakuForHistoryInternal(historyID uint, userID uint) error {
	db := database.GetDB()

	log.Printf("[弹幕发送] 步骤1: 开始处理历史记录 %d", historyID)

	// 获取历史记录
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		log.Printf("[弹幕发送] ❌ 历史记录不存在: %v", err)
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	log.Printf("[弹幕发送] 步骤2: 检查视频状态 (BV号=%s, 已发送=%v)", history.BvID, history.DanmakuSent)

	if history.BvID == "" {
		log.Printf("[弹幕发送] ❌ 视频尚未投稿")
		return fmt.Errorf("视频尚未投稿")
	}

	// 检查BV号格式
	if !strings.HasPrefix(history.BvID, "BV") {
		log.Printf("[弹幕发送] ❌ 无效的BV号格式: %s", history.BvID)
		return fmt.Errorf("无效的BV号格式")
	}

	if history.DanmakuSent {
		log.Printf("[弹幕发送] ⚠️ 弹幕已发送，跳过")
		return fmt.Errorf("弹幕已发送，请勿重复操作")
	}

	log.Printf("[弹幕发送] 步骤3: 获取房间和用户信息 (room_id=%s, user_id=%d)", history.RoomID, userID)

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		log.Printf("[弹幕发送] ❌ 房间配置不存在: %v", err)
		return fmt.Errorf("房间配置不存在: %w", err)
	}

	// 获取用户
	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		log.Printf("[弹幕发送] ❌ 用户不存在: %v", err)
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		log.Printf("[弹幕发送] ❌ 用户未登录 (uid=%d)", user.UID)
		return fmt.Errorf("用户未登录")
	}

	log.Printf("[弹幕发送] ✓ 用户信息验证通过 (uid=%d, uname=%s)", user.UID, user.Uname)

	// 获取弹幕列表（应用过滤规则）
	var danmakus []models.LiveMsg
	query := db.Where("session_id = ? AND sent = ?", history.SessionID, false).
		Where("message != '' AND message IS NOT NULL"). // 过滤空弹幕和抽奖弹幕
		Order("timestamp ASC")

	// 应用弹幕过滤规则
	if room.DmUlLevel > 0 {
		// 用户等级过滤（佩戴勋章的不受影响）
		query = query.Where("u_level >= ? OR medal_level > 0", room.DmUlLevel)
		log.Printf("[弹幕发送] 应用用户等级过滤: >= %d (佩戴勋章者不受限)", room.DmUlLevel)
	}

	if room.DmMedalLevel == 1 {
		// 必须佩戴粉丝勋章
		query = query.Where("medal_level > 0")
		log.Printf("[弹幕发送] 应用粉丝勋章过滤: 必须佩戴粉丝勋章")
	} else if room.DmMedalLevel == 2 {
		// 必须佩戴主播粉丝勋章
		if room.Uname != "" {
			query = query.Where("medal_name = ?", room.Uname)
			log.Printf("[弹幕发送] 应用粉丝勋章过滤: 必须佩戴主播【%s】的粉丝勋章", room.Uname)
		}
	}

	// 关键词屏蔽
	if room.DmKeywordBlacklist != "" {
		keywords := strings.Split(room.DmKeywordBlacklist, "\n")
		keywordCount := 0
		for _, keyword := range keywords {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" {
				query = query.Where("LOWER(message) NOT LIKE ?", "%"+strings.ToLower(keyword)+"%")
				keywordCount++
			}
		}
		if keywordCount > 0 {
			log.Printf("[弹幕发送] 应用关键词屏蔽: %d 个关键词", keywordCount)
		}
	}

	if err := query.Find(&danmakus).Error; err != nil {
		log.Printf("[弹幕发送] ❌ 查询弹幕失败: %v", err)
		return fmt.Errorf("查询弹幕失败: %w", err)
	}

	log.Printf("[弹幕发送] 步骤4: 查询到 %d 条弹幕 (session_id=%s)", len(danmakus), history.SessionID)

	// 应用去重逻辑
	if room.DmDistinct && len(danmakus) > 0 {
		beforeCount := len(danmakus)
		danmakus = s.deduplicateDanmakus(danmakus)
		log.Printf("[弹幕发送] 步骤5: 去重后剩余 %d 条弹幕 (去重了%d条)", len(danmakus), beforeCount-len(danmakus))
	}

	if len(danmakus) == 0 {
		log.Printf("[弹幕发送] ⚠️ 没有可发送的弹幕 (history_id=%d)", historyID)
		history.DanmakuSent = true
		history.DanmakuCount = 0
		db.Save(&history)
		return nil
	}

	log.Printf("[弹幕发送] 步骤6: 初始化发送进度 (总计 %d 条)", len(danmakus))

	// 初始化进度
	danmakuprogress.SetDanmakuProgress(int64(historyID), 0, len(danmakus), true, false)

	log.Printf("[弹幕发送] 步骤7: 获取视频信息 (BV号=%s)", history.BvID)

	// 获取视频分P信息
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	videoInfo, err := client.GetVideoInfo(history.BvID)
	if err != nil {
		log.Printf("[弹幕发送] ❌ 获取视频信息失败: %v", err)
		return fmt.Errorf("获取视频信息失败: %w", err)
	}

	log.Printf("[弹幕发送] ✓ 视频信息获取成功 (aid=%d, 分P数=%d)", videoInfo.Aid, len(videoInfo.Pages))

	log.Printf("[弹幕发送] 步骤8: 获取分P信息")

	// 获取所有分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND upload = ?", historyID, true).
		Order("start_time ASC").
		Find(&parts).Error; err != nil {
		log.Printf("[弹幕发送] ❌ 查询分P失败: %v", err)
		return fmt.Errorf("查询分P失败: %w", err)
	}

	if len(parts) == 0 {
		log.Printf("[弹幕发送] ❌ 没有已上传的分P")
		return fmt.Errorf("没有已上传的分P")
	}

	log.Printf("[弹幕发送] ✓ 找到 %d 个分P", len(parts))

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

	log.Printf("[弹幕发送] 步骤9: 开始映射弹幕到分P (映射成功 %d 条)", len(danmakuItems))

	// 批量发送弹幕
	if len(danmakuItems) > 0 {
		log.Printf("[弹幕发送] 步骤10: 开始批量发送 %d 条弹幕到视频 %s", len(danmakuItems), history.BvID)

		// 发送弹幕并更新进度
		successCount := 0
		for i, dm := range danmakuItems {
			err := client.SendDanmaku(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color)
			if err != nil {
				log.Printf("[弹幕发送] ❌ 第%d/%d条失败 (进度=%dms, 内容=%s): %v", i+1, len(danmakuItems), dm.Progress, dm.Message, err)
			} else {
				successCount++
				if (i+1)%10 == 0 || i == len(danmakuItems)-1 {
					log.Printf("[弹幕发送] ⏳ 进度: %d/%d (%.1f%%)", i+1, len(danmakuItems), float64(i+1)*100/float64(len(danmakuItems)))
				}
			}

			// 更新进度
			danmakuprogress.SetDanmakuProgress(int64(historyID), i+1, len(danmakuItems), true, false)
		}

		log.Printf("[弹幕发送] ✅ 发送完成: 成功 %d/%d 条 (成功率 %.1f%%)", successCount, len(danmakuItems), float64(successCount)*100/float64(len(danmakuItems)))

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

// deduplicateDanmakus 弹幕去重（参考biliupforjava的布隆过滤器实现）
func (s *DanmakuService) deduplicateDanmakus(danmakus []models.LiveMsg) []models.LiveMsg {
	seen := make(map[string]bool)
	result := make([]models.LiveMsg, 0, len(danmakus))

	for _, dm := range danmakus {
		// 使用消息内容作为去重key（忽略大小写和空白字符）
		// 参考 LiveMsgService.java 的实现
		key := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(dm.Message, " ", "")))
		if !seen[key] {
			seen[key] = true
			result = append(result, dm)
		} else {
			log.Printf("[弹幕发送] 去重: 过滤重复弹幕 '%s'", dm.Message)
		}
	}

	return result
}
