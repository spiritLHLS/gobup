package controllers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/services"
)

// TriggerFileScan 触发文件扫描（支持强制扫描）
func TriggerFileScan(c *gin.Context) {
	// 从查询参数获取是否强制扫描
	force := c.DefaultQuery("force", "false") == "true"

	log.Printf("[FileScan] 收到手动扫描请求 (force=%v)", force)

	// 加载配置
	config := services.LoadConfigFromDB()

	// 如果是强制扫描，设置ForceImport标志
	if force {
		config.ForceImport = true
		log.Printf("[FileScan] 启用强制扫描模式，将无视文件年龄限制")
	}

	// 执行扫描
	scanService := services.NewFileScanService()
	result, err := scanService.ScanAndImport(config)

	if err != nil {
		log.Printf("[FileScan] 扫描失败: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  "扫描失败: " + err.Error(),
		})
		return
	}

	// 返回扫描结果
	log.Printf("[FileScan] 扫描完成: 总文件=%d, 新导入=%d, 跳过=%d, 失败=%d",
		result.TotalFiles, result.NewFiles, result.SkippedFiles, result.FailedFiles)

	c.JSON(http.StatusOK, gin.H{
		"type":         "success",
		"msg":          "扫描完成",
		"totalFiles":   result.TotalFiles,
		"newFiles":     result.NewFiles,
		"skippedFiles": result.SkippedFiles,
		"failedFiles":  result.FailedFiles,
		"errors":       result.Errors,
	})
}

// PreviewFileScan 预览待扫描的文件（不实际导入）
func PreviewFileScan(c *gin.Context) {
	log.Printf("[FileScan] 收到预览扫描请求")

	// 加载配置
	config := services.LoadConfigFromDB()
	config.ForceImport = true // 预览时使用强制模式，显示所有文件

	// 执行预览扫描
	scanService := services.NewFileScanService()
	files, err := scanService.PreviewFiles(config)

	if err != nil {
		log.Printf("[FileScan] 预览失败: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  "预览失败: " + err.Error(),
		})
		return
	}

	// 过滤掉已在数据库中的文件
	var newFiles []*services.FilePreviewInfo
	for _, file := range files {
		if !file.InDatabase {
			newFiles = append(newFiles, file)
		}
	}

	log.Printf("[FileScan] 预览完成: 发现 %d 个新文件（总共 %d 个文件，已过滤 %d 个已入库文件）",
		len(newFiles), len(files), len(files)-len(newFiles))

	c.JSON(http.StatusOK, gin.H{
		"type":  "success",
		"msg":   "预览完成",
		"files": newFiles,
		"total": len(newFiles),
	})
}

// ImportSelectedFiles 导入选中的文件
func ImportSelectedFiles(c *gin.Context) {
	var req struct {
		FilePaths []string `json:"filePaths" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"msg":  "无效的请求参数",
		})
		return
	}

	log.Printf("[FileScan] 收到选择性导入请求，文件数: %d", len(req.FilePaths))

	// 执行导入
	scanService := services.NewFileScanService()
	result, err := scanService.ImportSelectedFiles(req.FilePaths)

	if err != nil {
		log.Printf("[FileScan] 导入失败: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  "导入失败: " + err.Error(),
		})
		return
	}

	// 返回导入结果
	log.Printf("[FileScan] 导入完成: 总文件=%d, 成功=%d, 失败=%d",
		result.TotalFiles, result.NewFiles, result.FailedFiles)

	c.JSON(http.StatusOK, gin.H{
		"type":        "success",
		"msg":         "导入完成",
		"totalFiles":  result.TotalFiles,
		"newFiles":    result.NewFiles,
		"failedFiles": result.FailedFiles,
		"errors":      result.Errors,
	})
}

// CleanCompletedFiles 清理已完成历史记录的xml和jpg文件
func CleanCompletedFiles(c *gin.Context) {
	log.Printf("[FileScan] 收到手动清理已完成文件请求")

	// 执行清理
	scanService := services.NewFileScanService()
	result, err := scanService.CleanCompletedFiles()

	if err != nil {
		log.Printf("[FileScan] 清理失败: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  "清理失败: " + err.Error(),
		})
		return
	}

	// 返回清理结果
	log.Printf("[FileScan] 清理完成: 检查历史记录=%d, 删除XML=%d, 删除JPG=%d, 跳过=%d",
		result.TotalHistories, result.DeletedXMLFiles, result.DeletedJPGFiles, result.SkippedHistories)

	c.JSON(http.StatusOK, gin.H{
		"type":             "success",
		"msg":              "清理完成",
		"totalHistories":   result.TotalHistories,
		"deletedXMLFiles":  result.DeletedXMLFiles,
		"deletedJPGFiles":  result.DeletedJPGFiles,
		"skippedHistories": result.SkippedHistories,
		"errors":           result.Errors,
	})
}
