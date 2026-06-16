package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

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
	fileExt := strings.ToLower(filepath.Ext(filePath))

	metadata := &FileMetadata{
		RoomID:    "unknown",
		Uname:     "未知主播",
		Title:     strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		AreaName:  "",
		StartTime: info.ModTime().Add(-time.Hour), // 默认假设录制1小时
		EndTime:   info.ModTime(),
		SessionID: "", // 初始为空，稍后会尝试从FLV文件读取
	}

	// 第一步：优先尝试从FLV文件中读取元数据（包含SessionID）
	if fileExt == ".flv" {
		flvParser := NewFLVParser()
		if flvMetadata, err := flvParser.ParseFLVFile(filePath); err == nil {
			// 成功读取FLV元数据
			if flvMetadata.SessionID != "" {
				metadata.SessionID = flvMetadata.SessionID
				log.Printf("[FileScan] 从FLV文件读取SessionID: %s", metadata.SessionID)
			}
			if flvMetadata.RoomID != "" {
				metadata.RoomID = flvMetadata.RoomID
			}
			if flvMetadata.Uname != "" {
				metadata.Uname = flvMetadata.Uname
			}
			if flvMetadata.Title != "" {
				metadata.Title = flvMetadata.Title
			}
			if !flvMetadata.StartTime.IsZero() {
				metadata.StartTime = flvMetadata.StartTime
			}
			if flvMetadata.Duration > 0 {
				metadata.EndTime = metadata.StartTime.Add(time.Duration(flvMetadata.Duration*1000) * time.Millisecond)
			}
		} else {
			log.Printf("[FileScan] 读取FLV元数据失败: %v，继续使用其他方法", err)
		}
	}

	// 第二步：尝试从目录结构中提取房间号（如果还未获取）
	if metadata.RoomID == "unknown" {
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
					if metadata.Uname == "未知主播" {
						metadata.Uname = fmt.Sprintf("房间%s", part)
					}
					break
				}
			}
		}
	}

	// 第三步：尝试从文件名解析（录播姬格式：录制-房间号-日期-时间-编号-标题.flv）
	if strings.HasPrefix(fileName, "录制-") || strings.HasPrefix(fileName, "record-") {
		fields := strings.Split(fileName, "-")
		if len(fields) >= 3 {
			if metadata.RoomID == "unknown" {
				metadata.RoomID = fields[1]
				if metadata.Uname == "未知主播" {
					metadata.Uname = fmt.Sprintf("房间%s", fields[1])
				}
			}

			// 尝试解析日期时间
			if len(fields) >= 4 {
				dateTimeStr := fields[2] + fields[3]
				if len(dateTimeStr) >= 14 {
					if t, err := time.Parse("20060102150405", dateTimeStr[:14]); err == nil {
						metadata.StartTime = t
						metadata.EndTime = t.Add(time.Hour) // 默认1小时
					}
				}
			}

			// 提取标题
			// 格式: 录制-{roomId}-{YYYYMMDD}-{HHMMSS}-{序号}-{标题...}.flv
			// fields: [录制(0), roomId(1), date(2), time(3), seq(4), title_part(5), ...]
			// len==5 时 fields[4] 是序号，没有标题字段，不能误用序号作标题
			// len>=6 时 fields[5:] 才是标题（标题本身可包含连字符）
			if len(fields) >= 6 && metadata.Title == strings.TrimSuffix(fileName, filepath.Ext(fileName)) {
				titleParts := append([]string(nil), fields[5:]...)
				// 去掉最后一段的文件扩展名（如 .flv）
				titleParts[len(titleParts)-1] = strings.TrimSuffix(
					titleParts[len(titleParts)-1],
					filepath.Ext(titleParts[len(titleParts)-1]),
				)
				if newTitle := strings.Join(titleParts, "-"); newTitle != "" {
					metadata.Title = newTitle
				}
			}
		}
	}

	// 验证时间的合理性（防止时间戳异常）
	if metadata.StartTime.After(time.Now()) {
		// 开始时间在未来，可能是时间戳错误，使用文件修改时间
		log.Printf("[FileScan] 警告: 文件开始时间在未来 (%v)，使用文件修改时间", metadata.StartTime)
		metadata.StartTime = info.ModTime().Add(-time.Hour)
		metadata.EndTime = info.ModTime()
	}
	if metadata.EndTime.Before(metadata.StartTime) {
		// 结束时间早于开始时间，修正
		log.Printf("[FileScan] 警告: 结束时间早于开始时间，自动修正")
		metadata.EndTime = metadata.StartTime.Add(time.Hour)
	}

	// 生成或验证 SessionID
	// 如果FLV文件中已经有SessionID，直接使用
	// 否则根据房间号和时间生成
	if metadata.SessionID == "" {
		// 对于同一房间的多场直播，使用更精细的时间粒度来区分
		// 策略：使用房间号+日期+小时来生成SessionID
		// 这样可以识别同一房间在不同时间段的直播，但同一小时内的分P会被合并
		sessionTimeStr := metadata.StartTime.Format("2006010215") // 精确到小时
		metadata.SessionID = fmt.Sprintf("%s_%s_scan", metadata.RoomID, sessionTimeStr)
	}

	return metadata
}

// getOrCreateHistory 获取或创建历史记录
func (s *FileScanService) getOrCreateHistory(db *gorm.DB, metadata *FileMetadata, room *models.RecordRoom) (*models.RecordHistory, error) {
	// 先尝试通过 SessionID 查找
	var history models.RecordHistory
	if err := db.Where("session_id = ?", metadata.SessionID).First(&history).Error; err == nil {
		// 找到已有记录，检查是否已投稿
		if history.Publish {
			// 已投稿的记录不应该被重复使用，创建新的历史记录
			log.Printf("[FileScan] SessionID相同但已投稿，创建新记录避免重复投稿: SessionID=%s, 已有BvID=%s",
				metadata.SessionID, history.BvID)
			// 修改SessionID以避免冲突
			metadata.SessionID = fmt.Sprintf("%s_%d", metadata.SessionID, time.Now().UnixNano())
		} else if history.Recording || history.Streaming {
			// 记录显示正在录制或直播中，但我们已经通过了直播状态检查
			// 可能是状态未及时更新，谨慎起见创建新记录
			log.Printf("[FileScan] SessionID相同但记录显示正在录制/直播，创建新记录: SessionID=%s",
				metadata.SessionID)
			metadata.SessionID = fmt.Sprintf("%s_%d", metadata.SessionID, time.Now().UnixNano())
		} else {
			// 未投稿且标题相似且未在录制，但需要再次确认该场直播确实已结束
			// 通过检查房间当前状态和历史记录的结束时间来判断
			liveStatusService := NewLiveStatusService()
			isHistoryFinished, _, checkErr := liveStatusService.IsRoomRecordingFinished(
				metadata.RoomID, history.EndTime, history.Title)

			if checkErr == nil && !isHistoryFinished {
				log.Printf("[FileScan] SessionID匹配但该场直播可能仍在进行，创建新记录: SessionID=%s",
					metadata.SessionID)
				metadata.SessionID = fmt.Sprintf("%s_%d", metadata.SessionID, time.Now().UnixNano())
			} else {
				// 确认直播已结束，可以安全合并
				if metadata.EndTime.After(history.EndTime) {
					history.EndTime = metadata.EndTime
					db.Save(&history)
				}
				// 同时更新开始时间（取更早的）
				if metadata.StartTime.Before(history.StartTime) {
					history.StartTime = metadata.StartTime
					db.Save(&history)
				}
				log.Printf("[FileScan] 合并到未投稿的已有记录（已确认直播结束）: ID=%d, SessionID=%s", history.ID, metadata.SessionID)
				return &history, nil
			}
		}
	}

	// 如果没有找到，尝试查找同一房间在时间窗口内的最近记录
	// 搜索范围：前后各9小时（总共18小时窗口），足以处理跨凌晨及间隔较长的情况
	// 避免过大的时间窗口导致误合并不同场次的直播
	var histories []models.RecordHistory

	// 搜索范围：文件开始时间前后各9小时
	searchStart := metadata.StartTime.Add(-9 * time.Hour)
	searchEnd := metadata.StartTime.Add(9 * time.Hour)

	err := db.Where("room_id = ? AND start_time >= ? AND start_time < ? AND publish = ?",
		metadata.RoomID, searchStart, searchEnd, false).
		Order("end_time DESC").
		Limit(20).
		Find(&histories).Error

	if err == nil && len(histories) > 0 {
		// 在合并前，检查房间当前是否在直播（避免将新开播的文件与旧记录合并）
		liveStatusService := NewLiveStatusService()
		roomInfo, roomErr := liveStatusService.GetRoomInfo(metadata.RoomID)
		isCurrentlyLive := roomErr == nil && roomInfo.Data.LiveStatus == 1

		// 检查时间差和连续性
		for _, h := range histories {
			// 跳过已投稿的记录，避免重复投稿
			if h.Publish {
				log.Printf("[FileScan] 跳过已投稿的记录: ID=%d, BvID=%s", h.ID, h.BvID)
				continue
			}

			// 如果房间当前正在直播，不要合并到旧记录（可能是新开的一场）
			if isCurrentlyLive {
				// 检查历史记录的结束时间，如果已经过去超过15分钟，说明是新的一场直播
				timeSinceHistoryEnd := metadata.StartTime.Sub(h.EndTime)
				if timeSinceHistoryEnd > 15*time.Minute {
					log.Printf("[FileScan] 房间正在直播且距上次结束>15分钟，不合并到旧记录: 旧记录结束=%v, 新文件开始=%v",
						h.EndTime, metadata.StartTime)
					continue
				}
			}

			timeDiff := metadata.StartTime.Sub(h.EndTime)

			// 处理文件时间倒序的情况（可能是扫盘顺序问题或文件时间戳不准）
			if timeDiff < 0 {
				// 新文件的开始时间早于历史记录的结束时间
				// 检查是否是时间重叠（可能是同一场直播的不同分段）
				// 只基于时间连续性判断，不依赖标题
				if timeDiff > -10*time.Minute {
					log.Printf("[FileScan] 检测到时间轻微重叠(%.1f分钟)，基于时间连续性判断为同一场直播，允许合并: 已有结束=%v, 新文件开始=%v",
						-timeDiff.Minutes(), h.EndTime, metadata.StartTime)
					// 更新历史记录的开始时间（取更早的）
					if metadata.StartTime.Before(h.StartTime) {
						h.StartTime = metadata.StartTime
					}
					if metadata.EndTime.After(h.EndTime) {
						h.EndTime = metadata.EndTime
					}
					// 如果新文件有更新的标题，更新历史记录的标题（处理标题变化情况）
					if metadata.Title != "" && metadata.Title != h.Title {
						log.Printf("[FileScan] 检测到标题变化，更新为最新: %s -> %s", h.Title, metadata.Title)
						h.Title = metadata.Title
					}
					db.Save(&h)
					log.Printf("[FileScan] 合并到已有历史记录(时间重叠): ID=%d, SessionID=%s", h.ID, h.SessionID)
					return &h, nil
				} else {
					// 时间差异过大，不合并
					log.Printf("[FileScan] 时间倒序且差异过大(%.1f分钟)，不合并", -timeDiff.Minutes())
					continue
				}
			}

			// 时间间隔检查：只有时间间隔小于30分钟，才认为是同一场直播，才合并
			// 考虑到可能的跨越凌晨情况和标题变化
			// 不再依赖标题相似度，因为标题可能在直播过程中改变
			const maxGapDuration = 30 * time.Minute
			if timeDiff >= 0 && timeDiff < maxGapDuration {
				// 进一步检查：如果该历史记录已有分P，检查最后一个分P的结束时间和当前文件的开始时间是否有间隔
				var lastPart models.RecordHistoryPart
				if err := db.Where("history_id = ?", h.ID).Order("end_time DESC").First(&lastPart).Error; err == nil {
					partGap := metadata.StartTime.Sub(lastPart.EndTime)
					if partGap >= maxGapDuration {
						log.Printf("[FileScan] 最后分P到新文件间隔过大(%.1f分钟)，不合并: 已有=%s, 新=%s",
							partGap.Minutes(), h.Title, metadata.Title)
						continue
					}
					// 如果间隔在可接受范围内但>10分钟，可能是直播过程中因为标题变化自动断开了
					// 在这种情况下，仍然合并，并更新标题
					if partGap > 10*time.Minute {
						log.Printf("[FileScan] 分P间隔%.1f分钟（可能是标题变化导致），继续合并: %s -> %s",
							partGap.Minutes(), h.Title, metadata.Title)
					}
				}

				// 找到可合并的历史记录
				if metadata.EndTime.After(h.EndTime) {
					h.EndTime = metadata.EndTime
				}
				// 如果新文件有更新的标题，更新历史记录的标题
				if metadata.Title != "" && metadata.Title != h.Title {
					log.Printf("[FileScan] 检测到标题变化，更新为最新: %s -> %s", h.Title, metadata.Title)
					h.Title = metadata.Title
				}
				db.Save(&h)
				log.Printf("[FileScan] 合并到已有历史记录: ID=%d, SessionID=%s (基于时间连续性: 间隔%.1f分钟<30分钟)",
					h.ID, h.SessionID, timeDiff.Minutes())
				return &h, nil
			}
			if timeDiff >= maxGapDuration {
				log.Printf("[FileScan] 时间间隔过大(%.1f分钟)，不合并: 已有=%s(%v), 新=%s(%v)",
					timeDiff.Minutes(), h.Title, h.EndTime, metadata.Title, metadata.StartTime)
			}
		}
	}

	// 创建新的历史记录
	// 尝试从直播间API获取真实的主播名
	uname := metadata.Uname
	liveStatusService := NewLiveStatusService()
	roomInfo, roomErr := liveStatusService.GetRoomInfo(metadata.RoomID)
	if roomErr == nil && roomInfo.Data.UID > 0 {
		userInfo, userErr := liveStatusService.GetUserInfo(roomInfo.Data.UID)
		if userErr == nil && userInfo.Data.Info.Uname != "" {
			uname = userInfo.Data.Info.Uname
			log.Printf("[FileScan] 从API获取主播名: %s (UID=%d)", uname, roomInfo.Data.UID)
		} else {
			log.Printf("[FileScan] 获取主播名失败: %v, 使用默认: %s", userErr, uname)
		}
	} else {
		log.Printf("[FileScan] 获取直播间信息失败: %v, 使用默认主播名: %s", roomErr, uname)
	}

	history = models.RecordHistory{
		RoomID:    metadata.RoomID,
		SessionID: metadata.SessionID,
		EventID:   fmt.Sprintf("scan_%s_%d", metadata.RoomID, time.Now().Unix()),
		Uname:     uname, // 使用从API获取的真实主播名
		Title:     metadata.Title,
		AreaName:  metadata.AreaName,
		StartTime: metadata.StartTime,
		EndTime:   metadata.EndTime,
		Recording: false,
		Streaming: false,
		Upload:    room.Upload,
		Publish:   false,
	}

	// 创建前清理同 session_id 的软删除记录（避免 UNIQUE 约束冲突）
	// 用户从 UI 删除历史记录时 GORM 只做软删除（设置 deleted_at），session_id 仍占用唯一索引
	var softDeletedHistory models.RecordHistory
	if err := db.Unscoped().Where("session_id = ? AND deleted_at IS NOT NULL", metadata.SessionID).First(&softDeletedHistory).Error; err == nil {
		log.Printf("[FileScan] 发现被软删除的历史记录占用了 session_id，永久删除以释放: SessionID=%s, ID=%d",
			metadata.SessionID, softDeletedHistory.ID)
		if err := db.Unscoped().Delete(&softDeletedHistory).Error; err != nil {
			log.Printf("[FileScan] 永久删除软删除记录失败: %v", err)
		}
	}

	if err := db.Create(&history).Error; err != nil {
		return nil, fmt.Errorf("创建历史记录失败: %w", err)
	}

	log.Printf("[FileScan] 创建新历史记录: ID=%d, SessionID=%s, RoomID=%s, Uname=%s",
		history.ID, history.SessionID, metadata.RoomID, history.Uname)

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
