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
	// 全局弹幕发送计数器（用于限制连续发送）
	globalDanmakuCounter     int
	globalDanmakuCounterLock sync.Mutex
	globalDanmakuLimit       = 100 // 连续发送100条后等待
	globalDanmakuWaitTime    = 3 * time.Minute
)

// shouldLogProgress 判断是否应该记录进度日志
// 规则：5, 10, 15, 20, 25, 50, 75, 100, 125...（之后每25条）
func shouldLogProgress(count int) bool {
	if count <= 0 {
		return false
	}
	// 前25条：每5条记录一次
	if count <= 25 && count%5 == 0 {
		return true
	}
	// 25条之后：每25条记录一次
	if count > 25 && count%25 == 0 {
		return true
	}
	return false
}

func NewDanmakuService() *DanmakuService {
	danmakuServiceOnce.Do(func() {
		danmakuServiceInstance = &DanmakuService{}
		danmakuServiceInstance.queueManager = NewDanmakuQueueManager(danmakuServiceInstance)
	})
	return danmakuServiceInstance
}

// getGlobalProxyPool 获取全局代理池配置
func (s *DanmakuService) getGlobalProxyPool() *bili.ProxyPool {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		// 配置不存在，使用默认（仅本地IP）
		return bili.NewProxyPool([]string{})
	}

	if !config.EnableDanmakuProxy || config.DanmakuProxyList == "" {
		// 未启用代理或未配置代理列表，仅使用本地IP
		return bili.NewProxyPool([]string{})
	}

	// 解析并创建代理池
	proxyURLs := bili.ParseProxyList(config.DanmakuProxyList)
	proxyPool := bili.NewProxyPool(proxyURLs)
	log.Printf("[弹幕发送] 使用全局代理池，共%d个IP (包含本地)", proxyPool.GetProxyCount())
	return proxyPool
}

// GetQueueManager 获取队列管理器
func (s *DanmakuService) GetQueueManager() *DanmakuQueueManager {
	return s.queueManager
}

// SendDanmakuForHistory 为历史记录发送弹幕（通过队列）
func (s *DanmakuService) SendDanmakuForHistory(historyID uint) error {
	// 添加到队列
	if err := s.queueManager.AddTask(historyID); err != nil {
		log.Printf("[弹幕发送] 添加到队列失败 (history_id=%d): %v", historyID, err)
		return err
	}

	log.Printf("[弹幕发送] 任务已加入队列 (history_id=%d, 队列长度=%d)",
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
			log.Printf("[弹幕发送] 验证用户%d (uid=%d) cookie失败: %v", user.ID, user.UID, err)
			continue
		}

		if !valid {
			log.Printf("[弹幕发送] 用户%d (uid=%d) cookie已失效", user.ID, user.UID)
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
		log.Printf("[弹幕发送] 历史记录不存在: %v", err)
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	log.Printf("[弹幕发送] 步骤2: 检查视频状态 (BV号=%s, 已发送=%v)", history.BvID, history.DanmakuSent)

	if history.BvID == "" {
		log.Printf("[弹幕发送] 视频尚未投稿")
		return fmt.Errorf("视频尚未投稿")
	}

	// 检查BV号格式
	if !strings.HasPrefix(history.BvID, "BV") {
		log.Printf("[弹幕发送] 无效的BV号格式: %s", history.BvID)
		return fmt.Errorf("无效的BV号格式")
	}

	if history.DanmakuSent {
		log.Printf("[弹幕发送] 弹幕已发送，跳过")
		return fmt.Errorf("弹幕已发送，请勿重复操作")
	}

	log.Printf("[弹幕发送] 步骤3: 获取有效的B站用户")

	// 获取所有有效用户
	validUsers, err := s.getValidUsers()
	if err != nil {
		log.Printf("[弹幕发送] 获取有效用户失败: %v", err)
		return err
	}

	log.Printf("[弹幕发送] 步骤4: 获取房间配置 (room_id=%s)", history.RoomID)

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		log.Printf("[弹幕发送] 房间配置不存在: %v", err)
		return fmt.Errorf("房间配置不存在: %w", err)
	}

	// 获取弹幕列表（过滤规则已在解析时应用）
	var danmakus []models.LiveMsg
	query := db.Where("session_id = ? AND sent = ?", history.SessionID, false).
		Order("timestamp ASC")

	if err := query.Find(&danmakus).Error; err != nil {
		log.Printf("[弹幕发送] 查询弹幕失败: %v", err)
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
		log.Printf("[弹幕发送] 没有可发送的弹幕 (history_id=%d)", historyID)
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
		log.Printf("[弹幕发送] 获取视频信息失败: %v", err)
		return fmt.Errorf("获取视频信息失败: %w", err)
	}

	log.Printf("[弹幕发送] ✓ 视频信息获取成功 (aid=%d, 分P数=%d)", videoInfo.Aid, len(videoInfo.Pages))

	log.Printf("[弹幕发送] 步骤9: 获取分P信息")

	// 获取所有分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND upload = ?", historyID, true).
		Order("start_time ASC").
		Find(&parts).Error; err != nil {
		log.Printf("[弹幕发送] 查询分P失败: %v", err)
		return fmt.Errorf("查询分P失败: %w", err)
	}

	if len(parts) == 0 {
		log.Printf("[弹幕发送] 没有已上传的分P")
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

	// 注意：XML弹幕的Timestamp字段已经是相对于该视频片段的时间（毫秒）
	// 多分P场景下，需要根据录制时间确定弹幕属于哪个分P
	for _, dm := range danmakus {
		// XML中的弹幕时间是相对于该XML对应的视频片段的
		// 我们直接使用这个时间作为progress即可
		progress := int(dm.Timestamp)

		// 默认发送到第一个分P（如果只有一个分P）
		if len(partTimeMap) == 1 {
			for _, timeRange := range partTimeMap {
				danmakuItems = append(danmakuItems, bili.DanmakuItem{
					CID:      timeRange.cid,
					BvID:     history.BvID,
					Progress: progress,
					Message:  dm.Message,
					Mode:     dm.Mode,
					FontSize: dm.FontSize,
					Color:    dm.Color,
				})

				// 更新弹幕记录
				dm.Sent = true
				dm.CID = timeRange.cid
				dm.Progress = progress
				dm.BvID = history.BvID
				db.Save(&dm)
				sentCount++
				break
			}
		} else {
			// 多分P场景：需要特殊处理
			// TODO: 如果一个XML对应多个分P，需要根据文件名或其他方式确定弹幕属于哪个分P
			// 目前简单处理：发送到第一个分P
			for _, timeRange := range partTimeMap {
				danmakuItems = append(danmakuItems, bili.DanmakuItem{
					CID:      timeRange.cid,
					BvID:     history.BvID,
					Progress: progress,
					Message:  dm.Message,
					Mode:     dm.Mode,
					FontSize: dm.FontSize,
					Color:    dm.Color,
				})

				dm.Sent = true
				dm.CID = timeRange.cid
				dm.Progress = progress
				dm.BvID = history.BvID
				db.Save(&dm)
				sentCount++
				break
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

		// 获取全局代理池配置
		proxyPool := s.getGlobalProxyPool()
		proxyCount := proxyPool.GetProxyCount()

		// 如果代理池有多个IP（除了本地IP），使用并行发送
		if proxyCount > 1 {
			log.Printf("[弹幕发送] 使用全局代理池并行发送模式 (%d个IP)", proxyCount)
			successCount = s.sendDanmakuWithProxyPool(validUsers, userDanmakuGroups, history.BvID, int64(historyID), proxyPool)
		} else {
			// 仅本地IP，使用传统的串行发送
			log.Printf("[弹幕发送] 使用传统串行发送模式（仅本地IP）")
			successCount = s.sendDanmakuSerial(validUsers, userDanmakuGroups, history.BvID, int64(historyID), len(danmakuItems))
		}

		log.Printf("[弹幕发送] 全部发送完成: 成功 %d/%d 条 (成功率 %.1f%%)",
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

// sendDanmakuSerial 串行发送弹幕（传统方式）
func (s *DanmakuService) sendDanmakuSerial(validUsers []models.BiliBiliUser, userDanmakuGroups [][]bili.DanmakuItem, bvid string, historyID int64, totalCount int) int {
	successCount := 0
	totalSent := 0

	// 用户串行发送（一个用户发送完后才轮到下一个用户）
	for userIdx, user := range validUsers {
		userDanmakus := userDanmakuGroups[userIdx]
		if len(userDanmakus) == 0 {
			continue
		}

		log.Printf("[弹幕发送] 用户%s开始发送 %d 条弹幕", user.Uname, len(userDanmakus))

		client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
		userSuccessCount := 0
		consecutiveFailures := 0 // 连续失败计数

		// 该用户发送其负责的所有弹幕
		for dmIdx, dm := range userDanmakus {
			totalSent++

			// 检查全局计数器，每发送100条后等待3分钟
			globalDanmakuCounterLock.Lock()
			globalDanmakuCounter++
			if globalDanmakuCounter >= globalDanmakuLimit {
				log.Printf("[弹幕发送] ⏸️  已连续发送%d条弹幕，等待%v以避免风控...", globalDanmakuCounter, globalDanmakuWaitTime)
				globalDanmakuCounterLock.Unlock()
				time.Sleep(globalDanmakuWaitTime)
				globalDanmakuCounterLock.Lock()
				globalDanmakuCounter = 0
				log.Printf("[弹幕发送] ▶️  等待结束，继续发送弹幕")
			}
			globalDanmakuCounterLock.Unlock()

			// 发送弹幕
			err := client.SendDanmakuWithoutWait(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color)
			if err != nil {
				consecutiveFailures++
				log.Printf("[弹幕发送] 用户%s 第%d/%d条失败 (连续失败%d次, 进度=%dms, 内容=%s): %v",
					user.Uname, dmIdx+1, len(userDanmakus), consecutiveFailures, dm.Progress, dm.Message, err)

				// 指数退避机制：30秒 -> 1分钟 -> 5分钟 -> 10分钟 -> 15分钟
				var waitTime time.Duration
				switch consecutiveFailures {
				case 1:
					waitTime = 30 * time.Second
				case 2:
					waitTime = 1 * time.Minute
				case 3:
					waitTime = 5 * time.Minute
				case 4:
					waitTime = 10 * time.Minute
				default:
					waitTime = 15 * time.Minute
				}
				log.Printf("[弹幕发送] 用户%s 连续失败%d次，等待%v后继续...", user.Uname, consecutiveFailures, waitTime)
				time.Sleep(waitTime)
			} else {
				userSuccessCount++
				successCount++
				consecutiveFailures = 0 // 成功后重置失败计数

				// 成功后添加随机等待（全局限流器已确保至少30秒间隔）
				// 这里添加3-8秒的额外随机延迟，总延迟在33-38秒之间
				extraWait := 3 + rand.Intn(6) // 3-8秒
				time.Sleep(time.Duration(extraWait) * time.Second)
			}

			// 更新进度 - 仅在特定间隔记录统计日志
			if shouldLogProgress(totalSent) || totalSent == totalCount {
				log.Printf("[弹幕发送] ⏳ 总进度: %d/%d (%.1f%%)",
					totalSent, totalCount, float64(totalSent)*100/float64(totalCount))
			}
			danmakuprogress.SetDanmakuProgress(historyID, totalSent, totalCount, true, false)
		}

		log.Printf("[弹幕发送] 用户%s 发送完成: 成功 %d/%d 条",
			user.Uname, userSuccessCount, len(userDanmakus))

		// 用户切换时额外等待，进一步降低风控风险
		if userIdx < len(validUsers)-1 {
			switchWait := 10 + rand.Intn(11) // 10-20秒
			log.Printf("[弹幕发送] 切换到下一个用户，等待%d秒...", switchWait)
			time.Sleep(time.Duration(switchWait) * time.Second)
		}
	}

	return successCount
}

// sendDanmakuWithProxyPool 使用全局代理池并行发送弹幕
func (s *DanmakuService) sendDanmakuWithProxyPool(validUsers []models.BiliBiliUser, userDanmakuGroups [][]bili.DanmakuItem, bvid string, historyID int64, proxyPool *bili.ProxyPool) int {
	var wg sync.WaitGroup
	var mu sync.Mutex
	totalSuccessCount := 0
	totalSent := 0

	// 为每个用户创建一个goroutine，所有用户共享全局代理池
	for userIdx, user := range validUsers {
		userDanmakus := userDanmakuGroups[userIdx]
		if len(userDanmakus) == 0 {
			continue
		}

		wg.Add(1)
		go func(user models.BiliBiliUser, danmakus []bili.DanmakuItem, userIdx int) {
			defer wg.Done()

			log.Printf("[弹幕发送] 用户%s 开始使用全局代理池发送 %d 条弹幕", user.Uname, len(danmakus))

			userSuccessCount := 0
			consecutiveFailures := 0

			log.Printf("[弹幕发送] 用户%s 开始发送 %d 条弹幕", user.Uname, len(danmakus))

			for dmIdx, dm := range danmakus {
				// 检查全局计数器，每发送100条后等待3分钟
				globalDanmakuCounterLock.Lock()
				globalDanmakuCounter++
				if globalDanmakuCounter >= globalDanmakuLimit {
					log.Printf("[弹幕发送] ⏸️  已连续发送%d条弹幕，等待%v以避免风控...", globalDanmakuCounter, globalDanmakuWaitTime)
					globalDanmakuCounterLock.Unlock()
					time.Sleep(globalDanmakuWaitTime)
					globalDanmakuCounterLock.Lock()
					globalDanmakuCounter = 0
					log.Printf("[弹幕发送] ▶️  等待结束，继续发送弹幕")
				}
				globalDanmakuCounterLock.Unlock()

				// 获取下一个可用代理（跳过不可达的代理）
				proxyInfo := proxyPool.GetNextAvailableProxy()
				if proxyInfo == nil {
					log.Printf("[弹幕发送] 用户%s 无法获取代理", user.Uname)
					break
				}

				// 创建带代理的客户端
				var client *bili.BiliClient
				if proxyInfo.IsLocal() {
					client = bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
				} else {
					client = bili.NewBiliClientWithProxy(user.AccessKey, user.Cookies, user.UID, proxyInfo.GetProxyURL())
				}

				// 发送弹幕（使用代理特定的限流器）
				err := client.SendDanmakuWithProxy(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color, proxyInfo)

				mu.Lock()
				totalSent++
				currentTotal := totalSent
				mu.Unlock()

				if err != nil {
					consecutiveFailures++
					log.Printf("[弹幕发送] 用户%s 代理%s 第%d/%d条失败 (连续失败%d次): %v",
						user.Uname, proxyInfo.String(), dmIdx+1, len(danmakus), consecutiveFailures, err)

					// 指数退避
					var waitTime time.Duration
					switch consecutiveFailures {
					case 1:
						waitTime = 30 * time.Second
					case 2:
						waitTime = 1 * time.Minute
					case 3:
						waitTime = 5 * time.Minute
					default:
						waitTime = 10 * time.Minute
					}
					if consecutiveFailures >= 3 {
						log.Printf("[弹幕发送] 用户%s 连续失败%d次，等待%v后继续...", user.Uname, consecutiveFailures, waitTime)
						time.Sleep(waitTime)
					}
				} else {
					userSuccessCount++
					mu.Lock()
					totalSuccessCount++
					mu.Unlock()
					consecutiveFailures = 0

					// 成功后添加3-8秒随机延迟（代理限流器已保证30秒基础间隔）
					extraWait := 3 + rand.Intn(6)
					time.Sleep(time.Duration(extraWait) * time.Second)
				}

				// 更新进度 - 仅在特定间隔记录统计日志
				mu.Lock()
				if shouldLogProgress(currentTotal) {
					log.Printf("[弹幕发送] ⏳ 总进度: %d 条已发送", currentTotal)
				}
				danmakuprogress.SetDanmakuProgress(historyID, currentTotal, -1, true, false)
				mu.Unlock()
			}

			log.Printf("[弹幕发送] 用户%s 发送完成: 成功 %d/%d 条",
				user.Uname, userSuccessCount, len(danmakus))
		}(user, userDanmakus, userIdx)
	}

	// 等待所有用户发送完成
	wg.Wait()
	return totalSuccessCount
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
