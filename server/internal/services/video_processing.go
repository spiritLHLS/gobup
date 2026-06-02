package services

import (
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

func NewVideoProcessingService() *VideoProcessingService {
	return &VideoProcessingService{}
}

// TranscodeForUpload creates a temporary MP4 for upload when room.EnablePreTranscode is enabled.
// The returned cleanup function removes the generated file. If transcoding is disabled, inputPath
// is returned and cleanup is a no-op.
func (s *VideoProcessingService) TranscodeForUpload(inputPath string, room *models.RecordRoom) (string, func(), error) {
	cleanup := func() {}
	if room == nil || !room.EnablePreTranscode {
		return inputPath, cleanup, nil
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", cleanup, fmt.Errorf("ffmpeg未安装或不在PATH中: %w", err)
	}
	if _, err := os.Stat(inputPath); err != nil {
		return "", cleanup, fmt.Errorf("源视频不可用: %w", err)
	}

	preset := normalizeTranscodePreset(room.TranscodePreset)
	crf := room.TranscodeCRF
	if crf < 18 || crf > 35 {
		crf = 23
	}
	audioBitrate := strings.TrimSpace(room.TranscodeAudioBitrate)
	if audioBitrate == "" {
		audioBitrate = "160k"
	}

	outputPath := generateProcessedVideoPath(inputPath, "xcode")
	args := []string{
		"-y",
		"-fflags", "+genpts",
		"-i", inputPath,
	}
	if room.TranscodeMaxWidth > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=w='min(%d,iw)':h=-2", room.TranscodeMaxWidth))
	}
	args = append(args,
		"-c:v", "libx264",
		"-preset", preset,
		"-crf", strconv.Itoa(crf),
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", audioBitrate,
		"-movflags", "+faststart",
		outputPath,
	)

	log.Printf("[转码] 开始上传前转码: input=%s, output=%s, preset=%s, crf=%d", inputPath, outputPath, preset, crf)
	output, err := exec.Command(ffmpegPath, args...).CombinedOutput()
	if err != nil {
		_ = os.Remove(outputPath)
		return "", cleanup, fmt.Errorf("ffmpeg转码失败: %w, output=%s", err, trimCommandOutput(output, 2000))
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return "", cleanup, fmt.Errorf("转码输出文件不存在: %w", err)
	}
	if info.Size() < 1024 {
		_ = os.Remove(outputPath)
		return "", cleanup, fmt.Errorf("转码输出文件异常小: %d bytes", info.Size())
	}

	log.Printf("[转码] 上传前转码完成: output=%s, size=%d MB", outputPath, info.Size()/1024/1024)
	return outputPath, func() {
		if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[转码] 清理临时转码文件失败: %s, err=%v", outputPath, err)
		}
	}, nil
}

// EnsureFrameCover extracts a JPG cover frame next to the source video and returns its path.
func (s *VideoProcessingService) EnsureFrameCover(part *models.RecordHistoryPart, room *models.RecordRoom) (string, error) {
	if part == nil || part.FilePath == "" {
		return "", fmt.Errorf("分P文件路径为空")
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
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
