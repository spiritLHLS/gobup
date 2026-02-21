package services

import (
	"log"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

// RoomAutoTaskService 房间自动任务服务
type RoomAutoTaskService struct{}

func NewRoomAutoTaskService() *RoomAutoTaskService {
	return &RoomAutoTaskService{}
}

// ProcessRoomAutoTasks 处理所有房间的自动任务
// 每30分钟执行一次，检查需要处理的房间
func (s *RoomAutoTaskService) ProcessRoomAutoTasks() error {
	db := database.GetDB()

	// 查找启用了自动任务的房间
	var rooms []models.RecordRoom
	if err := db.Where("auto_sync_info = ? OR auto_parse_danmaku = ?",
		true, true).Find(&rooms).Error; err != nil {
		return err
	}

	if len(rooms) == 0 {
		log.Println("[房间自动任务] 没有启用自动任务的房间")
		return nil
	}

	log.Printf("[房间自动任务] 发现 %d 个启用了自动任务的房间", len(rooms))

	for _, room := range rooms {
		// 检查是否需要执行同步任务（每30分钟执行一次）
		needSync := room.AutoSyncInfo && s.shouldSyncRoom(&room)

		if needSync {
			log.Printf("[房间自动任务] 处理房间 %s (%s) 的自动任务", room.RoomID, room.Uname)
			s.processRoomTasks(&room)
		}
	}

	return nil
}

// shouldSyncRoom 判断房间是否需要同步
func (s *RoomAutoTaskService) shouldSyncRoom(room *models.RecordRoom) bool {
	// 如果从未同步过，需要同步
	if room.LastSyncTime == nil {
		return true
	}

	// 检查距离上次同步是否超过30分钟
	return time.Since(*room.LastSyncTime) >= 30*time.Minute
}

// processRoomTasks 处理单个房间的自动任务
func (s *RoomAutoTaskService) processRoomTasks(room *models.RecordRoom) {
	db := database.GetDB()

	// 1. 自动解析弹幕（处理所有未解析的历史记录）
	if room.AutoParseDanmaku {
		var unparsedHistories []models.RecordHistory
		if err := db.Where("room_id = ? AND danmaku_count = 0",
			room.RoomID).Find(&unparsedHistories).Error; err == nil && len(unparsedHistories) > 0 {

			log.Printf("[房间自动任务] 房间 %s 找到 %d 条未解析弹幕的历史记录", room.RoomID, len(unparsedHistories))
			danmakuParserService := NewDanmakuXMLParser()

			for _, history := range unparsedHistories {
				log.Printf("[房间自动任务] 自动解析弹幕: history_id=%d", history.ID)
				if count, err := danmakuParserService.ParseDanmakuForHistory(history.ID); err != nil {
					log.Printf("[房间自动任务] 解析弹幕失败: %v", err)
				} else if count > 0 {
					log.Printf("[房间自动任务] 解析弹幕成功: %d 条", count)
				}
			}
		}
	}

	// 2. 查找该房间所有已投稿但未审核通过的历史记录（用于同步）
	var histories []models.RecordHistory
	if err := db.Where("room_id = ? AND bv_id != '' AND bv_id IS NOT NULL AND video_state != ?",
		room.RoomID, 1).Find(&histories).Error; err != nil {
		log.Printf("[房间自动任务] 查询历史记录失败: %v", err)
		// 更新同步时间
		now := time.Now()
		room.LastSyncTime = &now
		db.Save(room)
		return
	}

	if len(histories) == 0 {
		log.Printf("[房间自动任务] 房间 %s 没有待同步的历史记录", room.RoomID)
		// 更新同步时间
		now := time.Now()
		room.LastSyncTime = &now
		db.Save(room)
		return
	}

	log.Printf("[房间自动任务] 房间 %s 找到 %d 条待同步的历史记录", room.RoomID, len(histories))

	videoSyncService := NewVideoSyncService()

	for _, history := range histories {
		// 3. 自动同步视频信息
		if room.AutoSyncInfo {
			log.Printf("[房间自动任务] 同步视频信息: history_id=%d, bv_id=%s", history.ID, history.BvID)

			oldState := history.VideoState
			if err := videoSyncService.SyncVideoInfo(history.ID); err != nil {
				log.Printf("[房间自动任务] 同步失败: %v", err)
				continue
			}

			// 重新获取历史记录，检查状态变化
			if err := db.First(&history, history.ID).Error; err != nil {
				log.Printf("[房间自动任务] 重新获取历史记录失败: %v", err)
				continue
			}

			// 4. 检查是否审核通过（从非通过状态变为通过状态）
			if oldState != 1 && history.VideoState == 1 {
				log.Printf("[房间自动任务] 视频审核通过: history_id=%d, bv_id=%s", history.ID, history.BvID)
				
				// 检查同SessionID是否有待追加的已上传分P
				if room.MergeBySession && history.SessionID != "" && history.Publish && room.AutoUpload {
					log.Printf("[房间自动任务] 审核通过，检查是否有待追加分P: session_id=%s", history.SessionID)
					
					// 查询同SessionID的其他历史记录（未投稿但有已上传分P的）
					var pendingHistories []models.RecordHistory
					if err := db.Where("session_id = ? AND publish = ? AND room_id = ? AND id != ?", 
						history.SessionID, false, room.RoomID, history.ID).Find(&pendingHistories).Error; err == nil {
						
						for _, pendingHistory := range pendingHistories {
						// 检查是否有已上传的分P（仅统计原始非临时分P，与checkAndPublish保持一致）
						var uploadedCount int64
						var totalCount int64
						var recordingCount int64
						
						db.Model(&models.RecordHistoryPart{}).Where(
							"history_id = ? AND upload = ? AND file_delete = ? AND is_temp_file = ?", 
							pendingHistory.ID, true, false, false).Count(&uploadedCount)
						db.Model(&models.RecordHistoryPart{}).Where(
							"history_id = ? AND is_temp_file = ?", pendingHistory.ID, false).Count(&totalCount)
							db.Model(&models.RecordHistoryPart{}).Where(
								"history_id = ? AND recording = ?", pendingHistory.ID, true).Count(&recordingCount)
							
							// 如果有已上传分P，且所有分P都上传完成，且没有正在录制的分P，则触发追加
							if uploadedCount > 0 && totalCount == uploadedCount && recordingCount == 0 {
							log.Printf("[房间自动任务] 发现可追加的历史记录: history_id=%d, 已上传分P=%d，触发追加投稿",
								pendingHistory.ID, uploadedCount)

							// Bug4修复: 原来只设置 UploadStatus=2 但没有代码会读取该字段并触发投稿，逻辑死环。
							// 现在通过注入的 TriggerPublish 回调直接触发投稿，
							// PublishHistory 内部会检测 MergeBySession 并自动追加到已有投稿。
							if TriggerPublish != nil {
								if err := TriggerPublish(pendingHistory.ID, room.UploadUserID); err != nil {
									log.Printf("[房间自动任务] 触发追加投稿失败: history_id=%d, error=%v", pendingHistory.ID, err)
								} else {
									log.Printf("[房间自动任务] 追加投稿成功: history_id=%d 已追加到 %s", pendingHistory.ID, history.BvID)
								}
							} else {
								log.Printf("[房间自动任务] TriggerPublish 未注册，跳过追加: history_id=%d", pendingHistory.ID)
							}
							}
						}
					}
				}
			}
		}
	}

	// 更新房间的最后同步时间
	now := time.Now()
	room.LastSyncTime = &now
	if err := db.Save(room).Error; err != nil {
		log.Printf("[房间自动任务] 更新同步时间失败: %v", err)
	}

	log.Printf("[房间自动任务] 房间 %s 处理完成", room.RoomID)
}
