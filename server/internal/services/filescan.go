package services

import (
	"errors"
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

// ErrFileAlreadyExists 文件已存在错误
var ErrFileAlreadyExists = errors.New("文件已存在于数据库中")

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
	ForceImport       bool     // 强制导入，无视文件年龄限制（但保留1分钟安全检查）
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

// findValidWorkPath 查找有效的工作目录，按优先级尝试多个可能的路径
func findValidWorkPath() string {
	// 可能的路径列表，按优先级排序
	possiblePaths := []string{
		os.Getenv("WORK_PATH"), // 环境变量优先
		"/rec",                 // Docker部署的默认路径
		"./rec",                // Docker部署的相对路径
		"./data/recordings",    // 裸机部署的默认路径
		"/app/data/recordings", // Docker内部的另一个可能路径
		"/recordings",          // 另一种相对路径
		"./recordings",         // 另一种相对路径
		"/root/recordings",     // 另一种相对路径
	}

	for _, path := range possiblePaths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			log.Printf("[FileScan] 找到有效的工作目录: %s", path)
			return path
		}
	}

	log.Printf("[FileScan] 未找到有效的工作目录，使用默认值: ./data/recordings")
	return "./data/recordings"
}

// LoadConfigFromDB 从数据库加载扫描配置
func LoadConfigFromDB() *ScanConfig {
	db := database.GetDB()

	var sysConfig models.SystemConfig
	if err := db.First(&sysConfig).Error; err != nil {
		// 如果获取失败，返回默认配置
		log.Printf("[FileScan] 从数据库加载配置失败，使用默认配置: %v", err)
		return DefaultScanConfig(findValidWorkPath())
	}

	workPath := sysConfig.WorkPath
	if workPath == "" {
		workPath = findValidWorkPath()
	} else {
		// 即使配置了工作目录，也要验证其是否存在
		if _, err := os.Stat(workPath); os.IsNotExist(err) {
			log.Printf("[FileScan] 配置的工作目录不存在: %s，尝试查找其他有效路径", workPath)
			workPath = findValidWorkPath()
		}
	}

	config := &ScanConfig{
		WorkPath:          workPath,
		VideoExtensions:   []string{".flv", ".mp4", ".mkv", ".ts"},
		MinFileSize:       sysConfig.FileScanMinSize,
		MinFileAge:        sysConfig.FileScanMinAge,
		MaxFileAge:        sysConfig.FileScanMaxAge / 24,   // 转换为天
		ScanIntervalHours: sysConfig.FileScanInterval / 60, // 转换为小时
	}

	return config
}

// getCustomScanPaths 获取自定义扫描路径列表
func getCustomScanPaths() []string {
	db := database.GetDB()
	var sysConfig models.SystemConfig
	if err := db.First(&sysConfig).Error; err != nil {
		return []string{}
	}

	if sysConfig.CustomScanPaths == "" {
		return []string{}
	}

	// 分割路径，支持逗号分隔
	paths := strings.Split(sysConfig.CustomScanPaths, ",")
	validPaths := []string{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			// 验证路径是否存在
			if _, err := os.Stat(path); err == nil {
				validPaths = append(validPaths, path)
			} else {
				log.Printf("[FileScan] 自定义扫描路径不存在，跳过: %s", path)
			}
		}
	}

	return validPaths
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

	// 获取自定义扫描路径
	customPaths := getCustomScanPaths()

	// 先扫描自定义路径（优先）
	if len(customPaths) > 0 {
		log.Printf("[FileScan] 开始扫描自定义目录，共%d个路径", len(customPaths))
		for _, customPath := range customPaths {
			log.Printf("[FileScan] 扫描自定义目录: %s", customPath)
			if err := s.scanDirectory(customPath, config, result); err != nil {
				log.Printf("[FileScan] 扫描自定义目录失败: %s, error: %v", customPath, err)
			}
		}
	}

	// 然后扫描默认工作目录
	if config.WorkPath == "" {
		return result, fmt.Errorf("工作目录未配置")
	}

	if _, err := os.Stat(config.WorkPath); os.IsNotExist(err) {
		return result, fmt.Errorf("工作目录不存在: %s (提示: Docker部署请检查是否挂载了录播目录到/rec，裸机部署请确保./data/recordings存在)", config.WorkPath)
	}

	if config.ForceImport {
		log.Printf("[FileScan] 开始强制扫描默认目录: %s (无视文件年龄限制，仅保留1分钟安全检查)", config.WorkPath)
	} else {
		log.Printf("[FileScan] 开始扫描默认目录: %s (最小文件年龄=%d小时, 最大年龄=%d天)",
			config.WorkPath, config.MinFileAge, config.MaxFileAge)
	}

	if err := s.scanDirectory(config.WorkPath, config, result); err != nil {
		return result, err
	}

	log.Printf("[FileScan] 扫描完成: 总文件=%d, 新导入=%d, 跳过=%d, 失败=%d",
		result.TotalFiles, result.NewFiles, result.SkippedFiles, result.FailedFiles)

	return result, nil
}

// scanDirectory 扫描单个目录
func (s *FileScanService) scanDirectory(dirPath string, config *ScanConfig, result *ScanResult) error {
	// 遍历工作目录
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
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

		// 尝试从文件路径解析房间号
		metadata := s.parseFileMetadata(path, info)

		// 如果解析到房间号，检查直播状态（智能判断）
		if metadata != nil && metadata.RoomID != "" && metadata.RoomID != "unknown" {
			liveStatusService := NewLiveStatusService()
			isFinished, usedFallback, err := liveStatusService.IsRoomRecordingFinished(metadata.RoomID, info.ModTime(), metadata.Title)

			if err == nil {
				if !isFinished {
					// 直播未结束或文件未稳定，跳过
					if usedFallback {
						log.Printf("[FileScan] 跳过文件（保底逻辑：文件修改时间 < 1小时）: %s",
							filepath.Base(path))
					} else {
						log.Printf("[FileScan] 跳过文件（房间 %s 直播未结束或文件未稳定）: %s",
							metadata.RoomID, filepath.Base(path))
					}
					result.SkippedFiles++
					return nil
				}
				// 直播已结束且文件已稳定，继续处理
				if usedFallback {
					log.Printf("[FileScan] 文件可处理（保底逻辑：文件修改时间 >= 1小时）: %s",
						filepath.Base(path))
				} else {
					log.Printf("[FileScan] 文件可处理（房间 %s 直播已结束）: %s",
						metadata.RoomID, filepath.Base(path))
				}
			} else {
				// 理论上不应该到这里，因为新的实现总是返回 err == nil
				log.Printf("[FileScan] 检查房间 %s 状态异常: %v，使用配置的时间判断",
					metadata.RoomID, err)

				// 回退到配置的时间判断
				if !config.ForceImport && config.MinFileAge > 0 && fileAgeHours < config.MinFileAge {
					log.Printf("[FileScan] 跳过新文件（可能正在写入）: %s (年龄=%d小时, 需要>%d小时)",
						filepath.Base(path), fileAgeHours, config.MinFileAge)
					result.SkippedFiles++
					return nil
				}
			}
		} else {
			// 无法解析房间号，使用传统的时间判断
			// 检查文件是否太新（可能正在写入）- 除非是强制导入模式
			if !config.ForceImport && config.MinFileAge > 0 && fileAgeHours < config.MinFileAge {
				log.Printf("[FileScan] 跳过新文件（无房间号，使用时间判断）: %s (年龄=%d小时, 需要>%d小时)",
					filepath.Base(path), fileAgeHours, config.MinFileAge)
				result.SkippedFiles++
				return nil
			}
		}

		// 检查文件年龄是否过大 - 除非是强制导入模式
		fileAgeDays := fileAgeHours / 24
		if !config.ForceImport && config.MaxFileAge > 0 && fileAgeDays > config.MaxFileAge {
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
			if errors.Is(err, ErrFileAlreadyExists) {
				// 文件已存在，计入跳过数
				result.SkippedFiles++
			} else {
				// 真正的导入失败
				log.Printf("[FileScan] 导入文件失败: %s, error: %v", path, err)
				result.FailedFiles++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filepath.Base(path), err))
			}
		} else {
			// 导入成功
			result.NewFiles++
		}

		return nil
	})

	return err
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

// FilePreviewInfo 文件预览信息
type FilePreviewInfo struct {
	FilePath   string    `json:"filePath"`
	FileName   string    `json:"fileName"`
	FileSize   int64     `json:"fileSize"`
	ModTime    time.Time `json:"modTime"`
	RoomID     string    `json:"roomId"`
	Uname      string    `json:"uname"`
	Title      string    `json:"title"`
	InDatabase bool      `json:"inDatabase"` // 是否已在数据库中
}

// PreviewFiles 预览待导入的文件（不实际导入）
func (s *FileScanService) PreviewFiles(config *ScanConfig) ([]*FilePreviewInfo, error) {
	db := database.GetDB()
	var previews []*FilePreviewInfo

	if config.WorkPath == "" {
		return previews, fmt.Errorf("工作目录未配置")
	}

	if _, err := os.Stat(config.WorkPath); os.IsNotExist(err) {
		return previews, fmt.Errorf("工作目录不存在: %s", config.WorkPath)
	}

	log.Printf("[FileScan] 预览扫描目录: %s", config.WorkPath)

	// 遍历工作目录
	err := filepath.Walk(config.WorkPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
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

		// 检查文件大小
		if info.Size() < config.MinFileSize {
			return nil
		}

		// 额外安全检查：最近1分钟内修改过的文件不扫描
		if time.Since(info.ModTime()) < time.Minute {
			return nil
		}

		// 检查文件是否已在数据库
		var existingPart models.RecordHistoryPart
		inDatabase := db.Where("file_path = ?", path).First(&existingPart).Error == nil

		// 解析文件元数据
		metadata := s.parseFileMetadata(path, info)
		if metadata == nil {
			metadata = &FileMetadata{
				RoomID: "unknown",
				Uname:  "未知主播",
				Title:  filepath.Base(path),
			}
		}

		preview := &FilePreviewInfo{
			FilePath:   path,
			FileName:   filepath.Base(path),
			FileSize:   info.Size(),
			ModTime:    info.ModTime(),
			RoomID:     metadata.RoomID,
			Uname:      metadata.Uname,
			Title:      metadata.Title,
			InDatabase: inDatabase,
		}

		previews = append(previews, preview)

		return nil
	})

	if err != nil {
		return previews, fmt.Errorf("预览扫描失败: %w", err)
	}

	log.Printf("[FileScan] 预览完成: 发现 %d 个文件", len(previews))
	return previews, nil
}

// ImportSelectedFiles 导入选中的文件
func (s *FileScanService) ImportSelectedFiles(filePaths []string) (*ScanResult, error) {
	result := &ScanResult{
		Errors: make([]string, 0),
	}

	for _, filePath := range filePaths {
		info, err := os.Stat(filePath)
		if err != nil {
			log.Printf("[FileScan] 文件不存在: %s, error: %v", filePath, err)
			result.FailedFiles++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: 文件不存在", filepath.Base(filePath)))
			continue
		}

		// 尝试导入文件
		if err := s.importFile(filePath, info); err != nil {
			if errors.Is(err, ErrFileAlreadyExists) {
				// 文件已存在，计入跳过数
				result.SkippedFiles++
			} else {
				// 真正的导入失败
				log.Printf("[FileScan] 导入文件失败: %s, error: %v", filePath, err)
				result.FailedFiles++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filepath.Base(filePath), err))
			}
		} else {
			// 导入成功
			result.NewFiles++
		}
		result.TotalFiles++
	}

	log.Printf("[FileScan] 选择性导入完成: 总文件=%d, 新导入=%d, 失败=%d",
		result.TotalFiles, result.NewFiles, result.FailedFiles)

	return result, nil
}

// importFile 导入单个文件
func (s *FileScanService) importFile(filePath string, info os.FileInfo) error {
	db := database.GetDB()

	// 1. 检查文件是否已存在于数据库
	var existingPart models.RecordHistoryPart
	if err := db.Where("file_path = ?", filePath).First(&existingPart).Error; err == nil {
		// 文件已存在，检查对应的历史记录是否存在
		var existingHistory models.RecordHistory
		if err := db.Where("id = ?", existingPart.HistoryID).First(&existingHistory).Error; err != nil {
			// 历史记录不存在，这是一个孤儿分P记录，需要修复
			log.Printf("[FileScan] ⚠️  发现孤儿分P记录: PartID=%d, FilePath=%s, HistoryID=%d 不存在",
				existingPart.ID, filePath, existingPart.HistoryID)

			// 删除孤儿分P记录，重新导入
			if err := db.Delete(&existingPart).Error; err != nil {
				log.Printf("[FileScan] 删除孤儿分P记录失败: %v", err)
				return fmt.Errorf("删除孤儿分P记录失败: %w", err)
			}
			log.Printf("[FileScan] 已删除孤儿分P记录，将重新导入文件: %s", filePath)
			// 继续执行后续的导入逻辑
		} else {
			// 历史记录存在，文件正常跳过
			log.Printf("[FileScan] 文件已存在，跳过: %s", filePath)
			return ErrFileAlreadyExists
		}
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

	// 尝试解析弹幕XML文件
	xmlPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".xml"
	if _, err := os.Stat(xmlPath); err == nil {
		parser := NewDanmakuXMLParser()
		count, err := parser.ParseDanmakuFile(xmlPath, metadata.SessionID)
		if err != nil {
			log.Printf("[FileScan] ⚠️  解析弹幕失败 %s: %v", filepath.Base(xmlPath), err)
		} else {
			log.Printf("[FileScan] ✅ 成功解析 %d 条弹幕从 %s", count, filepath.Base(xmlPath))
		}
	}

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

	// 尝试从文件名解析（录播姬格式：录制-房间号-日期-时间-编号-标题.flv）
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

			// 提取标题 - 取最后一个 - 之后的内容
			// 格式: 录制-5050-20260101-183709-843-新年第一天直播 紧张.flv
			if len(fields) >= 5 {
				// 标题是最后一个字段，去除扩展名
				metadata.Title = fields[len(fields)-1]
				metadata.Title = strings.TrimSuffix(metadata.Title, filepath.Ext(metadata.Title))
			}
		}
	}

	// 生成 SessionID（使用 房间号+日期+时间 作为session标识，避免同一天多场直播被合并）
	// 使用小时级别的标识，同一小时内的视频可以合并
	sessionTimeStr := metadata.StartTime.Format("2006010215") // 精确到小时
	metadata.SessionID = fmt.Sprintf("%s_%s_scan", metadata.RoomID, sessionTimeStr)

	return metadata
}

// getOrCreateHistory 获取或创建历史记录
func (s *FileScanService) getOrCreateHistory(db *gorm.DB, metadata *FileMetadata, room *models.RecordRoom) (*models.RecordHistory, error) {
	// 先尝试通过 SessionID 查找
	var history models.RecordHistory
	if err := db.Where("session_id = ?", metadata.SessionID).First(&history).Error; err == nil {
		// 找到已有记录，检查标题是否一致
		// 如果标题差异较大，可能是不同的直播，不合并
		if !s.isSimilarTitle(history.Title, metadata.Title) {
			log.Printf("[FileScan] SessionID相同但标题差异过大，创建新记录: 已有=%s, 新=%s",
				history.Title, metadata.Title)
			// 修改SessionID以避免冲突
			metadata.SessionID = fmt.Sprintf("%s_%d", metadata.SessionID, time.Now().Unix())
		} else {
			// 标题相似，更新结束时间
			if metadata.EndTime.After(history.EndTime) {
				history.EndTime = metadata.EndTime
				db.Save(&history)
			}
			return &history, nil
		}
	}

	// 如果没有找到，尝试查找同一天同一房间的最近记录
	// 条件：时间差在2小时内 且 标题相似
	var histories []models.RecordHistory
	dayStart := metadata.StartTime.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	err := db.Where("room_id = ? AND start_time >= ? AND start_time < ?",
		metadata.RoomID, dayStart, dayEnd).
		Order("end_time DESC").
		Limit(10).
		Find(&histories).Error

	if err == nil && len(histories) > 0 {
		// 检查时间差和标题相似度
		for _, h := range histories {
			timeDiff := metadata.StartTime.Sub(h.EndTime)
			// 时间差在2小时内，且标题相似，才合并
			if timeDiff >= 0 && timeDiff < 2*time.Hour && s.isSimilarTitle(h.Title, metadata.Title) {
				// 找到可合并的历史记录
				if metadata.EndTime.After(h.EndTime) {
					h.EndTime = metadata.EndTime
					db.Save(&h)
				}
				log.Printf("[FileScan] 合并到已有历史记录: ID=%d, SessionID=%s (标题相似)", h.ID, h.SessionID)
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

// isSimilarTitle 判断两个标题是否相似
// 用于判断是否为同一场直播
func (s *FileScanService) isSimilarTitle(title1, title2 string) bool {
	// 移除常见的编号前缀（如 "193-"）
	cleanTitle1 := removeNumberPrefix(title1)
	cleanTitle2 := removeNumberPrefix(title2)

	// 如果清理后的标题完全相同，视为相似
	if cleanTitle1 == cleanTitle2 {
		return true
	}

	// 计算相似度（简单的包含关系判断）
	// 如果一个标题包含另一个标题的主要部分（长度>5），也视为相似
	if len(cleanTitle1) > 5 && len(cleanTitle2) > 5 {
		if strings.Contains(cleanTitle1, cleanTitle2) || strings.Contains(cleanTitle2, cleanTitle1) {
			return true
		}
	}

	// 计算编辑距离或其他相似度算法
	// 这里使用简单的单词匹配率
	similarity := calculateTitleSimilarity(cleanTitle1, cleanTitle2)
	return similarity > 0.6 // 相似度超过60%视为相似
}

// removeNumberPrefix 移除标题中的数字编号前缀
func removeNumberPrefix(title string) string {
	// 移除类似 "193-" 这样的前缀
	parts := strings.SplitN(title, "-", 2)
	if len(parts) == 2 {
		// 检查第一部分是否全是数字
		isNumber := true
		for _, c := range parts[0] {
			if c < '0' || c > '9' {
				isNumber = false
				break
			}
		}
		if isNumber {
			return strings.TrimSpace(parts[1])
		}
	}
	return title
}

// calculateTitleSimilarity 计算两个标题的相似度（0-1之间）
func calculateTitleSimilarity(title1, title2 string) float64 {
	// 简单的字符匹配算法
	if title1 == title2 {
		return 1.0
	}

	// 转换为rune数组以正确处理中文
	runes1 := []rune(title1)
	runes2 := []rune(title2)

	if len(runes1) == 0 || len(runes2) == 0 {
		return 0.0
	}

	// 计算最长公共子序列长度
	matchCount := 0
	for _, r1 := range runes1 {
		for _, r2 := range runes2 {
			if r1 == r2 {
				matchCount++
				break
			}
		}
	}

	// 相似度 = 匹配字符数 / 平均长度
	avgLen := float64(len(runes1)+len(runes2)) / 2.0
	return float64(matchCount) / avgLen
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
