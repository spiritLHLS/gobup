package services

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

type VideoSyncService struct{}

func NewVideoSyncService() *VideoSyncService {
	return &VideoSyncService{}
}

// SyncVideoInfo 同步单个视频信息
func (s *VideoSyncService) SyncVideoInfo(historyID uint) error {
	db := database.GetDB()

	// 获取历史记录
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	if history.BvID == "" {
		return fmt.Errorf("视频尚未投稿")
	}

	// 检查BV号格式，如果是错误的av格式（如"av115818859857681"），通过API获取正确的BV号
	if strings.HasPrefix(history.BvID, "av") && len(history.BvID) > 12 {
		log.Printf("检测到错误的BV号格式: %s，尝试通过AID获取正确的BV号", history.BvID)

		// 从BV号中提取AID
		avIDStr := strings.TrimPrefix(history.BvID, "av")
		avID, parseErr := strconv.ParseInt(avIDStr, 10, 64)

		// 如果有AvID，也尝试解析
		if parseErr != nil && history.AvID != "" {
			avID, parseErr = strconv.ParseInt(history.AvID, 10, 64)
		}

		if parseErr == nil && avID > 0 {
			// 获取房间配置和用户信息
			var room models.RecordRoom
			if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err == nil {
				var user models.BiliBiliUser
				if err := db.First(&user, room.UploadUserID).Error; err == nil && user.Login && user.UID > 0 {
					client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)

					// 通过API查找正确的BV号
					correctBvid, bvidErr := client.GetBvidByAid(user.UID, avID)
					if bvidErr == nil && correctBvid != "" && strings.HasPrefix(correctBvid, "BV") {
						log.Printf("成功通过API获取正确的BV号: %s -> %s (AID=%d)", history.BvID, correctBvid, avID)
						history.BvID = correctBvid
						db.Save(&history)
					} else {
						log.Printf("通过API获取BV号失败: %v", bvidErr)
					}
				}
			}
		}
	}

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间配置不存在: %w", err)
	}

	// 获取用户信息
	var user models.BiliBiliUser
	if err := db.First(&user, room.UploadUserID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}

	// 创建客户端
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)

	// 获取视频基本信息
	videoInfo, err := client.GetVideoInfo(history.BvID)
	if err != nil {
		// 如果获取失败，尝试使用Member API二次确认
		if strings.Contains(err.Error(), "code=-400") || strings.Contains(err.Error(), "code=-404") {
			log.Printf("获取视频信息失败(code=%s)，尝试使用Member API二次确认 BV号: %s",
				extractCodeFromError(err), history.BvID)

			// 使用Member API获取分P信息
			partInfo, partErr := client.GetVideoPartInfo(history.BvID)
			if partErr != nil {
				// Member API也失败，可能视频已被删除
				if strings.Contains(partErr.Error(), "code=-404") {
					log.Printf("Member API确认视频已删除: %s", history.BvID)
					history.VideoState = -404
					history.VideoStateDesc = "视频已删除"
					db.Save(&history)
					return fmt.Errorf("视频已删除: %w", err)
				}
				return fmt.Errorf("获取视频信息失败: %w", err)
			}

			// Member API返回成功但可能state不稳定，需要再次用带Cookie的API确认真实state
			if partInfo != nil && len(partInfo.Videos) > 0 {
				// 更新Aid信息
				if history.AvID == "" {
					history.AvID = strconv.FormatInt(partInfo.Videos[0].Aid, 10)
				}

				// 等待一下避免请求过快
				time.Sleep(800 * time.Millisecond)

				// 再次尝试获取视频信息（带Cookie）
				videoInfo, err = client.GetVideoInfo(history.BvID)
				if err != nil {
					// 仍然失败，保守处理，保持原状态
					log.Printf("二次确认仍失败，保持原状态: %s, error: %v", history.BvID, err)
					return fmt.Errorf("获取视频信息失败: %w", err)
				}
			} else {
				return fmt.Errorf("获取视频信息失败: %w", err)
			}
		} else {
			return fmt.Errorf("获取视频信息失败: %w", err)
		}
	}

	// 更新Aid
	if history.AvID == "" && videoInfo.Aid > 0 {
		history.AvID = strconv.FormatInt(videoInfo.Aid, 10)
	}

	// 记录之前的状态
	oldVideoState := history.VideoState

	// 更新视频状态
	// B站API返回的state值：
	// 0 = 正常公开（审核通过）
	// 1 = 审核中
	// 2 = 已下架
	// 3 = 仅自己可见（审核未通过或违规）
	// -1, -2, -3, -4 = 各种未通过状态
	//
	// 数据库存储的videoState（与前端一致）：
	// 1 = 已通过
	// 0 = 审核中
	// -1 = 未知
	// -2 = 未通过
	// 2 = 已下架
	// 3 = 仅自己可见
	switch videoInfo.State {
	case 0:
		// B站API state=0 表示已发布，转换为 videoState=1
		history.VideoState = 1
		history.VideoStateDesc = "已发布"
	case 1:
		// B站API state=1 表示审核中，转换为 videoState=0
		history.VideoState = 0
		history.VideoStateDesc = "审核中"
	case 2:
		history.VideoState = 2
		history.VideoStateDesc = "已下架"
	case 3:
		history.VideoState = 3
		history.VideoStateDesc = "仅自己可见"
	case -1, -2, -3, -4:
		history.VideoState = -2
		history.VideoStateDesc = "审核未通过"
	default:
		history.VideoState = -1
		history.VideoStateDesc = fmt.Sprintf("未知状态(%d)", videoInfo.State)
	}

	// 检测到从非通过状态变为通过状态，触发审核通过后的文件处理
	// videoState=1 表示已通过
	if oldVideoState != 1 && history.VideoState == 1 {
		log.Printf("视频 %s 审核通过，检查是否需要处理文件", history.BvID)
		if room.DeleteType == 11 || room.DeleteType == 12 {
			fileMoverSvc := NewFileMoverService()
			if err := fileMoverSvc.ProcessFilesByStrategy(historyID, room.DeleteType); err != nil {
				log.Printf("审核通过后文件处理失败: %v", err)
			} else {
				log.Printf("审核通过后文件处理成功: history_id=%d, strategy=%d", historyID, room.DeleteType)
			}
		}
	}

	// 获取分P详细信息（只更新CID为0的分P）
	partInfo, err := client.GetVideoPartInfo(history.BvID)
	if err != nil {
		log.Printf("获取分P详细信息失败: %v", err)
	} else {
		// 只更新CID为0的分P（避免覆盖已有数据）
		var parts []models.RecordHistoryPart
		if err := db.Where("history_id = ? AND c_id = 0", historyID).
			Order("start_time ASC").
			Find(&parts).Error; err == nil {

			for i, part := range parts {
				if i < len(partInfo.Videos) {
					videoPartData := partInfo.Videos[i]
					// 只更新CID、转码状态、分P序号
					updates := map[string]interface{}{
						"c_id":        videoPartData.CID,
						"xcode_state": videoPartData.XcodeState,
						"page":        videoPartData.Page,
					}
					if err := db.Model(&part).Updates(updates).Error; err != nil {
						log.Printf("更新分P%d CID失败: %v", part.ID, err)
					} else {
						log.Printf("更新分P%d CID: %d", part.ID, videoPartData.CID)
					}
				}
			}
		}
	}

	// 只更新同步时间和视频状态相关字段
	now := time.Now()
	updates := map[string]interface{}{
		"synced_at":        &now,
		"video_state":      history.VideoState,
		"video_state_desc": history.VideoStateDesc,
	}
	if history.AvID != "" {
		updates["av_id"] = history.AvID
	}
	// 更新主播名字（从直播间API获取，而不是从视频owner获取）
	liveStatusService := NewLiveStatusService()
	roomInfo, roomErr := liveStatusService.GetRoomInfo(history.RoomID)
	if roomErr == nil && roomInfo.Data.UID > 0 {
		userInfo, userErr := liveStatusService.GetUserInfo(roomInfo.Data.UID)
		if userErr == nil && userInfo.Data.Info.Uname != "" {
			updates["uname"] = userInfo.Data.Info.Uname
			log.Printf("更新主播名字: %s (UID=%d)", userInfo.Data.Info.Uname, roomInfo.Data.UID)
		} else {
			log.Printf("获取主播名字失败: %v", userErr)
		}
	} else {
		log.Printf("获取直播间信息失败: %v", roomErr)
	}
	if err := db.Model(&history).Updates(updates).Error; err != nil {
		log.Printf("更新历史记录同步信息失败: %v", err)
	}

	log.Printf("视频 %s 信息同步成功，状态: %s", history.BvID, history.VideoStateDesc)

	return nil
}

// CreateSyncTask 创建同步任务
func (s *VideoSyncService) CreateSyncTask(historyID uint) error {
	db := database.GetDB()

	// 检查是否已存在任务
	var existingTask models.VideoSyncTask
	if err := db.Where("history_id = ?", historyID).First(&existingTask).Error; err == nil {
		// 已存在任务，重置状态
		existingTask.Status = "pending"
		existingTask.RetryCount = 0
		existingTask.LastError = ""
		now := time.Now()
		existingTask.NextRunAt = &now
		db.Save(&existingTask)
		return nil
	}

	// 获取历史记录的BvID
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	if history.BvID == "" {
		return fmt.Errorf("视频尚未投稿")
	}

	// 创建新任务
	now := time.Now()
	task := models.VideoSyncTask{
		HistoryID: historyID,
		BvID:      history.BvID,
		Status:    "pending",
		NextRunAt: &now,
	}

	return db.Create(&task).Error
}

// ProcessPendingTasks 处理待执行的同步任务
func (s *VideoSyncService) ProcessPendingTasks() error {
	db := database.GetDB()

	// 查找待执行的任务（排除 next_run_at 为 NULL 的任务，这些是永久失败的任务）
	var tasks []models.VideoSyncTask
	now := time.Now()
	if err := db.Where("status IN ? AND next_run_at IS NOT NULL AND next_run_at <= ?",
		[]string{"pending", "failed"}, now).
		Limit(10).
		Find(&tasks).Error; err != nil {
		return err
	}

	if len(tasks) == 0 {
		return nil
	}

	log.Printf("开始处理 %d 个视频同步任务", len(tasks))

	for _, task := range tasks {
		// 标记为运行中
		task.Status = "running"
		db.Save(&task)

		// 执行同步
		err := s.SyncVideoInfo(task.HistoryID)
		if err != nil {
			log.Printf("同步任务失败 (history=%d): %v", task.HistoryID, err)
			task.Status = "failed"
			task.RetryCount++
			task.LastError = err.Error()

			// 检查是否是永久性错误（不应该重试）
			isPermanentError := strings.Contains(err.Error(), "历史记录不存在") ||
				strings.Contains(err.Error(), "record not found") ||
				strings.Contains(err.Error(), "房间配置不存在") ||
				strings.Contains(err.Error(), "用户不存在")

			if isPermanentError {
				// 永久性错误，直接标记为失败不再重试
				log.Printf("检测到永久性错误，将任务标记为永久失败并从队列移除 (history=%d): %v", task.HistoryID, err)
				task.Status = "failed"
				task.NextRunAt = nil // 设置为NULL以从队列中移除
			} else {
				// 如果重试次数过多，标记为永久失败不再重试
				if task.RetryCount >= 5 {
					log.Printf("重试次数超过限制，将任务标记为永久失败并从队列移除 (history=%d, retries=%d)", task.HistoryID, task.RetryCount)
					task.Status = "failed"
					task.NextRunAt = nil
				} else {
					// 设置下次重试时间（指数退避）
					retryDelay := time.Duration(task.RetryCount*task.RetryCount) * time.Minute
					if retryDelay > 60*time.Minute {
						retryDelay = 60 * time.Minute
					}
					nextRun := time.Now().Add(retryDelay)
					task.NextRunAt = &nextRun
				}
			}
		} else {
			task.Status = "completed"
			task.LastError = ""
			task.NextRunAt = nil
		}

		db.Save(&task)
		time.Sleep(2 * time.Second) // 避免频繁请求
	}

	return nil
}

// RetryFailedTasks 重试失败的任务
func (s *VideoSyncService) RetryFailedTasks() error {
	db := database.GetDB()

	var tasks []models.VideoSyncTask
	if err := db.Where("status = ? AND retry_count < ?", "failed", 5).
		Find(&tasks).Error; err != nil {
		return err
	}

	for _, task := range tasks {
		task.Status = "pending"
		now := time.Now()
		task.NextRunAt = &now
		db.Save(&task)
	}

	log.Printf("重置 %d 个失败任务", len(tasks))
	return nil
}

// CleanCompletedTasks 清理已完成的任务（保留最近7天）
func (s *VideoSyncService) CleanCompletedTasks() error {
	db := database.GetDB()

	cutoff := time.Now().AddDate(0, 0, -7)
	result := db.Where("status = ? AND updated_at < ?", "completed", cutoff).
		Delete(&models.VideoSyncTask{})

	log.Printf("清理了 %d 个已完成的同步任务", result.RowsAffected)
	return result.Error
}

// extractCodeFromError 从错误信息中提取code值
func extractCodeFromError(err error) string {
	if err == nil {
		return ""
	}

	// 匹配 "code=-400" 或 "code=-404" 等格式
	re := regexp.MustCompile(`code=(-?\d+)`)
	matches := re.FindStringSubmatch(err.Error())
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}
