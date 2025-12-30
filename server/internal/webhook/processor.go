package webhook

import (
"encoding/json"
"fmt"
"log"
"path/filepath"
"time"

"github.com/gobup/server/internal/database"
"github.com/gobup/server/internal/models"
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
	case FileOpened:
		return p.handleFileOpened(event)
	case FileClosed:
		return p.handleFileClosed(event)
	default:
		log.Printf("未处理的事件类型: %s", event.EventType)
	}

	return nil
}

func (p *Processor) handleFileOpened(event WebhookEvent) error {
	var data FileEventData
	if err := json.Unmarshal(event.EventData, &data); err != nil {
		return err
	}

	log.Printf("录制开始: 房间%d - %s", data.RoomID, data.Title)

	var room models.RecordRoom
	db := database.GetDB()

	roomID := fmt.Sprintf("%d", data.RoomID)
	if err := db.Where("room_id = ?", roomID).First(&room).Error; err != nil {
		room = models.RecordRoom{
			RoomID: roomID,
			Uname:  data.Name,
			Title:  data.Title,
			Upload: true,
		}
		db.Create(&room)
	}

	history := models.RecordHistory{
		RoomID:    roomID,
		SessionID: data.SessionID,
		Title:     data.Title,
		StartTime: time.Now(),
		Recording: true,
	}
	db.Create(&history)

	return nil
}

func (p *Processor) handleFileClosed(event WebhookEvent) error {
	var data FileEventData
	if err := json.Unmarshal(event.EventData, &data); err != nil {
		return err
	}

	log.Printf("录制结束: 房间%d - %s, 文件: %s", data.RoomID, data.Title, data.FilePath)

	db := database.GetDB()
	roomID := fmt.Sprintf("%d", data.RoomID)

	var history models.RecordHistory
	if err := db.Where("session_id = ?", data.SessionID).First(&history).Error; err != nil {
		history = models.RecordHistory{
			RoomID:    roomID,
			SessionID: data.SessionID,
			Title:     data.Title,
			StartTime: time.Now().Add(-time.Hour),
		}
		db.Create(&history)
	}

	history.EndTime = time.Now()
	history.Recording = false
	db.Save(&history)

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
		EndTime:   history.EndTime,
		Recording: false,
		Upload:    false,
	}
	db.Create(&part)

	var room models.RecordRoom
	if err := db.Where("room_id = ?", roomID).First(&room).Error; err == nil {
		if room.Upload {
			go p.uploadService.UploadPart(&part, &history, &room)
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

		if room.Upload {
			go p.uploadService.UploadPart(&part, &history, &room)
		}
	}

	return nil
}
