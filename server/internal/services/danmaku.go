package services

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

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
func (s *DanmakuService) SendDanmakuForHistory(historyID uint) error {
	// 添加到队列
	if err := s.queueManager.AddTask(historyID); err != nil {
		log.Printf("[弹幕发送] ❌ 添加到队列失败 (history_id=%d): %v", historyID, err)
		return err
	}

	log.Printf("[弹幕发送] ✅ 任务已加入队列 (history_id=%d, 队列长度=%d)",
		historyID, s.queueManager.GetQueueLength(historyID))
	return nil
}

// getValidUsers 获取所有已登录且Cookie有效的用户
func (s *DanmakuService) getValidUsers() ([]models.BiliBiliUser, error) {
	db := database.GetDB()

	var users []models.BiliBiliUser
	if err := db.Where("login = ?", true).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	// 验证每个用户的cookie有效性
	validUsers := make([]models.BiliBiliUser, 0, len(users))
	for _, user := range users {
		if user.Cookies == "" {
			continue
		}

		// 验证cookie
		valid, err := bili.ValidateCookie(user.Cookies)
		if err != nil {
			log.Printf("[弹幕发送] ⚠️ 验证用户%d (uid=%d) cookie失败: %v", user.ID, user.UID, err)
			continue
		}

		if !valid {
			log.Printf("[弹幕发送] ⚠️ 用户%d (uid=%d) cookie已失效", user.ID, user.UID)
			// 更新用户登录状态
			user.Login = false
			db.Save(&user)
			continue
		}

		validUsers = append(validUsers, user)
		log.Printf("[弹幕发送] ✓ 用户%d (uid=%d, uname=%s) cookie验证通过", user.ID, user.UID, user.Uname)
	}

	if len(validUsers) == 0 {
		return nil, fmt.Errorf("没有可用的已登录B站用户")
	}

	log.Printf("[弹幕发送] 找到 %d 个有效的B站用户", len(validUsers))
	return validUsers, nil
}

// sendDanmakuForHistoryWithSerialUsers 使用多个用户串行发送弹幕
func (s *DanmakuService) sendDanmakuForHistoryWithSerialUsers(historyID uint) error {
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

	log.Printf("[弹幕发送] 步骤3: 获取有效的B站用户")

	// 获取所有有效用户
	validUsers, err := s.getValidUsers()
	if err != nil {
		log.Printf("[弹幕发送] ❌ 获取有效用户失败: %v", err)
		return err
	}

	log.Printf("[弹幕发送] 步骤4: 获取房间配置 (room_id=%s)", history.RoomID)

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		log.Printf("[弹幕发送] ❌ 房间配置不存在: %v", err)
		return fmt.Errorf("房间配置不存在: %w", err)
	}

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
		// 必须佩戴主播粉丝勋章（通过房间ID匹配）
		query = query.Where("medal_room_id = ?", history.RoomID)
		log.Printf("[弹幕发送] 应用粉丝勋章过滤: 必须佩戴主播【%s】(房间%s)的粉丝勋章", room.Uname, history.RoomID)
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

	log.Printf("[弹幕发送] 步骤5: 查询到 %d 条弹幕 (session_id=%s)", len(danmakus), history.SessionID)

	// 应用去重逻辑
	if room.DmDistinct && len(danmakus) > 0 {
		beforeCount := len(danmakus)
		danmakus = s.deduplicateDanmakus(danmakus)
		log.Printf("[弹幕发送] 步骤6: 去重后剩余 %d 条弹幕 (去重了%d条)", len(danmakus), beforeCount-len(danmakus))
	}

	if len(danmakus) == 0 {
		log.Printf("[弹幕发送] ⚠️ 没有可发送的弹幕 (history_id=%d)", historyID)
		history.DanmakuSent = true
		history.DanmakuCount = 0
		db.Save(&history)
		return nil
	}

	log.Printf("[弹幕发送] 步骤7: 初始化发送进度 (总计 %d 条)", len(danmakus))

	// 初始化进度
	danmakuprogress.SetDanmakuProgress(int64(historyID), 0, len(danmakus), true, false)

	log.Printf("[弹幕发送] 步骤8: 获取视频信息 (BV号=%s)", history.BvID)

	// 使用第一个有效用户获取视频信息
	firstUser := validUsers[0]
	client := bili.NewBiliClient(firstUser.AccessKey, firstUser.Cookies, firstUser.UID)
	videoInfo, err := client.GetVideoInfo(history.BvID)
	if err != nil {
		log.Printf("[弹幕发送] ❌ 获取视频信息失败: %v", err)
		return fmt.Errorf("获取视频信息失败: %w", err)
	}

	log.Printf("[弹幕发送] ✓ 视频信息获取成功 (aid=%d, 分P数=%d)", videoInfo.Aid, len(videoInfo.Pages))

	log.Printf("[弹幕发送] 步骤9: 获取分P信息")

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

	log.Printf("[弹幕发送] 步骤10: 开始映射弹幕到分P (映射成功 %d 条)", len(danmakuItems))

	// 串行发送弹幕（多个用户轮流发送，每个用户维护自己的随机间隔）
	if len(danmakuItems) > 0 {
		log.Printf("[弹幕发送] 步骤11: 开始使用 %d 个用户串行发送 %d 条弹幕到视频 %s",
			len(validUsers), len(danmakuItems), history.BvID)

		userCount := len(validUsers)
		successCount := 0
		totalSent := 0

		// 将弹幕按用户分组
		userDanmakuGroups := make([][]bili.DanmakuItem, userCount)
		for i := 0; i < userCount; i++ {
			userDanmakuGroups[i] = make([]bili.DanmakuItem, 0)
		}

		// 轮流分配弹幕给各个用户
		for i, dm := range danmakuItems {
			userIdx := i % userCount
			userDanmakuGroups[userIdx] = append(userDanmakuGroups[userIdx], dm)
		}

		// 用户串行发送（一个用户发送完后才轮到下一个用户）
		for userIdx, user := range validUsers {
			userDanmakus := userDanmakuGroups[userIdx]
			if len(userDanmakus) == 0 {
				continue
			}

			log.Printf("[弹幕发送] 👤 用户%s开始发送 %d 条弹幕", user.Uname, len(userDanmakus))

			client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
			userSuccessCount := 0
			consecutiveFailures := 0 // 连续失败计数

			// 该用户发送其负责的所有弹幕
			for dmIdx, dm := range userDanmakus {
				totalSent++

				// 发送弹幕
				err := client.SendDanmakuWithoutWait(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color)
				if err != nil {
					consecutiveFailures++
					log.Printf("[弹幕发送] ❌ 用户%s 第%d/%d条失败 (连续失败%d次, 进度=%dms, 内容=%s): %v",
						user.Uname, dmIdx+1, len(userDanmakus), consecutiveFailures, dm.Progress, dm.Message, err)

					// 指数退避机制
					if consecutiveFailures >= 3 {
						// 连续失败3次或以上，等待10分钟
						log.Printf("[弹幕发送] ⚠️ 用户%s 连续失败%d次，等待10分钟后继续...", user.Uname, consecutiveFailures)
						time.Sleep(10 * time.Minute)
						consecutiveFailures = 0 // 重置计数器
					} else if consecutiveFailures == 2 {
						// 连续失败2次，等待2分钟
						log.Printf("[弹幕发送] ⚠️ 用户%s 连续失败2次，等待2分钟后继续...", user.Uname)
						time.Sleep(2 * time.Minute)
					} else {
						// 首次失败，等待30秒
						log.Printf("[弹幕发送] ⚠️ 用户%s 发送失败，等待30秒后继续...", user.Uname)
						time.Sleep(30 * time.Second)
					}
				} else {
					userSuccessCount++
					successCount++
					consecutiveFailures = 0 // 成功后重置失败计数

					// 成功后添加随机等待（全局限流器已确保至少20秒间隔）
					// 这里只添加0-10秒的额外随机延迟，使发送时间更加随机化
					extraWait := rand.Intn(11) // 0-10秒

					if extraWait > 0 {
						log.Printf("[弹幕发送] ✓ 用户%s 第%d/%d条成功，额外等待%d秒...",
							user.Uname, dmIdx+1, len(userDanmakus), extraWait)
						time.Sleep(time.Duration(extraWait) * time.Second)
					} else {
						log.Printf("[弹幕发送] ✓ 用户%s 第%d/%d条成功",
							user.Uname, dmIdx+1, len(userDanmakus))
					}
				}

				// 更新进度
				if totalSent%10 == 0 || totalSent == len(danmakuItems) {
					log.Printf("[弹幕发送] ⏳ 总进度: %d/%d (%.1f%%)",
						totalSent, len(danmakuItems), float64(totalSent)*100/float64(len(danmakuItems)))
				}
				danmakuprogress.SetDanmakuProgress(int64(historyID), totalSent, len(danmakuItems), true, false)
			}

			log.Printf("[弹幕发送] ✅ 用户%s 发送完成: 成功 %d/%d 条",
				user.Uname, userSuccessCount, len(userDanmakus))

			// 用户切换时额外等待，进一步降低风控风险
			if userIdx < len(validUsers)-1 {
				switchWait := 10 + rand.Intn(11) // 10-20秒
				log.Printf("[弹幕发送] 🔄 切换到下一个用户，等待%d秒...", switchWait)
				time.Sleep(time.Duration(switchWait) * time.Second)
			}
		}

		log.Printf("[弹幕发送] ✅ 全部发送完成: 成功 %d/%d 条 (成功率 %.1f%%)",
			successCount, len(danmakuItems), float64(successCount)*100/float64(len(danmakuItems)))

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
