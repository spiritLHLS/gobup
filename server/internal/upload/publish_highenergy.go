package upload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

func (s *Service) createAndQueueHighEnergyClip(sourceHistoryID uint) {
	db := database.GetDB()

	var sourceHistory models.RecordHistory
	if err := db.First(&sourceHistory, sourceHistoryID).Error; err != nil {
		log.Printf("[高能剪辑] 获取源历史记录失败: history_id=%d, err=%v", sourceHistoryID, err)
		return
	}
	if sourceHistory.IsHighlight {
		return
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", sourceHistory.RoomID).First(&room).Error; err != nil {
		log.Printf("[高能剪辑] 获取房间配置失败: room_id=%s, err=%v", sourceHistory.RoomID, err)
		return
	}
	if !room.Upload {
		log.Printf("[高能剪辑] 房间未启用上传，跳过高光稿件入队: room_id=%s", room.RoomID)
		return
	}
	if !roomHasUploadUserOrStrategy(&room) {
		log.Printf("[高能剪辑] 房间未配置上传账号或账号策略，跳过高光稿件入队: room_id=%s", room.RoomID)
		return
	}

	log.Printf("[高能剪辑] 开始生成高光稿件: source_history_id=%d", sourceHistoryID)
	outputFile, err := services.NewHighEnergyCutService().CutHighEnergySegments(sourceHistoryID)
	if err != nil {
		log.Printf("[高能剪辑] 生成失败: source_history_id=%d, err=%v", sourceHistoryID, err)
		return
	}

	fileInfo, err := os.Stat(outputFile)
	if err != nil {
		log.Printf("[高能剪辑] 输出文件不可用: file=%s, err=%v", outputFile, err)
		return
	}

	now := time.Now()
	startTime := sourceHistory.StartTime
	if startTime.IsZero() {
		startTime = now
	}
	endTime := sourceHistory.EndTime
	if endTime.IsZero() || !endTime.After(startTime) {
		endTime = startTime.Add(time.Second)
	}

	title := strings.TrimSpace(sourceHistory.Title)
	if title == "" {
		title = "高能剪辑"
	} else {
		title = title + " - 高能剪辑"
	}

	sessionBase := strings.TrimSpace(sourceHistory.SessionID)
	if sessionBase == "" {
		sessionBase = fmt.Sprintf("%s_%d", sourceHistory.RoomID, sourceHistory.ID)
	}
	highlightSessionID := fmt.Sprintf("%s_highlight_%d", sessionBase, now.UnixNano())

	highlightHistory := models.RecordHistory{
		EventID:      fmt.Sprintf("highlight_%d", sourceHistory.ID),
		RoomID:       sourceHistory.RoomID,
		SessionID:    highlightSessionID,
		Uname:        sourceHistory.Uname,
		Title:        title,
		AreaName:     sourceHistory.AreaName,
		StartTime:    startTime,
		EndTime:      endTime,
		Upload:       true,
		Publish:      false,
		Recording:    false,
		Streaming:    false,
		FilePath:     outputFile,
		FileSize:     fileInfo.Size(),
		UploadStatus: 0,
		CoverURL:     sourceHistory.CoverURL,
		IsHighlight:  true,
		Message:      fmt.Sprintf("由历史记录 %d 自动生成的高能剪辑", sourceHistory.ID),
	}
	if err := db.Create(&highlightHistory).Error; err != nil {
		log.Printf("[高能剪辑] 创建高光历史记录失败: file=%s, err=%v", outputFile, err)
		return
	}

	highlightPart := models.RecordHistoryPart{
		HistoryID:    highlightHistory.ID,
		RoomID:       highlightHistory.RoomID,
		SessionID:    highlightHistory.SessionID,
		Title:        title,
		LiveTitle:    sourceHistory.Title,
		AreaName:     sourceHistory.AreaName,
		FilePath:     outputFile,
		FileName:     filepath.Base(outputFile),
		FileSize:     fileInfo.Size(),
		Duration:     int(endTime.Sub(startTime).Seconds()),
		StartTime:    startTime,
		EndTime:      endTime,
		Recording:    false,
		Upload:       false,
		Uploading:    false,
		Page:         1,
		TempFileType: "high_energy",
	}
	if err := db.Create(&highlightPart).Error; err != nil {
		log.Printf("[高能剪辑] 创建高光分P失败: history_id=%d, file=%s, err=%v", highlightHistory.ID, outputFile, err)
		return
	}

	if err := s.UploadPart(&highlightPart, &highlightHistory, &room); err != nil {
		log.Printf("[高能剪辑] 高光分P入队失败: part_id=%d, err=%v", highlightPart.ID, err)
		return
	}
	log.Printf("[高能剪辑] 高光稿件已创建并加入上传队列: history_id=%d, part_id=%d, file=%s",
		highlightHistory.ID, highlightPart.ID, outputFile)
}
