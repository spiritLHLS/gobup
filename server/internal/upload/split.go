package upload

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

// splitLargeFile 将大文件分割成多个Part（当分片数超过10000时）
func (s *Service) splitLargeFile(originalPart *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) error {
	db := database.GetDB()

	fileInfo, err := os.Stat(originalPart.FilePath)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	var chunkSize int64
	switch room.Line {
	case "app":
		chunkSize = 2 * 1024 * 1024
	default:
		chunkSize = 5 * 1024 * 1024
	}

	totalChunks := bili.CalculateChunkCount(fileInfo.Size(), chunkSize)
	maxChunksPerPart := int64(9000)
	numParts := (totalChunks + maxChunksPerPart - 1) / maxChunksPerPart

	totalDuration := originalPart.Duration
	if totalDuration == 0 {
		log.Printf("[自动分P] 警告：原始Part没有时长信息，将平均分割")
		totalDuration = int(numParts) * 3600
	}

	durationPerPart := totalDuration / int(numParts)
	log.Printf("[自动分P] 将文件分割成 %d 个Part，每个Part约 %d 秒", numParts, durationPerPart)

	baseDir := filepath.Dir(originalPart.FilePath)
	baseNameWithoutExt := strings.TrimSuffix(filepath.Base(originalPart.FilePath), filepath.Ext(originalPart.FilePath))
	ext := filepath.Ext(originalPart.FilePath)

	var newParts []*models.RecordHistoryPart
	cleanupOnErr := true
	defer func() {
		if !cleanupOnErr {
			return
		}
		for _, np := range newParts {
			if np.FilePath != "" {
				_ = os.Remove(np.FilePath)
			}
			if np.ID != 0 {
				db.Delete(np)
			}
		}
	}()

	for i := int64(0); i < numParts; i++ {
		startTime := int(i) * durationPerPart
		duration := durationPerPart
		if i == numParts-1 {
			duration = totalDuration - startTime
		}

		outputFileName := fmt.Sprintf("%s_part%d%s", baseNameWithoutExt, i+1, ext)
		outputPath := filepath.Join(baseDir, outputFileName)

		ffmpegArgs := []string{"-y"}
		if startTime > 0 {
			ffmpegArgs = append(ffmpegArgs, "-ss", fmt.Sprintf("%d", startTime))
		}
		ffmpegArgs = append(ffmpegArgs,
			"-i", originalPart.FilePath,
			"-t", fmt.Sprintf("%d", duration),
			"-c", "copy",
			"-avoid_negative_ts", "1",
			outputPath,
		)

		log.Printf("[自动分P] 正在切割Part %d/%d: %s (时长: %ds)", i+1, numParts, outputFileName, duration)
		cmd := exec.Command("ffmpeg", ffmpegArgs...)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[自动分P] ffmpeg输出: %s", string(output))
			return fmt.Errorf("ffmpeg切割失败 (Part %d): %w", i+1, err)
		}

		splitFileInfo, err := os.Stat(outputPath)
		if err != nil {
			return fmt.Errorf("获取切割文件信息失败: %w", err)
		}

		newPart := &models.RecordHistoryPart{
			HistoryID:    originalPart.HistoryID,
			RoomID:       originalPart.RoomID,
			SessionID:    originalPart.SessionID,
			Title:        originalPart.Title,
			LiveTitle:    originalPart.LiveTitle,
			AreaName:     originalPart.AreaName,
			FilePath:     outputPath,
			FileName:     outputFileName,
			FileSize:     splitFileInfo.Size(),
			Duration:     duration,
			StartTime:    originalPart.StartTime.Add(time.Duration(startTime) * time.Second),
			EndTime:      originalPart.StartTime.Add(time.Duration(startTime+duration) * time.Second),
			Recording:    false,
			Upload:       false,
			Uploading:    false,
			Page:         0,
			XcodeState:   0,
			IsTempFile:   false,
			SourcePartID: originalPart.ID,
			TempFileType: "split",
		}

		if err := db.Create(newPart).Error; err != nil {
			return fmt.Errorf("创建新Part记录失败: %w", err)
		}

		newParts = append(newParts, newPart)
		log.Printf("[自动分P] 创建Part %d/%d 成功: id=%d, size=%d, duration=%d", i+1, numParts, newPart.ID, newPart.FileSize, newPart.Duration)
	}

	if err := os.Remove(originalPart.FilePath); err != nil {
		log.Printf("[自动分P] 删除原始文件失败: %v (文件: %s)", err, originalPart.FilePath)
	} else {
		log.Printf("[自动分P] 原始文件已删除: %s", originalPart.FilePath)
	}

	originalPart.Upload = false
	originalPart.FileDelete = true
	originalPart.UploadErrorMsg = fmt.Sprintf("文件过大(分片数%d>10000)，已自动分割成%d个子分P", totalChunks, numParts)
	originalPart.UploadErrorType = UploadErrorTypeFile
	db.Save(originalPart)

	log.Printf("[自动分P] 文件分割完成，已创建 %d 个新Part，原Part(id=%d)标记为已处理", len(newParts), originalPart.ID)

	for _, newPart := range newParts {
		if err := s.UploadPart(newPart, history, room); err != nil {
			log.Printf("[自动分P] 将新Part(id=%d)加入上传队列失败: %v", newPart.ID, err)
		} else {
			log.Printf("[自动分P] 新Part(id=%d)已加入上传队列", newPart.ID)
		}
	}

	cleanupOnErr = false
	return nil
}
