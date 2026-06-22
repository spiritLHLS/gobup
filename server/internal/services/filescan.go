package services

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

// ErrFileAlreadyExists 文件已存在错误
var ErrFileAlreadyExists = errors.New("文件已存在于数据库中")

// danmakuBurnOutputRe 匹配弹幕烧录输出文件的正则: *_danmaku_YYYYMMDD_HHMMSS.mp4
// 这类文件是由 DanmakuBurnService.generateOutputPath 生成的临时文件，不应被当作新录制导入
var danmakuBurnOutputRe = regexp.MustCompile(`_danmaku_\d{8}_\d{6}\.mp4$`)

// isDanmakuBurnOutput 判断文件是否为弹幕烧录的临时输出文件
func isDanmakuBurnOutput(filename string) bool {
	return danmakuBurnOutputRe.MatchString(filename)
}

// ErrScanAlreadyRunning 扫描已在运行错误
var ErrScanAlreadyRunning = errors.New("文件扫描任务已在运行中，请稍后再试")

// scanRunning 防止同一进程内多个 ScanAndImport 调用并发执行。
// 无论来自定时调度还是 HTTP 手动触发，均共用此标记。
var scanRunning atomic.Bool

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

	maxFileAge := sysConfig.FileScanMaxAge / 24 // 转换为天
	if maxFileAge < 1 {
		if sysConfig.FileScanMaxAge > 0 {
			maxFileAge = 1 // 不足 24小时的值至少保留 1 天
		} else {
			maxFileAge = 30 // 0 表示不限制，默认 30 天
		}
	}
	scanIntervalHours := sysConfig.FileScanInterval / 60 // 转换为小时
	if scanIntervalHours < 1 {
		if sysConfig.FileScanInterval > 0 {
			scanIntervalHours = 1 // 不足 60 分钟的值至少 1 小时
		} else {
			scanIntervalHours = 1 // 0 表示不定时，默认 1 小时
		}
	}

	config := &ScanConfig{
		WorkPath:          workPath,
		VideoExtensions:   []string{".flv", ".mp4", ".mkv", ".ts"},
		MinFileSize:       sysConfig.FileScanMinSize,
		MinFileAge:        sysConfig.FileScanMinAge,
		MaxFileAge:        maxFileAge,
		ScanIntervalHours: scanIntervalHours,
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
	// 防止并发扫描（定时任务 + HTTP 手动触发 同时触发时，第二个直接返回）
	if !scanRunning.CompareAndSwap(false, true) {
		log.Println("[FileScan] 扫描任务已在运行中，跳过本次触发")
		return nil, ErrScanAlreadyRunning
	}
	defer scanRunning.Store(false)

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

		// 跳过弹幕烧录产生的临时文件（*_danmaku_YYYYMMDD_HHMMSS.mp4）
		// 这类文件已直接以 is_temp_file=true 入库，不应被当作独立录制重新导入
		if isDanmakuBurnOutput(filepath.Base(path)) {
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
// 扫描顺序与 ScanAndImport 一致：先自定义路径，再工作目录
func (s *FileScanService) PreviewFiles(config *ScanConfig) ([]*FilePreviewInfo, error) {
	db := database.GetDB()
	var previews []*FilePreviewInfo
	// 用于去重，防止同一文件被多个路径扫到
	seen := make(map[string]struct{})

	// walkDir 是内部辅助函数，遍历单个目录并追加结果
	walkDir := func(dirPath string) error {
		return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			// 去重
			if _, already := seen[path]; already {
				return nil
			}
			seen[path] = struct{}{}

			ext := strings.ToLower(filepath.Ext(path))
			if !s.isVideoFile(ext, config.VideoExtensions) {
				return nil
			}
			// 跳过弹幕烧录产生的临时文件（*_danmaku_YYYYMMDD_HHMMSS.mp4）
			if isDanmakuBurnOutput(filepath.Base(path)) {
				return nil
			}
			if info.Size() < config.MinFileSize {
				return nil
			}
			// 最近1分钟内修改过的文件不扫描
			if time.Since(info.ModTime()) < time.Minute {
				return nil
			}

			// 检查文件是否已在数据库（同时验证对应的历史记录存在，避免孤儿分P导致文件被误判为已入库）
			var existingPart models.RecordHistoryPart
			inDatabase := false
			if db.Where("file_path = ?", path).First(&existingPart).Error == nil {
				// 分P记录存在，再确认其历史记录也存在（未被软删除）
				var linkedHistory models.RecordHistory
				if db.Where("id = ?", existingPart.HistoryID).First(&linkedHistory).Error == nil {
					inDatabase = true
				}
				// 若历史记录不存在则视为孤儿分P，inDatabase 保持 false，让用户可以重新导入
			}

			// 解析文件元数据
			metadata := s.parseFileMetadata(path, info)
			if metadata == nil {
				metadata = &FileMetadata{
					RoomID: "unknown",
					Uname:  "未知主播",
					Title:  filepath.Base(path),
				}
			}

			previews = append(previews, &FilePreviewInfo{
				FilePath:   path,
				FileName:   filepath.Base(path),
				FileSize:   info.Size(),
				ModTime:    info.ModTime(),
				RoomID:     metadata.RoomID,
				Uname:      metadata.Uname,
				Title:      metadata.Title,
				InDatabase: inDatabase,
			})
			return nil
		})
	}

	// 1. 先扫描自定义路径（与 ScanAndImport 行为一致）
	customPaths := getCustomScanPaths()
	for _, cp := range customPaths {
		log.Printf("[FileScan] 预览扫描自定义目录: %s", cp)
		if err := walkDir(cp); err != nil {
			log.Printf("[FileScan] 预览扫描自定义目录失败: %s, error: %v", cp, err)
		}
	}

	// 2. 扫描工作目录（WorkPath 不存在时仅警告，不中断——自定义路径的结果仍可返回）
	if config.WorkPath == "" {
		if len(customPaths) == 0 {
			return previews, fmt.Errorf("工作目录未配置，且没有有效的自定义扫描路径")
		}
		log.Printf("[FileScan] 预览：工作目录未配置，仅扫描自定义路径")
	} else if _, err := os.Stat(config.WorkPath); os.IsNotExist(err) {
		if len(customPaths) == 0 {
			return previews, fmt.Errorf("工作目录不存在: %s (提示: Docker部署请检查是否挂载了录播目录到/rec，裸机部署请确保./data/recordings存在)", config.WorkPath)
		}
		log.Printf("[FileScan] 预览：工作目录不存在 (%s)，仅扫描自定义路径", config.WorkPath)
	} else {
		log.Printf("[FileScan] 预览扫描工作目录: %s", config.WorkPath)
		if err := walkDir(config.WorkPath); err != nil {
			log.Printf("[FileScan] 预览扫描工作目录失败: %v", err)
		}
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
			log.Printf("[FileScan] 发现孤儿分P记录: PartID=%d, FilePath=%s, HistoryID=%d 不存在",
				existingPart.ID, filePath, existingPart.HistoryID)

			// 删除孤儿分P记录，重新导入
			if err := db.Delete(&existingPart).Error; err != nil {
				log.Printf("[FileScan] 删除孤儿分P记录失败: %v", err)
				return fmt.Errorf("删除孤儿分P记录失败: %w", err)
			}
			log.Printf("[FileScan] 已删除孤儿分P记录，将重新导入文件: %s", filePath)
			// 继续执行后续的导入逻辑
		} else {
			// 历史记录存在，进一步检查是否已投稿
			if existingHistory.Publish {
				log.Printf("[FileScan] 警告: 文件已存在于已投稿的历史记录中: PartID=%d, HistoryID=%d, BvID=%s",
					existingPart.ID, existingHistory.ID, existingHistory.BvID)
				// 已投稿的文件不应该被重新导入，防止重复投稿
				return fmt.Errorf("文件已存在于已投稿的历史记录中，不允许重复导入")
			}
			if existingPart.Upload {
				log.Printf("[FileScan] 警告: 文件已存在且已上传: PartID=%d, HistoryID=%d, CID=%d",
					existingPart.ID, existingHistory.ID, existingPart.CID)
				// 已上传的文件不应该被重新导入
				return fmt.Errorf("文件已存在且已上传，不允许重复导入")
			}
			// 文件正常跳过
			log.Printf("[FileScan] 文件已存在，跳过: %s", filePath)
			return ErrFileAlreadyExists
		}
	}

	// 2. 从文件路径解析房间信息
	metadata := s.parseFileMetadata(filePath, info)
	if metadata == nil {
		return fmt.Errorf("无法解析文件元数据")
	}

	// 2.5 再次检查直播状态
	if metadata.RoomID != "" && metadata.RoomID != "unknown" {
		liveStatusService := NewLiveStatusService()
		isFinished, usedFallback, err := liveStatusService.IsRoomRecordingFinished(metadata.RoomID, info.ModTime(), metadata.Title)
		if err == nil && !isFinished {
			if usedFallback {
				log.Printf("[FileScan] 拒绝导入：文件修改时间过近（保底逻辑 < 1小时）: %s", filePath)
			} else {
				log.Printf("[FileScan] 拒绝导入：房间 %s 直播未结束或文件未稳定: %s", metadata.RoomID, filePath)
			}
			return fmt.Errorf("直播未结束，拒绝导入文件")
		}

		// 额外检查：如果存在相同SessionID的历史记录，确保该场直播已经完全结束
		var existingHistory models.RecordHistory
		existingHistoryQuery := db.Where("session_id = ?", metadata.SessionID)
		if dayStart, dayEnd, ok := models.LiveSessionDayRange(metadata.StartTime); ok {
			existingHistoryQuery = existingHistoryQuery.Where("start_time >= ? AND start_time < ?", dayStart, dayEnd)
		}
		if err := existingHistoryQuery.First(&existingHistory).Error; err == nil {
			// 检查该历史记录是否标记为正在录制或直播中
			if existingHistory.Recording || existingHistory.Streaming {
				log.Printf("[FileScan] 拒绝导入：该Session的历史记录显示仍在录制/直播中: SessionID=%s, Recording=%v, Streaming=%v",
					metadata.SessionID, existingHistory.Recording, existingHistory.Streaming)
				return fmt.Errorf("该场直播仍在进行中，拒绝导入文件")
			}
		}
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

	// 4. 查找或创建历史记录（使用事务保护，防止并发问题）
	var history *models.RecordHistory
	var createErr error

	// 重试机制：防止并发导致的SessionID冲突
	for retry := 0; retry < 3; retry++ {
		history, createErr = s.getOrCreateHistory(db, metadata, &room)
		if createErr == nil {
			break
		}
		// 如果是SessionID冲突，修改SessionID后重试（使用小写比较兼容 SQLite 的 "UNIQUE" 和 MySQL 的 "Duplicate"）
		errMsgLower := strings.ToLower(createErr.Error())
		if strings.Contains(errMsgLower, "duplicate") || strings.Contains(errMsgLower, "unique") {
			log.Printf("[FileScan] 检测到SessionID冲突，修改后重试 (第%d次)", retry+1)
			metadata.SessionID = fmt.Sprintf("%s_%d_%d", metadata.SessionID, time.Now().Unix(), retry)
			time.Sleep(time.Millisecond * 100) // 短暂等待
			continue
		}
		// 其他错误直接返回
		break
	}
	if createErr != nil {
		return fmt.Errorf("获取或创建历史记录失败: %w", createErr)
	}

	// 5. 创建分P记录（三次检查防止重复）
	// 第一次检查：确认文件不存在
	var existingPartCheck models.RecordHistoryPart
	if err := db.Where("file_path = ?", filePath).First(&existingPartCheck).Error; err == nil {
		log.Printf("[FileScan] 文件在导入过程中已被其他进程导入，跳过: %s", filePath)
		return ErrFileAlreadyExists
	}

	// 第二次检查：确认文件不属于其他历史记录
	var otherHistoryParts []models.RecordHistoryPart
	if err := db.Where("file_path = ? AND history_id != ?", filePath, history.ID).Find(&otherHistoryParts).Error; err == nil && len(otherHistoryParts) > 0 {
		log.Printf("[FileScan] 错误: 文件已存在于其他历史记录中: %s, 其他HistoryID=%d",
			filePath, otherHistoryParts[0].HistoryID)
		return fmt.Errorf("文件已存在于其他历史记录中，不能重复导入")
	}

	// 第三次检查：确认该历史记录未被投稿
	if history.Publish {
		log.Printf("[FileScan] 错误: 尝试将文件导入到已投稿的历史记录: HistoryID=%d, BvID=%s",
			history.ID, history.BvID)
		return fmt.Errorf("不能将文件导入到已投稿的历史记录")
	}
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
		// 获取房间配置用于应用过滤规则
		var room models.RecordRoom
		var roomPtr *models.RecordRoom
		if err := db.Where("room_id = ?", metadata.RoomID).First(&room).Error; err == nil {
			roomPtr = &room
		} else {
			log.Printf("[FileScan] 警告: 未找到房间配置 (room_id=%s)，将不应用过滤规则", metadata.RoomID)
		}

		parser := NewDanmakuXMLParser()
		count, err := parser.ParseDanmakuFile(xmlPath, metadata.SessionID, roomPtr, part.ID)
		if err != nil {
			log.Printf("[FileScan] 解析弹幕失败 %s: %v", filepath.Base(xmlPath), err)
		} else {
			log.Printf("[FileScan] 成功解析 %d 条弹幕从 %s", count, filepath.Base(xmlPath))
		}
	}

	return nil
}

// FileMetadata 文件元数据
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

// FileToClean 待清理的文件信息
type FileToClean struct {
	FilePath   string `json:"filePath"`   // 文件路径
	FileType   string `json:"fileType"`   // 文件类型 xml/jpg
	FileSize   int64  `json:"fileSize"`   // 文件大小（字节）
	HistoryID  uint   `json:"historyId"`  // 关联的历史记录ID
	RoomName   string `json:"roomName"`   // 房间名称
	Title      string `json:"title"`      // 标题
	RecordTime string `json:"recordTime"` // 录制时间
}

// CleanCompletedFilesPreviewResult 清理已完成文件的预览结果
type CleanCompletedFilesPreviewResult struct {
	TotalHistories int           `json:"totalHistories"` // 检查的历史记录总数
	FilesToClean   []FileToClean `json:"filesToClean"`   // 待清理的文件列表
}

// CleanCompletedFilesResult 清理已完成文件的结果
type CleanCompletedFilesResult struct {
	TotalHistories   int      `json:"totalHistories"`   // 检查的历史记录总数
	DeletedXMLFiles  int      `json:"deletedXMLFiles"`  // 删除的XML文件数
	DeletedJPGFiles  int      `json:"deletedJPGFiles"`  // 删除的JPG文件数
	SkippedHistories int      `json:"skippedHistories"` // 跳过的历史记录数
	Errors           []string `json:"errors"`           // 错误信息列表
}

// CleanCompletedFiles 清理已上传投稿成功且解析弹幕完成且已发送弹幕的历史记录的xml和jpg文件
func (s *FileScanService) CleanCompletedFiles() (*CleanCompletedFilesResult, error) {
	db := database.GetDB()
	result := &CleanCompletedFilesResult{
		Errors: make([]string, 0),
	}

	// 查询所有已上传投稿成功(uploadStatus=2)且弹幕已发送(danmakuSent=true)的历史记录
	var histories []models.RecordHistory
	if err := db.Where("upload_status = ? AND danmaku_sent = ?", 2, true).Find(&histories).Error; err != nil {
		return nil, fmt.Errorf("查询历史记录失败: %w", err)
	}

	result.TotalHistories = len(histories)
	log.Printf("[FileScan] 开始清理已完成的历史记录文件，共 %d 条记录", result.TotalHistories)

	for _, history := range histories {
		// 获取该历史记录的所有分P
		var parts []models.RecordHistoryPart
		if err := db.Where("history_id = ?", history.ID).Find(&parts).Error; err != nil {
			errMsg := fmt.Sprintf("查询历史记录 %d 的分P失败: %v", history.ID, err)
			log.Printf("[FileScan] %s", errMsg)
			result.Errors = append(result.Errors, errMsg)
			result.SkippedHistories++
			continue
		}

		// 遍历每个分P，删除对应的xml和jpg文件
		for _, part := range parts {
			if part.FilePath == "" {
				continue
			}

			// 构造xml和jpg文件路径
			basePathWithoutExt := strings.TrimSuffix(part.FilePath, filepath.Ext(part.FilePath))
			xmlPath := basePathWithoutExt + ".xml"
			jpgPath := basePathWithoutExt + ".jpg"

			// 删除xml文件
			if _, err := os.Stat(xmlPath); err == nil {
				if err := os.Remove(xmlPath); err != nil {
					errMsg := fmt.Sprintf("删除xml文件失败: %s, error: %v", xmlPath, err)
					log.Printf("[FileScan] %s", errMsg)
					result.Errors = append(result.Errors, errMsg)
				} else {
					result.DeletedXMLFiles++
					log.Printf("[FileScan] 删除xml文件: %s", xmlPath)
				}
			}

			// 删除jpg文件
			if _, err := os.Stat(jpgPath); err == nil {
				if err := os.Remove(jpgPath); err != nil {
					errMsg := fmt.Sprintf("删除jpg文件失败: %s, error: %v", jpgPath, err)
					log.Printf("[FileScan] %s", errMsg)
					result.Errors = append(result.Errors, errMsg)
				} else {
					result.DeletedJPGFiles++
					log.Printf("[FileScan] 删除jpg文件: %s", jpgPath)
				}
			}
		}
	}

	log.Printf("[FileScan] 清理已完成文件结束: 检查 %d 条历史记录, 删除 %d 个xml文件, %d 个jpg文件, 跳过 %d 条记录",
		result.TotalHistories, result.DeletedXMLFiles, result.DeletedJPGFiles, result.SkippedHistories)

	return result, nil
}

// GetCompletedFilesPreview 获取待清理文件的预览列表
func (s *FileScanService) GetCompletedFilesPreview() (*CleanCompletedFilesPreviewResult, error) {
	db := database.GetDB()
	result := &CleanCompletedFilesPreviewResult{
		FilesToClean: make([]FileToClean, 0),
	}

	// 查询所有已上传投稿成功(uploadStatus=2)且弹幕已发送(danmakuSent=true)的历史记录
	var histories []models.RecordHistory
	if err := db.Where("upload_status = ? AND danmaku_sent = ?", 2, true).Find(&histories).Error; err != nil {
		return nil, fmt.Errorf("查询历史记录失败: %w", err)
	}

	result.TotalHistories = len(histories)
	log.Printf("[FileScan] 开始预览可清理的历史记录文件，共 %d 条记录", result.TotalHistories)

	for _, history := range histories {
		// 获取该历史记录的所有分P
		var parts []models.RecordHistoryPart
		if err := db.Where("history_id = ?", history.ID).Find(&parts).Error; err != nil {
			log.Printf("[FileScan] 查询历史记录 %d 的分P失败: %v", history.ID, err)
			continue
		}

		// 遍历每个分P，检查xml和jpg文件
		for _, part := range parts {
			if part.FilePath == "" {
				continue
			}

			// 构造xml和jpg文件路径
			basePathWithoutExt := strings.TrimSuffix(part.FilePath, filepath.Ext(part.FilePath))
			xmlPath := basePathWithoutExt + ".xml"
			jpgPath := basePathWithoutExt + ".jpg"

			// 检查xml文件
			if info, err := os.Stat(xmlPath); err == nil {
				result.FilesToClean = append(result.FilesToClean, FileToClean{
					FilePath:   xmlPath,
					FileType:   "xml",
					FileSize:   info.Size(),
					HistoryID:  history.ID,
					RoomName:   history.RoomName,
					Title:      history.Title,
					RecordTime: history.StartTime.Format("2006-01-02 15:04:05"),
				})
			}

			// 检查jpg文件
			if info, err := os.Stat(jpgPath); err == nil {
				result.FilesToClean = append(result.FilesToClean, FileToClean{
					FilePath:   jpgPath,
					FileType:   "jpg",
					FileSize:   info.Size(),
					HistoryID:  history.ID,
					RoomName:   history.RoomName,
					Title:      history.Title,
					RecordTime: history.StartTime.Format("2006-01-02 15:04:05"),
				})
			}
		}
	}

	log.Printf("[FileScan] 预览完成: 检查 %d 条历史记录, 找到 %d 个可清理文件",
		result.TotalHistories, len(result.FilesToClean))

	return result, nil
}

// CleanSelectedFiles 清理用户选择的文件
func (s *FileScanService) CleanSelectedFiles(filePaths []string) (*CleanCompletedFilesResult, error) {
	result := &CleanCompletedFilesResult{
		Errors: make([]string, 0),
	}

	log.Printf("[FileScan] 开始清理用户选择的文件，共 %d 个", len(filePaths))

	for _, filePath := range filePaths {
		// 检查文件是否存在
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			errMsg := fmt.Sprintf("文件不存在: %s", filePath)
			log.Printf("[FileScan] %s", errMsg)
			result.Errors = append(result.Errors, errMsg)
			continue
		}

		// 删除文件
		if err := os.Remove(filePath); err != nil {
			errMsg := fmt.Sprintf("删除文件失败: %s, error: %v", filePath, err)
			log.Printf("[FileScan] %s", errMsg)
			result.Errors = append(result.Errors, errMsg)
		} else {
			// 根据文件扩展名计数
			ext := strings.ToLower(filepath.Ext(filePath))
			if ext == ".xml" {
				result.DeletedXMLFiles++
			} else if ext == ".jpg" || ext == ".jpeg" {
				result.DeletedJPGFiles++
			}
			log.Printf("[FileScan] 删除文件: %s", filePath)
		}
	}

	log.Printf("[FileScan] 清理选中文件完成: 删除 %d 个xml文件, %d 个jpg文件",
		result.DeletedXMLFiles, result.DeletedJPGFiles)

	return result, nil
}
