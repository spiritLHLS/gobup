package controllers

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
	"github.com/gobup/server/internal/upload"
)

var historyUploadService *upload.Service

// SetHistoryUploadService 设置上传服务
func SetHistoryUploadService(svc *upload.Service) {
	historyUploadService = svc
}

func ListHistories(c *gin.Context) {
	db := database.GetDB()

	// 支持 POST 请求体（新接口）
	type ListRequest struct {
		Page      int    `json:"page" form:"page"`
		PageSize  int    `json:"pageSize" form:"pageSize"`
		RoomId    string `json:"roomId" form:"roomId"`
		BvId      string `json:"bvId" form:"bvId"`
		ViewType  string `json:"viewType" form:"viewType"` // working | archived
		From      string `json:"from" form:"from"`
		To        string `json:"to" form:"to"`
		Recording *bool  `json:"recording"`
		Upload    *int   `json:"upload"`
		Publish   *bool  `json:"publish"`
	}

	var req ListRequest
	req.Page = 1
	req.PageSize = 10
	req.ViewType = "working"

	if c.Request.Method == "POST" {
		c.ShouldBindJSON(&req)
	} else {
		c.ShouldBindQuery(&req)
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 200 {
		req.PageSize = 10
	}

	// 构建查询
	query := db.Model(&models.RecordHistory{})

	// 添加搜索条件
	if req.RoomId != "" {
		query = query.Where("room_id = ?", req.RoomId)
	}
	if req.BvId != "" {
		query = query.Where("bv_id = ?", req.BvId)
	}
	if req.Recording != nil {
		query = query.Where("recording = ?", *req.Recording)
	}
	if req.Publish != nil {
		if *req.Publish {
			query = query.Where("bv_id IS NOT NULL AND bv_id != ''")
		} else {
			query = query.Where("(bv_id IS NULL OR bv_id = '')")
		}
	}

	// upload_status 过滤：0=未上传, 1=上传中, 2=已上传
	if req.Upload != nil {
		query = query.Where("upload_status = ?", *req.Upload)
	}

	// 日期范围过滤
	if req.From != "" {
		query = query.Where("start_time >= ?", req.From+" 00:00:00")
	}
	if req.To != "" {
		query = query.Where("start_time <= ?", req.To+" 23:59:59")
	}

	// viewType 过滤：working = 录制中或最近7天未整理；archived = 其余
	if req.ViewType == "working" {
		sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
		query = query.Where("recording = ? OR start_time >= ?", true, sevenDaysAgo)
	} else if req.ViewType == "archived" {
		sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
		query = query.Where("recording = ? AND start_time < ?", false, sevenDaysAgo)
	}

	// 统计 workingCount（用于标签徽章）
	var workingCount int64
	sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
	db.Model(&models.RecordHistory{}).Where("recording = ? OR start_time >= ?", true, sevenDaysAgo).Count(&workingCount)

	// 获取总数
	var total int64
	query.Count(&total)

	// 分页查询
	var histories []models.RecordHistory
	offset := (req.Page - 1) * req.PageSize
	query.Order("end_time DESC").Limit(req.PageSize).Offset(offset).Find(&histories)

	// 用单条聚合 SQL 替代 N+1 的 4 次 COUNT 查询
	if len(histories) > 0 {
		historyIDs := make([]uint, len(histories))
		for i, h := range histories {
			historyIDs[i] = h.ID
		}

		type partStats struct {
			HistoryID      uint
			PartCount      int64
			UploadCount    int64
			RecordCount    int64
			UploadingCount int64
		}
		var statsRows []partStats
		db.Model(&models.RecordHistoryPart{}).
			Select("history_id, COUNT(*) as part_count, SUM(CASE WHEN upload=1 THEN 1 ELSE 0 END) as upload_count, SUM(CASE WHEN recording=1 THEN 1 ELSE 0 END) as record_count, SUM(CASE WHEN uploading=1 THEN 1 ELSE 0 END) as uploading_count").
			Where("history_id IN ?", historyIDs).
			Group("history_id").
			Scan(&statsRows)

		statsMap := make(map[uint]partStats, len(statsRows))
		for _, s := range statsRows {
			statsMap[s.HistoryID] = s
		}

		for i := range histories {
			s := statsMap[histories[i].ID]
			histories[i].PartCount = int(s.PartCount)
			histories[i].UploadPartCount = int(s.UploadCount)
			histories[i].RecordPartCount = int(s.RecordCount)

			// 计算上传状态
			if s.UploadingCount > 0 {
				histories[i].UploadStatus = 1 // 上传中
			} else if s.UploadCount > 0 && s.UploadCount == s.PartCount {
				histories[i].UploadStatus = 2 // 全部已上传
			} else if s.UploadCount > 0 {
				histories[i].UploadStatus = 2 // 部分已上传（对外仍显示为已上传）
			} else {
				histories[i].UploadStatus = 0 // 未上传
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"list":         histories,
		"total":        total,
		"workingCount": workingCount,
	})
}

func ExportHistories(c *gin.Context) {
	db := database.GetDB()

	type ExportRequest struct {
		RoomId    string `json:"roomId" form:"roomId"`
		BvId      string `json:"bvId" form:"bvId"`
		ViewType  string `json:"viewType" form:"viewType"`
		From      string `json:"from" form:"from"`
		To        string `json:"to" form:"to"`
		Recording *bool  `json:"recording"`
		Upload    *int   `json:"upload"`
		Publish   *bool  `json:"publish"`
	}

	var req ExportRequest
	req.ViewType = "working"
	_ = c.ShouldBindJSON(&req)

	query := db.Model(&models.RecordHistory{})
	if req.RoomId != "" {
		query = query.Where("room_id = ?", req.RoomId)
	}
	if req.BvId != "" {
		query = query.Where("bv_id = ?", req.BvId)
	}
	if req.Recording != nil {
		query = query.Where("recording = ?", *req.Recording)
	}
	if req.Publish != nil {
		if *req.Publish {
			query = query.Where("bv_id IS NOT NULL AND bv_id != ''")
		} else {
			query = query.Where("(bv_id IS NULL OR bv_id = '')")
		}
	}
	if req.Upload != nil {
		query = query.Where("upload_status = ?", *req.Upload)
	}
	if req.From != "" {
		query = query.Where("start_time >= ?", req.From+" 00:00:00")
	}
	if req.To != "" {
		query = query.Where("start_time <= ?", req.To+" 23:59:59")
	}
	if req.ViewType == "working" {
		sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
		query = query.Where("recording = ? OR start_time >= ?", true, sevenDaysAgo)
	} else if req.ViewType == "archived" {
		sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
		query = query.Where("recording = ? AND start_time < ?", false, sevenDaysAgo)
	}

	var histories []models.RecordHistory
	if err := query.Order("end_time DESC").Limit(10000).Find(&histories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"type": "error", "msg": "导出查询失败"})
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=gobup-history-%s.csv", time.Now().Format("20060102-150405")))
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"ID", "房间ID", "主播", "标题", "开始时间", "结束时间", "BV号", "AV号", "上传状态", "投稿状态", "视频状态", "消息"})
	for _, history := range histories {
		_ = writer.Write([]string{
			strconv.FormatUint(uint64(history.ID), 10),
			history.RoomID,
			history.Uname,
			history.Title,
			history.StartTime.Format("2006-01-02 15:04:05"),
			history.EndTime.Format("2006-01-02 15:04:05"),
			history.BvID,
			history.AvID,
			strconv.Itoa(history.UploadStatus),
			strconv.FormatBool(history.Publish),
			history.VideoStateDesc,
			history.Message,
		})
	}
	writer.Flush()
}

func UpdateHistory(c *gin.Context) {
	var history models.RecordHistory
	if err := c.ShouldBindJSON(&history); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()
	db.Save(&history)
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "更新成功"})
}

func DeleteHistory(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	// 获取历史记录以获得 session_id
	var history models.RecordHistory
	if err := db.First(&history, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"type": "error", "msg": "历史记录不存在"})
		return
	}

	// 删除弹幕解析记录
	if history.SessionID != "" {
		db.Delete(&models.LiveMsg{}, "session_id = ?", history.SessionID)
	}

	// 先删除所有分P记录
	db.Delete(&models.RecordHistoryPart{}, "history_id = ?", id)
	// 再删除历史记录
	db.Delete(&models.RecordHistory{}, id)

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "删除成功"})
}

func RePublishHistory(c *gin.Context) {
	id := c.Param("id")
	historyID, _ := strconv.ParseUint(id, 10, 32)

	type RepublishReq struct {
		UserID uint `json:"userId"`
	}

	var req RepublishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "用户ID缺失"})
		return
	}

	if historyUploadService == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}

	if err := historyUploadService.PublishHistory(uint(historyID), req.UserID); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "发布成功"})
}

func UpdatePublishStatus(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "历史记录不存在"})
		return
	}

	history.Publish = false
	history.BvID = ""
	db.Save(&history)

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "更新成功"})
}

// BatchUpdateStatus 批量更新稿件状态
func BatchUpdateStatus(c *gin.Context) {
	type BatchUpdateReq struct {
		IDs    []uint `json:"ids" binding:"required"`
		Status string `json:"status" binding:"required"` // "publish", "unpublish", "upload", "cancel"
	}

	var req BatchUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IDs不能为空"})
		return
	}

	db := database.GetDB()

	switch req.Status {
	case "publish":
		// 批量标记为已发布
		db.Model(&models.RecordHistory{}).Where("id IN ?", req.IDs).Updates(map[string]interface{}{
			"publish": true,
		})
	case "unpublish":
		// 批量取消发布状态
		db.Model(&models.RecordHistory{}).Where("id IN ?", req.IDs).Updates(map[string]interface{}{
			"publish": false,
			"bv_id":   "",
		})
	case "upload":
		// 批量标记为待上传
		db.Model(&models.RecordHistory{}).Where("id IN ?", req.IDs).Updates(map[string]interface{}{
			"upload_status": 0,
		})
	case "cancel":
		// 批量取消上传状态
		db.Model(&models.RecordHistory{}).Where("id IN ?", req.IDs).Updates(map[string]interface{}{
			"upload_status": 0,
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的状态: " + req.Status})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":  "success",
		"msg":   "批量更新成功",
		"count": len(req.IDs),
	})
}

// CleanOldHistories 清理旧的历史记录
func CleanOldHistories(c *gin.Context) {
	type CleanReq struct {
		Days int `json:"days"` // 保留最近N天的记录，默认30天
	}

	var req CleanReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Days <= 0 {
		req.Days = 30 // 默认保留30天
	}

	db := database.GetDB()

	// 计算截止时间
	cutoffTime := time.Now().AddDate(0, 0, -req.Days)

	// 只删除未上传、未发布、不在上传中的旧记录
	// 原来条件未排除 upload_status>0 的记录，会删除上传中/已上传的记录并导致数据循环损坏
	// 先查询需要删除的历史记录 ID
	var toDelete []models.RecordHistory
	db.Select("id", "session_id").
		Where("end_time < ? AND publish = false AND upload_status = 0 AND recording = false", cutoffTime).
		Find(&toDelete)

	var deletedCount int64
	if len(toDelete) > 0 {
		deleteIDs := make([]uint, len(toDelete))
		sessionIDs := make([]string, 0, len(toDelete))
		for i, h := range toDelete {
			deleteIDs[i] = h.ID
			if h.SessionID != "" {
				sessionIDs = append(sessionIDs, h.SessionID)
			}
		}
		// 级联删除关联弹幕和分P
		if len(sessionIDs) > 0 {
			db.Delete(&models.LiveMsg{}, "session_id IN ?", sessionIDs)
		}
		db.Delete(&models.RecordHistoryPart{}, "history_id IN ?", deleteIDs)
		result := db.Delete(&models.RecordHistory{}, deleteIDs)
		deletedCount = result.RowsAffected
	}

	c.JSON(http.StatusOK, gin.H{
		"type":         "success",
		"msg":          "清理完成",
		"deletedCount": deletedCount,
	})
}

// BatchDelete 批量删除历史记录
func BatchDelete(c *gin.Context) {
	type BatchDeleteReq struct {
		IDs []uint `json:"ids" binding:"required"`
	}

	var req BatchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IDs不能为空"})
		return
	}

	db := database.GetDB()

	// 将 IDs 转为 uint64切片以支持 GORM WHERE IN 查询
	ids := make([]uint, len(req.IDs))
	copy(ids, req.IDs)

	// 级联删除：先获取 session_id 用于删除 LiveMsg
	var histories []models.RecordHistory
	db.Select("id", "session_id").Where("id IN ?", ids).Find(&histories)
	sessionIDs := make([]string, 0, len(histories))
	for _, h := range histories {
		if h.SessionID != "" {
			sessionIDs = append(sessionIDs, h.SessionID)
		}
	}

	// 删除关联弹幕
	if len(sessionIDs) > 0 {
		db.Delete(&models.LiveMsg{}, "session_id IN ?", sessionIDs)
	}
	// 删除关联分P
	db.Delete(&models.RecordHistoryPart{}, "history_id IN ?", ids)
	// 删除历史记录本身
	result := db.Delete(&models.RecordHistory{}, ids)

	c.JSON(http.StatusOK, gin.H{
		"type":  "success",
		"msg":   "批量删除成功",
		"count": result.RowsAffected,
	})
}

// UploadHistory 上传历史记录的所有分P
func UploadHistory(c *gin.Context) {
	id := c.Param("id")
	historyID, _ := strconv.ParseUint(id, 10, 32)

	type UploadReq struct {
		UserID uint `json:"userId"`
	}

	var req UploadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "用户ID缺失"})
		return
	}

	db := database.GetDB()

	// 获取历史记录
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "历史记录不存在"})
		return
	}

	// 获取房间信息
	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "房间不存在"})
		return
	}

	// 获取所有未上传且未正在上传中的分P
	// 加上 uploading=false 过滤，防止用户重复手动触发导致同一分P入队两次
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND upload = ? AND recording = ? AND uploading = ?", historyID, false, false, false).
		Order("start_time ASC").
		Find(&parts).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "查询分P失败"})
		return
	}

	if len(parts) == 0 {
		c.JSON(http.StatusOK, gin.H{"type": "warning", "msg": "没有待上传的分P"})
		return
	}

	if historyUploadService == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}

	log.Printf("开始上传历史记录 %d 的 %d 个分P", historyID, len(parts))

	// 将所有分P添加到上传队列
	var successCount int
	for i := range parts {
		log.Printf("添加分P到上传队列: part_id=%d, file=%s", parts[i].ID, parts[i].FileName)
		if err := historyUploadService.UploadPart(&parts[i], &history, &room); err != nil {
			log.Printf("添加分P到队列失败: part_id=%d, error=%v", parts[i].ID, err)
			continue
		}
		successCount++
	}

	if successCount == 0 {
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  "所有分P添加到队列失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":  "success",
		"msg":   fmt.Sprintf("已将%d个分P添加到上传队列", successCount),
		"count": successCount,
	})
}

// BatchUploadHistory 批量上传历史记录
func BatchUploadHistory(c *gin.Context) {
	type BatchUploadReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
		UserID     uint   `json:"userId" binding:"required"`
	}

	var req BatchUploadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	if historyUploadService == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}

	db := database.GetDB()
	totalParts := 0
	successHistories := 0

	// 一次性加载全部历史记录，避免 N+1
	var histories []models.RecordHistory
	if err := db.Where("id IN ?", req.HistoryIDs).Find(&histories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"type": "error", "msg": "查询历史记录失败"})
		return
	}

	// 收集所有 room_id，一次性查询房间信息
	roomIDSet := make(map[string]struct{})
	for _, h := range histories {
		if h.RoomID != "" {
			roomIDSet[h.RoomID] = struct{}{}
		}
	}
	roomIDs := make([]string, 0, len(roomIDSet))
	for rid := range roomIDSet {
		roomIDs = append(roomIDs, rid)
	}
	var rooms []models.RecordRoom
	if len(roomIDs) > 0 {
		db.Where("room_id IN ?", roomIDs).Find(&rooms)
	}
	roomMap := make(map[string]*models.RecordRoom, len(rooms))
	for i := range rooms {
		roomMap[rooms[i].RoomID] = &rooms[i]
	}

	// 逐条处获取分P并入队（分P查询本身已有索引，无法简单批量合并入队逻辑）
	for i := range histories {
		history := &histories[i]
		room, ok := roomMap[history.RoomID]
		if !ok {
			log.Printf("[批量上传] 房间不存在 history_id=%d, room_id=%s", history.ID, history.RoomID)
			continue
		}

		// 获取所有未上传且未正在上传中的分P
		// 加上 uploading=false 过滤，防止批量操作导致重复入队
		var parts []models.RecordHistoryPart
		if err := db.Where("history_id = ? AND upload = ? AND recording = ? AND uploading = ?", history.ID, false, false, false).
			Order("start_time ASC").
			Find(&parts).Error; err != nil {
			log.Printf("[批量上传] 查询分P失败 history_id=%d", history.ID)
			continue
		}

		if len(parts) == 0 {
			continue
		}

		// 添加所有分P到上传队列
		for j := range parts {
			if err := historyUploadService.UploadPart(&parts[j], history, room); err != nil {
				log.Printf("[批量上传] 添加分P失败 part_id=%d: %v", parts[j].ID, err)
				continue
			}
			totalParts++
		}
		successHistories++
	}

	log.Printf("[批量上传] 已添加 %d 个历史记录共 %d 个分P到队列",
		successHistories, totalParts)

	c.JSON(http.StatusOK, gin.H{
		"type":      "success",
		"msg":       fmt.Sprintf("已将%d个历史记录共%d个分P添加到上传队列", successHistories, totalParts),
		"histories": successHistories,
		"parts":     totalParts,
		"total":     len(req.HistoryIDs),
	})
}

// BatchPublishHistory 批量投稿历史记录
func BatchPublishHistory(c *gin.Context) {
	type BatchPublishReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
		UserID     uint   `json:"userId" binding:"required"`
	}

	var req BatchPublishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	if historyUploadService == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "上传服务未初始化"})
		return
	}

	successCount := 0
	failedCount := 0

	for _, historyID := range req.HistoryIDs {
		if err := historyUploadService.PublishHistory(historyID, req.UserID); err != nil {
			log.Printf("[批量投稿] 投稿失败 history_id=%d: %v", historyID, err)
			failedCount++
			continue
		}
		successCount++
	}

	log.Printf("[批量投稿] 完成 %d/%d (失败 %d)",
		successCount, len(req.HistoryIDs), failedCount)

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"msg":     fmt.Sprintf("投稿完成：成功%d个，失败%d个", successCount, failedCount),
		"success": successCount,
		"failed":  failedCount,
		"total":   len(req.HistoryIDs),
	})
}

// BatchResetStatus 批量重置状态
func BatchResetStatus(c *gin.Context) {
	type BatchResetReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
		Upload     bool   `json:"upload"`
		Publish    bool   `json:"publish"`
		Danmaku    bool   `json:"danmaku"`
		Files      bool   `json:"files"`
	}

	var req BatchResetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	db := database.GetDB()

	// 一次性加载全部历史记录，避免 N+1
	var histories []models.RecordHistory
	if err := db.Where("id IN ?", req.HistoryIDs).Find(&histories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"type": "error", "msg": "查询历史记录失败"})
		return
	}

	historyIDs := make([]uint, len(histories))
	for i, h := range histories {
		historyIDs[i] = h.ID
	}

	// 批量更新分P状态（仅当需要时执行一次）
	if req.Upload && len(historyIDs) > 0 {
		db.Model(&models.RecordHistoryPart{}).
			Where("history_id IN ?", historyIDs).
			Updates(map[string]interface{}{
				"upload":           false,
				"uploading":        false,
				"upload_error_msg": "",
			})
	}

	// 构建批量适用的历史记录更新字段
	historyUpdates := make(map[string]interface{})
	if req.Upload {
		historyUpdates["upload_status"] = 0
	}
	if req.Publish {
		historyUpdates["publish"] = false
		historyUpdates["bv_id"] = ""
		historyUpdates["video_state"] = -1
		historyUpdates["video_state_desc"] = ""
	}
	if req.Danmaku {
		historyUpdates["danmaku_sent"] = false
		historyUpdates["danmaku_count"] = 0
	}

	successCount := 0
	if len(historyUpdates) > 0 && len(historyIDs) > 0 {
		// 批量更新公共字段
		db.Model(&models.RecordHistory{}).Where("id IN ?", historyIDs).Updates(historyUpdates)
		successCount = len(historyIDs)
	}

	// Files 清空需要逐条检查 FilePath 是否非空，收集后批量更新
	if req.Files {
		var fileIDs []uint
		for _, h := range histories {
			if h.FilePath != "" {
				fileIDs = append(fileIDs, h.ID)
			}
		}
		if len(fileIDs) > 0 {
			db.Model(&models.RecordHistory{}).Where("id IN ?", fileIDs).Update("file_path", "")
			if len(historyUpdates) == 0 {
				successCount = len(fileIDs)
			}
		}
	}

	log.Printf("[批量重置] 重置完成 %d/%d", successCount, len(req.HistoryIDs))

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"msg":     fmt.Sprintf("重置完成：成功%d个", successCount),
		"success": successCount,
		"total":   len(req.HistoryIDs),
	})
}

// BatchDeleteWithFiles 批量删除记录和文件
func BatchDeleteWithFiles(c *gin.Context) {
	type BatchDeleteReq struct {
		HistoryIDs []uint `json:"historyIds" binding:"required"`
	}

	var req BatchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	db := database.GetDB()
	successCount := 0

	for _, historyID := range req.HistoryIDs {
		var history models.RecordHistory
		if err := db.First(&history, historyID).Error; err != nil {
			continue
		}

		// 删除文件（包括相关文件）
		moverService := services.NewFileMoverService()
		if history.FilePath != "" {
			if _, err := os.Stat(history.FilePath); err == nil {
				if err := os.Remove(history.FilePath); err != nil {
					log.Printf("[批量删除] 删除文件失败: %s, error: %v", history.FilePath, err)
				} else {
					log.Printf("[批量删除] 已删除主文件: %s", history.FilePath)
				}
				// 删除相关文件
				moverService.DeleteRelatedFiles(history.FilePath)
			}
		}

		// 获取所有分P并删除文件
		var parts []models.RecordHistoryPart
		db.Where("history_id = ?", historyID).Find(&parts)
		for _, part := range parts {
			// 统一使用 FilePath 字段（与单个删除保持一致）
			if part.FilePath != "" {
				if _, err := os.Stat(part.FilePath); err == nil {
					if err := os.Remove(part.FilePath); err != nil {
						log.Printf("[批量删除] 删除分P文件失败: %s, error: %v", part.FilePath, err)
					} else {
						log.Printf("[批量删除] 已删除分P主文件: %s", part.FilePath)
					}
					// 删除相关文件（xml弹幕、jpg/jpeg封面、ass/srt字幕等）
					moverService.DeleteRelatedFiles(part.FilePath)
				}
			}
		}

		// 删除数据库记录
		db.Delete(&models.RecordHistoryPart{}, "history_id = ?", historyID)
		db.Delete(&history)
		successCount++
	}

	log.Printf("[批量重置] 重置完成 %d/%d", successCount, len(req.HistoryIDs))

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"msg":     fmt.Sprintf("删除完成：成功%d个", successCount),
		"success": successCount,
		"total":   len(req.HistoryIDs),
	})
}

// ManualSetPublishInfo 手动设置投稿信息
// 用于在系统无法自动判定投稿状态时，手动填写投稿信息标记为已投稿
func ManualSetPublishInfo(c *gin.Context) {
	id := c.Param("id")
	historyID, _ := strconv.ParseUint(id, 10, 32)

	type ManualPublishReq struct {
		AvID  string `json:"avId"`                    // AV号（可选，会从BVID解析）
		BvID  string `json:"bvId" binding:"required"` // BV号（必填）
		Force bool   `json:"force"`                   // 是否强制覆盖（即使已有投稿信息）
	}

	var req ManualPublishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "请提供有效的BV号"})
		return
	}

	// 验证BV号格式
	if len(req.BvID) != 12 || req.BvID[:2] != "BV" {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "BV号格式错误，应为12位且以BV开头"})
		return
	}

	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "历史记录不存在"})
		return
	}

	// 如果已经有投稿信息且不强制覆盖，提示用户
	if history.Publish && history.BvID != "" && !req.Force {
		c.JSON(http.StatusOK, gin.H{
			"type": "warning",
			"msg":  fmt.Sprintf("该历史记录已有投稿信息 (BV: %s)，如需覆盖请勾选强制覆盖", history.BvID),
		})
		return
	}

	// 如果提供了AV号，使用提供的；否则尝试从BVID转换
	avID := req.AvID
	if avID == "" {
		// 将BV号转换为AV号
		aid := Bv2Av(req.BvID)
		if aid > 0 {
			avID = fmt.Sprintf("%d", aid)
		}
	}

	// 更新历史记录
	updates := map[string]interface{}{
		"bv_id":   req.BvID,
		"publish": true,
		"message": "手动设置投稿信息",
	}

	if avID != "" {
		updates["av_id"] = avID
	}

	if err := db.Model(&history).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "更新失败: " + err.Error()})
		return
	}

	log.Printf("手动设置投稿信息 history_id=%d, bv_id=%s, av_id=%s", historyID, req.BvID, avID)

	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"msg":  fmt.Sprintf("已成功标记为投稿 (BV: %s)", req.BvID),
		"data": gin.H{
			"bvId": req.BvID,
			"avId": avID,
		},
	})
}

// Bv2Av 将BV号转换为AV号
// 算法参考: https://github.com/SocialSisterYi/bilibili-API-collect
func Bv2Av(bv string) int64 {
	const (
		xorCode  = int64(23442827791579)
		maskCode = int64(2251799813685247)
		maxAid   = int64(1) << 51
		base     = 58
		alphabet = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	)

	// 验证BV号格式
	if len(bv) != 12 || !strings.HasPrefix(bv, "BV") {
		return 0
	}

	// 创建字母表索引映射
	charMap := make(map[byte]int64)
	for i, c := range alphabet {
		charMap[byte(c)] = int64(i)
	}

	// 转换为字节数组并交换回原位置
	bytes := []byte(bv)
	bytes[3], bytes[9] = bytes[9], bytes[3]
	bytes[4], bytes[7] = bytes[7], bytes[4]

	// 从58进制转回10进制
	var tmp int64 = 0
	for i := 2; i < len(bytes); i++ {
		tmp = tmp*base + charMap[bytes[i]]
	}

	// 异或运算并移除掩码
	return (tmp ^ xorCode) & maskCode
}
