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
	"gorm.io/gorm"
)

// FileScanService 文件扫描服务，用于定期扫描录制目录，发现未入库的文件
type FileScanService struct{}

func NewFileScanService() *FileScanService {
	return &FileScanService{}
}

// ScanConfig 扫描配置
type ScanConfig struct {
	WorkPath          string   // 工作目录
	VideoExtensions   []string // 支持的视频扩展名
	MinFileSize       int64    // 最小文件大小（字节），小于此值的文件会被忽略
	MinFileAge        int      // 最小文件年龄（小时），避免扫描正在写入的文件
	MaxFileAge        int      // 最大文件年龄（天），超过此天数的文件会被忽略
	ScanIntervalHours int      // 扫描间隔（小时）
}

// DefaultScanConfig 返回默认的扫描配置
func DefaultScanConfig(workPath string) *ScanConfig {
	return &ScanConfig{
		WorkPath:          workPath,
		VideoExtensions:   []string{".flv", ".mp4", ".mkv", ".ts"},
		MinFileSize:       1024 * 1024, // 1MB
		MinFileAge:        12,          // 12小时，避免扫描正在写入的文件
		MaxFileAge:        30,          // 30天
		ScanIntervalHours: 1,           // 每小时扫描一次
	}
}

// LoadConfigFromDB 从数据库加载扫描配置
func LoadConfigFromDB() *ScanConfig {
	db := database.GetDB()

	var sysConfig models.SystemConfig
	if err := db.First(&sysConfig).Error; err != nil {
		// 如果获取失败，返回默认配置
		log.Printf("[FileScan] 从数据库加载配置失败，使用默认配置: %v", err)
		return DefaultScanConfig(os.Getenv("WORK_PATH"))
	}

	workPath := sysConfig.WorkPath
	if workPath == "" {
		workPath = os.Getenv("WORK_PATH")
		if workPath == "" {
			workPath = "./data/recordings"
		}
	}

	return &ScanConfig{
		WorkPath:          workPath,
		VideoExtensions:   []string{".flv", ".mp4", ".mkv", ".ts"},
		MinFileSize:       sysConfig.FileScanMinSize,
		MinFileAge:        sysConfig.FileScanMinAge,
		MaxFileAge:        sysConfig.FileScanMaxAge / 24,   // 转换为天
		ScanIntervalHours: sysConfig.FileScanInterval / 60, // 转换为小时
	}
}

// ScanResult 扫描结果
type ScanResult struct {
	TotalFiles   int
	NewFiles     int
	SkippedFiles int
	FailedFiles  int
	Errors       []string
}

// ScanAndImport 扫描并导入未入库的录制文件
func (s *FileScanService) ScanAndImport(config *ScanConfig) (*ScanResult, error) {
	result := &ScanResult{
		Errors: make([]string, 0),
	}

	if config.WorkPath == "" {
		return result, fmt.Errorf("工作目录未配置")
	}

	if _, err := os.Stat(config.WorkPath); os.IsNotExist(err) {
		return result, fmt.Errorf("工作目录不存在: %s", config.WorkPath)
	}

	log.Printf("[FileScan] 开始扫描目录: %s (最小文件年龄=%d小时, 最大年龄=%d天)",
		config.WorkPath, config.MinFileAge, config.MaxFileAge/24)

	// 遍历工作目录
	err := filepath.Walk(config.WorkPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[FileScan] 访问路径失败: %s, error: %v", path, err)
			return nil // 继续扫描其他文件
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 检查文件扩展名
		ext := strings.ToLower(filepath.Ext(path))
		if !s.isVideoFile(ext, config.VideoExtensions) {
			return nil
		}

		result.TotalFiles++

		// 检查文件大小
		if info.Size() < config.MinFileSize {
			log.Printf("[FileScan] 跳过小文件: %s (size=%d)", path, info.Size())
			result.SkippedFiles++
			return nil
		}

		// 计算文件年龄（小时）
		fileAgeHours := int(time.Since(info.ModTime()).Hours())

		// 检查文件是否太新（可能正在写入）
		if config.MinFileAge > 0 && fileAgeHours < config.MinFileAge {
			log.Printf("[FileScan] 跳过新文件（可能正在写入）: %s (年龄=%d小时, 需要>%d小时)",
				filepath.Base(path), fileAgeHours, config.MinFileAge)
			result.SkippedFiles++
			return nil
		}

		// 检查文件年龄是否过大
		fileAgeDays := fileAgeHours / 24
		if config.MaxFileAge > 0 && fileAgeDays > config.MaxFileAge {
			log.Printf("[FileScan] 跳过旧文件: %s (年龄=%d天, 最大=%d天)",
				filepath.Base(path), fileAgeDays, config.MaxFileAge)
			result.SkippedFiles++
			return nil
		}

		// 额外安全检查：最近1分钟内修改过的文件不扫描（双重保险）
		if time.Since(info.ModTime()) < time.Minute {
			log.Printf("[FileScan] 跳过正在修改的文件: %s (最后修改: %v)",
				filepath.Base(path), info.ModTime())
			result.SkippedFiles++
			return nil
		}

		// 尝试导入文件
		if err := s.importFile(path, info); err != nil {
			log.Printf("[FileScan] 导入文件失败: %s, error: %v", path, err)
			result.FailedFiles++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filepath.Base(path), err))
		} else {
			result.NewFiles++
		}

		return nil
	})

	if err != nil {
		return result, fmt.Errorf("扫描目录失败: %w", err)
	}

	log.Printf("[FileScan] 扫描完成: 总文件=%d, 新导入=%d, 跳过=%d, 失败=%d",
		result.TotalFiles, result.NewFiles, result.SkippedFiles, result.FailedFiles)

	return result, nil
}

// isVideoFile 检查是否是视频文件
func (s *FileScanService) isVideoFile(ext string, extensions []string) bool {
	for _, validExt := range extensions {
		if ext == validExt {
			return true
		}
	}
	return false
}

// importFile 导入单个文件
func (s *FileScanService) importFile(filePath string, info os.FileInfo) error {
	db := database.GetDB()

	// 1. 检查文件是否已存在于数据库
	var existingPart models.RecordHistoryPart
	if err := db.Where("file_path = ?", filePath).First(&existingPart).Error; err == nil {
		// 文件已存在，跳过
		log.Printf("[FileScan] 文件已存在，跳过: %s", filePath)
		return nil
	}

	// 2. 从文件路径解析房间信息
	metadata := s.parseFileMetadata(filePath, info)
	if metadata == nil {
		return fmt.Errorf("无法解析文件元数据")
	}

	// 3. 查找或创建房间
	var room models.RecordRoom
	if err := db.Where("room_id = ?", metadata.RoomID).First(&room).Error; err != nil {
		// 房间不存在，创建默认房间
		room = models.RecordRoom{
			RoomID: metadata.RoomID,
			Uname:  metadata.Uname,
			Title:  metadata.Title,
			Upload: true,
		}
		if err := db.Create(&room).Error; err != nil {
			return fmt.Errorf("创建房间失败: %w", err)
		}
		log.Printf("[FileScan] 创建新房间: RoomID=%s, Uname=%s", room.RoomID, room.Uname)
	}

	// 4. 查找或创建历史记录
	history, err := s.getOrCreateHistory(db, metadata, &room)
	if err != nil {
		return fmt.Errorf("获取或创建历史记录失败: %w", err)
	}

	// 5. 创建分P记录
	part := models.RecordHistoryPart{
		HistoryID: history.ID,
		RoomID:    metadata.RoomID,
		SessionID: metadata.SessionID,
		Title:     filepath.Base(filePath),
		LiveTitle: metadata.Title,
		AreaName:  metadata.AreaName,
		FilePath:  filePath,
		FileName:  filepath.Base(filePath),
		FileSize:  info.Size(),
		StartTime: metadata.StartTime,
		EndTime:   metadata.EndTime,
		Recording: false,
		Upload:    false, // 默认不自动上传扫描到的文件，需要手动触发
	}

	if err := db.Create(&part).Error; err != nil {
		return fmt.Errorf("创建分P记录失败: %w", err)
	}

	log.Printf("[FileScan] 成功导入文件: %s -> HistoryID=%d, PartID=%d",
		filepath.Base(filePath), history.ID, part.ID)

	return nil
}

// FileMetadata 文件元数据
type FileMetadata struct {
	RoomID    string
	Uname     string
	Title     string
	AreaName  string
	SessionID string
	StartTime time.Time
	EndTime   time.Time
}

// parseFileMetadata 从文件路径和文件信息解析元数据
func (s *FileScanService) parseFileMetadata(filePath string, info os.FileInfo) *FileMetadata {
	// 尝试从文件名解析信息
	// 期望格式示例:
	// - 录制-5050-20250101-120000-标题.flv
	// - 5050/20250101/120000-标题.flv
	// - RoomID/Date/Time-Title.flv

	fileName := filepath.Base(filePath)
	dirPath := filepath.Dir(filePath)

	metadata := &FileMetadata{
		RoomID:    "unknown",
		Uname:     "未知主播",
		Title:     strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		AreaName:  "",
		StartTime: info.ModTime().Add(-time.Hour), // 默认假设录制1小时
		EndTime:   info.ModTime(),
	}

	// 尝试从目录结构中提取房间号
	// 例如: /path/to/work/5050/2025/01/01/file.flv
	parts := strings.Split(dirPath, string(os.PathSeparator))
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		// 检查是否是纯数字（可能是房间号）
		if len(part) > 0 && len(part) <= 10 {
			isNumber := true
			for _, c := range part {
				if c < '0' || c > '9' {
					isNumber = false
					break
				}
			}
			if isNumber && len(part) >= 4 { // 房间号至少4位
				metadata.RoomID = part
				metadata.Uname = fmt.Sprintf("房间%s", part)
				break
			}
		}
	}

	// 尝试从文件名解析（录播姬格式：录制-房间号-日期-时间-标题.flv）
	if strings.HasPrefix(fileName, "录制-") || strings.HasPrefix(fileName, "record-") {
		fields := strings.Split(fileName, "-")
		if len(fields) >= 3 {
			metadata.RoomID = fields[1]
			metadata.Uname = fmt.Sprintf("房间%s", fields[1])

			// 尝试解析日期时间
			if len(fields) >= 4 {
				dateTimeStr := fields[2] + fields[3]
				if t, err := time.Parse("20060102150405", dateTimeStr[:14]); err == nil {
					metadata.StartTime = t.Add(-time.Hour)
					metadata.EndTime = t
				}
			}

			// 提取标题
			if len(fields) >= 5 {
				titleParts := fields[4:]
				metadata.Title = strings.Join(titleParts, "-")
				metadata.Title = strings.TrimSuffix(metadata.Title, filepath.Ext(fileName))
			}
		}
	}

	// 生成 SessionID（使用 房间号+日期 作为session标识）
	dateStr := metadata.StartTime.Format("20060102")
	metadata.SessionID = fmt.Sprintf("%s_%s_scan", metadata.RoomID, dateStr)

	return metadata
}

// getOrCreateHistory 获取或创建历史记录
func (s *FileScanService) getOrCreateHistory(db *gorm.DB, metadata *FileMetadata, room *models.RecordRoom) (*models.RecordHistory, error) {
	// 先尝试通过 SessionID 查找
	var history models.RecordHistory
	if err := db.Where("session_id = ?", metadata.SessionID).First(&history).Error; err == nil {
		// 找到已有记录，更新结束时间
		if metadata.EndTime.After(history.EndTime) {
			history.EndTime = metadata.EndTime
			db.Save(&history)
		}
		return &history, nil
	}

	// 如果没有找到，尝试查找同一天同一房间的最近记录（时间差在6小时内视为同一场直播）
	var histories []models.RecordHistory
	dayStart := metadata.StartTime.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	err := db.Where("room_id = ? AND start_time >= ? AND start_time < ?",
		metadata.RoomID, dayStart, dayEnd).
		Order("end_time DESC").
		Limit(5).
		Find(&histories).Error

	if err == nil && len(histories) > 0 {
		// 检查时间差
		for _, h := range histories {
			timeDiff := metadata.StartTime.Sub(h.EndTime)
			if timeDiff >= 0 && timeDiff < 6*time.Hour {
				// 找到可合并的历史记录
				if metadata.EndTime.After(h.EndTime) {
					h.EndTime = metadata.EndTime
					db.Save(&h)
				}
				log.Printf("[FileScan] 合并到已有历史记录: ID=%d, SessionID=%s", h.ID, h.SessionID)
				return &h, nil
			}
		}
	}

	// 创建新的历史记录
	history = models.RecordHistory{
		RoomID:    metadata.RoomID,
		SessionID: metadata.SessionID,
		EventID:   fmt.Sprintf("scan_%s_%d", metadata.RoomID, time.Now().Unix()),
		Uname:     metadata.Uname,
		Title:     metadata.Title,
		AreaName:  metadata.AreaName,
		StartTime: metadata.StartTime,
		EndTime:   metadata.EndTime,
		Recording: false,
		Streaming: false,
		Upload:    room.Upload,
		Publish:   false,
	}

	if err := db.Create(&history).Error; err != nil {
		return nil, fmt.Errorf("创建历史记录失败: %w", err)
	}

	log.Printf("[FileScan] 创建新历史记录: ID=%d, SessionID=%s, RoomID=%s",
		history.ID, history.SessionID, metadata.RoomID)

	return &history, nil
}

// ScanOrphanFiles 扫描孤儿文件（数据库中有记录但文件不存在）
func (s *FileScanService) ScanOrphanFiles() error {
	db := database.GetDB()

	var parts []models.RecordHistoryPart
	if err := db.Where("file_delete = ? AND file_moved = ?", false, false).Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P记录失败: %w", err)
	}

	orphanCount := 0
	for _, part := range parts {
		if part.FilePath == "" {
			continue
		}

		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			// 文件不存在，标记为已删除
			part.FileDelete = true
			db.Save(&part)
			orphanCount++
			log.Printf("[FileScan] 发现孤儿记录: PartID=%d, FilePath=%s", part.ID, part.FilePath)
		}
	}

	log.Printf("[FileScan] 孤儿文件扫描完成: 发现%d个孤儿记录", orphanCount)
	return nil
}
