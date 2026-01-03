package controllers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/services"
)

// CheckDataConsistency 检查数据一致性
func CheckDataConsistency(c *gin.Context) {
	// 从查询参数获取是否为预览模式
	dryRun := c.DefaultQuery("dryRun", "true") == "true"

	log.Printf("[DataRepair] 收到数据一致性检查请求 (dryRun=%v)", dryRun)

	repairService := services.NewDataRepairService()
	result, err := repairService.CheckAndRepairDataConsistency(dryRun)

	if err != nil {
		log.Printf("[DataRepair] 数据一致性检查失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"type": "error",
			"msg":  "数据一致性检查失败: " + err.Error(),
		})
		return
	}

	// 返回结果
	c.JSON(http.StatusOK, gin.H{
		"type":                  "success",
		"msg":                   "数据一致性检查完成",
		"dryRun":                dryRun,
		"orphanParts":           result.OrphanParts,
		"emptyHistories":        result.EmptyHistories,
		"createdHistories":      result.CreatedHistories,
		"deletedEmptyHistories": result.DeletedEmptyHistories,
		"updatedHistoryTimes":   result.UpdatedHistoryTimes,
		"reassignedParts":       result.ReassignedParts,
		"errors":                result.Errors,
	})
}

// RepairDataConsistency 修复数据一致性
func RepairDataConsistency(c *gin.Context) {
	log.Printf("[DataRepair] 收到数据一致性修复请求")

	repairService := services.NewDataRepairService()
	result, err := repairService.CheckAndRepairDataConsistency(false) // dryRun=false 执行实际修复

	if err != nil {
		log.Printf("[DataRepair] 数据一致性修复失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"type": "error",
			"msg":  "数据一致性修复失败: " + err.Error(),
		})
		return
	}

	// 返回结果
	c.JSON(http.StatusOK, gin.H{
		"type":                  "success",
		"msg":                   "数据一致性修复完成",
		"orphanParts":           result.OrphanParts,
		"emptyHistories":        result.EmptyHistories,
		"createdHistories":      result.CreatedHistories,
		"deletedEmptyHistories": result.DeletedEmptyHistories,
		"updatedHistoryTimes":   result.UpdatedHistoryTimes,
		"reassignedParts":       result.ReassignedParts,
		"errors":                result.Errors,
	})
}
