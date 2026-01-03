package controllers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/upload"
)

var historyUploadService *upload.Service

// SetHistoryUploadService 设置上传服务
func SetHistoryUploadService(svc *upload.Service) {
	historyUploadService = svc
}

func ListHistories(c *gin.Context) {
	db := database.GetDB()

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	roomId := c.Query("roomId")
	bvId := c.Query("bvId")

	// 构建查询
	query := db.Model(&models.RecordHistory{})

	// 添加搜索条件
	if roomId != "" {
		query = query.Where("room_id = ?", roomId)
	}
	if bvId != "" {
		query = query.Where("bv_id = ?", bvId)
	}

	// 获取总数
	var total int64
	query.Count(&total)

	// 分页查询
	var histories []models.RecordHistory
	offset := (page - 1) * pageSize
	query.Order("end_time DESC").Limit(pageSize).Offset(offset).Find(&histories)

	// 统计每个历史记录的分P信息
	for i := range histories {
		var partCount int64
		var uploadPartCount int64
		var recordPartCount int64
		var uploadingPartCount int64

		db.Model(&models.RecordHistoryPart{}).Where("history_id = ?", histories[i].ID).Count(&partCount)
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND upload = ?", histories[i].ID, true).Count(&uploadPartCount)
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND recording = ?", histories[i].ID, true).Count(&recordPartCount)
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND uploading = ?", histories[i].ID, true).Count(&uploadingPartCount)

		histories[i].PartCount = int(partCount)
		histories[i].UploadPartCount = int(uploadPartCount)
		histories[i].RecordPartCount = int(recordPartCount)

		// 计算上传状态
		if uploadingPartCount > 0 {
			histories[i].UploadStatus = 1 // 上传中
		} else if uploadPartCount > 0 && uploadPartCount == partCount {
			histories[i].UploadStatus = 2 // 全部已上传
		} else if uploadPartCount > 0 {
			histories[i].UploadStatus = 2 // 部分已上传，也标记为已上传
		} else {
			histories[i].UploadStatus = 0 // 未上传
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"list":  histories,
		"total": total,
	})
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

	// 只删除未上传、未发布的旧记录
	result := db.Where("end_time < ? AND publish = false", cutoffTime).
		Delete(&models.RecordHistory{})

	c.JSON(http.StatusOK, gin.H{
		"type":         "success",
		"msg":          "清理完成",
		"deletedCount": result.RowsAffected,
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
	result := db.Delete(&models.RecordHistory{}, req.IDs)

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

	// 获取所有未上传的分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND upload = ? AND recording = ?", historyID, false, false).
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

	for _, historyID := range req.HistoryIDs {
		// 获取历史记录
		var history models.RecordHistory
		if err := db.First(&history, historyID).Error; err != nil {
			log.Printf("[批量上传] ⚠️  历史记录不存在 history_id=%d", historyID)
			continue
		}

		// 获取房间信息
		var room models.RecordRoom
		if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
			log.Printf("[批量上传] ⚠️  房间不存在 history_id=%d, room_id=%s", historyID, history.RoomID)
			continue
		}

		// 获取所有未上传的分P
		var parts []models.RecordHistoryPart
		if err := db.Where("history_id = ? AND upload = ? AND recording = ?", historyID, false, false).
			Order("start_time ASC").
			Find(&parts).Error; err != nil {
			log.Printf("[批量上传] ⚠️  查询分P失败 history_id=%d", historyID)
			continue
		}

		if len(parts) == 0 {
			continue
		}

		// 添加所有分P到上传队列
		for i := range parts {
			if err := historyUploadService.UploadPart(&parts[i], &history, &room); err != nil {
				log.Printf("[批量上传] ⚠️  添加分P失败 part_id=%d: %v", parts[i].ID, err)
				continue
			}
			totalParts++
		}
		successHistories++
	}

	log.Printf("[批量上传] ✅ 已添加 %d 个历史记录共 %d 个分P到队列",
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
			log.Printf("[批量投稿] ⚠️  投稿失败 history_id=%d: %v", historyID, err)
			failedCount++
			continue
		}
		successCount++
	}

	log.Printf("[批量投稿] ✅ 完成 %d/%d (失败 %d)",
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
	successCount := 0

	for _, historyID := range req.HistoryIDs {
		var history models.RecordHistory
		if err := db.First(&history, historyID).Error; err != nil {
			log.Printf("[批量重置] ⚠️  历史记录不存在 history_id=%d", historyID)
			continue
		}

		updates := make(map[string]interface{})

		if req.Upload {
			updates["upload_status"] = 0
			// 重置分P的上传状态
			db.Model(&models.RecordHistoryPart{}).
				Where("history_id = ?", historyID).
				Updates(map[string]interface{}{"upload": false})
		}

		if req.Publish {
			updates["publish"] = false
			updates["bv_id"] = ""
			updates["video_state"] = -1
			updates["video_state_desc"] = ""
		}

		if req.Danmaku {
			updates["danmaku_sent"] = false
			updates["danmaku_count"] = 0
		}

		if req.Files && history.FilePath != "" {
			// 删除文件
			filePath := history.FilePath
			if _, err := os.Stat(filePath); err == nil {
				os.Remove(filePath)
			}
			updates["file_path"] = ""
		}

		if len(updates) > 0 {
			db.Model(&history).Updates(updates)
			successCount++
		}
	}

	log.Printf("[批量重置] ✅ 重置完成 %d/%d", successCount, len(req.HistoryIDs))

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

		// 删除文件
		if history.FilePath != "" {
			if _, err := os.Stat(history.FilePath); err == nil {
				os.Remove(history.FilePath)
				log.Printf("[批量删除] 删除文件: %s", history.FilePath)
			}
		}

		// 获取所有分P并删除文件
		var parts []models.RecordHistoryPart
		db.Where("history_id = ?", historyID).Find(&parts)
		for _, part := range parts {
			if part.FileName != "" {
				if _, err := os.Stat(part.FileName); err == nil {
					os.Remove(part.FileName)
				}
			}
		}

		// 删除数据库记录
		db.Delete(&models.RecordHistoryPart{}, "history_id = ?", historyID)
		db.Delete(&history)
		successCount++
	}

	log.Printf("[批量删除] ✅ 删除完成 %d/%d", successCount, len(req.HistoryIDs))

	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"msg":     fmt.Sprintf("删除完成：成功%d个", successCount),
		"success": successCount,
		"total":   len(req.HistoryIDs),
	})
}
