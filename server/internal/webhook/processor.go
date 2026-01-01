package webhook

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
	"github.com/gobup/server/internal/upload"
)

type EventType string

const (
	FileOpening    EventType = "FileOpening"
	FileOpened     EventType = "FileOpened"
	FileClosed     EventType = "FileClosed"
	SessionStarted EventType = "SessionStarted"
	SessionEnded   EventType = "SessionEnded"
)

type WebhookEvent struct {
	EventType      EventType       `json:"EventType"`
	EventTimestamp string          `json:"EventTimestamp"`
	EventID        string          `json:"EventId"`
	EventData      json.RawMessage `json:"EventData"`
}

type FileEventData struct {
	RelativePath   string `json:"RelativePath"`
	FileOpenTime   string `json:"FileOpenTime"`
	FileCloseTime  string `json:"FileCloseTime"`
	FilePath       string `json:"FilePath"`
	SessionID      string `json:"SessionId"`
	RoomID         int    `json:"RoomId"`
	ShortID        int    `json:"ShortId"`
	Name           string `json:"Name"`
	Title          string `json:"Title"`
	AreaNameParent string `json:"AreaNameParent"`
	AreaNameChild  string `json:"AreaNameChild"`
	FileSize       int64  `json:"FileSize"`
}

type BlrecEvent struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Date      string          `json:"date"`
	Timestamp int64           `json:"timestamp"`
}

type BlrecVideoData struct {
	RoomID    int    `json:"room_id"`
	RoomTitle string `json:"room_title"`
	Username  string `json:"username"`
	VideoPath string `json:"path"`
	Size      int64  `json:"size"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

type Processor struct {
	uploadService *upload.Service
}

func NewProcessor() *Processor {
	return &Processor{
		uploadService: upload.NewService(),
	}
}

func (p *Processor) Process(eventData interface{}) error {
	jsonData, _ := json.Marshal(eventData)
	var event WebhookEvent
	if err := json.Unmarshal(jsonData, &event); err != nil {
		return p.processBlrecEvent(jsonData)
	}

	switch event.EventType {
	case SessionStarted:
		return p.handleSessionStarted(event)
	case SessionEnded:
		return p.handleSessionEnded(event)
	case FileOpened:
		return p.handleFileOpened(event)
	case FileClosed:
		return p.handleFileClosed(event)
	default:
		log.Printf("未处理的事件类型: %s", event.EventType)
	}

	return nil
}

func (p *Processor) handleSessionStarted(event WebhookEvent) error {
	var data FileEventData
	if err := json.Unmarshal(event.EventData, &data); err != nil {
		return err
	}

	log.Printf("[INFO] 录制会话开始: 房间%d - %s, SessionID=%s", data.RoomID, data.Title, data.SessionID)

	db := database.GetDB()
	roomID := fmt.Sprintf("%d", data.RoomID)

	// 查找或创建房间
	var room models.RecordRoom
	if err := db.Where("room_id = ?", roomID).First(&room).Error; err != nil {
		room = models.RecordRoom{
			RoomID: roomID,
			Uname:  data.Name,
			Title:  data.Title,
			Upload: true,
		}
		if err := db.Create(&room).Error; err != nil {
			log.Printf("[ERROR] 创建房间失败: %v", err)
			return err
		}
		log.Printf("[INFO] 创建新房间: RoomID=%s, Uname=%s", roomID, data.Name)
	} else {
		// 更新房间信息
		room.Uname = data.Name
		room.Title = data.Title
		room.SessionID = data.SessionID
		room.Recording = true
		db.Save(&room)
		log.Printf("[INFO] 更新房间信息: RoomID=%s", roomID)
	}

	// 检查是否已存在该 SessionID 的历史记录
	var existingHistory models.RecordHistory
	if err := db.Where("session_id = ?", data.SessionID).First(&existingHistory).Error; err == nil {
		log.Printf("[WARN] 历史记录已存在，跳过创建: SessionID=%s, HistoryID=%d", data.SessionID, existingHistory.ID)
		// 更新 room 的 historyId 关联
		room.HistoryID = existingHistory.ID
		db.Save(&room)
		return nil
	}

	// 创建新的历史记录
	startTime := time.Now()
	history := models.RecordHistory{
		RoomID:    roomID,
		SessionID: data.SessionID,
		EventID:   event.EventID,
		Title:     data.Title,
		Uname:     data.Name,
		AreaName:  data.AreaNameChild,
		StartTime: startTime,
		EndTime:   startTime,
		Recording: true,
		Streaming: true,
		Upload:    room.Upload,
	}

	if err := db.Create(&history).Error; err != nil {
		log.Printf("[ERROR] 创建历史记录失败: %v", err)
		return err
	}

	log.Printf("[INFO] 成功创建历史记录: ID=%d, SessionID=%s, RoomID=%s", history.ID, history.SessionID, roomID)

	// 更新房间的 historyId 关联
	room.HistoryID = history.ID
	db.Save(&room)

	return nil
}

func (p *Processor) handleSessionEnded(event WebhookEvent) error {
	var data FileEventData
	if err := json.Unmarshal(event.EventData, &data); err != nil {
		return err
	}

	log.Printf("[INFO] 录制会话结束: 房间%d - %s, SessionID=%s", data.RoomID, data.Title, data.SessionID)

	db := database.GetDB()
	roomID := fmt.Sprintf("%d", data.RoomID)

	// 查找历史记录
	var history models.RecordHistory
	if err := db.Where("session_id = ?", data.SessionID).First(&history).Error; err != nil {
		log.Printf("[WARN] 未找到对应的历史记录: SessionID=%s", data.SessionID)
		return nil
	}

	// 更新历史记录状态
	history.EndTime = time.Now()
	history.Recording = false
	history.Streaming = false
	if err := db.Save(&history).Error; err != nil {
		log.Printf("[ERROR] 更新历史记录失败: %v", err)
		return err
	}

	log.Printf("[INFO] 成功更新历史记录结束状态: HistoryID=%d", history.ID)

	// 更新房间状态
	var room models.RecordRoom
	if err := db.Where("room_id = ?", roomID).First(&room).Error; err == nil {
		room.Recording = false
		room.Streaming = false
		room.SessionID = ""
		db.Save(&room)
	}

	return nil
}

func (p *Processor) handleFileOpened(event WebhookEvent) error {
	var data FileEventData
	if err := json.Unmarshal(event.EventData, &data); err != nil {
		return err
	}

	log.Printf("[INFO] 文件开始录制: 房间%d - %s, 文件: %s, EventID=%s", data.RoomID, data.Title, data.RelativePath, event.EventID)

	// 检查是否已处理过该事件（防止重复）
	db := database.GetDB()
	if event.EventID != "" {
		var existingHistory models.RecordHistory
		if err := db.Where("event_id = ?", event.EventID).First(&existingHistory).Error; err == nil {
			log.Printf("[WARN] FileOpened事件已处理过，跳过: EventID=%s, HistoryID=%d", event.EventID, existingHistory.ID)
			return nil
		}
	}

	roomID := fmt.Sprintf("%d", data.RoomID)

	// 查找或创建房间
	var room models.RecordRoom
	if err := db.Where("room_id = ?", roomID).First(&room).Error; err != nil {
		room = models.RecordRoom{
			RoomID: roomID,
			Uname:  data.Name,
			Title:  data.Title,
			Upload: true,
		}
		db.Create(&room)
		log.Printf("[INFO] 创建新房间: RoomID=%s", roomID)
	}

	// 查找对应的历史记录
	var history models.RecordHistory
	if err := db.Where("session_id = ?", data.SessionID).First(&history).Error; err != nil {
		// 如果没有找到，创建一个新的历史记录（兼容没有 SessionStarted 事件的情况）
		log.Printf("[WARN] 未找到SessionID对应的历史记录，创建新记录: SessionID=%s", data.SessionID)
		startTime := time.Now()
		if data.FileOpenTime != "" {
			if t, err := time.Parse(time.RFC3339, data.FileOpenTime); err == nil {
				startTime = t
			}
		}
		history = models.RecordHistory{
			RoomID:    roomID,
			SessionID: data.SessionID,
			EventID:   data.SessionID,
			Title:     data.Title,
			Uname:     data.Name,
			AreaName:  data.AreaNameChild,
			StartTime: startTime,
			EndTime:   startTime,
			Recording: true,
			Upload:    room.Upload,
		}
		if err := db.Create(&history).Error; err != nil {
			log.Printf("[ERROR] 创建历史记录失败: %v", err)
			return err
		}
		log.Printf("[INFO] 成功创建历史记录: ID=%d", history.ID)

		// 更新房间的 historyId 关联
		room.HistoryID = history.ID
		db.Save(&room)
	}

	return nil
}

func (p *Processor) handleFileClosed(event WebhookEvent) error {
	var data FileEventData
	if err := json.Unmarshal(event.EventData, &data); err != nil {
		log.Printf("[ERROR] 解析 FileClosed 事件数据失败: %v", err)
		return err
	}

	log.Printf("[INFO] 录制结束: 房间%d - %s, 文件: %s", data.RoomID, data.Title, data.FilePath)

	db := database.GetDB()
	roomID := fmt.Sprintf("%d", data.RoomID)

	log.Printf("[DEBUG] 开始处理 FileClosed 事件: RoomID=%s, SessionID=%s, FilePath=%s, EventID=%s", roomID, data.SessionID, data.FilePath, event.EventID)

	// 首先检查文件是否已经存在（避免重复导入）
	var existingPart models.RecordHistoryPart
	if err := db.Where("file_path = ?", data.FilePath).First(&existingPart).Error; err == nil {
		log.Printf("[WARN] 文件已存在，跳过重复导入: FilePath=%s, PartID=%d, HistoryID=%d", data.FilePath, existingPart.ID, existingPart.HistoryID)
		return nil // 不返回错误，因为这是正常的跳过情况
	}

	// 检查是否存在相同EventID的记录（防止重复事件）
	var existingPartByEvent models.RecordHistoryPart
	if event.EventID != "" && db.Where("file_path = ? OR (history_id IN (SELECT id FROM record_histories WHERE event_id = ?))", data.FilePath, event.EventID).First(&existingPartByEvent).Error == nil {
		log.Printf("[WARN] EventID重复，跳过: EventID=%s, ExistingPartID=%d", event.EventID, existingPartByEvent.ID)
		return nil
	}

	// 优先通过 SessionID 查找历史记录
	var history models.RecordHistory
	found := false
	if err := db.Where("session_id = ?", data.SessionID).First(&history).Error; err == nil {
		found = true
		log.Printf("[INFO] 通过SessionID找到历史记录: ID=%d, SessionID=%s", history.ID, data.SessionID)
	} else {
		// 如果通过 SessionID 找不到，尝试通过 room 的 historyId 查找
		var room models.RecordRoom
		if err := db.Where("room_id = ?", roomID).First(&room).Error; err == nil && room.HistoryID > 0 {
			if err := db.First(&history, room.HistoryID).Error; err == nil {
				// 检查时间是否合理（6小时内）
				if time.Since(history.EndTime) < 6*time.Hour {
					found = true
					log.Printf("[INFO] 通过Room.HistoryID找到历史记录: ID=%d", history.ID)
				} else {
					log.Printf("[WARN] Room.HistoryID对应的历史记录太旧: ID=%d, EndTime=%v", history.ID, history.EndTime)
				}
			}
		}

		// 如果还是找不到，尝试在最近6小时内查找同一房间的历史记录
		if !found {
			var recentHistories []models.RecordHistory
			sixHoursAgo := time.Now().Add(-6 * time.Hour)
			if err := db.Where("room_id = ? AND end_time >= ? AND recording = ?", roomID, sixHoursAgo, false).
				Order("end_time DESC").Limit(1).Find(&recentHistories).Error; err == nil && len(recentHistories) > 0 {
				history = recentHistories[0]
				found = true
				log.Printf("[INFO] 通过时间范围找到最近的历史记录: ID=%d, EndTime=%v", history.ID, history.EndTime)
			}
		}
	}

	if !found {
		log.Printf("[WARN] 未找到已有历史记录，创建新记录: SessionID=%s", data.SessionID)

		// 解析时间
		startTime := time.Now().Add(-time.Hour)
		if data.FileOpenTime != "" {
			if t, err := time.Parse(time.RFC3339, data.FileOpenTime); err == nil {
				startTime = t
				log.Printf("[DEBUG] 使用 FileOpenTime: %v", startTime)
			} else {
				log.Printf("[WARN] FileOpenTime 解析失败: %v, 使用默认时间", err)
			}
		}

		history = models.RecordHistory{
			RoomID:    roomID,
			SessionID: data.SessionID,
			EventID:   data.SessionID,
			Title:     data.Title,
			Uname:     data.Name,
			AreaName:  data.AreaNameChild,
			StartTime: startTime,
			Recording: false,
			Upload:    true,
		}
		if err := db.Create(&history).Error; err != nil {
			log.Printf("[ERROR] 创建历史记录失败: %v, RoomID=%s, SessionID=%s", err, roomID, data.SessionID)
			return err
		}
		log.Printf("[INFO] 成功创建历史记录: ID=%d, SessionID=%s", history.ID, data.SessionID)
	}

	// 解析结束时间
	endTime := time.Now()
	if data.FileCloseTime != "" {
		if t, err := time.Parse(time.RFC3339, data.FileCloseTime); err == nil {
			endTime = t
			log.Printf("[DEBUG] 使用 FileCloseTime: %v", endTime)
		} else {
			log.Printf("[WARN] FileCloseTime 解析失败: %v, 使用当前时间", err)
		}
	}

	history.EndTime = endTime
	history.Recording = false
	if err := db.Save(&history).Error; err != nil {
		log.Printf("[ERROR] 更新历史记录失败: %v, HistoryID=%d", err, history.ID)
		return err
	}
	log.Printf("[INFO] 成功更新历史记录: ID=%d", history.ID)

	part := models.RecordHistoryPart{
		HistoryID: history.ID,
		RoomID:    roomID,
		SessionID: data.SessionID,
		Title:     filepath.Base(data.FilePath),
		LiveTitle: data.Title,
		AreaName:  data.AreaNameChild,
		FilePath:  data.FilePath,
		FileName:  filepath.Base(data.FilePath),
		FileSize:  data.FileSize,
		StartTime: history.StartTime,
		EndTime:   endTime,
		Recording: false,
		Upload:    false,
	}
	if err := db.Create(&part).Error; err != nil {
		log.Printf("[ERROR] 创建分P记录失败: %v, FilePath=%s, HistoryID=%d", err, data.FilePath, history.ID)
		return err
	}
	log.Printf("[INFO] 成功创建分P记录: ID=%d, FilePath=%s, FileSize=%d", part.ID, part.FilePath, part.FileSize)

	var room models.RecordRoom
	if err := db.Where("room_id = ?", roomID).First(&room).Error; err == nil {
		// 处理录制完成后的文件策略 (DeleteType 1, 2)
		if room.DeleteType == 1 || room.DeleteType == 2 {
			fileMoverSvc := services.NewFileMoverService()
			if err := fileMoverSvc.ProcessFilesByStrategy(history.ID, room.DeleteType); err != nil {
				log.Printf("文件处理失败: %v", err)
			}
		}

		if room.Upload {
			if err := p.uploadService.UploadPart(&part, &history, &room); err != nil {
				log.Printf("添加上传任务到队列失败: %v", err)
			}
		}
	}

	return nil
}

func (p *Processor) processBlrecEvent(jsonData []byte) error {
	var event BlrecEvent
	if err := json.Unmarshal(jsonData, &event); err != nil {
		return err
	}

	if event.Type == "VideoFileCompleted" || event.Type == "video_file_completed" {
		var data BlrecVideoData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return err
		}

		log.Printf("Blrec录制完成: 房间%d - %s", data.RoomID, data.VideoPath)

		db := database.GetDB()
		roomID := fmt.Sprintf("%d", data.RoomID)

		var room models.RecordRoom
		if err := db.Where("room_id = ?", roomID).First(&room).Error; err != nil {
			room = models.RecordRoom{
				RoomID: roomID,
				Uname:  data.Username,
				Title:  data.RoomTitle,
				Upload: true,
			}
			db.Create(&room)
		}

		history := models.RecordHistory{
			RoomID:    roomID,
			Title:     data.RoomTitle,
			StartTime: time.Unix(data.StartTime, 0),
			EndTime:   time.Unix(data.EndTime, 0),
			Recording: false,
		}
		db.Create(&history)

		part := models.RecordHistoryPart{
			HistoryID: history.ID,
			RoomID:    roomID,
			Title:     filepath.Base(data.VideoPath),
			LiveTitle: data.RoomTitle,
			FilePath:  data.VideoPath,
			FileName:  filepath.Base(data.VideoPath),
			FileSize:  data.Size,
			StartTime: time.Unix(data.StartTime, 0),
			EndTime:   time.Unix(data.EndTime, 0),
			Recording: false,
			Upload:    false,
		}
		db.Create(&part)

		// 处理录制完成后的文件策略 (DeleteType 1, 2)
		if room.DeleteType == 1 || room.DeleteType == 2 {
			fileMoverSvc := services.NewFileMoverService()
			if err := fileMoverSvc.ProcessFilesByStrategy(history.ID, room.DeleteType); err != nil {
				log.Printf("文件处理失败: %v", err)
			}
		}

		if room.Upload {
			if err := p.uploadService.UploadPart(&part, &history, &room); err != nil {
				log.Printf("添加上传任务到队列失败: %v", err)
			}
		}
	}

	return nil
}
