package services

import (
	"fmt"
	"io"
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

// ProcessFilesByStrategy 根据策略处理文件
func (s *FileMoverService) ProcessFilesByStrategy(historyID uint, strategy int) error {
	switch strategy {
	case 0: // 不处理
		return nil
	case 1: // 上传前删除
		return s.deleteFiles(historyID)
	case 2: // 上传前移动
		return s.MoveFilesForHistory(historyID)
	case 3: // 上传后删除
		return s.deleteFiles(historyID)
	case 4: // 上传后移动
		return s.MoveFilesForHistory(historyID)
	case 5: // 上传前复制
		return s.copyFiles(historyID)
	case 6: // 上传后复制
		return s.copyFiles(historyID)
	case 7: // 上传完成后立即删除
		return s.deleteFiles(historyID)
	case 8: // N天后删除移动（需要定时任务支持）
		return s.scheduleDelayedDelete(historyID)
	case 9: // 投稿成功后删除
		return s.deleteFiles(historyID)
	case 10: // 投稿成功后移动
		return s.MoveFilesForHistory(historyID)
	case 11: // 审核通过后复制
		return s.copyFiles(historyID)
	default:
		return fmt.Errorf("未知的文件处理策略: %d", strategy)
	}
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

	_, err = io.Copy(destination, source)
	return err
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

// deleteFiles 删除历史记录的所有文件
func (s *FileMoverService) deleteFiles(historyID uint) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	// 获取所有分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	successCount := 0
	for _, part := range parts {
		if part.FileDelete {
			continue
		}

		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			part.FileDelete = true
			db.Save(&part)
			continue
		}

		if err := os.Remove(part.FilePath); err != nil {
			log.Printf("删除文件失败: %s, error: %v", part.FilePath, err)
			continue
		}

		// 删除相关文件
		s.deleteRelatedFiles(part.FilePath)

		part.FileDelete = true
		db.Save(&part)
		successCount++
		log.Printf("已删除文件: %s", part.FilePath)
	}

	history.FilesMoved = true
	db.Save(&history)

	log.Printf("历史记录 %d: 成功删除 %d/%d 个文件", historyID, successCount, len(parts))
	return nil
}

// copyFiles 复制历史记录的所有文件
func (s *FileMoverService) copyFiles(historyID uint) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间配置不存在: %w", err)
	}

	if room.MoveDir == "" {
		return fmt.Errorf("未配置目标目录")
	}

	// 创建目标目录
	if err := os.MkdirAll(room.MoveDir, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	successCount := 0
	for _, part := range parts {
		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			continue
		}

		fileName := filepath.Base(part.FilePath)
		targetPath := filepath.Join(room.MoveDir, fileName)

		if err := s.copyFile(part.FilePath, targetPath); err != nil {
			log.Printf("复制文件失败: %s -> %s, error: %v", part.FilePath, targetPath, err)
			continue
		}

		// 复制相关文件
		s.copyRelatedFiles(filepath.Dir(part.FilePath), room.MoveDir, strings.TrimSuffix(fileName, filepath.Ext(fileName)))

		successCount++
		log.Printf("已复制文件: %s -> %s", part.FilePath, targetPath)
	}

	log.Printf("历史记录 %d: 成功复制 %d/%d 个文件到 %s", historyID, successCount, len(parts), room.MoveDir)
	return nil
}

// scheduleDelayedDelete 计划延迟删除
func (s *FileMoverService) scheduleDelayedDelete(historyID uint) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间配置不存在: %w", err)
	}

	if room.DeleteDay <= 0 {
		return fmt.Errorf("未配置删除天数")
	}

	log.Printf("历史记录 %d 将在 %d 天后删除文件", historyID, room.DeleteDay)
	// 这里应该创建定时任务，简化实现
	return nil
}

// deleteRelatedFiles 删除相关文件
func (s *FileMoverService) deleteRelatedFiles(filePath string) {
	dir := filepath.Dir(filePath)
	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	extensions := []string{".xml", ".jpg", ".png", ".json", ".txt", ".ass", ".srt"}

	for _, ext := range extensions {
		relatedFile := filepath.Join(dir, baseName+ext)
		if _, err := os.Stat(relatedFile); err == nil {
			os.Remove(relatedFile)
			log.Printf("已删除相关文件: %s", relatedFile)
		}
	}
}

// copyRelatedFiles 复制相关文件
func (s *FileMoverService) copyRelatedFiles(sourceDir, targetDir, baseName string) {
	extensions := []string{".xml", ".jpg", ".png", ".json", ".txt", ".ass", ".srt"}

	for _, ext := range extensions {
		sourceFile := filepath.Join(sourceDir, baseName+ext)
		if _, err := os.Stat(sourceFile); err == nil {
			targetFile := filepath.Join(targetDir, baseName+ext)
			if err := s.copyFile(sourceFile, targetFile); err != nil {
				log.Printf("复制相关文件失败: %s -> %s, error: %v", sourceFile, targetFile, err)
			} else {
				log.Printf("已复制相关文件: %s", targetFile)
			}
		}
	}
}
