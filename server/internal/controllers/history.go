package controllers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/upload"
)

func ListHistories(c *gin.Context) {
	db := database.GetDB()
	var histories []models.RecordHistory
	db.Order("end_time DESC").Limit(100).Find(&histories)
	c.JSON(http.StatusOK, gin.H{
		"list":  histories,
		"total": len(histories),
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

	uploadService := upload.NewService()
	if err := uploadService.PublishHistory(uint(historyID), req.UserID); err != nil {
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
	result := db.Where("end_time < ? AND upload_status = 0 AND publish = false", cutoffTime).
		Delete(&models.RecordHistory{})

	c.JSON(http.StatusOK, gin.H{
		"type":         "success",
		"msg":          "清理完成",
		"deletedCount": result.RowsAffected,
		"days":         req.Days,
	})
}

// BatchDelete 批量删除记录
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
