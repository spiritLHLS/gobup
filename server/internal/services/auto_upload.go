package services

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

// TriggerPublish 是供 services 包内部使用的投稿回调，由 scheduler 在启动时注入。
// Bug4修复: room_auto_tasks.go 需要触发投稿，但 services 包不能导入 upload 包（循环依赖）。
// 通过函数变量解耦，scheduler 注册，services 调用。
var TriggerPublish func(historyID uint, userID uint) error

// AutoUploadService 自动上传服务
type AutoUploadService struct{}

// NewAutoUploadService 创建自动上传服务
func NewAutoUploadService() *AutoUploadService {
	return &AutoUploadService{}
}

// ProcessPendingUploads 处理待上传的分P
// 根据房间的Upload和AutoUpload设置，自动将录制完成的分P加入上传队列
func (s *AutoUploadService) ProcessPendingUploads() error {
	db := database.GetDB()

	// 查询所有启用了上传功能的房间
	var rooms []models.RecordRoom
	if err := db.Where("upload = ?", true).Order("priority DESC, id ASC").Find(&rooms).Error; err != nil {
		return err
	}

	if len(rooms) == 0 {
		return nil
	}

	log.Printf("[自动上传] 开始检查 %d 个房间的待上传分P", len(rooms))

	totalProcessed := 0
	totalQueued := 0

	for _, room := range rooms {
		// 只处理启用了"自动上传分P"的房间
		if !room.AutoUpload {
			continue
		}

		// 固定账号模式必须配置上传用户；负载均衡模式可由上传服务选择账号。
		if !hasUploadUserOrStrategy(&room) {
			continue
		}
		if ok, window := isRoomUploadWindowOpen(&room, time.Now()); !ok {
			log.Printf("[自动上传] 房间 %s 当前不在上传时间窗口(%s)，跳过本轮", room.RoomID, window)
			continue
		}

		// 查询该房间所有录制完成但未上传的分P
		// 条件：recording=false（录制完成）, upload=false（未上传）, uploading=false（未在上传中）
		var parts []models.RecordHistoryPart
		if err := db.Where(
			"room_id = ? AND recording = ? AND upload = ? AND uploading = ? AND upload_paused = ? AND upload_cancelled = ?",
			room.RoomID, false, false, false, false, false,
		).Order("start_time ASC").Find(&parts).Error; err != nil {
			log.Printf("[自动上传] 查询房间 %s 的待上传分P失败: %v", room.RoomID, err)
			continue
		}

		if len(parts) == 0 {
			continue
		}

		log.Printf("[自动上传] 房间 %s (%s) 有 %d 个待上传分P", room.RoomID, room.Uname, len(parts))

		// 为每个分P加入上传队列
		for _, part := range parts {
			// 获取对应的历史记录
			var history models.RecordHistory
			if err := db.First(&history, part.HistoryID).Error; err != nil {
				log.Printf("[自动上传] 获取历史记录失败: part_id=%d, history_id=%d, error=%v",
					part.ID, part.HistoryID, err)
				continue
			}

			// 检查文件是否存在
			if part.FilePath == "" {
				log.Printf("[自动上传] 跳过没有文件路径的分P: part_id=%d", part.ID)
				continue
			}

			// 跳过速率限制冷却期中的分P
			if part.RateLimitCooldownAt != nil && time.Now().Before(*part.RateLimitCooldownAt) {
				remainingTime := time.Until(*part.RateLimitCooldownAt)
				log.Printf("[自动上传] 跳过速率限制冷却期中的分P: part_id=%d, 剩余%.0f分钟",
					part.ID, remainingTime.Minutes())
				continue
			}

			// 将分P加入上传队列
			// 注意：这里需要通过upload服务来加入队列，不能直接操作
			// 由于循环依赖问题，我们返回需要上传的分P列表，由调用方处理
			totalProcessed++
		}

		if len(parts) > 0 {
			totalQueued += len(parts)
		}
	}

	if totalProcessed > 0 {
		log.Printf("[自动上传] 检查完成，发现 %d 个待上传分P", totalProcessed)
	}

	return nil
}

// GetPendingUploadParts 获取所有待上传的分P（供upload服务调用）
func (s *AutoUploadService) GetPendingUploadParts() ([]PendingUploadTask, error) {
	db := database.GetDB()

	allowUploadWhileRecording := false
	var sysConfig models.SystemConfig
	if err := db.First(&sysConfig).Error; err == nil {
		allowUploadWhileRecording = sysConfig.UploadWhileRecording
	}

	// 查询所有启用了上传功能和自动上传的房间
	var rooms []models.RecordRoom
	if err := db.Where("upload = ? AND auto_upload = ?", true, true).Order("priority DESC, id ASC").Find(&rooms).Error; err != nil {
		return nil, err
	}

	var tasks []PendingUploadTask

	for _, room := range rooms {
		// 固定账号模式必须配置上传用户；负载均衡模式可由上传服务选择账号。
		if !hasUploadUserOrStrategy(&room) {
			continue
		}
		if ok, window := isRoomUploadWindowOpen(&room, time.Now()); !ok {
			log.Printf("[自动上传] 房间 %s 当前不在上传时间窗口(%s)，跳过本轮", room.RoomID, window)
			continue
		}

		// 查询该房间所有录制完成但未上传的分P
		// 注意：排除临时文件（is_temp_file=true），临时文件由 uploadPartInternal 内部直接入队
		// 自动调度器只负责原始录制分P，避免弹幕烧录版/切分文件被重复入队
		// file_delete=false：排除物理文件已删除的分P（如大文件切分时退役的原始分P），防止反复重试不存在的文件
		var parts []models.RecordHistoryPart
		if err := db.Where(
			"room_id = ? AND recording = ? AND upload = ? AND uploading = ? AND is_temp_file = ? AND file_delete = ? AND upload_paused = ? AND upload_cancelled = ?",
			room.RoomID, false, false, false, false, false, false, false,
		).Order("start_time ASC").Find(&parts).Error; err != nil {
			log.Printf("[自动上传] 查询房间 %s 的待上传分P失败: %v", room.RoomID, err)
			continue
		}

		for _, part := range parts {
			// 跳过没有文件路径的分P
			if part.FilePath == "" {
				continue
			}

			// 跳过速率限制冷却期中的分P
			if part.RateLimitCooldownAt != nil && time.Now().Before(*part.RateLimitCooldownAt) {
				continue
			}

			// 获取对应的历史记录
			var history models.RecordHistory
			if err := db.First(&history, part.HistoryID).Error; err != nil {
				log.Printf("[自动上传] 获取历史记录失败: part_id=%d, history_id=%d, error=%v",
					part.ID, part.HistoryID, err)
				continue
			}

			// 权限检查：历史记录是否允许上传
			if !history.Upload {
				log.Printf("[自动上传] 历史记录禁止上传，跳过: history_id=%d, part_id=%d", history.ID, part.ID)
				continue
			}
			if !allowUploadWhileRecording && (history.Recording || history.Streaming) {
				log.Printf("[自动上传] 直播仍在进行且未开启边录制边上传，跳过: history_id=%d, part_id=%d", history.ID, part.ID)
				continue
			}

			// 检查文件是否已稳定（写入完毕5分钟未变动）
			if !s.isFileStable(part.FilePath, 5*time.Minute) {
				log.Printf("[自动上传] 文件尚未稳定，跳过: part_id=%d, file=%s", part.ID, part.FilePath)
				continue
			}

			// 检查该分P是否属于仍在进行中的直播（同场直播分P间隔不超过10分钟）
			if s.isLiveStreamInProgress(&part, db) {
				if !allowUploadWhileRecording {
					log.Printf("[自动上传] 该分P所属的直播仍在进行中且未开启边录制边上传，跳过: part_id=%d", part.ID)
					continue
				}
				log.Printf("[自动上传] 已开启边录制边上传，文件稳定后预先上传: part_id=%d", part.ID)
			}

			tasks = append(tasks, PendingUploadTask{
				Part:    part,
				History: history,
				Room:    room,
			})
		}
	}

	// 额外：补充扫描所有上传失败的弹幕烧录临时分P（is_temp_file=true, temp_file_type=danmaku_burn）
	// uploadPartInternal 内部只做一次入队，若上传失败后服务未重启则永远不会重试。
	// 此处每10分钟周期调度时主动发现并重新入队，对烧录成功但上传失败的文件进行自动重试。
	var failedBurnedParts []models.RecordHistoryPart
	if err := db.Where(
		"is_temp_file = ? AND temp_file_type = ? AND upload = ? AND uploading = ? AND file_delete = ? AND upload_paused = ? AND upload_cancelled = ?",
		true, "danmaku_burn", false, false, false, false, false,
	).Find(&failedBurnedParts).Error; err != nil {
		log.Printf("[自动上传] 查询失败的弹幕烧录分P失败: %v", err)
	} else {
		for _, part := range failedBurnedParts {
			if part.FilePath == "" {
				continue
			}

			// 检查物理文件是否仍然存在（如果不存在则跳过，由 CleanOrphanedTempParts 处理）
			if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
				continue
			}

			// 跳过速率限制冷却期中的分P
			if part.RateLimitCooldownAt != nil && time.Now().Before(*part.RateLimitCooldownAt) {
				continue
			}

			// 获取对应的历史记录
			var history models.RecordHistory
			if err := db.First(&history, part.HistoryID).Error; err != nil {
				log.Printf("[自动上传] 获取弹幕烧录分P历史记录失败: part_id=%d, history_id=%d, error=%v",
					part.ID, part.HistoryID, err)
				continue
			}

			// 权限检查：历史记录是否允许上传
			if !history.Upload {
				continue
			}

			// 获取对应的房间配置
			var room models.RecordRoom
			if err := db.Where("room_id = ?", part.RoomID).First(&room).Error; err != nil {
				log.Printf("[自动上传] 获取弹幕烧录分P房间配置失败: part_id=%d, room_id=%s, error=%v",
					part.ID, part.RoomID, err)
				continue
			}

			if !room.Upload || !hasUploadUserOrStrategy(&room) {
				continue
			}
			if ok, window := isRoomUploadWindowOpen(&room, time.Now()); !ok {
				log.Printf("[自动上传] 弹幕烧录分P房间 %s 当前不在上传时间窗口(%s)，跳过本轮", room.RoomID, window)
				continue
			}

			log.Printf("[自动上传] 发现上传失败的弹幕烧录分P，重新入队: part_id=%d, file=%s", part.ID, part.FilePath)
			tasks = append(tasks, PendingUploadTask{
				Part:    part,
				History: history,
				Room:    room,
			})
		}
	}

	// 额外：补充扫描所有上传失败的大文件切分分P（temp_file_type=split）
	// splitLargeFile 创建子分P并直接入队，若上传失败后服务未重启则不会被自动重试。
	// 注意：切分分P现为 is_temp_file=false，使用 temp_file_type='split' 作为判据。
	// 此处每10分钟周期调度时主动发现并重新入队（正常扫描也能覆盖，此处作为双重保障）。
	var failedSplitParts []models.RecordHistoryPart
	if err := db.Where(
		"temp_file_type = ? AND upload = ? AND uploading = ? AND file_delete = ? AND upload_paused = ? AND upload_cancelled = ?",
		"split", false, false, false, false, false,
	).Find(&failedSplitParts).Error; err != nil {
		log.Printf("[自动上传] 查询失败的split分P失败: %v", err)
	} else {
		for _, part := range failedSplitParts {
			if part.FilePath == "" {
				continue
			}

			// 检查物理文件是否仍然存在（如果不存在则跳过，由 CleanOrphanedTempParts 处理）
			if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
				continue
			}

			// 跳过速率限制冷却期中的分P
			if part.RateLimitCooldownAt != nil && time.Now().Before(*part.RateLimitCooldownAt) {
				continue
			}

			// 获取对应的历史记录
			var history models.RecordHistory
			if err := db.First(&history, part.HistoryID).Error; err != nil {
				log.Printf("[自动上传] 获取split分P历史记录失败: part_id=%d, history_id=%d, error=%v",
					part.ID, part.HistoryID, err)
				continue
			}

			if !history.Upload {
				continue
			}

			var room models.RecordRoom
			if err := db.Where("room_id = ?", part.RoomID).First(&room).Error; err != nil {
				log.Printf("[自动上传] 获取split分P房间配置失败: part_id=%d, room_id=%s, error=%v",
					part.ID, part.RoomID, err)
				continue
			}

			if !room.Upload || !hasUploadUserOrStrategy(&room) {
				continue
			}
			if ok, window := isRoomUploadWindowOpen(&room, time.Now()); !ok {
				log.Printf("[自动上传] split分P房间 %s 当前不在上传时间窗口(%s)，跳过本轮", room.RoomID, window)
				continue
			}

			log.Printf("[自动上传] 发现上传失败的split分P，重新入队: part_id=%d, file=%s", part.ID, part.FilePath)
			tasks = append(tasks, PendingUploadTask{
				Part:    part,
				History: history,
				Room:    room,
			})
		}
	}

	return tasks, nil
}

func isRoomUploadWindowOpen(room *models.RecordRoom, now time.Time) (bool, string) {
	if room == nil || !room.UploadWindowEnabled {
		return true, ""
	}

	start, startText := parseRoomUploadClock(room.UploadWindowStart, "00:00")
	end, endText := parseRoomUploadClock(room.UploadWindowEnd, "23:59")
	window := startText + "-" + endText
	if start == end {
		return true, window
	}

	current := now.Hour()*60 + now.Minute()
	if start < end {
		return current >= start && current <= end, window
	}
	return current >= start || current <= end, window
}

func hasUploadUserOrStrategy(room *models.RecordRoom) bool {
	if room == nil {
		return false
	}
	if room.UploadUserID != 0 {
		return true
	}
	switch strings.TrimSpace(room.UploadUserStrategy) {
	case "round_robin", "least_queue":
		return true
	default:
		return false
	}
}

func parseRoomUploadClock(value, fallback string) (int, string) {
	text := strings.TrimSpace(value)
	if text == "" {
		text = fallback
	}
	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		text = fallback
		parts = strings.Split(text, ":")
	}
	hour, errHour := strconv.Atoi(parts[0])
	minute, errMinute := strconv.Atoi(parts[1])
	if errHour != nil || errMinute != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		text = fallback
		parts = strings.Split(text, ":")
		hour, _ = strconv.Atoi(parts[0])
		minute, _ = strconv.Atoi(parts[1])
	}
	return hour*60 + minute, text
}

// isFileStable 检查文件是否已稳定（修改时间超过指定时长）
func (s *AutoUploadService) isFileStable(filePath string, stableDuration time.Duration) bool {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("[自动上传] 无法获取文件信息: %s, error=%v", filePath, err)
		return false
	}

	timeSinceModified := time.Since(fileInfo.ModTime())
	if timeSinceModified < stableDuration {
		log.Printf("[自动上传] 文件修改时间过近: %s, 距离上次修改%.1f分钟 < %.1f分钟",
			filePath, timeSinceModified.Minutes(), stableDuration.Minutes())
		return false
	}

	return true
}

// isLiveStreamInProgress 检查该分P所属的直播是否仍在进行中
// 判断依据：
// 1. 查询同一SessionID的所有分P（包括正在录制的），检查最后一个分P的结束时间
// 2. 如果最后一个分P的结束时间距离现在不超过10分钟，认为直播仍在进行
// 3. 如果该SessionID的历史记录标记为正在录制/直播，则直播仍在进行
func (s *AutoUploadService) isLiveStreamInProgress(part *models.RecordHistoryPart, db *gorm.DB) bool {
	// 1. 检查该SessionID的历史记录状态
	var history models.RecordHistory
	err := db.Where("session_id = ? AND room_id = ?", part.SessionID, part.RoomID).
		First(&history).Error

	if err == nil {
		// 找到历史记录，检查是否正在录制/直播
		if history.Recording || history.Streaming {
			log.Printf("[自动上传] 检测到直播仍在进行（历史记录标记）: session_id=%s, history_id=%d, Recording=%v, Streaming=%v",
				part.SessionID, history.ID, history.Recording, history.Streaming)
			return true
		}
	}

	// 2. 查询同一SessionID的所有分P（包括正在录制的），按结束时间倒序
	var latestPart models.RecordHistoryPart
	err = db.Where("session_id = ? AND room_id = ?", part.SessionID, part.RoomID).
		Order("end_time DESC").
		First(&latestPart).Error

	if err != nil {
		// 没有找到同SessionID的分P，可能是第一个分P或数据异常
		return false
	}

	// 3. 检查最后一个分P的结束时间距离现在是否不超过10分钟
	// 注意：需要排除当前分P本身，避免误判
	if latestPart.ID != part.ID {
		timeSinceLastPart := time.Since(latestPart.EndTime)
		if timeSinceLastPart <= 10*time.Minute {
			log.Printf("[自动上传] 检测到直播可能仍在进行（最后分P间隔）: session_id=%s, 最后分P(id=%d)结束于%.1f分钟前",
				part.SessionID, latestPart.ID, timeSinceLastPart.Minutes())
			return true
		}
	}

	// 4. 检查是否有正在录制的分P
	var recordingPart models.RecordHistoryPart
	err = db.Where("session_id = ? AND room_id = ? AND recording = ?", part.SessionID, part.RoomID, true).
		First(&recordingPart).Error

	if err == nil {
		log.Printf("[自动上传] 检测到有正在录制的分P: session_id=%s, part_id=%d",
			part.SessionID, recordingPart.ID)
		return true
	}

	return false
}

// PendingUploadTask 待上传任务
type PendingUploadTask struct {
	Part    models.RecordHistoryPart
	History models.RecordHistory
	Room    models.RecordRoom
}

// CleanOrphanedTempParts 清理孤立的临时分P记录（文件已不存在但DB记录仍为未上传状态）
// 典型场景：弹幕烧录失败/被手动删除，导致 is_temp_file=true 的分P记录永远卡在 upload=false
func (s *AutoUploadService) CleanOrphanedTempParts() {
	db := database.GetDB()

	var stuckParts []models.RecordHistoryPart
	if err := db.Where("is_temp_file = ? AND upload = ? AND uploading = ? AND file_delete = ?",
		true, false, false, false).Find(&stuckParts).Error; err != nil {
		log.Printf("[自动上传] 查询孤立临时分P失败: %v", err)
		return
	}

	if len(stuckParts) == 0 {
		return
	}

	cleaned := 0
	for _, part := range stuckParts {
		if part.FilePath == "" {
			continue
		}
		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			log.Printf("[自动上传] 临时分P文件不存在，标记为已删除: part_id=%d, file=%s", part.ID, part.FilePath)
			part.FileDelete = true
			part.UploadErrorMsg = "临时文件不存在，已自动清理"
			part.UploadErrorType = "file"
			db.Save(&part)
			cleaned++
		}
	}

	if cleaned > 0 {
		log.Printf("[自动上传] 清理孤立临时分P完成: 共清理 %d 条记录", cleaned)
	}
}
