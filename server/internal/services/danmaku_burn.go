package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

const (
	// DanmakuFactory 路径（可通过环境变量配置）
	DanmakuFactoryPath = "/usr/local/bin/danmakufactory/DanmakuFactory"

	// DanmakuFontName 弹幕字体名（容器内通过 font-wqy-zenhei 安装）
	DanmakuFontName = "WenQuanYi Zen Hei"

	// DanmakuFontsDir 字体目录（供 libass 查找字体）
	DanmakuFontsDir = "/usr/share/fonts"
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

	// 去重检查：若已存在该源分P的烧录记录且文件完好，直接返回，避免重复烧录
	var existingBurnedPart models.RecordHistoryPart
	if err := db.Where(
		"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ? AND file_delete = ?",
		part.ID, true, "danmaku_burn", false,
	).First(&existingBurnedPart).Error; err == nil {
		if existingBurnedPart.Upload {
			log.Printf("[弹幕烧录] 烧录版已上传，跳过重复烧录: source_part_id=%d, burned_part_id=%d", part.ID, existingBurnedPart.ID)
			return existingBurnedPart.FilePath, nil
		}
		if _, statErr := os.Stat(existingBurnedPart.FilePath); statErr == nil {
			log.Printf("[弹幕烧录] 烧录版文件已存在，跳过重复烧录: source_part_id=%d, file=%s", part.ID, existingBurnedPart.FilePath)
			return existingBurnedPart.FilePath, nil
		}
		// 文件已不存在，清理孤立DB记录，继续重新烧录
		log.Printf("[弹幕烧录] 烧录版文件缺失，删除孤立记录后重新烧录: burned_part_id=%d", existingBurnedPart.ID)
		db.Delete(&existingBurnedPart)
	}

	log.Printf("[弹幕烧录] 开始为视频烧录弹幕: %s", part.FilePath)
	log.Printf("[弹幕烧录] 弹幕文件: %s", xmlPath)

	// 1. 将XML转换为ASS字幕文件
	// 优先使用 DanmakuFactory，如果不可用则使用内置转换
	assPath, err := s.convertXMLToASSWithFactory(xmlPath, history, room)
	if err != nil {
		log.Printf("[弹幕烧录] DanmakuFactory 转换失败，尝试使用内置转换: %v", err)
		assPath, err = s.convertXMLToASS(xmlPath, history, room)
		if err != nil {
			return "", fmt.Errorf("转换弹幕为ASS失败: %w", err)
		}
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

// convertXMLToASSWithFactory 使用 DanmakuFactory 将XML转换为ASS
func (s *DanmakuBurnService) convertXMLToASSWithFactory(xmlPath string, history *models.RecordHistory, room *models.RecordRoom) (string, error) {
	// 获取 DanmakuFactory 路径
	factoryPath := os.Getenv("DANMAKU_FACTORY_PATH")
	if factoryPath == "" {
		factoryPath = DanmakuFactoryPath
	}

	// 检查 DanmakuFactory 是否存在
	if _, err := os.Stat(factoryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("DanmakuFactory 未安装: %s", factoryPath)
	}

	// 生成输出ASS文件路径
	assPath := strings.TrimSuffix(xmlPath, filepath.Ext(xmlPath)) + "_danmaku.ass"

	log.Printf("[弹幕烧录] 使用 DanmakuFactory 转换: %s -> %s", xmlPath, assPath)

	// 构建命令参数
	// DanmakuFactory -i input.xml -o output.ass -r 1920x1080 -d -1 -S 38 --scrollarea 0.75 --displayarea 0.8
	args := []string{
		"-i", xmlPath,
		"-o", assPath,
		"-r", "1920x1080",      // 分辨率
		"-d", "-1",             // 弹幕密度：-1=不重叠
		"--scrollarea", "0.75", // 滚动弹幕显示区域：75%
		"--displayarea", "0.8", // 全部弹幕显示区域：80%
	}

	// 根据房间配置调整弹幕样式
	switch room.DanmakuBurnStyle {
	case "compact":
		args = append(args, "-S", "32") // 字号32px
	case "large":
		args = append(args, "-S", "48") // 字号48px
	default:
		args = append(args, "-S", "38") // 默认字号38px
	}

	// 执行 DanmakuFactory
	cmd := exec.Command(factoryPath, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("[弹幕烧录] DanmakuFactory 执行失败: %s", string(output))
		return "", fmt.Errorf("DanmakuFactory 执行失败: %w", err)
	}

	// 检查输出文件是否生成
	if _, err := os.Stat(assPath); os.IsNotExist(err) {
		return "", fmt.Errorf("ASS文件未生成: %s", assPath)
	}

	log.Printf("[弹幕烧录] DanmakuFactory 转换成功: %s", assPath)
	return assPath, nil
}

// convertXMLToASS 将XML弹幕转换为ASS字幕格式（内置实现，作为后备方案）
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
		log.Printf("[弹幕烧录] 数据库中没有弹幕数据（session_id=%s），尝试先解析XML文件", history.SessionID)
		
		// 如果数据库中没有弹幕，尝试解析XML文件
		parser := NewDanmakuXMLParser()
		var roomPtr *models.RecordRoom
		if room.ID > 0 {
			roomPtr = room
		}
		
		// 解析XML文件到数据库
		count, err := parser.ParseDanmakuFile(xmlPath, history.SessionID, roomPtr)
		if err != nil {
			return "", fmt.Errorf("解析弹幕失败: %w", err)
		}
		
		log.Printf("[弹幕烧录] XML解析完成，导入了 %d 条弹幕", count)
		
		// 重新查询弹幕
		if err := db.Where("session_id = ?", history.SessionID).
			Order("timestamp ASC").
			Find(&danmakus).Error; err != nil {
			return "", fmt.Errorf("重新查询弹幕失败: %w", err)
		}
		
		if len(danmakus) == 0 {
			return "", fmt.Errorf("没有弹幕数据可供烧录")
		}
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
YCbCr Matrix: TV.709

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
%s

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`, style)
}

// getASSStyle 根据配置获取ASS样式
// 固定参数（按图片设置）：不透明度79%、字号100%（38px）、显示区域50%
func (s *DanmakuBurnService) getASSStyle(styleName string) string {
	// 不透明度79% = 0.79 * 255 = 201 = C9（十六进制）
	// BackColour的alpha部分设置为79%的不透明度
	// 注意：字体使用容器内安装的 WenQuanYi Zen Hei（通过 font-wqy-zenhei 安装）
	// 若通过环境变量 DANMAKU_FONT_NAME 自定义字体，仍以 burnWithFFmpeg 中的 force_style 为准
	switch styleName {
	case "compact":
		// 紧凑样式：小字号（字号100%=32px）
		return "Style: Danmaku,WenQuanYi Zen Hei,32,&HC9FFFFFF,&HC9FFFFFF,&H00000000,&HC9000000,0,0,0,0,100,100,0,0,1,1.5,0,2,10,10,10,1"
	case "large":
		// 大字号样式（字号100%=48px）
		return "Style: Danmaku,WenQuanYi Zen Hei,48,&HC9FFFFFF,&HC9FFFFFF,&H00000000,&HC9000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1"
	default:
		// 默认样式（字号100%=38px，不透明度79%）
		return "Style: Danmaku,WenQuanYi Zen Hei,38,&HC9FFFFFF,&HC9FFFFFF,&H00000000,&HC9000000,0,0,0,0,100,100,0,0,1,1.8,0,2,10,10,10,1"
	}
}

// convertDanmakuToASSEvent 将弹幕转换为ASS事件
// 按图片设置：显示区域50%、速度适中、启用滚动/固定/彩色，不启用高级弹幕
func (s *DanmakuBurnService) convertDanmakuToASSEvent(dm models.LiveMsg) string {
	// 过滤高级弹幕（mode 7, 8, 9等特殊弹幕）
	if dm.Mode >= 7 {
		return "" // 不启用高级弹幕
	}

	// 时间戳转换为ASS时间格式 (时:分:秒.毫秒)
	startTime := s.formatASSTime(dm.Timestamp)
	// 速度适中：滚动弹幕显示10秒，固定弹幕显示5秒
	duration := int64(5000)
	if dm.Mode == 1 || dm.Mode == 6 {
		duration = 10000 // 滚动弹幕10秒（适中速度）
	}
	endTime := s.formatASSTime(dm.Timestamp + duration)

	// 转义特殊字符
	text := s.escapeASSText(dm.Message)

	// 显示区域：50%，即只在屏幕上半部分显示（540px以内）
	// 1080p分辨率，50%显示区域 = 0~540px
	maxY := 540 // 显示区域50%
	minY := 50  // 上边距

	// 根据弹幕模式设置效果
	effect := ""
	alignment := "2" // 底部居中
	yPos := minY + (maxY-minY)/2

	switch dm.Mode {
	case 1, 6: // 滚动弹幕（1=普通滚动，6=彩色滚动）
		// 从右向左滚动，速度适中（10秒横跨屏幕）
		yPos = minY + int(dm.Timestamp%int64(maxY-minY))
		effect = fmt.Sprintf("\\move(2000,%d,-200,%d)", yPos, yPos)
		alignment = "7" // 左上
	case 4: // 底部固定弹幕
		yPos = maxY - 50 // 底部位置但在显示区域内
		alignment = "2" // 底部居中
	case 5: // 顶部固定弹幕
		yPos = minY + 30
		alignment = "8" // 顶部居中
	default:
		// 其他模式当作滚动处理
		yPos = minY + int(dm.Timestamp%int64(maxY-minY))
		effect = fmt.Sprintf("\\move(2000,%d,-200,%d)", yPos, yPos)
		alignment = "7"
	}

	// 颜色转换（支持彩色弹幕）
	color := s.convertColorToASS(dm.Color)

	// 构建ASS事件行
	// 注意：这里手动设置Y坐标以实现显示区域限制
	if effect != "" {
		return fmt.Sprintf("Dialogue: 0,%s,%s,Danmaku,,0,0,0,%s,{\\c%s}%s\n",
			startTime, endTime, effect, color, text)
	} else {
		return fmt.Sprintf("Dialogue: 0,%s,%s,Danmaku,,0,0,0,,{\\c%s\\pos(960,%d)\\an%s}%s\n",
			startTime, endTime, color, yPos, alignment, text)
	}
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

	// 转义ASS路径：统一为正斜杠，并转义冒号（Windows盘符路径需要）
	assPathEscaped := strings.ReplaceAll(assPath, "\\", "/")
	assPathEscaped = strings.ReplaceAll(assPathEscaped, ":", "\\:")

	// 确定字体目录和字体名（支持环境变量覆盖）
	fontsDir := os.Getenv("DANMAKU_FONTS_DIR")
	if fontsDir == "" {
		fontsDir = DanmakuFontsDir
	}
	fontName := os.Getenv("DANMAKU_FONT_NAME")
	if fontName == "" {
		fontName = DanmakuFontName
	}

	// subtitles 滤镜参数：
	// - fontsdir: 告知 libass 到哪里找字体（容器内需安装 font-wqy-zenhei）
	// - force_style: 强制覆盖 ASS 文件中的字体名，防止 Microsoft YaHei 等字体找不到
	vfFilter := fmt.Sprintf("subtitles='%s':fontsdir=%s:force_style='Fontname=%s'",
		assPathEscaped, fontsDir, fontName)

	// ffmpeg命令：烧录字幕
	// -c:v libx264: 显式指定H.264编码避免FLV等容器用错编码器
	// -crf 18:      视觉无损质量（0最好，51最差，18接近无损）
	// -preset fast: 编码速度（ultrafast/superfast/fast/medium/slow）
	// -c:a copy:    音频直接复制，不重新编码
	args := []string{
		"-i", videoPath,
		"-vf", vfFilter,
		"-c:v", "libx264",
		"-crf", "18",
		"-preset", "fast",
		"-c:a", "copy",
		"-y", // 覆盖已存在的文件
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

// generateOutputPath 生成输出文件路径（统一输出为mp4，确保H.264编码兼容性）
func (s *DanmakuBurnService) generateOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))

	// 生成带时间戳的文件名，避免冲突
	// 统一使用 .mp4 输出，配合 -c:v libx264 确保兼容性
	timestamp := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("%s_danmaku_%s.mp4", baseName, timestamp))
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
