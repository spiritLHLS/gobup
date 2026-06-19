package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/ratelimit"
)

type ExportConfigParams struct {
	ExportRoom     bool `json:"rooms"`
	ExportUser     bool `json:"users"`
	ExportHistory  bool `json:"histories"`
	IncludeSecrets bool `json:"includeSecrets"`
}

// ExportConfig 导出配置
func ExportConfig(c *gin.Context) {
	var params ExportConfigParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	db := database.GetDB()
	configData := make(map[string]interface{})

	// 导出房间配置
	if params.ExportRoom {
		var rooms []models.RecordRoom
		db.Find(&rooms)
		configData["roomList"] = rooms
	}

	// 导出用户配置
	if params.ExportUser {
		var users []models.BiliBiliUser
		db.Find(&users)
		if params.IncludeSecrets {
			configData["userList"] = users
		} else {
			configData["userList"] = safeBiliUserResponses(users)
			configData["userListRedacted"] = true
		}
	}

	// 导出历史记录
	if params.ExportHistory {
		var histories []models.RecordHistory
		db.Limit(1000).Order("start_time DESC").Find(&histories)

		historyIDs := make([]uint, len(histories))
		for i, h := range histories {
			historyIDs[i] = h.ID
		}
		if len(historyIDs) > 0 {
			type partCountRow struct {
				HistoryID uint
				Count     int64
			}
			var countRows []partCountRow
			db.Model(&models.RecordHistoryPart{}).
				Select("history_id, COUNT(*) AS count").
				Where("history_id IN ?", historyIDs).
				Group("history_id").
				Scan(&countRows)
			partCountByHistory := make(map[uint]int, len(countRows))
			for _, row := range countRows {
				partCountByHistory[row.HistoryID] = int(row.Count)
			}
			for i := range histories {
				histories[i].PartCount = partCountByHistory[histories[i].ID]
			}
		}

		configData["historyList"] = histories

		// 导出对应的分P数据
		var parts []models.RecordHistoryPart
		if len(historyIDs) > 0 {
			db.Where("history_id IN ?", historyIDs).Find(&parts)
			configData["partList"] = parts
		}
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=gobup_config_%s.json",
		fmt.Sprintf("%d", time.Now().Unix())))
	c.JSON(http.StatusOK, configData)
}

// ImportConfig 导入配置
func ImportConfig(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上传文件失败"})
		return
	}

	// 读取文件内容
	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "打开文件失败"})
		return
	}
	defer fileContent.Close()

	var configData map[string]json.RawMessage
	if err := json.NewDecoder(fileContent).Decode(&configData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析配置文件失败"})
		return
	}

	db := database.GetDB()

	// 导入用户配置
	if userListData, ok := configData["userList"]; ok {
		var userList []models.BiliBiliUser
		if err := json.Unmarshal(userListData, &userList); err == nil {
			userIDMap := make(map[uint]uint) // 旧ID -> 新ID
			for _, user := range userList {
				oldID := user.ID
				user.ID = 0 // 清空ID，让数据库自动生成
				if user.Cookies == "" {
					user.Login = false
					user.AccessKey = ""
					user.RefreshToken = ""
					user.CookieInfo = ""
				}

				// 检查是否已存在
				var existing models.BiliBiliUser
				result := db.Where("uid = ?", user.UID).First(&existing)
				if result.Error == nil {
					user.ID = existing.ID
				}

				db.Save(&user)
				userIDMap[oldID] = user.ID
			}
			// 保存ID映射供后续使用
			c.Set("userIDMap", userIDMap)
		}
	}

	// 导入房间配置
	if roomListData, ok := configData["roomList"]; ok {
		var roomList []models.RecordRoom
		if err := json.Unmarshal(roomListData, &roomList); err == nil {
			userIDMap, _ := c.Get("userIDMap")
			idMap, _ := userIDMap.(map[uint]uint)
			for _, room := range roomList {
				room.ID = 0

				// 映射用户ID
				if newUserID, ok := idMap[room.UploadUserID]; ok {
					room.UploadUserID = newUserID
				}

				// 检查是否已存在
				var existing models.RecordRoom
				result := db.Where("room_id = ?", room.RoomID).First(&existing)
				if result.Error == nil {
					room.ID = existing.ID
				}

				db.Save(&room)
			}
		}
	}

	// 导入历史记录
	if historyListData, ok := configData["historyList"]; ok {
		var historyList []models.RecordHistory
		if err := json.Unmarshal(historyListData, &historyList); err == nil {
			historyIDMap := make(map[uint]uint)
			for _, history := range historyList {
				oldID := history.ID
				history.ID = 0

				// 检查是否已存在
				var existing models.RecordHistory
				result := db.Where("session_id = ?", history.SessionID).First(&existing)
				if result.Error == nil {
					history.ID = existing.ID
				}

				db.Save(&history)
				historyIDMap[oldID] = history.ID
			}
			c.Set("historyIDMap", historyIDMap)
		}
	}

	// 导入分P数据
	if partListData, ok := configData["partList"]; ok {
		var partList []models.RecordHistoryPart
		if err := json.Unmarshal(partListData, &partList); err == nil {
			historyIDMap, _ := c.Get("historyIDMap")
			idMap, _ := historyIDMap.(map[uint]uint)
			for _, part := range partList {
				part.ID = 0

				// 映射历史记录ID
				if newHistoryID, ok := idMap[part.HistoryID]; ok {
					part.HistoryID = newHistoryID
				}

				// 检查是否已存在
				var existing models.RecordHistoryPart
				result := db.Where("file_path = ?", part.FilePath).First(&existing)
				if result.Error == nil {
					part.ID = existing.ID
				}

				db.Save(&part)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "导入成功"})
}

// GetSystemConfig 获取系统配置
func GetSystemConfig(c *gin.Context) {
	db := database.GetDB()

	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  "获取配置失败",
		})
		return
	}

	c.JSON(http.StatusOK, config)
}

// UpdateSystemConfig 更新系统配置
func UpdateSystemConfig(c *gin.Context) {
	var req models.SystemConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"type": "error",
			"msg":  "配置不存在",
		})
		return
	}

	// 更新配置
	config.AutoFileScan = req.AutoFileScan
	config.EnableFileWatcher = req.EnableFileWatcher
	config.FileScanInterval = req.FileScanInterval
	config.FileScanMinAge = req.FileScanMinAge
	config.FileScanMinSize = req.FileScanMinSize
	config.FileScanMaxAge = req.FileScanMaxAge
	config.WorkPath = req.WorkPath
	config.CustomScanPaths = req.CustomScanPaths
	config.EnableOrphanScan = req.EnableOrphanScan
	config.OrphanScanInterval = req.OrphanScanInterval
	config.EnableDanmakuProxy = req.EnableDanmakuProxy
	config.DanmakuProxyList = req.DanmakuProxyList
	config.AutoDataRepair = req.AutoDataRepair
	config.UploadSpeedLimitMBps = req.UploadSpeedLimitMBps
	config.UploadWhileRecording = req.UploadWhileRecording
	config.PublishWhileRecording = req.PublishWhileRecording
	config.PublishMode = strings.TrimSpace(req.PublishMode)
	config.PublishAgentEndpoint = strings.TrimSpace(req.PublishAgentEndpoint)
	config.PublishAgentToken = strings.TrimSpace(req.PublishAgentToken)
	config.PublishAgentTimeout = req.PublishAgentTimeout
	config.AgentPurpose = strings.TrimSpace(req.AgentPurpose)
	config.AgentInstallerSource = strings.TrimSpace(req.AgentInstallerSource)
	config.AgentControllerBaseURL = strings.TrimRight(strings.TrimSpace(req.AgentControllerBaseURL), "/")
	config.AgentGitHubRepo = strings.TrimSpace(req.AgentGitHubRepo)
	config.AgentCDNBaseURL = strings.TrimRight(strings.TrimSpace(req.AgentCDNBaseURL), "/")
	config.FileCheckMode = strings.TrimSpace(req.FileCheckMode)
	config.DanmakuBurnStyle = strings.TrimSpace(req.DanmakuBurnStyle)
	config.DanmakuFontSize = req.DanmakuFontSize
	config.DanmakuFontColor = strings.TrimSpace(req.DanmakuFontColor)
	config.DanmakuScrollArea = req.DanmakuScrollArea
	config.DanmakuDisplayArea = req.DanmakuDisplayArea

	// 参数验证
	normalizeSystemConfig(&config)

	if err := db.Save(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存失败"})
		return
	}
	ratelimit.SetGlobalRateLimit(config.UploadSpeedLimitMBps)

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "配置更新成功", "data": config})
}

// ToggleSystemConfig 切换单个配置项
func ToggleSystemConfig(c *gin.Context) {
	type ToggleRequest struct {
		Key   string `json:"key" binding:"required"`
		Value bool   `json:"value"`
	}

	var req ToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "配置不存在"})
		return
	}

	switch req.Key {
	case "autoFileScan":
		config.AutoFileScan = req.Value
	case "enableFileWatcher":
		config.EnableFileWatcher = req.Value
	case "enableOrphanScan":
		config.EnableOrphanScan = req.Value
	case "enableDanmakuProxy":
		config.EnableDanmakuProxy = req.Value
	case "autoDataRepair":
		config.AutoDataRepair = req.Value
	case "uploadWhileRecording":
		config.UploadWhileRecording = req.Value
	case "publishWhileRecording":
		config.PublishWhileRecording = req.Value
	default:
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "未知的配置项"})
		return
	}
	normalizeSystemConfig(&config)

	if err := db.Save(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "配置已更新", "data": config})
}

func normalizeSystemConfig(config *models.SystemConfig) {
	if config.FileScanInterval < 10 {
		config.FileScanInterval = 10
	}
	if config.FileScanMinAge < 1 {
		config.FileScanMinAge = 1
	}
	if config.UploadSpeedLimitMBps < 0 {
		config.UploadSpeedLimitMBps = 0
	}
	switch strings.ToLower(strings.TrimSpace(config.PublishMode)) {
	case "remote":
		config.PublishMode = "remote"
	default:
		config.PublishMode = "local"
	}
	if config.PublishAgentTimeout < 3 {
		config.PublishAgentTimeout = 3
	}
	if config.PublishAgentTimeout > 600 {
		config.PublishAgentTimeout = 600
	}
	config.AgentPurpose = models.NormalizeAgentPurpose(config.AgentPurpose)
	config.AgentInstallerSource = models.NormalizeAgentInstallerSource(config.AgentInstallerSource)
	if strings.TrimSpace(config.AgentGitHubRepo) == "" {
		config.AgentGitHubRepo = "spiritlhls/gobup"
	}
	config.FileCheckMode = models.NormalizeFileCheckMode(config.FileCheckMode)
	switch config.DanmakuBurnStyle {
	case "compact", "large":
	default:
		config.DanmakuBurnStyle = "default"
	}
	if config.DanmakuFontSize < 0 {
		config.DanmakuFontSize = 0
	}
	if config.DanmakuFontSize > 0 && config.DanmakuFontSize < 12 {
		config.DanmakuFontSize = 12
	}
	if config.DanmakuFontSize > 72 {
		config.DanmakuFontSize = 72
	}
	if config.DanmakuScrollArea <= 0 {
		config.DanmakuScrollArea = 0.75
	}
	if config.DanmakuScrollArea < 0.1 {
		config.DanmakuScrollArea = 0.1
	}
	if config.DanmakuScrollArea > 1 {
		config.DanmakuScrollArea = 1
	}
	if config.DanmakuDisplayArea <= 0 {
		config.DanmakuDisplayArea = 0.8
	}
	if config.DanmakuDisplayArea < 0.1 {
		config.DanmakuDisplayArea = 0.1
	}
	if config.DanmakuDisplayArea > 1 {
		config.DanmakuDisplayArea = 1
	}
}

// GetSystemStats 获取系统统计信息
func GetSystemStats(c *gin.Context) {
	db := database.GetDB()

	var stats struct {
		TotalRecordings int64 `json:"totalRecordings"` // 总录制数（历史记录总数）
		UploadedCount   int64 `json:"uploadedCount"`   // 已上传（upload_status=2）
		PendingCount    int64 `json:"pendingCount"`    // 待处理（未上传或上传中）
		FailedCount     int64 `json:"failedCount"`     // 失败（上传失败或发布失败）
	}

	// 总录制数：所有历史记录
	db.Model(&models.RecordHistory{}).Count(&stats.TotalRecordings)

	// 已上传：上传状态为2（已上传）
	db.Model(&models.RecordHistory{}).Where("upload_status = ?", 2).Count(&stats.UploadedCount)

	// 待处理：上传状态为0（未上传）或1（上传中）
	db.Model(&models.RecordHistory{}).Where("upload_status IN ?", []int{0, 1}).Count(&stats.PendingCount)

	// 失败：video_state = 2（审核未通过）或 message 包含"失败"字样的已上传记录
	db.Model(&models.RecordHistory{}).Where("video_state = ? OR (upload_status = ? AND message LIKE ?)", 2, 2, "%失败%").Count(&stats.FailedCount)

	c.JSON(http.StatusOK, stats)
}

// CleanupDatabase 数据库瘦身 - 删除已软删除的记录
func CleanupDatabase(c *gin.Context) {
	db := database.GetDB()

	// 统计待清理的记录数
	var deletedPartsCount int64
	var orphanHistoriesCount int64

	// 1. 统计已软删除的分P（file_delete=true），这些会被删除
	db.Model(&models.RecordHistoryPart{}).Where("file_delete = ?", true).Count(&deletedPartsCount)

	// 2. 统计删除软删除分P后会变成孤立的历史记录数
	// 即：删除 file_delete=true 的分P后，没有任何分P的历史记录
	// 分两种情况：
	//   (1) 当前已经没有任何分P的历史记录
	//   (2) 当前只有 file_delete=true 的分P，删除后会变成没有分P
	db.Raw(`
		SELECT COUNT(DISTINCT h.id) 
		FROM record_histories h
		WHERE NOT EXISTS (
			SELECT 1 FROM record_history_parts p 
			WHERE p.history_id = h.id AND p.file_delete = false
		)
	`).Scan(&orphanHistoriesCount)

	log.Printf("[数据库瘦身] 预览: 软删除分P=%d条, 孤立历史记录=%d条", deletedPartsCount, orphanHistoriesCount)

	// 如果是预览模式，只返回统计信息
	preview := c.Query("preview") == "true"
	if preview {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "预览成功",
			"data": gin.H{
				"deletedPartsCount":    deletedPartsCount,
				"orphanHistoriesCount": orphanHistoriesCount,
			},
		})
		return
	}

	// 执行清理
	tx := db.Begin()

	// 1. 删除所有软删除的分P记录
	result := tx.Where("file_delete = ?", true).Delete(&models.RecordHistoryPart{})
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "删除分P记录失败: " + result.Error.Error(),
		})
		return
	}
	actualDeletedParts := result.RowsAffected
	log.Printf("[数据库瘦身] 已删除 %d 条软删除的分P记录", actualDeletedParts)

	// 2. 删除删除分P后变成孤立的历史记录（没有任何分P的）
	var orphanIDs []uint
	tx.Raw(`
		SELECT h.id FROM record_histories h
		WHERE NOT EXISTS (
			SELECT 1 FROM record_history_parts p 
			WHERE p.history_id = h.id
		)
	`).Scan(&orphanIDs)

	actualDeletedHistories := 0
	if len(orphanIDs) > 0 {
		result = tx.Where("id IN ?", orphanIDs).Delete(&models.RecordHistory{})
		if result.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": -1,
				"msg":  "删除历史记录失败: " + result.Error.Error(),
			})
			return
		}
		actualDeletedHistories = int(result.RowsAffected)
		log.Printf("[数据库瘦身] 已删除 %d 条孤立的历史记录", actualDeletedHistories)
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": -1,
			"msg":  "事务提交失败: " + err.Error(),
		})
		return
	}

	log.Printf("[数据库瘦身] 清理完成: 分P记录=%d条, 历史记录=%d条", actualDeletedParts, actualDeletedHistories)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "数据库瘦身完成",
		"data": gin.H{
			"deletedPartsCount":    actualDeletedParts,
			"orphanHistoriesCount": actualDeletedHistories,
		},
	})
}
