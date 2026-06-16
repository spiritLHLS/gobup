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

	// 构建更新后的分P列表：以B站API返回的当前分P列表为基础，追加弹幕版
	// 这样可以确保与B站实际状态严格一致，避免遗漏或重复
	var allVideoParts []bili.PublishVideoPartRequest
	for i, v := range archiveDetail.Videos {
		// 尝试从DB查找对应记录以获取 PartTitle 模板所需元数据，找不到则使用B站返回的 part 名
		partTitle := v.Part
		if room.PartTitleTemplate != "" {
			var dbPart models.RecordHistoryPart
			if dbErr := db.Where("c_id = ? AND file_delete = ?", v.CID, false).First(&dbPart).Error; dbErr == nil {
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
	allVideoParts = append(allVideoParts, bili.PublishVideoPartRequest{
		Title:    partTitle,
		Desc:     "",
		Filename: burnedPart.FileName,
		Cid:      int64(burnedPart.CID),
	})

	log.Printf("[回补弹幕版] 更新后总分P数: %d (B站现有%d + 弹幕版1)",
		len(allVideoParts), len(archiveDetail.Videos))

	// 使用原视频的信息进行编辑
	title := archiveDetail.Archive.Title
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
	db.Save(&burnedPart)
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

	oneHourAgo := time.Now().Add(-1 * time.Hour)

	for _, room := range rooms {
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
			s.appendBurnedPartsForApprovedHistory(&history, &room)
		}
	}

	return nil
}

// appendBurnedPartsForApprovedHistory 对单条已审核通过的历史记录执行弹幕回补逻辑。
// 遍历该历史记录下的每个原始分P，若满足以下所有条件则异步触发烧录+上传+追加：
//  1. 原始视频文件还在磁盘上
//  2. 同名 .xml 弹幕文件存在
//  3. 该分P尚未有对应的已追加弹幕版（appended_to_video=false 或无记录）
func (s *Service) appendBurnedPartsForApprovedHistory(history *models.RecordHistory, room *models.RecordRoom) {
	db := database.GetDB()

	// 获取该历史记录的所有已上传原始分P（非临时文件）
	var originalParts []models.RecordHistoryPart
	if err := db.Where(
		"history_id = ? AND upload = ? AND is_temp_file = ? AND recording = ?",
		history.ID, true, false, false,
	).Find(&originalParts).Error; err != nil {
		log.Printf("[弹幕回补] 查询历史记录 %d 的原始分P失败: %v", history.ID, err)
		return
	}

	if len(originalParts) == 0 {
		return
	}

	burnService := services.NewDanmakuBurnService()

	for _, part := range originalParts {
		// 1. 检查是否已有追加成功的弹幕烧录版
		var appendedCount int64
		db.Model(&models.RecordHistoryPart{}).Where(
			"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ? AND appended_to_video = ?",
			part.ID, true, "danmaku_burn", true,
		).Count(&appendedCount)
		if appendedCount > 0 {
			continue // 已追加，跳过
		}

		// 2. 检查是否已有上传完成但 appended_to_video=false 的烧录版（可能 EditVideo 失败需重试）
		var pendingAppend models.RecordHistoryPart
		if err := db.Where(
			"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ? AND c_id > 0 AND appended_to_video = ?",
			part.ID, true, "danmaku_burn", true, false,
		).First(&pendingAppend).Error; err == nil {
			// 已上传但未成功追加，直接重新触发 UpdatePublishedVideoWithBurnedParts
			log.Printf("[弹幕回补] 发现已上传但未追加的弹幕版，重新触发追加: burned_part_id=%d", pendingAppend.ID)
			go func(pid uint) {
				s.appendBurnedSem <- struct{}{}
				defer func() { <-s.appendBurnedSem }()
				if err := s.UpdatePublishedVideoWithBurnedParts(pid); err != nil {
					log.Printf("[弹幕回补] 重新追加失败: burned_part_id=%d, err=%v", pid, err)
				}
			}(pendingAppend.ID)
			continue
		}

		// 3. 检查是否已有正在烧录/上传中的记录（任意状态，避免重复触发）
		var anyBurnedCount int64
		db.Model(&models.RecordHistoryPart{}).Where(
			"source_part_id = ? AND is_temp_file = ? AND temp_file_type = ?",
			part.ID, true, "danmaku_burn",
		).Count(&anyBurnedCount)
		if anyBurnedCount > 0 {
			// 已有烧录记录（upload=false：正在上传中，或文件不存在待重试等），跳过避免重复
			continue
		}

		// 4. 检查原始视频文件是否存在
		if part.FilePath == "" {
			continue
		}
		if _, err := os.Stat(part.FilePath); os.IsNotExist(err) {
			log.Printf("[弹幕回补] 原始视频文件不存在，跳过: history_id=%d, part_id=%d, file=%s",
				history.ID, part.ID, part.FilePath)
			continue
		}

		// 5. 检查 XML 弹幕文件是否存在
		xmlPath := burnService.FindDanmakuXML(part.FilePath)
		if xmlPath == "" {
			log.Printf("[弹幕回补] XML弹幕文件不存在，跳过: history_id=%d, part_id=%d, video=%s",
				history.ID, part.ID, part.FilePath)
			continue
		}

		log.Printf("[弹幕回补] 开始异步烧录并追加弹幕版: history_id=%d, part_id=%d, video=%s, xml=%s",
			history.ID, part.ID, part.FilePath, xmlPath)

		// 异步执行烧录 → 上传 → 追加（避免阻塞定时任务）
		go func(p models.RecordHistoryPart, h models.RecordHistory, r models.RecordRoom) {
			bs := services.NewDanmakuBurnService()
			burnedPath, err := bs.BurnDanmakuToVideo(&p, &h, &r)
			if err != nil {
				log.Printf("[弹幕回补] 烧录失败: part_id=%d, err=%v", p.ID, err)
				// 创建失败标记，防止定时任务对同一损坏/无效XML无限重试
				failedMarker := &models.RecordHistoryPart{
					HistoryID:    p.HistoryID,
					SourcePartID: p.ID,
					IsTempFile:   true,
					TempFileType: "danmaku_burn",
					Upload:       false,
				}
				if createErr := database.GetDB().Create(failedMarker).Error; createErr != nil {
					log.Printf("[弹幕回补] 创建失败标记出错: part_id=%d, err=%v", p.ID, createErr)
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
}
