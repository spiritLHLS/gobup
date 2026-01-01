package controllers

import (
	"fmt"
	"log"
	"net/http"
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

		db.Model(&models.RecordHistoryPart{}).Where("history_id = ?", histories[i].ID).Count(&partCount)
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND upload = ?", histories[i].ID, true).Count(&uploadPartCount)
		db.Model(&models.RecordHistoryPart{}).Where("history_id = ? AND recording = ?", histories[i].ID, true).Count(&recordPartCount)

		histories[i].PartCount = int(partCount)
		histories[i].UploadPartCount = int(uploadPartCount)
		histories[i].RecordPartCount = int(recordPartCount)
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
