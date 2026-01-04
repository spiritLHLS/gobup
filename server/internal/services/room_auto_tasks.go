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
	if err := db.Where("auto_sync_info = ? OR auto_send_danmaku = ? OR auto_parse_danmaku = ?",
		true, true, true).Find(&rooms).Error; err != nil {
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
	danmakuService := NewDanmakuService()

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

				// 4a. 自动发送弹幕（如果启用且未发送且有弹幕）
				if room.AutoSendDanmaku && !history.DanmakuSent && history.DanmakuCount > 0 {
					log.Printf("[房间自动任务] 自动发送弹幕: history_id=%d, 弹幕数=%d", history.ID, history.DanmakuCount)
					if err := danmakuService.SendDanmakuForHistory(history.ID); err != nil {
						log.Printf("[房间自动任务] 发送弹幕失败: %v", err)
					} else {
						log.Printf("[房间自动任务] 弹幕已加入发送队列")
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
