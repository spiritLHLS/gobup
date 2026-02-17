package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

// DanmakuBurnService 弹幕烧录服务
type DanmakuBurnService struct{}

func NewDanmakuBurnService() *DanmakuBurnService {
	return &DanmakuBurnService{}
}

// BurnDanmakuToVideo 将弹幕烧录到视频文件
// 返回：生成的带弹幕视频路径，错误
func (s *DanmakuBurnService) BurnDanmakuToVideo(part *models.RecordHistoryPart, history *models.RecordHistory, room *models.RecordRoom) (string, error) {
	db := database.GetDB()

	// 检查源视频文件是否存在
	if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("源视频文件不存在: %s", part.FilePath)
	}

	// 检查是否有对应的XML弹幕文件
	xmlPath := s.findDanmakuXML(part.FilePath)
	if xmlPath == "" {
		log.Printf("[弹幕烧录] 未找到弹幕XML文件，跳过烧录: %s", part.FilePath)
		return "", fmt.Errorf("未找到弹幕XML文件")
	}

	log.Printf("[弹幕烧录] 开始为视频烧录弹幕: %s", part.FilePath)
	log.Printf("[弹幕烧录] 弹幕文件: %s", xmlPath)

	// 1. 将XML转换为ASS字幕文件
	assPath, err := s.convertXMLToASS(xmlPath, history, room)
	if err != nil {
		return "", fmt.Errorf("转换弹幕为ASS失败: %w", err)
	}
	defer os.Remove(assPath) // 临时文件，用完删除

	log.Printf("[弹幕烧录] ASS字幕文件生成: %s", assPath)

	// 2. 使用ffmpeg烧录弹幕
	outputPath := s.generateOutputPath(part.FilePath)
	if err := s.burnWithFFmpeg(part.FilePath, assPath, outputPath); err != nil {
		return "", fmt.Errorf("ffmpeg烧录失败: %w", err)
	}

	// 3. 获取生成文件的信息
	outputFileInfo, err := os.Stat(outputPath)
	if err != nil {
		return "", fmt.Errorf("获取输出文件信息失败: %w", err)
	}

	log.Printf("[弹幕烧录] 烧录完成: %s (大小: %d MB)", outputPath, outputFileInfo.Size()/1024/1024)

	// 4. 创建新的Part记录（标记为临时文件）
	burnedPart := &models.RecordHistoryPart{
		HistoryID:    part.HistoryID,
		RoomID:       part.RoomID,
		SessionID:    part.SessionID,
		Title:        part.Title + " (弹幕版)",
		LiveTitle:    part.LiveTitle,
		AreaName:     part.AreaName,
		FilePath:     outputPath,
		FileName:     filepath.Base(outputPath),
		FileSize:     outputFileInfo.Size(),
		Duration:     part.Duration,
		StartTime:    part.StartTime,
		EndTime:      part.EndTime,
		Recording:    false,
		Upload:       false,
		Uploading:    false,
		IsTempFile:   true,           // 标记为临时文件
		SourcePartID: part.ID,        // 记录源Part ID
		TempFileType: "danmaku_burn", // 临时文件类型
	}

	if err := db.Create(burnedPart).Error; err != nil {
		os.Remove(outputPath) // 创建记录失败，删除文件
		return "", fmt.Errorf("创建弹幕版Part记录失败: %w", err)
	}

	log.Printf("[弹幕烧录] 弹幕版Part创建成功: ID=%d", burnedPart.ID)

	return outputPath, nil
}

// findDanmakuXML 查找对应的弹幕XML文件
func (s *DanmakuBurnService) findDanmakuXML(videoPath string) string {
	basePath := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	xmlPath := basePath + ".xml"

	if _, err := os.Stat(xmlPath); err == nil {
		return xmlPath
	}

	return ""
}

// convertXMLToASS 将XML弹幕转换为ASS字幕格式
func (s *DanmakuBurnService) convertXMLToASS(xmlPath string, history *models.RecordHistory, room *models.RecordRoom) (string, error) {
	db := database.GetDB()

	// 从数据库读取弹幕（已去重和过滤）
	var danmakus []models.LiveMsg
	if err := db.Where("session_id = ?", history.SessionID).
		Order("timestamp ASC").
		Find(&danmakus).Error; err != nil {
		return "", fmt.Errorf("查询弹幕失败: %w", err)
	}

	if len(danmakus) == 0 {
		return "", fmt.Errorf("没有弹幕数据")
	}

	log.Printf("[弹幕烧录] 读取到 %d 条弹幕", len(danmakus))

	// 生成ASS文件路径
	assPath := strings.TrimSuffix(xmlPath, filepath.Ext(xmlPath)) + "_temp.ass"

	// 创建ASS文件
	f, err := os.Create(assPath)
	if err != nil {
		return "", fmt.Errorf("创建ASS文件失败: %w", err)
	}
	defer f.Close()

	// 写入ASS头部
	style := s.getASSStyle(room.DanmakuBurnStyle)
	f.WriteString(s.getASSHeader(style))

	// 写入弹幕事件
	for _, dm := range danmakus {
		event := s.convertDanmakuToASSEvent(dm)
		f.WriteString(event)
	}

	return assPath, nil
}

// getASSHeader 生成ASS文件头部
func (s *DanmakuBurnService) getASSHeader(style string) string {
	return fmt.Sprintf(`[Script Info]
Title: Bilibili Danmaku
ScriptType: v4.00+
WrapStyle: 2
PlayResX: 1920
PlayResY: 1080
ScaledBorderAndShadow: yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
%s

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`, style)
}

// getASSStyle 根据配置获取ASS样式
func (s *DanmakuBurnService) getASSStyle(styleName string) string {
	switch styleName {
	case "compact":
		// 紧凑样式：小字号
		return "Style: Danmaku,Microsoft YaHei,32,&H00FFFFFF,&H00FFFFFF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,1.5,0,2,10,10,10,1"
	case "large":
		// 大字号样式
		return "Style: Danmaku,Microsoft YaHei,48,&H00FFFFFF,&H00FFFFFF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1"
	default:
		// 默认样式
		return "Style: Danmaku,Microsoft YaHei,38,&H00FFFFFF,&H00FFFFFF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,1.8,0,2,10,10,10,1"
	}
}

// convertDanmakuToASSEvent 将弹幕转换为ASS事件
func (s *DanmakuBurnService) convertDanmakuToASSEvent(dm models.LiveMsg) string {
	// 时间戳转换为ASS时间格式 (时:分:秒.毫秒)
	startTime := s.formatASSTime(dm.Timestamp)
	endTime := s.formatASSTime(dm.Timestamp + 8000) // 弹幕显示8秒

	// 转义特殊字符
	text := s.escapeASSText(dm.Message)

	// 根据弹幕模式设置效果
	effect := ""
	alignment := "2" // 顶部居中

	switch dm.Mode {
	case 1: // 滚动弹幕
		// 从右向左滚动
		effect = fmt.Sprintf("\\move(2000,100,100,100)")
		alignment = "7" // 左上
	case 4: // 底部弹幕
		alignment = "2" // 底部居中
	case 5: // 顶部弹幕
		alignment = "8" // 顶部居中
	}

	// 颜色转换
	color := s.convertColorToASS(dm.Color)

	// 构建ASS事件行
	return fmt.Sprintf("Dialogue: 0,%s,%s,Danmaku,,0,0,0,%s,{\\c%s}%s\n",
		startTime, endTime, effect, color, text)
}

// formatASSTime 格式化ASS时间格式
func (s *DanmakuBurnService) formatASSTime(timestampMs int64) string {
	totalSeconds := timestampMs / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	centiseconds := (timestampMs % 1000) / 10

	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, centiseconds)
}

// escapeASSText 转义ASS文本中的特殊字符
func (s *DanmakuBurnService) escapeASSText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "{", "\\{")
	text = strings.ReplaceAll(text, "}", "\\}")
	text = strings.ReplaceAll(text, "\n", "\\N")
	return text
}

// convertColorToASS 将十进制颜色转换为ASS颜色格式
func (s *DanmakuBurnService) convertColorToASS(color int) string {
	// B站颜色是RGB格式，ASS需要BGR格式
	r := (color >> 16) & 0xFF
	g := (color >> 8) & 0xFF
	b := color & 0xFF

	return fmt.Sprintf("&H%02X%02X%02X&", b, g, r)
}

// burnWithFFmpeg 使用ffmpeg烧录字幕
func (s *DanmakuBurnService) burnWithFFmpeg(videoPath, assPath, outputPath string) error {
	// 检查ffmpeg是否可用
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg未安装或不在PATH中: %w", err)
	}

	// ffmpeg命令：烧录字幕
	// -i: 输入视频
	// -vf subtitles: 使用字幕过滤器烧录
	// -c:a copy: 音频直接复制，不重新编码
	// -preset fast: 快速编码（可选：ultrafast, fast, medium）
	args := []string{
		"-i", videoPath,
		"-vf", fmt.Sprintf("subtitles='%s':force_style='Fontname=Microsoft YaHei,Fontsize=38'",
			strings.ReplaceAll(assPath, "\\", "/")), // Windows路径需要转换
		"-c:a", "copy",
		"-preset", "fast", // 快速编码
		"-y",              // 覆盖已存在的文件
		outputPath,
	}

	log.Printf("[弹幕烧录] 执行ffmpeg命令...")

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("[弹幕烧录] ffmpeg输出: %s", string(output))
		return fmt.Errorf("ffmpeg执行失败: %w", err)
	}

	return nil
}

// generateOutputPath 生成输出文件路径
func (s *DanmakuBurnService) generateOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	ext := filepath.Ext(inputPath)

	// 生成带时间戳的文件名，避免冲突
	timestamp := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("%s_danmaku_%s%s", baseName, timestamp, ext))
}

// CleanTempFiles 清理临时文件（上传成功后调用）
func (s *DanmakuBurnService) CleanTempFiles(historyID uint) error {
	db := database.GetDB()

	// 查找该历史记录的所有临时文件且已上传的Part
	var tempParts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND is_temp_file = ? AND upload = ?",
		historyID, true, true).Find(&tempParts).Error; err != nil {
		return fmt.Errorf("查询临时文件失败: %w", err)
	}

	log.Printf("[临时文件清理] 找到 %d 个需要清理的临时文件 (history_id=%d)", len(tempParts), historyID)

	successCount := 0
	for _, part := range tempParts {
		// 删除物理文件
		if part.FilePath != "" {
			if err := os.Remove(part.FilePath); err != nil && !os.IsNotExist(err) {
				log.Printf("[临时文件清理] 删除文件失败: %s, error: %v", part.FilePath, err)
				continue
			}
			log.Printf("[临时文件清理] ✓ 已删除临时文件: %s", part.FilePath)
		}

		// 更新数据库标记
		part.FileDelete = true
		db.Save(&part)
		successCount++
	}

	log.Printf("[临时文件清理] 清理完成: 成功 %d/%d", successCount, len(tempParts))
	return nil
}

// CleanTempFilesBySessionID 按SessionID清理临时文件（投稿成功后调用）
func (s *DanmakuBurnService) CleanTempFilesBySessionID(sessionID string) error {
	db := database.GetDB()

	// 查找该SessionID的所有临时文件且已上传的Part
	var tempParts []models.RecordHistoryPart
	if err := db.Where("session_id = ? AND is_temp_file = ? AND upload = ?",
		sessionID, true, true).Find(&tempParts).Error; err != nil {
		return fmt.Errorf("查询临时文件失败: %w", err)
	}

	log.Printf("[临时文件清理] SessionID=%s 找到 %d 个需要清理的临时文件", sessionID, len(tempParts))

	successCount := 0
	for _, part := range tempParts {
		if part.FilePath != "" {
			if err := os.Remove(part.FilePath); err != nil && !os.IsNotExist(err) {
				log.Printf("[临时文件清理] 删除文件失败: %s, error: %v", part.FilePath, err)
				continue
			}
			log.Printf("[临时文件清理] ✓ 已删除临时文件: %s", part.FilePath)
		}

		part.FileDelete = true
		db.Save(&part)
		successCount++
	}

	log.Printf("[临时文件清理] 清理完成: 成功 %d/%d", successCount, len(tempParts))
	return nil
}
