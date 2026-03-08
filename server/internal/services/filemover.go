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

// ─── 文件操作常量 ────────────────────────────────────────────────────────────

// FileOpTrigger 触发时机
const (
	FileOpTriggerDisabled      = 0 // 不处理
	FileOpTriggerAfterPart     = 1 // 分P上传完成后
	FileOpTriggerBeforePublish = 2 // 全部分P上传完成、投稿前
	FileOpTriggerAfterPublish  = 3 // 投稿成功后
	FileOpTriggerAfterReview   = 4 // 审核通过后
)

// FileOpAction 操作类型
const (
	FileOpActionNothing = 0 // 不处理
	FileOpActionDelete  = 1 // 删除
	FileOpActionMove    = 2 // 移动
	FileOpActionCopy    = 3 // 复制
)

// FileOpScope 操作范围（位掩码）
const (
	FileOpScopeVideo   = 1 // 视频文件 (.flv/.mp4/.mkv/.ts 等)
	FileOpScopeDanmaku = 2 // 弹幕文件 (.xml)
	FileOpScopeCover   = 4 // 封面文件 (.jpg/.jpeg/.png)
	FileOpScopeAll     = 7 // 全部文件
)

// TriggerFileOp 在特定事件发生时触发文件操作（根据房间配置）。
// event 必须与 room.FileOpTrigger 吻合才会执行操作，否则直接返回 nil。
func (s *FileMoverService) TriggerFileOp(historyID uint, room *models.RecordRoom, event int) error {
	if room == nil || room.FileOpTrigger != event || room.FileOpTrigger == FileOpTriggerDisabled {
		return nil
	}
	if room.FileOpAction == FileOpActionNothing {
		return nil
	}
	if room.FileOpDelay > 0 {
		return s.scheduleFileOp(historyID, room.FileOpAction, room.FileOpScope, room.FileOpDelay)
	}
	return s.executeFileOp(historyID, room.FileOpAction, room.FileOpScope, room.MoveDir)
}

// executeFileOp 根据 action 和 scope 立即执行文件操作
func (s *FileMoverService) executeFileOp(historyID uint, action, scope int, moveDir string) error {
	switch action {
	case FileOpActionNothing:
		return nil
	case FileOpActionDelete:
		return s.deleteByScope(historyID, scope)
	case FileOpActionMove:
		return s.moveByScope(historyID, scope, moveDir)
	case FileOpActionCopy:
		return s.copyByScope(historyID, scope, moveDir)
	default:
		return fmt.Errorf("未知文件操作类型: %d", action)
	}
}

// scheduleFileOp 将文件操作计划到 delayDays 天后执行（由定时任务 ProcessScheduledDeletes 消费）
func (s *FileMoverService) scheduleFileOp(historyID uint, action, scope, delayDays int) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	if history.ScheduledDeleteAt != nil {
		log.Printf("历史记录 %d 已有计划操作时间: %s，跳过", historyID, history.ScheduledDeleteAt.Format("2006-01-02 15:04:05"))
		return nil
	}

	if delayDays <= 0 {
		delayDays = 3
	}
	opAt := time.Now().Add(time.Duration(delayDays) * 24 * time.Hour)
	history.ScheduledDeleteAt = &opAt
	history.ScheduledOpAction = action
	history.ScheduledOpScope = scope
	if err := db.Save(&history).Error; err != nil {
		return fmt.Errorf("保存计划操作时间失败: %w", err)
	}

	log.Printf("历史记录 %d 已计划在 %d 天后（%s）执行文件操作 (action=%d, scope=%d)",
		historyID, delayDays, opAt.Format("2006-01-02 15:04:05"), action, scope)
	return nil
}

// removeIfExists 尝试删除文件，文件不存在时静默跳过
func (s *FileMoverService) removeIfExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

// deleteByScope 按作用域删除文件（scope 为位掩码：FileOpScopeVideo/Danmaku/Cover）
func (s *FileMoverService) deleteByScope(historyID uint, scope int) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	successCount := 0
	for _, part := range parts {
		if part.FileDelete {
			continue
		}
		if part.Uploading {
			log.Printf("跳过正在上传中的分P: part_id=%d, file=%s", part.ID, part.FilePath)
			continue
		}
		if part.FilePath == "" {
			continue
		}

		dir := filepath.Dir(part.FilePath)
		base := strings.TrimSuffix(filepath.Base(part.FilePath), filepath.Ext(part.FilePath))
		deletedVideo := false

		// 删除视频文件
		if scope&FileOpScopeVideo != 0 {
			if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
				part.FileDelete = true
				db.Save(&part)
				deletedVideo = true
			} else if err := os.Remove(part.FilePath); err != nil {
				log.Printf("删除视频文件失败: %s, error: %v", part.FilePath, err)
			} else {
				part.FileDelete = true
				db.Save(&part)
				deletedVideo = true
				log.Printf("已删除视频文件: %s", part.FilePath)
			}
		}

		// 删除弹幕/字幕文件 (.xml/.json/.txt/.ass/.srt) — 临时分P不处理，避免误删原始分P的配套文件
		if scope&FileOpScopeDanmaku != 0 && !part.IsTempFile {
			for _, ext := range []string{".xml", ".json", ".txt", ".ass", ".srt"} {
				p := filepath.Join(dir, base+ext)
				if err := s.removeIfExists(p); err != nil {
					log.Printf("删除弹幕/字幕文件失败: %s, error: %v", p, err)
				}
			}
		}

		// 删除封面文件 (.jpg/.jpeg/.png/.cover.jpg) — 临时分P同样跳过
		if scope&FileOpScopeCover != 0 && !part.IsTempFile {
			for _, ext := range []string{".jpg", ".jpeg", ".png", ".cover.jpg"} {
				coverPath := filepath.Join(dir, base+ext)
				if err := s.removeIfExists(coverPath); err != nil {
					log.Printf("删除封面文件失败: %s, error: %v", coverPath, err)
				}
			}
		}

		if deletedVideo {
			successCount++
		}
	}

	// 仅在处理了视频文件时标记 FilesMoved（防止定时任务或其他逻辑重复触发）
	if scope&FileOpScopeVideo != 0 {
		history.FilesMoved = true
		db.Save(&history)
	}

	log.Printf("历史记录 %d: 成功删除 %d/%d 个分P的文件 (scope=%d)", historyID, successCount, len(parts), scope)
	return nil
}

// moveByScope 按作用域移动文件到 moveDir/<roomID>/<sessionID>/ 子目录
func (s *FileMoverService) moveByScope(historyID uint, scope int, moveDir string) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	if scope&FileOpScopeVideo != 0 && history.FilesMoved {
		return fmt.Errorf("视频文件已移动，跳过")
	}

	if moveDir == "" {
		var room models.RecordRoom
		if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
			return fmt.Errorf("房间配置不存在: %w", err)
		}
		moveDir = room.MoveDir
	}
	if moveDir == "" {
		return fmt.Errorf("未配置目标目录")
	}

	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	movedCount := 0
	errs := []string{}
	for _, part := range parts {
		if part.FileMoved || part.FileDelete || part.FilePath == "" {
			continue
		}

		dir := filepath.Dir(part.FilePath)
		fileName := filepath.Base(part.FilePath)
		base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
		targetDir := filepath.Join(moveDir, history.RoomID, history.SessionID)

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			errs = append(errs, fmt.Sprintf("创建目录失败 %s: %v", targetDir, err))
			continue
		}

		movedVideo := false
		if scope&FileOpScopeVideo != 0 {
			if _, err := os.Stat(part.FilePath); !os.IsNotExist(err) {
				dst := filepath.Join(targetDir, fileName)
				if err := s.moveFile(part.FilePath, dst); err != nil {
					errs = append(errs, fmt.Sprintf("移动视频文件失败 %s: %v", fileName, err))
				} else {
					movedVideo = true
					log.Printf("已移动视频文件: %s -> %s", part.FilePath, dst)
				}
			}
		}

		if scope&FileOpScopeDanmaku != 0 {
			for _, ext := range []string{".xml", ".json", ".txt", ".ass", ".srt"} {
				src := filepath.Join(dir, base+ext)
				if _, err := os.Stat(src); err == nil {
					if err := s.moveFile(src, filepath.Join(targetDir, base+ext)); err != nil {
						log.Printf("移动弹幕/字幕文件失败: %s, error: %v", src, err)
					}
				}
			}
		}

		if scope&FileOpScopeCover != 0 {
			for _, ext := range []string{".jpg", ".jpeg", ".png", ".cover.jpg"} {
				src := filepath.Join(dir, base+ext)
				if _, err := os.Stat(src); err == nil {
					if err := s.moveFile(src, filepath.Join(targetDir, base+ext)); err != nil {
						log.Printf("移动封面文件失败: %s, error: %v", src, err)
					}
				}
			}
		}

		if movedVideo {
			part.FileMoved = true
			part.FileDelete = false
			db.Save(&part)
			movedCount++
		}
	}

	if movedCount > 0 && scope&FileOpScopeVideo != 0 {
		history.FilesMoved = true
		db.Save(&history)
	}

	if len(errs) > 0 {
		return fmt.Errorf("部分文件移动失败: %s", strings.Join(errs, "; "))
	}
	log.Printf("历史记录 %d: 成功移动 %d/%d 个分P文件 (scope=%d)", historyID, movedCount, len(parts), scope)
	return nil
}

// copyByScope 按作用域复制文件到 moveDir/<roomID>/<sessionID>/ 子目录（原文件保留）
func (s *FileMoverService) copyByScope(historyID uint, scope int, moveDir string) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	if moveDir == "" {
		var room models.RecordRoom
		if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
			return fmt.Errorf("房间配置不存在: %w", err)
		}
		moveDir = room.MoveDir
	}
	if moveDir == "" {
		return fmt.Errorf("未配置目标目录")
	}

	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	copiedCount := 0
	for _, part := range parts {
		if part.FilePath == "" {
			continue
		}

		dir := filepath.Dir(part.FilePath)
		fileName := filepath.Base(part.FilePath)
		base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
		targetDir := filepath.Join(moveDir, history.RoomID, history.SessionID)

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			log.Printf("创建目录失败 %s: %v", targetDir, err)
			continue
		}

		copiedVideo := false
		if scope&FileOpScopeVideo != 0 {
			if _, err := os.Stat(part.FilePath); err == nil {
				dst := filepath.Join(targetDir, fileName)
				if err := s.copyFile(part.FilePath, dst); err != nil {
					log.Printf("复制视频文件失败: %s -> %s, error: %v", part.FilePath, dst, err)
					continue
				}
				log.Printf("已复制视频文件: %s -> %s", part.FilePath, dst)
				copiedVideo = true
			}
		}

		if scope&FileOpScopeDanmaku != 0 {
			for _, ext := range []string{".xml", ".json", ".txt", ".ass", ".srt"} {
				src := filepath.Join(dir, base+ext)
				if _, err := os.Stat(src); err == nil {
					if err := s.copyFile(src, filepath.Join(targetDir, base+ext)); err != nil {
						log.Printf("复制弹幕/字幕文件失败: %s, error: %v", src, err)
					}
				}
			}
		}

		if scope&FileOpScopeCover != 0 {
			for _, ext := range []string{".jpg", ".jpeg", ".png", ".cover.jpg"} {
				src := filepath.Join(dir, base+ext)
				if _, err := os.Stat(src); err == nil {
					if err := s.copyFile(src, filepath.Join(targetDir, base+ext)); err != nil {
						log.Printf("复制封面文件失败: %s, error: %v", src, err)
					}
				}
			}
		}

		if copiedVideo {
			copiedCount++
		}
	}

	// 复制不修改原文件状态，但标记 FilesMoved 防止被定时任务重复触发
	if copiedCount > 0 {
		history.FilesMoved = true
		db.Save(&history)
	}

	log.Printf("历史记录 %d: 成功复制 %d/%d 个分P文件 (scope=%d)", historyID, copiedCount, len(parts), scope)
	return nil
}

// BackfillFileOps 对存量历史记录补执行文件操作。
// 用于版本升级后首次启动时将新的文件操作规则应用到历史已完成但未处理的记录上。
// 此函数是幂等的：files_moved=true 或已设置 scheduled_delete_at 的记录会被跳过。
func (s *FileMoverService) BackfillFileOps() error {
	db := database.GetDB()

	// 获取所有配置了文件操作的房间
	var rooms []models.RecordRoom
	if err := db.Where("file_op_trigger != 0 AND file_op_action != 0").Find(&rooms).Error; err != nil {
		return fmt.Errorf("查询房间配置失败: %w", err)
	}

	if len(rooms) == 0 {
		return nil
	}

	log.Printf("[回填] 开始对 %d 个房间的存量历史记录补执行文件操作", len(rooms))
	total := 0

	for _, room := range rooms {
		// 基础条件：文件未处理、无待执行计划、非录制中
		baseQuery := db.Where(
			"room_id = ? AND files_moved = ? AND scheduled_delete_at IS NULL AND recording = ?",
			room.RoomID, false, false,
		)

		var histories []models.RecordHistory
		var err error
		switch room.FileOpTrigger {
		case FileOpTriggerAfterPart, FileOpTriggerBeforePublish:
			// 以 streaming=false 作为所有分P已完成的代理判断
			err = baseQuery.Where("streaming = ?", false).Find(&histories).Error
		case FileOpTriggerAfterPublish:
			err = baseQuery.Where("publish = ?", true).Find(&histories).Error
		case FileOpTriggerAfterReview:
			err = baseQuery.Where("video_state = ?", 1).Find(&histories).Error
		default:
			continue
		}
		if err != nil {
			log.Printf("[回填] 查询历史记录失败 room=%s: %v", room.RoomID, err)
			continue
		}

		for _, history := range histories {
			log.Printf("[回填] 补执行文件操作: history_id=%d, room=%s, trigger=%d, action=%d, scope=%d, delay=%d",
				history.ID, room.RoomID, room.FileOpTrigger, room.FileOpAction, room.FileOpScope, room.FileOpDelay)
			if room.FileOpDelay > 0 {
				if err := s.scheduleFileOp(history.ID, room.FileOpAction, room.FileOpScope, room.FileOpDelay); err != nil {
					log.Printf("[回填] 计划文件操作失败: history_id=%d, error=%v", history.ID, err)
				}
			} else {
				if err := s.executeFileOp(history.ID, room.FileOpAction, room.FileOpScope, room.MoveDir); err != nil {
					log.Printf("[回填] 执行文件操作失败: history_id=%d, error=%v", history.ID, err)
				}
			}
			total++
			time.Sleep(200 * time.Millisecond) // 避免IO过载
		}
	}

	log.Printf("[回填] 完成，共处理 %d 条历史记录", total)
	return nil
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

// moveRelatedFiles 移动相关文件（xml弹幕、封面、其他视频格式等）
func (s *FileMoverService) moveRelatedFiles(sourceDir, targetDir, baseName string) {
	// 常见的相关文件扩展名（包括其他视频格式，以防转码或多格式录制）
	extensions := []string{".xml", ".jpg", ".jpeg", ".cover.jpg", ".png", ".json", ".txt", ".ass", ".srt", ".flv", ".mp4", ".mkv", ".ts", ".avi"}

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

// AutoMoveFiles 自动移动已完成投稿的文件（定时任务调用）。
// 只处理仍在使用旧版 MoveDir 配置（file_op_action=0）的房间；
// 已迁移到新文件操作系统（file_op_action != 0）的房间由 TriggerFileOp/BackfillFileOps 处理。
func (s *FileMoverService) AutoMoveFiles() error {
	db := database.GetDB()

	// 查找需要移动的历史记录：
	//   - 已投稿（publish=true）、文件未处理（files_moved=false）、有BV号
	//   - JOIN record_rooms，仅处理配置了 MoveDir 且未启用新文件操作的房间（file_op_action=0）
	var histories []models.RecordHistory
	if err := db.Joins("JOIN record_rooms r ON r.room_id = record_histories.room_id").
		Where("record_histories.publish = ? AND record_histories.files_moved = ? AND record_histories.bv_id != ? AND r.move_dir != ? AND r.file_op_action = ? AND r.deleted_at IS NULL",
			true, false, "", "", 0).
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

// deleteFiles 删除历史记录的所有文件（视频+弹幕+封面）
func (s *FileMoverService) deleteFiles(historyID uint) error {
	return s.deleteByScope(historyID, FileOpScopeAll)
}

// deleteVideoFilesOnly 仅删除视频文件本身，保留弹幕(.xml)、封面(.jpg/.png)等相关文件
func (s *FileMoverService) deleteVideoFilesOnly(historyID uint) error {
	return s.deleteByScope(historyID, FileOpScopeVideo)
}
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

	// 标记文件已处理，避免定时任务重复触发复制
	if successCount > 0 {
		history.FilesMoved = true
		db.Save(&history)
	}

	log.Printf("历史记录 %d: 成功复制 %d/%d 个文件到 %s", historyID, successCount, len(parts), room.MoveDir)
	return nil
}

// ProcessScheduledDeletes 处理到期的计划文件操作任务（由定时任务调用）
func (s *FileMoverService) ProcessScheduledDeletes() error {
	db := database.GetDB()

	var histories []models.RecordHistory
	now := time.Now()
	// 加上 recording = false 过滤，避免误处理还在录制中的历史记录
	if err := db.Where("scheduled_delete_at IS NOT NULL AND scheduled_delete_at <= ? AND recording = ?", now, false).
		Find(&histories).Error; err != nil {
		return fmt.Errorf("查询计划操作记录失败: %w", err)
	}

	if len(histories) == 0 {
		return nil
	}

	log.Printf("[计划操作] 发现 %d 个到期的计划文件操作任务", len(histories))

	for _, history := range histories {
		action := history.ScheduledOpAction
		scope := history.ScheduledOpScope
		// 向后兼容：旧版 scheduleDelayedDelete 写入的记录 ScheduledOpAction=0（GORM默认值），
		// 且 ScheduledOpScope 会被 GORM AutoMigrate 填为列默认值 1（仅视频），而旧逻辑是删除全部文件。
		// 因此当 action==0 时，同时把 scope 也重置为全部，确保行为与升级前完全一致。
		if action == FileOpActionNothing {
			action = FileOpActionDelete
			scope = FileOpScopeAll
		}

		moveDir := ""
		if action == FileOpActionMove || action == FileOpActionCopy {
			var room models.RecordRoom
			if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err == nil {
				moveDir = room.MoveDir
			}
		}

		log.Printf("[计划操作] 开始执行: history_id=%d, bv_id=%s, action=%d, scope=%d", history.ID, history.BvID, action, scope)
		if err := s.executeFileOp(history.ID, action, scope, moveDir); err != nil {
			log.Printf("[计划操作] 执行失败: history_id=%d, error=%v", history.ID, err)
			continue
		}
		// 清除计划操作时间标记
		// 注意：不能用 Update("field", nil) — GORM v2 会忽略 nil 指针导致字段无法清空。用原始 SQL 确保生效。
		if err := db.Exec("UPDATE record_histories SET scheduled_delete_at = NULL WHERE id = ?", history.ID).Error; err != nil {
			log.Printf("[计划操作] 清除标记失败: history_id=%d, error=%v", history.ID, err)
		}
		log.Printf("[计划操作] ✓ 计划操作执行完成: history_id=%d", history.ID)
		time.Sleep(500 * time.Millisecond) // 避免IO过载
	}

	return nil
}

// DeleteRelatedFiles 删除相关文件（xml弹幕、jpg封面、png、json、txt、ass、srt字幕等）
// 严格基于主文件路径进行精确匹配，确保100%对应关系，避免误删
func (s *FileMoverService) DeleteRelatedFiles(filePath string) {
	// 参数验证
	if filePath == "" {
		log.Printf("[DeleteRelatedFiles] 警告: 文件路径为空，跳过删除")
		return
	}

	// 验证主文件确实存在或曾经存在（可能已被删除）
	dir := filepath.Dir(filePath)
	if dir == "" || dir == "." {
		log.Printf("[DeleteRelatedFiles] 警告: 无效的文件路径 %s，跳过删除", filePath)
		return
	}

	// 获取不带扩展名的基础文件名（确保精确匹配）
	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if baseName == "" {
		log.Printf("[DeleteRelatedFiles] 警告: 无法提取文件名 %s，跳过删除", filePath)
		return
	}

	// 相关文件扩展名列表（包括其他视频格式，以防转码或多格式录制）
	extensions := []string{".xml", ".jpg", ".jpeg", ".cover.jpg", ".png", ".json", ".txt", ".ass", ".srt", ".flv", ".mp4", ".mkv", ".ts", ".avi"}

	log.Printf("[DeleteRelatedFiles] 开始删除 %s 的相关文件", filePath)

	for _, ext := range extensions {
		// 构造精确的相关文件路径（同目录 + 同基础名 + 特定扩展名）
		relatedFile := filepath.Join(dir, baseName+ext)

		// 二次验证：确保构造的路径与原文件在同一目录
		if filepath.Dir(relatedFile) != dir {
			log.Printf("[DeleteRelatedFiles] 安全检查失败: 路径不匹配 %s", relatedFile)
			continue
		}

		// 检查文件是否存在
		if _, err := os.Stat(relatedFile); err == nil {
			// 最后确认：文件名前缀必须完全匹配
			relatedBaseName := strings.TrimSuffix(filepath.Base(relatedFile), filepath.Ext(relatedFile))
			if relatedBaseName != baseName {
				log.Printf("[DeleteRelatedFiles] 安全检查失败: 文件名不匹配 %s (期望: %s, 实际: %s)",
					relatedFile, baseName, relatedBaseName)
				continue
			}

			// 执行删除
			if err := os.Remove(relatedFile); err != nil {
				log.Printf("[DeleteRelatedFiles] 删除相关文件失败: %s, error: %v", relatedFile, err)
			} else {
				log.Printf("[DeleteRelatedFiles] ✓ 已删除相关文件: %s", relatedFile)
			}
		} else if !os.IsNotExist(err) {
			log.Printf("[DeleteRelatedFiles] 检查文件失败: %s, error: %v", relatedFile, err)
		}
	}
}

// copyRelatedFiles 复制相关文件
func (s *FileMoverService) copyRelatedFiles(sourceDir, targetDir, baseName string) {
	extensions := []string{".xml", ".jpg", ".jpeg", ".cover.jpg", ".png", ".json", ".txt", ".ass", ".srt", ".flv", ".mp4", ".mkv", ".ts", ".avi"}

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
