package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

type ExportConfigParams struct {
	ExportRoom    bool `json:"rooms"`
	ExportUser    bool `json:"users"`
	ExportHistory bool `json:"histories"`
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
		configData["userList"] = users
	}

	// 导出历史记录
	if params.ExportHistory {
		var histories []models.RecordHistory
		db.Limit(1000).Order("start_time DESC").Find(&histories)

		// 统计每个历史记录的分P信息
		for i := range histories {
			var partCount int64
			db.Model(&models.RecordHistoryPart{}).Where("history_id = ?", histories[i].ID).Count(&partCount)
			histories[i].PartCount = int(partCount)
		}

		configData["historyList"] = histories

		// 导出对应的分P数据
		var parts []models.RecordHistoryPart
		historyIDs := make([]uint, len(histories))
		for i, h := range histories {
			historyIDs[i] = h.ID
		}
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
