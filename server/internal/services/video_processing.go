package services

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gobup/server/internal/models"
)

// VideoProcessingService wraps optional FFmpeg-based processing before upload/publish.
type VideoProcessingService struct{}

type TranscodeProgressCallback func(percent int, msg string)

func NewVideoProcessingService() *VideoProcessingService {
	return &VideoProcessingService{}
}

const maxBiliCoverBytes = 5 * 1024 * 1024

type transcodeSettings struct {
	VideoCodec   string
	VideoEncoder string
	Preset       string
	CRF          int
	MaxWidth     int
	AudioBitrate string
}

// LogVideoProcessingAvailability records whether optional video tools can be used.
func LogVideoProcessingAvailability() {
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		log.Printf("[视频处理] ffmpeg 不可用，转码和截帧封面将自动跳过: %v", err)
		return
	}
	log.Printf("[视频处理] ffmpeg 可用: %s", ffmpegPath)
}

// TranscodeForUpload creates a temporary MP4 for upload when room.EnablePreTranscode is enabled.
// The returned cleanup function removes the generated file. If transcoding is disabled, inputPath
// is returned and cleanup is a no-op.
func (s *VideoProcessingService) TranscodeForUpload(inputPath string, room *models.RecordRoom) (string, func(), error) {
	return s.TranscodeForUploadWithProgress(inputPath, room, nil)
}

// TranscodeForUploadWithProgress creates a temporary MP4 and reports FFmpeg progress when possible.
func (s *VideoProcessingService) TranscodeForUploadWithProgress(inputPath string, room *models.RecordRoom, onProgress TranscodeProgressCallback) (string, func(), error) {
	cleanup := func() {}
	if room == nil || !room.EnablePreTranscode {
		return inputPath, cleanup, nil
	}

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		log.Printf("[转码] ffmpeg 不可用，跳过上传前转码并继续上传原文件: %v", err)
		return inputPath, cleanup, nil
	}
	if _, err := os.Stat(inputPath); err != nil {
		return "", cleanup, fmt.Errorf("源视频不可用: %w", err)
	}
	emitTranscodeProgress(onProgress, 0, "准备转码")

	settings := resolveTranscodeSettings(room)
	outputPath := generateProcessedVideoPath(inputPath, "xcode")
	args := buildTranscodeArgs(inputPath, outputPath, settings)

	log.Printf("[转码] 开始上传前转码: input=%s, output=%s, codec=%s, preset=%s, crf=%d",
		inputPath, outputPath, settings.VideoCodec, settings.Preset, settings.CRF)
	if err := runFFmpegTranscodeWithProgress(ffmpegPath, args, inputPath, onProgress); err != nil {
		_ = os.Remove(outputPath)
		log.Printf("[转码] ffmpeg 转码失败，清理临时文件并继续上传原文件: %v", err)
		return inputPath, cleanup, nil
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		log.Printf("[转码] 转码输出文件不存在，继续上传原文件: %v", err)
		return inputPath, cleanup, nil
	}
	if info.Size() < 1024 {
		_ = os.Remove(outputPath)
		log.Printf("[转码] 转码输出文件异常小，清理临时文件并继续上传原文件: %d bytes", info.Size())
		return inputPath, cleanup, nil
	}

	log.Printf("[转码] 上传前转码完成: output=%s, size=%d MB", outputPath, info.Size()/1024/1024)
	emitTranscodeProgress(onProgress, 100, "转码完成")
	return outputPath, func() {
		if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[转码] 清理临时转码文件失败: %s, err=%v", outputPath, err)
		}
	}, nil
}

func runFFmpegTranscodeWithProgress(ffmpegPath string, args []string, inputPath string, onProgress TranscodeProgressCallback) error {
	cmd := exec.Command(ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建ffmpeg进度管道失败: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	durationMs := probeVideoDurationMs(inputPath)
	lastPercent := -1

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动ffmpeg失败: %w", err)
	}

	scanDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			percent, ok := parseFFmpegProgressPercent(line, durationMs)
			if !ok {
				continue
			}
			if percent > 99 {
				percent = 99
			}
			if percent > lastPercent {
				lastPercent = percent
				emitTranscodeProgress(onProgress, percent, fmt.Sprintf("转码中 %d%%", percent))
			}
		}
		scanDone <- scanner.Err()
	}()

	waitErr := cmd.Wait()
	scanErr := <-scanDone
	if scanErr != nil {
		log.Printf("[转码] 读取ffmpeg进度失败: %v", scanErr)
	}
	if waitErr != nil {
		return fmt.Errorf("%w, output=%s", waitErr, trimCommandOutput(stderr.Bytes(), 2000))
	}
	return nil
}

func emitTranscodeProgress(onProgress TranscodeProgressCallback, percent int, msg string) {
	if onProgress != nil {
		onProgress(minInt(maxInt(percent, 0), 100), msg)
	}
}

func resolveTranscodeSettings(room *models.RecordRoom) transcodeSettings {
	preset := normalizeTranscodePreset("")
	crf := 23
	maxWidth := 0
	audioBitrate := "160k"
	codec := models.TranscodeVideoCodecH264

	if room != nil {
		preset = normalizeTranscodePreset(room.TranscodePreset)
		crf = room.TranscodeCRF
		if crf < 18 || crf > 35 {
			crf = 23
		}
		maxWidth = room.TranscodeMaxWidth
		audioBitrate = strings.TrimSpace(room.TranscodeAudioBitrate)
		if audioBitrate == "" {
			audioBitrate = "160k"
		}
		codec = models.NormalizeTranscodeVideoCodec(room.TranscodeVideoCodec)
	}

	encoder := "libx264"
	if codec == models.TranscodeVideoCodecH265 {
		encoder = "libx265"
	}

	return transcodeSettings{
		VideoCodec:   codec,
		VideoEncoder: encoder,
		Preset:       preset,
		CRF:          crf,
		MaxWidth:     maxWidth,
		AudioBitrate: audioBitrate,
	}
}

func buildTranscodeArgs(inputPath, outputPath string, settings transcodeSettings) []string {
	args := []string{
		"-y",
		"-fflags", "+genpts",
		"-i", inputPath,
	}
	if settings.MaxWidth > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=w='min(%d,iw)':h=-2", settings.MaxWidth))
	}
	args = append(args,
		"-c:v", settings.VideoEncoder,
	)
	if settings.VideoCodec == models.TranscodeVideoCodecH265 {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args,
		"-preset", settings.Preset,
		"-crf", strconv.Itoa(settings.CRF),
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", settings.AudioBitrate,
		"-movflags", "+faststart",
		"-progress", "pipe:1",
		"-nostats",
		outputPath,
	)
	return args
}

func probeVideoDurationMs(inputPath string) int64 {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0
	}
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	}
	output, err := exec.Command(ffprobePath, args...).Output()
	if err != nil {
		return 0
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int64(seconds * 1000)
}

func parseFFmpegProgressPercent(line string, durationMs int64) (int, bool) {
	if durationMs <= 0 {
		return 0, false
	}
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return 0, false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	var progressMs int64
	switch key {
	case "out_time_ms", "out_time_us":
		raw, err := strconv.ParseInt(value, 10, 64)
		if err != nil || raw < 0 {
			return 0, false
		}
		progressMs = raw
		if raw > durationMs*10 {
			progressMs = raw / 1000
		}
	case "out_time":
		parsedMs, ok := parseFFmpegClockMs(value)
		if !ok {
			return 0, false
		}
		progressMs = parsedMs
	default:
		return 0, false
	}

	percent := int(progressMs * 100 / durationMs)
	return minInt(maxInt(percent, 0), 100), true
}

func parseFFmpegClockMs(value string) (int64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	minutes, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, false
	}
	if hours < 0 || minutes < 0 || seconds < 0 {
		return 0, false
	}
	totalSeconds := float64(hours*3600+minutes*60) + seconds
	return int64(totalSeconds * 1000), true
}

// EnsureFrameCover extracts a JPG cover frame next to the source video and returns its path.
func (s *VideoProcessingService) EnsureFrameCover(part *models.RecordHistoryPart, room *models.RecordRoom) (string, error) {
	if part == nil || part.FilePath == "" {
		return "", fmt.Errorf("分P文件路径为空")
	}

	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return "", fmt.Errorf("ffmpeg未安装或不在PATH中: %w", err)
	}

	coverPath := strings.TrimSuffix(part.FilePath, filepath.Ext(part.FilePath)) + ".cover.jpg"
	if info, err := os.Stat(coverPath); err == nil && info.Size() > 1024 {
		return coverPath, nil
	}

	second := 5
	if room != nil && room.CoverFrameSecond >= 0 {
		second = room.CoverFrameSecond
	}

	if err := extractCoverFrame(ffmpegPath, part.FilePath, coverPath, second); err != nil {
		if second != 0 {
			log.Printf("[封面截取] 第 %d 秒截取失败，降级尝试第 0 秒: %v", second, err)
			if fallbackErr := extractCoverFrame(ffmpegPath, part.FilePath, coverPath, 0); fallbackErr != nil {
				return "", fallbackErr
			}
		} else {
			return "", err
		}
	}

	info, err := os.Stat(coverPath)
	if err != nil {
		return "", fmt.Errorf("封面文件未生成: %w", err)
	}
	if info.Size() < 512 {
		_ = os.Remove(coverPath)
		return "", fmt.Errorf("封面文件异常小: %d bytes", info.Size())
	}
	if info.Size() > maxBiliCoverBytes {
		if err := recompressCover(ffmpegPath, coverPath); err != nil {
			return "", err
		}
		if info, err = os.Stat(coverPath); err != nil {
			return "", fmt.Errorf("压缩后封面文件不可用: %w", err)
		}
		if info.Size() > maxBiliCoverBytes {
			return "", fmt.Errorf("封面文件超过5MB限制: %d bytes", info.Size())
		}
	}
	return coverPath, nil
}

func extractCoverFrame(ffmpegPath, inputPath, outputPath string, second int) error {
	args := []string{
		"-y",
		"-ss", strconv.Itoa(second),
		"-i", inputPath,
		"-frames:v", "1",
		"-q:v", "2",
		outputPath,
	}
	output, err := exec.Command(ffmpegPath, args...).CombinedOutput()
	if err != nil {
		_ = os.Remove(outputPath)
		return fmt.Errorf("ffmpeg截取封面失败: %w, output=%s", err, trimCommandOutput(output, 1200))
	}
	return nil
}

func recompressCover(ffmpegPath, coverPath string) error {
	tmpPath := strings.TrimSuffix(coverPath, filepath.Ext(coverPath)) + ".compressed.jpg"
	args := []string{
		"-y",
		"-i", coverPath,
		"-vf", "scale='min(1920,iw)':-2",
		"-q:v", "5",
		tmpPath,
	}
	output, err := exec.Command(ffmpegPath, args...).CombinedOutput()
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("封面压缩失败: %w, output=%s", err, trimCommandOutput(output, 1200))
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		return fmt.Errorf("压缩封面未生成: %w", err)
	}
	if info.Size() < 512 {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("压缩封面异常小: %d bytes", info.Size())
	}
	if err := os.Rename(tmpPath, coverPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("替换压缩封面失败: %w", err)
	}
	return nil
}

func findFFmpeg() (string, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", err
	}
	if err := exec.Command(ffmpegPath, "-version").Run(); err != nil {
		return "", err
	}
	return ffmpegPath, nil
}

func generateProcessedVideoPath(inputPath, suffix string) string {
	dir := filepath.Dir(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	timestamp := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s.mp4", base, suffix, timestamp))
}

func normalizeTranscodePreset(preset string) string {
	trimmed := strings.TrimSpace(preset)
	switch trimmed {
	case "ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow":
		return trimmed
	default:
		return "veryfast"
	}
}

func trimCommandOutput(output []byte, limit int) string {
	text := strings.TrimSpace(string(output))
	if len(text) <= limit {
		return text
	}
	return "...(省略前段)...\n" + text[len(text)-limit:]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
