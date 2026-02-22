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
	// 严格只使用 DanmakuFactory 二进制，不使用任何自实现的转换逻辑
	assPath, err := s.convertXMLToASSWithFactory(xmlPath, history, room)
	if err != nil {
		return "", fmt.Errorf("DanmakuFactory 转换弹幕为ASS失败（仅支持DanmakuFactory，不启用内置转换）: %w", err)
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
		"-r", "1920x1080", // 分辨率
		"-d", "-1", // 弹幕密度：-1=不重叠
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
