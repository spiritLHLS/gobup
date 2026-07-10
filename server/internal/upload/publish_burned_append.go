package upload

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

const maxBurnedAppendAttemptsPerRun = 3

type danmakuBackfillStats struct {
	appendAttempts      int
	appendErrors        int
	appendCooldownSkips int
	appendAttemptSkips  int
	rateLimitStops      int
	missingSourceMarked int
	missingXML          int
	factoryUnavailable  int
	startedBurns        int
}

// UpdatePublishedVideoWithBurnedParts 回补更新已投稿视频，追加弹幕版分P
// 当弹幕版分P上传完成后，检查对应的历史记录是否已投稿，如果已投稿且没有弹幕版，则追加弹幕版分P
func (s *Service) UpdatePublishedVideoWithBurnedParts(burnedPartID uint) error {
	// 内存级防重入：防止 Path A（上传完成立即调用）与 Path B/C（定时任务发现 appended_to_video=false）并发调用
	// 导致同一弹幕版分P 在 B 站视频里出现两次
	if _, loaded := s.appendingBurnedParts.LoadOrStore(burnedPartID, true); loaded {
		log.Printf("[回补弹幕版] 烧录版分P %d 正在追加中，跳过并发调用", burnedPartID)
		return nil
	}
	defer s.appendingBurnedParts.Delete(burnedPartID)

	db := database.GetDB()

	// 获取弹幕版分P
	var burnedPart models.RecordHistoryPart
	if err := db.First(&burnedPart, burnedPartID).Error; err != nil {
		return fmt.Errorf("弹幕版分P不存在: %w", err)
	}

	// 检查是否为弹幕烧录产生的临时文件
	if !burnedPart.IsTempFile || burnedPart.TempFileType != "danmaku_burn" {
		log.Printf("[回补弹幕版] 跳过：不是弹幕烧录分P (part_id=%d)", burnedPartID)
		return nil
	}

	// 检查是否已上传
	if !burnedPart.Upload {
		log.Printf("[回补弹幕版] 跳过：弹幕版尚未上传完成 (part_id=%d)", burnedPartID)
		return nil
	}

	// 获取对应的历史记录
	var history models.RecordHistory
	if err := db.First(&history, burnedPart.HistoryID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	// 检查是否已投稿
	if !history.Publish || history.AvID == "" {
		log.Printf("[回补弹幕版] 跳过：历史记录尚未投稿 (history_id=%d)", history.ID)
		return nil
	}

	// 获取房间配置
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间不存在: %w", err)
	}

	// 获取用户信息
	var user models.BiliBiliUser
	if err := db.First(&user, room.UploadUserID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}
	if !user.Enabled {
		return fmt.Errorf("用户已禁用")
	}

	log.Printf("[回补弹幕版] 开始处理: history_id=%d, aid=%s, burned_part_id=%d",
		history.ID, history.AvID, burnedPartID)

	// 先从B站API获取视频当前状态，以准确判断弹幕版是否已追加过
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	aidInt, err := strconv.ParseInt(history.AvID, 10, 64)
	if err != nil {
		return fmt.Errorf("解析AID失败: %w", err)
	}

	archiveDetail, err := client.GetArchiveDetailByAid(aidInt)
	if err != nil {
		return fmt.Errorf("获取原视频信息失败: %w", err)
	}

	// 使用B站API返回的实际分P列表判断弹幕版CID是否已在视频中
	// 不能依赖DB中 upload=true 的记录来判断，因为 upload=true 只表示文件已上传到CDN，不代表已提交到视频
	for _, v := range archiveDetail.Videos {
		if v.CID == burnedPart.CID {
			log.Printf("[回补弹幕版] 跳过：弹幕版CID=%d 已在B站视频中 (part_id=%d)", burnedPart.CID, burnedPartID)
			return nil
		}
	}
	if skip, reason := shouldSkipTooShortPublishPart(burnedPart); skip {
		log.Printf("[回补弹幕版] 跳过不可追加弹幕版: burned_part_id=%d, reason=%s", burnedPart.ID, reason)
		markPublishPartSkipped(db, &burnedPart, reason)
		return fmt.Errorf("弹幕版分P不可追加: %s", reason)
	}

	// 构建更新后的分P列表：以B站API返回的当前分P列表为基础，追加弹幕版
	// 这样可以确保与B站实际状态严格一致，避免遗漏或重复
	dbPartByCID := make(map[int64]models.RecordHistoryPart, len(archiveDetail.Videos))
	if room.PartTitleTemplate != "" && len(archiveDetail.Videos) > 0 {
		cids := make([]int64, 0, len(archiveDetail.Videos))
		for _, v := range archiveDetail.Videos {
			cids = append(cids, v.CID)
		}
		var dbParts []models.RecordHistoryPart
		if err := db.Where("c_id IN ? AND file_delete = ?", cids, false).Find(&dbParts).Error; err == nil {
			for _, part := range dbParts {
				dbPartByCID[part.CID] = part
			}
		}
	}

	var allVideoParts []bili.PublishVideoPartRequest
	for i, v := range archiveDetail.Videos {
		// 尝试从DB查找对应记录以获取 PartTitle 模板所需元数据，找不到则使用B站返回的 part 名
		partTitle := v.Part
		if room.PartTitleTemplate != "" {
			if dbPart, ok := dbPartByCID[v.CID]; ok {
				partTemplateData := map[string]interface{}{
					"index":     i + 1,
					"startTime": dbPart.StartTime,
					"areaName":  dbPart.AreaName,
					"uname":     history.Uname,
					"title":     history.Title,
					"roomId":    history.RoomID,
					"fileName":  dbPart.FileName,
				}
				partTitle = s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)
			}
		}
		partTitle = normalizeBiliPartTitle(i+1, partTitle)
		allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: v.Filename,
			Cid:      v.CID,
		})
	}

	// 追加弹幕版分P
	partTemplateData := map[string]interface{}{
		"index":     len(allVideoParts) + 1,
		"startTime": burnedPart.StartTime,
		"areaName":  burnedPart.AreaName,
		"uname":     history.Uname,
		"title":     history.Title,
		"roomId":    history.RoomID,
		"fileName":  burnedPart.FileName,
	}
	partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData) + "（弹幕版）"
	partTitle = normalizeBiliPartTitle(len(allVideoParts)+1, partTitle)
	allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
		Title:    partTitle,
		Desc:     "",
		Filename: burnedPart.FileName,
		Cid:      int64(burnedPart.CID),
	})

	log.Printf("[回补弹幕版] 更新后总分P数: %d (B站现有%d + 弹幕版1)",
		len(allVideoParts), len(archiveDetail.Videos))

	// 使用原视频的信息进行编辑
	title := normalizeBiliPublishTitle("稿件标题", archiveDetail.Archive.Title)
	desc := archiveDetail.Archive.Desc
	tags := strings.Join(archiveDetail.Archive.Tag, ",")
	tid := archiveDetail.Archive.Tid
	copyright := archiveDetail.Archive.Copyright
	cover := archiveDetail.Archive.Pic
	source := archiveDetail.Archive.Source

	// 调用EditVideo API追加弹幕版分P
	log.Printf("[回补弹幕版] 调用EditVideo API: aid=%d, 总分P=%d", aidInt, len(allVideoParts))
	if err := client.EditVideo(aidInt, title, desc, tags, tid, copyright, cover, allVideoParts, source); err != nil {
		errStr := err.Error()
		// code=21588: 该文件内容无法被识别（Bilibili CDN上的文件损坏/无效）
		// code=21054: 视频文件不存在（CDN上已被清除）
		// 这类错误不可通过重试恢复，需要删除损坏的烧录记录并重新触发一次完整的烧录+上传流程
		if strings.Contains(errStr, "code=21588") || strings.Contains(errStr, "code=21054") {
			log.Printf("[回补弹幕版] 检测到不可恢复的CDN文件错误 (burned_part_id=%d): %v", burnedPartID, err)
			log.Printf("[回补弹幕版] 将删除损坏的烧录记录，下次回补检查周期将重新触发烧录 (source_part_id=%d)", burnedPart.SourcePartID)
			// 删除本地烧录文件（如果还在磁盘上）
			if burnedPart.FilePath != "" {
				if removeErr := os.Remove(burnedPart.FilePath); removeErr != nil && !os.IsNotExist(removeErr) {
					log.Printf("[回补弹幕版] 删除损坏烧录文件失败: %v", removeErr)
				} else {
					log.Printf("[回补弹幕版] 已删除损坏烧录文件: %s", burnedPart.FilePath)
				}
			}
			// 硬删除DB记录（不用软删除，避免 anyBurnedCount 仍能查到）
			if dbErr := db.Unscoped().Delete(&burnedPart).Error; dbErr != nil {
				log.Printf("[回补弹幕版] 删除损坏烧录DB记录失败: burned_part_id=%d, err=%v", burnedPartID, dbErr)
			} else {
				log.Printf("[回补弹幕版] 已删除损坏烧录DB记录: burned_part_id=%d", burnedPartID)
			}
			return fmt.Errorf("追加弹幕版分P失败(CDN文件损坏，已重置等待重新烧录): %w", err)
		}
		return fmt.Errorf("追加弹幕版分P失败: %w", err)
	}

	log.Printf("[回补弹幕版] 更新成功: aid=%d, bvid=%s, 已追加弹幕版分P", aidInt, history.BvID)

	// 标记弹幕版分P为已追加到视频
	burnedPart.AppendedToVideo = true
	db.Model(&burnedPart).Updates(map[string]interface{}{
		"appended_to_video":      true,
		"upload_error_msg":       "",
		"upload_error_type":      "",
		"rate_limit_retry_count": 0,
		"rate_limit_cooldown_at": nil,
	})
	log.Printf("[回补弹幕版] 已标记 AppendedToVideo=true: burned_part_id=%d", burnedPart.ID)

	// 清理已追加的弹幕烧录视频文件（物理文件已无需保留）
	if burnedPart.SessionID != "" {
		burnService := services.NewDanmakuBurnService()
		_ = burnService.CleanAppendedBurnedPartsBySessionID(burnedPart.SessionID)
	}

	// 更新历史记录的消息
	history.Message = fmt.Sprintf("投稿成功（已追加弹幕版）")
	db.Save(&history)

	// 推送通知
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "投稿") {
		s.wxPusher.NotifyPublishSuccess(room.UploadUserID, room.Wxuid, history.Uname,
			fmt.Sprintf("%s (已追加弹幕版)", title), history.BvID)
	}

	return nil
}

// AppendDanmakuBurnedPartsToApprovedVideos 定时任务入口：
// 对所有已审核通过（video_state=1）且 approved_at 距今超过 1 小时的投稿，
// 检查各原始分P是否还有未追加的弹幕烧录版。如果原始录制视频文件和对应的
// XML 弹幕文件仍然存在，则触发烧录 → 上传 → 追加到 B 站视频的完整流程。
func (s *Service) AppendDanmakuBurnedPartsToApprovedVideos() error {
	db := database.GetDB()

	// 只处理启用了弹幕烧录功能的房间
	var rooms []models.RecordRoom
	if err := db.Where("enable_danmaku_burn = ? AND upload = ?", true, true).Find(&rooms).Error; err != nil {
		return fmt.Errorf("[弹幕回补] 查询房间失败: %w", err)
	}

	if len(rooms) == 0 {
		return nil
	}

	stats := &danmakuBackfillStats{}
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	stopRun := false
	factoryAvailable := true
	if _, err := services.CheckDanmakuFactoryAvailable(); err != nil {
		factoryAvailable = false
	}

	for _, room := range rooms {
		if stopRun {
			break
		}
		if room.UploadUserID == 0 {
			continue
		}

		// 查找该房间已投稿、已审核通过且 approved_at 至少 1 小时前的历史记录
		var histories []models.RecordHistory
		if err := db.Where(
			"room_id = ? AND publish = ? AND video_state = ? AND approved_at IS NOT NULL AND approved_at <= ?",
			room.RoomID, true, 1, oneHourAgo,
		).Find(&histories).Error; err != nil {
			log.Printf("[弹幕回补] 查询房间 %s 的历史记录失败: %v", room.RoomID, err)
			continue
		}

		if len(histories) == 0 {
			continue
		}

		log.Printf("[弹幕回补] 房间 %s 发现 %d 条符合弹幕回补条件的历史记录", room.RoomID, len(histories))

		for _, history := range histories {
			if !s.appendBurnedPartsForApprovedHistory(&history, &room, stats, factoryAvailable) {
				stopRun = true
				break
			}
		}
	}
	if stats.appendAttempts > 0 || stats.appendCooldownSkips > 0 || stats.missingSourceMarked > 0 ||
		stats.missingXML > 0 || stats.factoryUnavailable > 0 || stats.startedBurns > 0 || stats.rateLimitStops > 0 {
		log.Printf("[弹幕回补] 扫描完成: 追加尝试=%d, 追加失败=%d, 冷却跳过=%d, 限额跳过=%d, 限流早停=%d, 源文件缺失标记=%d, XML缺失=%d, DanmakuFactory不可用=%d, 新触发烧录=%d",
			stats.appendAttempts, stats.appendErrors, stats.appendCooldownSkips, stats.appendAttemptSkips, stats.rateLimitStops,
			stats.missingSourceMarked, stats.missingXML, stats.factoryUnavailable, stats.startedBurns)
	}

	return nil
}

// appendBurnedPartsForApprovedHistory 对单条已审核通过的历史记录执行弹幕回补逻辑。
// 遍历该历史记录下的每个原始分P，若满足以下所有条件则异步触发烧录+上传+追加：
//  1. 原始视频文件还在磁盘上
//  2. 同名 .xml 弹幕文件存在
//  3. 该分P尚未有对应的已追加弹幕版（appended_to_video=false 或无记录）
func (s *Service) appendBurnedPartsForApprovedHistory(history *models.RecordHistory, room *models.RecordRoom, stats *danmakuBackfillStats, factoryAvailable bool) bool {
	db := database.GetDB()

	// 获取该历史记录的所有已上传原始分P（非临时文件）
	var originalParts []models.RecordHistoryPart
	if err := db.Where(
		"history_id = ? AND upload = ? AND is_temp_file = ? AND recording = ?",
		history.ID, true, false, false,
	).Find(&originalParts).Error; err != nil {
		log.Printf("[弹幕回补] 查询历史记录 %d 的原始分P失败: %v", history.ID, err)
		return true
	}

	if len(originalParts) == 0 {
		return true
	}

	sourceIDs := make([]uint, 0, len(originalParts))
	for _, part := range originalParts {
		sourceIDs = append(sourceIDs, part.ID)
	}
	anyBurnedSourceSet := make(map[uint]struct{}, len(originalParts))
	appendedSourceSet := make(map[uint]struct{}, len(originalParts))
	pendingAppendBySource := make(map[uint]models.RecordHistoryPart, len(originalParts))
	var burnedParts []models.RecordHistoryPart
	if err := db.Where(
		"source_part_id IN ? AND is_temp_file = ? AND temp_file_type = ?",
		sourceIDs, true, "danmaku_burn",
	).Order("created_at ASC").Find(&burnedParts).Error; err != nil {
		log.Printf("[弹幕回补] 查询历史记录 %d 的烧录分P失败: %v", history.ID, err)
		return true
	}
	for _, burned := range burnedParts {
		anyBurnedSourceSet[burned.SourcePartID] = struct{}{}
		if burned.AppendedToVideo {
			appendedSourceSet[burned.SourcePartID] = struct{}{}
			continue
		}
		if burned.Upload && burned.CID > 0 {
			if _, exists := pendingAppendBySource[burned.SourcePartID]; !exists {
				pendingAppendBySource[burned.SourcePartID] = burned
			}
		}
	}

	burnService := services.NewDanmakuBurnService()

	for _, part := range originalParts {
		// 1. 检查是否已有追加成功的弹幕烧录版
		if _, ok := appendedSourceSet[part.ID]; ok {
			continue // 已追加，跳过
		}

		// 2. 检查是否已有上传完成但 appended_to_video=false 的烧录版（可能 EditVideo 失败需重试）
		if pendingAppend, ok := pendingAppendBySource[part.ID]; ok {
			if pendingAppend.RateLimitCooldownAt != nil && pendingAppend.RateLimitCooldownAt.After(time.Now()) {
				stats.appendCooldownSkips++
				continue
			}
			if stats.appendAttempts >= maxBurnedAppendAttemptsPerRun {
				stats.appendAttemptSkips++
				return false
			}
			stats.appendAttempts++
			log.Printf("[弹幕回补] 发现已上传但未追加的弹幕版，重新触发追加: burned_part_id=%d", pendingAppend.ID)
			if err := s.UpdatePublishedVideoWithBurnedParts(pendingAppend.ID); err != nil {
				stats.appendErrors++
				if s.markBurnedAppendFailure(&pendingAppend, err) {
					stats.rateLimitStops++
					return false
				}
				log.Printf("[弹幕回补] 重新追加失败: burned_part_id=%d, err=%v", pendingAppend.ID, err)
			}
			continue
		}

		// 3. 检查是否已有正在烧录/上传中的记录（任意状态，避免重复触发）
		if _, ok := anyBurnedSourceSet[part.ID]; ok {
			// 已有烧录记录（upload=false：正在上传中，或文件不存在待重试等），跳过避免重复
			continue
		}

		// 4. 检查原始视频文件是否存在
		if part.FilePath == "" {
			s.markDanmakuBurnSkipped(&part, "源视频文件路径为空，已停止该分P弹幕回补", UploadErrorTypeFile)
			stats.missingSourceMarked++
			continue
		}
		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			s.markDanmakuBurnSkipped(&part, "源视频文件不存在，已停止该分P弹幕回补: "+part.FilePath, UploadErrorTypeFile)
			stats.missingSourceMarked++
			continue
		}

		// 5. 检查 XML 弹幕文件是否存在
		xmlPath := burnService.FindDanmakuXML(part.FilePath)
		if xmlPath == "" {
			stats.missingXML++
			continue
		}
		if !factoryAvailable {
			stats.factoryUnavailable++
			continue
		}

		log.Printf("[弹幕回补] 开始异步烧录并追加弹幕版: history_id=%d, part_id=%d, video=%s, xml=%s",
			history.ID, part.ID, part.FilePath, xmlPath)

		// 异步执行烧录 → 上传 → 追加（避免阻塞定时任务）
		stats.startedBurns++
		go func(p models.RecordHistoryPart, h models.RecordHistory, r models.RecordRoom) {
			bs := services.NewDanmakuBurnService()
			burnedPath, err := bs.BurnDanmakuToVideo(&p, &h, &r)
			if err != nil {
				errorType := classifyUploadError(err)
				if shouldAutoStopErrorType(errorType, 1) {
					s.markDanmakuBurnSkipped(&p, "弹幕回补烧录失败，已停止该分P自动回补: "+err.Error(), errorType)
				} else {
					log.Printf("[弹幕回补] 烧录失败: part_id=%d, err=%v", p.ID, err)
				}
				return
			}

			// 查询新生成的烧录版 Part 记录
			var burnedPart models.RecordHistoryPart
			if dbErr := database.GetDB().Where(
				"file_path = ? AND is_temp_file = ? AND temp_file_type = ?",
				burnedPath, true, "danmaku_burn",
			).First(&burnedPart).Error; dbErr != nil {
				log.Printf("[弹幕回补] 查询烧录版分P记录失败: part_id=%d, err=%v", p.ID, dbErr)
				return
			}

			log.Printf("[弹幕回补] 烧录完成，加入上传队列: burned_part_id=%d", burnedPart.ID)
			if uploadErr := s.UploadPart(&burnedPart, &h, &r); uploadErr != nil {
				log.Printf("[弹幕回补] 烧录版入队失败: burned_part_id=%d, err=%v", burnedPart.ID, uploadErr)
			}
		}(part, *history, *room)
	}
	return true
}

func (s *Service) markBurnedAppendFailure(part *models.RecordHistoryPart, err error) bool {
	if part == nil || part.ID == 0 || err == nil {
		return false
	}
	errorType := classifyUploadError(err)
	retryCount := part.RateLimitRetryCount + 1
	message := fmt.Sprintf("追加弹幕版分P失败: %v", err)
	updates := map[string]interface{}{
		"upload_error_msg":  message,
		"upload_error_type": errorType,
	}
	if cooldownDelay, ok := autoTaskCooldownDuration(errorType, retryCount); ok {
		cooldown := time.Now().Add(cooldownDelay)
		updates["rate_limit_retry_count"] = retryCount
		updates["rate_limit_cooldown_at"] = &cooldown
		part.RateLimitRetryCount = retryCount
		part.RateLimitCooldownAt = &cooldown
		if errorType == UploadErrorTypeRateLimit {
			log.Printf("[弹幕回补] B站接口限流，停止本轮回补并冷却至 %s: burned_part_id=%d",
				cooldown.Format("2006-01-02 15:04:05"), part.ID)
		}
	} else if shouldAutoStopErrorType(errorType, retryCount) {
		updates["upload_cancelled"] = true
		updates["rate_limit_cooldown_at"] = nil
		part.UploadCancelled = true
		part.RateLimitCooldownAt = nil
	}
	if dbErr := database.GetDB().Model(part).Updates(updates).Error; dbErr != nil {
		log.Printf("[弹幕回补] 记录追加失败状态失败: burned_part_id=%d, err=%v", part.ID, dbErr)
	}
	part.UploadErrorMsg = message
	part.UploadErrorType = errorType
	return errorType == UploadErrorTypeRateLimit
}
