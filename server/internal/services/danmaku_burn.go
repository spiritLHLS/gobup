package services

import (
	"encoding/json"
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

	// 1. 将XML转换为ASS字幕文件（输出到临时目录，避免中文路径问题）
	// 严格只使用 DanmakuFactory 二进制，不使用任何自实现的转换逻辑
	assPath, err := s.convertXMLToASSWithFactory(xmlPath, history, room, part.FilePath)
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

// probeVideoResolution 使用 ffprobe 探测视频分辨率，返回 "WxH" 字符串。
// 失败时返回默认值 "1920x1080"。
func (s *DanmakuBurnService) probeVideoResolution(videoPath string) string {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		log.Printf("[弹幕烧录] ffprobe 不可用，使用默认分辨率 1920x1080: %v", err)
		return "1920x1080"
	}

	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "v:0",
		videoPath,
	}

	out, err := exec.Command(ffprobePath, args...).Output()
	if err != nil {
		log.Printf("[弹幕烧录] ffprobe 执行失败，使用默认分辨率 1920x1080: %v", err)
		return "1920x1080"
	}

	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Streams) == 0 {
		log.Printf("[弹幕烧录] ffprobe 解析失败，使用默认分辨率 1920x1080: %v", err)
		return "1920x1080"
	}

	w := result.Streams[0].Width
	h := result.Streams[0].Height
	if w <= 0 || h <= 0 {
		log.Printf("[弹幕烧录] ffprobe 返回无效分辨率 (%dx%d)，使用默认 1920x1080", w, h)
		return "1920x1080"
	}

	res := fmt.Sprintf("%dx%d", w, h)
	log.Printf("[弹幕烧录] 探测视频分辨率: %s", res)
	return res
}

// convertXMLToASSWithFactory 使用 DanmakuFactory 将XML转换为ASS。
// 输出文件写到系统临时目录（ASCII路径），避免 libavfilter subtitles filter 因中文路径解析失败。
func (s *DanmakuBurnService) convertXMLToASSWithFactory(xmlPath string, history *models.RecordHistory, room *models.RecordRoom, videoPath string) (string, error) {
	// 获取 DanmakuFactory 路径
	factoryPath := os.Getenv("DANMAKU_FACTORY_PATH")
	if factoryPath == "" {
		factoryPath = DanmakuFactoryPath
	}

	// 检查 DanmakuFactory 是否存在
	if _, err := os.Stat(factoryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("DanmakuFactory 未安装: %s", factoryPath)
	}

	// 将ASS写到系统临时目录（ASCII-only路径），避免含中文/特殊字符的路径传入ffmpeg subtitles滤镜时失败
	// 注意：用 CreateTemp 只是为了获得一个唯一的临时路径；必须在调用 DanmakuFactory 之前
	// 删除该空文件——否则 DanmakuFactory 检测到目标文件已存在后在非交互模式下不会写入内容，
	// 导致输出文件始终为 0 字节，进而触发"ASS文件未生成或为空"错误。
	tmpFile, err := os.CreateTemp("", "gobup_danmaku_*.ass")
	if err != nil {
		return "", fmt.Errorf("创建临时ASS文件失败: %w", err)
	}
	assPath := tmpFile.Name()
	tmpFile.Close()
	// 删除空占位文件，让 DanmakuFactory 自行创建（避免"file already exists"跳过写入）
	os.Remove(assPath)

	log.Printf("[弹幕烧录] 使用 DanmakuFactory 转换: %s -> %s (临时路径)", xmlPath, assPath)

	// 探测源视频分辨率，为弹幕定位提供正确的画布尺寸
	resolution := s.probeVideoResolution(videoPath)

	// 确定字号
	fontSize := "38"
	switch room.DanmakuBurnStyle {
	case "compact":
		fontSize = "32"
	case "large":
		fontSize = "48"
	}

	// 构建命令参数
	// -d 0  ：弹幕密度0（不过滤重叠），保留所有弹幕；-d N(>0) 会丢弃重叠弹幕
	// 参考: https://github.com/hihkm/DanmakuFactory
	args := []string{
		"-i", xmlPath,
		"-o", assPath,
		"-r", resolution,
		"-d", "-1",
		"-S", fontSize,
		"--scrollarea", "0.75",
		"--displayarea", "0.8",
	}

	log.Printf("[弹幕烧录] DanmakuFactory 命令: %s %s", factoryPath, strings.Join(args, " "))

	cmd := exec.Command(factoryPath, args...)
	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))

	if err != nil {
		os.Remove(assPath) // 清理失败的临时文件
		log.Printf("[弹幕烧录] DanmakuFactory 输出: %s", outStr)
		return "", fmt.Errorf("DanmakuFactory 执行失败: %w", err)
	}

	if outStr != "" {
		log.Printf("[弹幕烧录] DanmakuFactory 输出: %s", outStr)
	}

	// 检查输出文件是否生成且非空
	fi, err := os.Stat(assPath)
	if err != nil || fi.Size() == 0 {
		os.Remove(assPath)
		return "", fmt.Errorf("ASS文件未生成或为空: %s", assPath)
	}

	log.Printf("[弹幕烧录] DanmakuFactory 转换成功: %s (大小: %d bytes)", assPath, fi.Size())
	return assPath, nil
}

// burnWithFFmpeg 使用ffmpeg将ASS字幕烧录进视频
func (s *DanmakuBurnService) burnWithFFmpeg(videoPath, assPath, outputPath string) error {
	// 检查ffmpeg是否可用
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg未安装或不在PATH中: %w", err)
	}

	// assPath 已是 ASCII 临时路径，但仍需符合 ffmpeg subtitles 滤镜的转义规则：
	//   - 反斜杠 → 正斜杠（Windows路径兼容）
	//   - 冒号   → \:  （Windows盘符 C: 等）
	assPathEscaped := strings.ReplaceAll(assPath, "\\", "/")
	assPathEscaped = strings.ReplaceAll(assPathEscaped, ":", "\\:")

	// 确定字体目录（供 libass 查找字体）
	fontsDir := os.Getenv("DANMAKU_FONTS_DIR")
	if fontsDir == "" {
		fontsDir = DanmakuFontsDir
	}

	// 字体名（支持环境变量覆盖）
	fontName := os.Getenv("DANMAKU_FONT_NAME")
	if fontName == "" {
		fontName = DanmakuFontName
	}

	// subtitles 滤镜：
	//   - 不再使用 force_style 覆盖字体（DanmakuFactory 已在ASS中写好样式）
	//   - fontsdir 路径用单引号包裹，防止路径含空格时截断
	vfFilter := fmt.Sprintf("subtitles='%s':fontsdir='%s'", assPathEscaped, fontsDir)

	// 判断源文件是否为FLV，FLV时间戳可能不规则，需要加 -fflags +genpts 重新生成时间戳
	isFLV := strings.ToLower(filepath.Ext(videoPath)) == ".flv"

	// ffmpeg命令：
	//   -fflags +genpts : 对 FLV 等时间戳不规则来源重新生成 PTS，避免字幕轨不同步
	//   -c:v libx264   : 显式H.264编码
	//   -crf 18        : 视觉无损画质
	//   -preset fast   : 编码速度
	//   -c:a copy      : 音频直接复制
	var args []string
	if isFLV {
		args = append(args, "-fflags", "+genpts")
	}
	args = append(args,
		"-i", videoPath,
		"-vf", vfFilter,
		"-c:v", "libx264",
		"-crf", "18",
		"-preset", "fast",
		"-c:a", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)

	log.Printf("[弹幕烧录] 执行ffmpeg命令: ffmpeg %s", strings.Join(args, " "))

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()

	// 无论成功失败都输出 ffmpeg 末尾日志（截取最后2000字节避免过长）
	outStr := string(output)
	if len(outStr) > 2000 {
		outStr = "...(省略前段)...\n" + outStr[len(outStr)-2000:]
	}
	if err != nil {
		log.Printf("[弹幕烧录] ffmpeg 执行失败:\n%s", outStr)
		return fmt.Errorf("ffmpeg执行失败: %w", err)
	}
	log.Printf("[弹幕烧录] ffmpeg 执行成功（末尾日志）:\n%s", outStr)

	// 验证输出文件存在且大小合理
	fi, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("输出文件不存在: %w", err)
	}
	if fi.Size() < 1024 {
		return fmt.Errorf("输出文件异常小 (%d bytes)，烧录可能失败", fi.Size())
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
// 对 danmaku_burn 类型的临时文件，仅当 appended_to_video=true 时才删除物理文件。
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
		// 弹幕烧录版：仅当已追加到视频（appended_to_video=true）才删除物理文件
		if part.TempFileType == "danmaku_burn" && !part.AppendedToVideo {
			log.Printf("[临时文件清理] 跳过未追加的弹幕烧录版: part_id=%d, file=%s", part.ID, part.FilePath)
			continue
		}

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

// FindDanmakuXML 查找对应的弹幕XML文件（供外部包调用）
func (s *DanmakuBurnService) FindDanmakuXML(videoPath string) string {
	return s.findDanmakuXML(videoPath)
}

// CleanTempFilesBySessionID 按SessionID清理临时文件（投稿成功后调用）
// 对 danmaku_burn 类型的临时文件，仅当 appended_to_video=true 时才清理，
// 防止删除尚未追加到视频中的弹幕版物理文件。
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
		// 弹幕烧录版：仅当已追加到视频（appended_to_video=true）才删除物理文件
		// 未追加的情况由弹幕回补定时任务处理完后再触发清理
		if part.TempFileType == "danmaku_burn" && !part.AppendedToVideo {
			log.Printf("[临时文件清理] 跳过未追加的弹幕烧录版: part_id=%d, file=%s", part.ID, part.FilePath)
			continue
		}

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

// CleanSplitTempFilesBySessionID 按SessionID清理 split（切分）类型的临时文件。
// 只清理 temp_file_type='split' 的已上传分P，不触碰弹幕烧录版文件。
// 由审核通过事件调用，弹幕烧录版在 AppendDanmakuBurnedPartsToApprovedVideos 追加并标记后清理。
func (s *DanmakuBurnService) CleanSplitTempFilesBySessionID(sessionID string) error {
	db := database.GetDB()

	var tempParts []models.RecordHistoryPart
	if err := db.Where("session_id = ? AND is_temp_file = ? AND upload = ? AND temp_file_type = ?",
		sessionID, true, true, "split").Find(&tempParts).Error; err != nil {
		return fmt.Errorf("查询split临时文件失败: %w", err)
	}

	log.Printf("[split清理] SessionID=%s 找到 %d 个split临时文件", sessionID, len(tempParts))

	successCount := 0
	for _, part := range tempParts {
		if part.FilePath != "" {
			if err := os.Remove(part.FilePath); err != nil && !os.IsNotExist(err) {
				log.Printf("[split清理] 删除文件失败: %s, error: %v", part.FilePath, err)
				continue
			}
			log.Printf("[split清理] ✓ 已删除split临时文件: %s", part.FilePath)
		}
		part.FileDelete = true
		db.Save(&part)
		successCount++
	}

	log.Printf("[split清理] 清理完成: 成功 %d/%d", successCount, len(tempParts))
	return nil
}

// CleanAppendedBurnedPartsBySessionID 清理审核通过后、已追加到视频的弹幕烧录文件。
// 由 AppendDanmakuBurnedPartsToApprovedVideos 在每个分P追加成功后调用。
func (s *DanmakuBurnService) CleanAppendedBurnedPartsBySessionID(sessionID string) error {
	db := database.GetDB()

	var tempParts []models.RecordHistoryPart
	if err := db.Where(
		"session_id = ? AND is_temp_file = ? AND upload = ? AND temp_file_type = ? AND appended_to_video = ?",
		sessionID, true, true, "danmaku_burn", true,
	).Find(&tempParts).Error; err != nil {
		return fmt.Errorf("查询已追加弹幕烧录文件失败: %w", err)
	}

	successCount := 0
	for _, part := range tempParts {
		if part.FileDelete {
			continue // 已经清理过了
		}
		if part.FilePath != "" {
			if err := os.Remove(part.FilePath); err != nil && !os.IsNotExist(err) {
				log.Printf("[弹幕烧录清理] 删除文件失败: %s, error: %v", part.FilePath, err)
				continue
			}
			log.Printf("[弹幕烧录清理] ✓ 已删除已追加的弹幕烧录文件: %s", part.FilePath)
		}
		part.FileDelete = true
		db.Save(&part)
		successCount++
	}

	if successCount > 0 {
		log.Printf("[弹幕烧录清理] SessionID=%s 清理了 %d 个已追加的弹幕烧录文件", sessionID, successCount)
	}
	return nil
}
