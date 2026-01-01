package services

import (
	"fmt"
	"log"
	"strconv"
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
		return fmt.Errorf("获取视频信息失败: %w", err)
	}

	// 更新Aid
	if history.AvID == "" && videoInfo.Aid > 0 {
		history.AvID = strconv.FormatInt(videoInfo.Aid, 10)
	}

	// 记录之前的状态
	oldVideoState := history.VideoState

	// 更新视频状态
	history.VideoState = videoInfo.State
	switch videoInfo.State {
	case -1, -2, -3, -4:
		history.VideoStateDesc = "审核未通过"
	case 0:
		history.VideoStateDesc = "审核中"
	case 1:
		history.VideoStateDesc = "已通过"
	default:
		history.VideoStateDesc = fmt.Sprintf("未知状态(%d)", videoInfo.State)
	}

	// 检测到从非通过状态变为通过状态，触发审核通过后的文件处理
	if oldVideoState != 1 && videoInfo.State == 1 {
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

	// 获取分P详细信息
	partInfo, err := client.GetVideoPartInfo(history.BvID)
	if err != nil {
		log.Printf("获取分P详细信息失败: %v", err)
	} else {
		// 更新分P的CID和转码状态
		var parts []models.RecordHistoryPart
		if err := db.Where("history_id = ?", historyID).
			Order("start_time ASC").
			Find(&parts).Error; err == nil {

			for i, part := range parts {
				if i < len(partInfo.Videos) {
					videoPartData := partInfo.Videos[i]
					part.CID = videoPartData.CID
					part.XcodeState = videoPartData.XcodeState
					part.Page = videoPartData.Page
					db.Save(&part)
				}
			}
		}
	}

	// 更新同步时间
	now := time.Now()
	history.SyncedAt = &now
	db.Save(&history)

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

	// 查找待执行的任务
	var tasks []models.VideoSyncTask
	now := time.Now()
	if err := db.Where("status IN ? AND (next_run_at IS NULL OR next_run_at <= ?)",
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

			// 设置下次重试时间（指数退避）
			retryDelay := time.Duration(task.RetryCount*task.RetryCount) * time.Minute
			if retryDelay > 60*time.Minute {
				retryDelay = 60 * time.Minute
			}
			nextRun := time.Now().Add(retryDelay)
			task.NextRunAt = &nextRun

			// 如果重试次数过多，标记为失败不再重试
			if task.RetryCount >= 5 {
				task.Status = "failed"
				task.NextRunAt = nil
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
