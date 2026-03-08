package services

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

type HighEnergyCutService struct{}

func NewHighEnergyCutService() *HighEnergyCutService {
	return &HighEnergyCutService{}
}

// CutHighEnergySegments 根据弹幕密度剪辑高能片段
func (s *HighEnergyCutService) CutHighEnergySegments(historyID uint) (string, error) {
	db := database.GetDB()

	// 获取历史记录
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return "", fmt.Errorf("历史记录不存在: %w", err)
	}

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return "", fmt.Errorf("房间配置不存在: %w", err)
	}

	if !room.HighEnergyCut {
		return "", fmt.Errorf("未启用高能剪辑")
	}

	// 获取弹幕数据
	var danmakus []models.LiveMsg
	if err := db.Where("session_id = ?", history.SessionID).
		Order("timestamp ASC").
		Find(&danmakus).Error; err != nil {
		return "", fmt.Errorf("查询弹幕失败: %w", err)
	}

	if len(danmakus) == 0 {
		return "", fmt.Errorf("没有弹幕数据")
	}

	// 计算弹幕密度
	timeWindow := 10 * 1000 // 10秒时间窗口（毫秒）
	densityMap := s.calculateDanmakuDensity(danmakus, timeWindow)

	// 获取高能阈值
	threshold := s.calculateThreshold(densityMap, room.PercentileRank)

	// 识别高能片段
	segments := s.identifyHighEnergySegments(densityMap, threshold, timeWindow)

	if len(segments) == 0 {
		return "", fmt.Errorf("未识别到高能片段")
	}

	// 合并相近的片段
	segments = s.mergeNearSegments(segments, 30*1000) // 30秒内的片段合并

	log.Printf("识别到 %d 个高能片段", len(segments))

	// 获取原视频文件
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).
		Order("start_time ASC").
		Find(&parts).Error; err != nil {
		return "", fmt.Errorf("查询分P失败: %w", err)
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("没有视频文件")
	}

	// 过滤掉没有文件路径的分P
	var validParts []models.RecordHistoryPart
	for _, p := range parts {
		if p.FilePath != "" {
			validParts = append(validParts, p)
		}
	}
	if len(validParts) == 0 {
		return "", fmt.Errorf("所有分P均无文件路径")
	}

	outputFile := filepath.Join(filepath.Dir(validParts[0].FilePath),
		fmt.Sprintf("highlight_%d.mp4", time.Now().Unix()))

	// 支持多分P：将高能片段按分P时间轴切分并合并
	if err := s.cutMultiPartHighEnergy(&history, validParts, outputFile, segments); err != nil {
		return "", fmt.Errorf("视频剪辑失败: %w", err)
	}

	log.Printf("高能剪辑完成: %s", outputFile)
	return outputFile, nil
}

// DanmakuDensity 弹幕密度数据点
type DanmakuDensity struct {
	Timestamp int64 // 时间点（毫秒）
	Count     int   // 弹幕数量
}

// calculateDanmakuDensity 计算弹幕密度（双指针滑动窗口，O(n + k)，原 O(n²) 已修复）
func (s *HighEnergyCutService) calculateDanmakuDensity(danmakus []models.LiveMsg, windowMs int) []DanmakuDensity {
	if len(danmakus) == 0 {
		return nil
	}

	// 找出时间范围（弹幕已按 timestamp ASC 排序）
	minTime := danmakus[0].Timestamp
	maxTime := danmakus[len(danmakus)-1].Timestamp

	windowLen := int64(windowMs)
	step := int64(windowMs / 2) // 窗口重叠 50%

	var densities []DanmakuDensity

	// 双指针：left/right 均只向右移动，总复杂度 O(n + k)
	left, right := 0, 0
	count := 0

	for t := minTime; t <= maxTime; t += step {
		windowEnd := t + windowLen

		// 向右扩展：将所有 timestamp < windowEnd 的弹幕纳入窗口
		for right < len(danmakus) && danmakus[right].Timestamp < windowEnd {
			count++
			right++
		}

		// 向右收缩：将所有 timestamp < t（窗口左边界）的弹幕移出窗口
		for left < right && danmakus[left].Timestamp < t {
			count--
			left++
		}

		densities = append(densities, DanmakuDensity{
			Timestamp: t,
			Count:     count,
		})
	}

	return densities
}

// calculateThreshold 计算高能阈值（基于百分位数）
func (s *HighEnergyCutService) calculateThreshold(densities []DanmakuDensity, percentile float64) int {
	if len(densities) == 0 {
		return 0
	}

	// 防止 percentile<=0 导致阈值取最小局株尌，把所有帧都判为高能
	if percentile <= 0 {
		percentile = 0.5
	}
	if percentile > 1 {
		percentile = 1
	}

	counts := make([]int, len(densities))
	for i, d := range densities {
		counts[i] = d.Count
	}
	sort.Ints(counts)

	index := int(float64(len(counts)) * percentile)
	if index >= len(counts) {
		index = len(counts) - 1
	}

	// 阈值至少为 1，避免把没有弹幕的片段也就入结果
	v := counts[index]
	if v < 1 {
		v = 1
	}
	return v
}

// TimeSegment 时间片段
type TimeSegment struct {
	Start int64 // 开始时间（毫秒）
	End   int64 // 结束时间（毫秒）
}

// identifyHighEnergySegments 识别高能片段
func (s *HighEnergyCutService) identifyHighEnergySegments(densities []DanmakuDensity, threshold int, windowMs int) []TimeSegment {
	var segments []TimeSegment
	var currentSegment *TimeSegment

	for _, d := range densities {
		if d.Count >= threshold {
			if currentSegment == nil {
				currentSegment = &TimeSegment{Start: d.Timestamp}
			}
			currentSegment.End = d.Timestamp + int64(windowMs)
		} else {
			if currentSegment != nil {
				segments = append(segments, *currentSegment)
				currentSegment = nil
			}
		}
	}

	// 处理最后一个片段
	if currentSegment != nil {
		segments = append(segments, *currentSegment)
	}

	return segments
}

// mergeNearSegments 合并相近的片段
func (s *HighEnergyCutService) mergeNearSegments(segments []TimeSegment, gapMs int64) []TimeSegment {
	if len(segments) == 0 {
		return segments
	}

	var merged []TimeSegment
	current := segments[0]

	for i := 1; i < len(segments); i++ {
		if segments[i].Start-current.End <= gapMs {
			// 合并
			current.End = segments[i].End
		} else {
			merged = append(merged, current)
			current = segments[i]
		}
	}
	merged = append(merged, current)

	return merged
}

// cutVideoSegments 使用ffmpeg剪辑视频片段
func (s *HighEnergyCutService) cutVideoSegments(inputFile, outputFile string, segments []TimeSegment) error {
	if len(segments) == 0 {
		return fmt.Errorf("没有片段可剪辑")
	}

	// 检查ffmpeg是否可用
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg未安装或不在PATH中: %w", err)
	}

	// 为每个片段生成临时文件
	var tempFiles []string
	tempDir := filepath.Dir(outputFile)

	for i, seg := range segments {
		tempFile := filepath.Join(tempDir, fmt.Sprintf("temp_segment_%d.mp4", i))
		tempFiles = append(tempFiles, tempFile)

		startSec := float64(seg.Start) / 1000.0
		duration := float64(seg.End-seg.Start) / 1000.0

		// ffmpeg命令：剪辑片段
		args := []string{
			"-i", inputFile,
			"-ss", fmt.Sprintf("%.3f", startSec),
			"-t", fmt.Sprintf("%.3f", duration),
			"-c", "copy", // 快速复制，不重新编码
			"-y",
			tempFile,
		}

		cmd := exec.Command("ffmpeg", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("ffmpeg剪辑失败: %s, output: %s", err, string(output))
			// 清理临时文件
			for _, tf := range tempFiles {
				os.Remove(tf)
			}
			return fmt.Errorf("剪辑片段%d失败: %w", i, err)
		}
	}

	// 合并所有片段
	if err := s.concatenateVideos(tempFiles, outputFile); err != nil {
		// 清理临时文件
		for _, tf := range tempFiles {
			os.Remove(tf)
		}
		return err
	}

	// 清理临时文件
	for _, tf := range tempFiles {
		os.Remove(tf)
	}

	return nil
}

// concatenateVideos 合并多个视频文件
func (s *HighEnergyCutService) concatenateVideos(inputFiles []string, outputFile string) error {
	if len(inputFiles) == 1 {
		// 只有一个文件，直接重命名
		return os.Rename(inputFiles[0], outputFile)
	}

	// 创建 concat 列表文件
	concatFile := filepath.Join(filepath.Dir(outputFile), "concat_list.txt")
	f, err := os.Create(concatFile)
	if err != nil {
		return err
	}

	for _, inputFile := range inputFiles {
		fmt.Fprintf(f, "file '%s'\n", inputFile)
	}
	f.Close()
	defer os.Remove(concatFile)

	// 使用ffmpeg concat协议合并
	args := []string{
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile,
		"-c", "copy",
		"-y",
		outputFile,
	}

	cmd := exec.Command("ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("ffmpeg合并失败: %s, output: %s", err, string(output))
		return fmt.Errorf("合并视频失败: %w", err)
	}

	return nil
}

// cutMultiPartHighEnergy 从多分P视频中切出高能片段并合并
// segments 是相对于直播开始时间的毫秒时间段
func (s *HighEnergyCutService) cutMultiPartHighEnergy(
	history *models.RecordHistory,
	parts []models.RecordHistoryPart,
	outputFile string,
	segments []TimeSegment,
) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg未安装或不在PATH中: %w", err)
	}

	historyStartMs := history.StartTime.UnixMilli()
	tempDir := filepath.Dir(outputFile)
	var tempFiles []string
	cutIdx := 0

	for _, seg := range segments {
		for _, part := range parts {
			if part.FilePath == "" {
				continue
			}

			// 计算分P相对于历史开始的偏移量（毫秒）
			partOffsetMs := part.StartTime.UnixMilli() - historyStartMs
			var partDurationMs int64
			if part.Duration > 0 {
				partDurationMs = int64(part.Duration) * 1000
			} else {
				partDurationMs = part.EndTime.UnixMilli() - part.StartTime.UnixMilli()
			}
			if partDurationMs <= 0 {
				log.Printf("[高能剪辑] 分P %d 时长为0，跳过", part.ID)
				continue
			}

			// 计算片段与分P的交叉部分
			overlapStart := seg.Start
			if overlapStart < partOffsetMs {
				overlapStart = partOffsetMs
			}
			overlapEnd := seg.End
			if overlapEnd > partOffsetMs+partDurationMs {
				overlapEnd = partOffsetMs + partDurationMs
			}
			if overlapEnd <= overlapStart {
				continue
			}

			// 转为分P内部的局部时间
			localStartMs := overlapStart - partOffsetMs
			localDurationMs := overlapEnd - overlapStart

			tempFile := filepath.Join(tempDir, fmt.Sprintf("he_cut_%d.mp4", cutIdx))
			cutIdx++

			args := []string{
				"-i", part.FilePath,
				"-ss", fmt.Sprintf("%.3f", float64(localStartMs)/1000.0),
				"-t", fmt.Sprintf("%.3f", float64(localDurationMs)/1000.0),
				"-c", "copy",
				"-y", tempFile,
			}
			cmd := exec.Command("ffmpeg", args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("[高能剪辑] ffmpeg切片失败 part=%d seg=[%d,%d]: %s\n%s",
					part.ID, seg.Start, seg.End, err, string(output))
				for _, tf := range tempFiles {
					os.Remove(tf)
				}
				return fmt.Errorf("切片失败 part=%d: %w", part.ID, err)
			}
			tempFiles = append(tempFiles, tempFile)
		}
	}

	if len(tempFiles) == 0 {
		return fmt.Errorf("没有可合并的片段")
	}

	if err := s.concatenateVideos(tempFiles, outputFile); err != nil {
		for _, tf := range tempFiles {
			os.Remove(tf)
		}
		return err
	}

	for _, tf := range tempFiles {
		os.Remove(tf)
	}
	return nil
}

// EstimateOutputDuration 估算输出视频时长
func (s *HighEnergyCutService) EstimateOutputDuration(segments []TimeSegment) float64 {
	var total int64
	for _, seg := range segments {
		total += seg.End - seg.Start
	}
	return math.Round(float64(total)/1000.0*100) / 100 // 保留2位小数，秒
}
