package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

type FileMoverService struct{}

func NewFileMoverService() *FileMoverService {
	return &FileMoverService{}
}

// MoveFilesForHistory 移动历史记录的所有相关文件
func (s *FileMoverService) MoveFilesForHistory(historyID uint) error {
	db := database.GetDB()

	// 获取历史记录
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	if history.FilesMoved {
		return fmt.Errorf("文件已移动")
	}

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间配置不存在: %w", err)
	}

	if room.MoveDir == "" {
		return fmt.Errorf("未配置目标目录")
	}

	// 获取所有分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	movedFiles := 0
	errors := []string{}

	for _, part := range parts {
		if part.FileMoved || part.FileDelete {
			continue
		}

		if part.FilePath == "" {
			continue
		}

		// 检查源文件是否存在
		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			log.Printf("文件不存在，跳过: %s", part.FilePath)
			part.FileDelete = true
			db.Save(&part)
			continue
		}

		// 构建目标路径
		sourceDir := filepath.Dir(part.FilePath)
		baseName := filepath.Base(part.FilePath)
		fileName := strings.TrimSuffix(baseName, filepath.Ext(baseName))

		// 目标目录结构: moveDir/roomId/sessionId/
		targetDir := filepath.Join(room.MoveDir, room.RoomID, history.SessionID)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			errors = append(errors, fmt.Sprintf("创建目录失败 %s: %v", targetDir, err))
			continue
		}

		// 移动视频文件
		targetPath := filepath.Join(targetDir, baseName)
		if err := s.moveFile(part.FilePath, targetPath); err != nil {
			errors = append(errors, fmt.Sprintf("移动文件失败 %s: %v", baseName, err))
			continue
		}

		// 移动相关文件（弹幕、封面等）
		s.moveRelatedFiles(sourceDir, targetDir, fileName)

		// 更新记录
		part.FileMoved = true
		part.FileDelete = false
		db.Save(&part)
		movedFiles++

		log.Printf("文件已移动: %s -> %s", part.FilePath, targetPath)
	}

	// 更新历史记录
	if movedFiles > 0 {
		history.FilesMoved = true
		db.Save(&history)
		log.Printf("历史记录 %d 的文件移动完成，共移动 %d 个文件", historyID, movedFiles)
	}

	if len(errors) > 0 {
		return fmt.Errorf("部分文件移动失败: %s", strings.Join(errors, "; "))
	}

	return nil
}

// moveFile 移动文件
func (s *FileMoverService) moveFile(src, dst string) error {
	// 先尝试重命名（同一文件系统下更快）
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// 如果重命名失败（跨文件系统），则复制后删除
	if err := s.copyFile(src, dst); err != nil {
		return err
	}

	return os.Remove(src)
}

// copyFile 复制文件
func (s *FileMoverService) copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	buf := make([]byte, 1024*1024) // 1MB buffer
	for {
		n, err := source.Read(buf)
		if n > 0 {
			if _, err := destination.Write(buf[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
	}

	return nil
}

// moveRelatedFiles 移动相关文件（xml弹幕、封面等）
func (s *FileMoverService) moveRelatedFiles(sourceDir, targetDir, baseName string) {
	// 常见的相关文件扩展名
	extensions := []string{".xml", ".jpg", ".png", ".json", ".txt"}

	for _, ext := range extensions {
		sourceFile := filepath.Join(sourceDir, baseName+ext)
		if _, err := os.Stat(sourceFile); err == nil {
			targetFile := filepath.Join(targetDir, baseName+ext)
			if err := s.moveFile(sourceFile, targetFile); err != nil {
				log.Printf("移动相关文件失败 %s: %v", sourceFile, err)
			} else {
				log.Printf("移动相关文件: %s", filepath.Base(sourceFile))
			}
		}
	}
}

// AutoMoveFiles 自动移动已完成投稿的文件（定时任务调用）
func (s *FileMoverService) AutoMoveFiles() error {
	db := database.GetDB()

	// 查找需要移动的历史记录
	var histories []models.RecordHistory
	if err := db.Where("publish = ? AND files_moved = ? AND bv_id != ?", true, false, "").
		Find(&histories).Error; err != nil {
		return err
	}

	log.Printf("发现 %d 个需要移动文件的历史记录", len(histories))

	for _, history := range histories {
		if err := s.MoveFilesForHistory(history.ID); err != nil {
			log.Printf("移动历史记录 %d 的文件失败: %v", history.ID, err)
		}
		time.Sleep(time.Second) // 避免IO过载
	}

	return nil
}
