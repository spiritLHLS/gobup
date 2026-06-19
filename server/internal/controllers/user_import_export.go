package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm/clause"
)

type userExportRequest struct {
	IDs            []uint `json:"ids"`
	IncludeSecrets bool   `json:"includeSecrets"`
}

type userImportPayload struct {
	UserList []models.BiliBiliUser `json:"userList"`
}

func ExportBiliUsers(c *gin.Context) {
	var req userExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	db := database.GetDB()
	query := db.Where("uid != ?", -1).Order("created_at DESC")
	if len(req.IDs) > 0 {
		query = query.Where("id IN ?", req.IDs)
	}

	var users []models.BiliBiliUser
	if err := query.Find(&users).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "查询用户失败"})
		return
	}

	payload := gin.H{
		"exportedAt":      time.Now().Format(time.RFC3339),
		"includeSecrets":  req.IncludeSecrets,
		"userListVersion": 1,
	}
	if req.IncludeSecrets {
		payload["userList"] = users
	} else {
		payload["userList"] = safeBiliUserResponses(users)
		payload["userListRedacted"] = true
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=gobup_users_%d.json", time.Now().Unix()))
	c.JSON(http.StatusOK, payload)
}

func ImportBiliUsers(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "上传文件失败"})
		return
	}
	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "打开文件失败"})
		return
	}
	defer reader.Close()

	var raw json.RawMessage
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "解析用户文件失败"})
		return
	}

	var payload userImportPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.UserList == nil {
		if err := json.Unmarshal(raw, &payload.UserList); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "用户文件格式不正确"})
			return
		}
	}

	db := database.GetDB()
	imported := 0
	skipped := 0
	for _, user := range payload.UserList {
		if user.UID <= 0 {
			skipped++
			continue
		}
		user.ID = 0
		user.DeletedAt.Valid = false
		if strings.TrimSpace(user.Cookies) == "" {
			user.Login = false
			user.AccessKey = ""
			user.RefreshToken = ""
			user.CookieInfo = ""
		}
		if strings.TrimSpace(user.Uname) == "" {
			user.Uname = fmt.Sprintf("UID%d", user.UID)
		}

		updates := []string{
			"uname", "face", "cookies", "access_key", "refresh_token", "login", "enabled",
			"level", "vip_type", "vip_status", "moral", "cookie_info", "login_time",
			"expire_time", "last_check_time", "last_check_error", "wx_push_token",
			"daily_upload_quota", "updated_at", "deleted_at",
		}
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "uid"}},
			DoUpdates: clause.AssignmentColumns(updates),
		}).Create(&user).Error; err != nil {
			skipped++
			continue
		}
		imported++
	}

	c.JSON(http.StatusOK, gin.H{
		"type":     "success",
		"msg":      fmt.Sprintf("导入完成：成功%d个，跳过%d个", imported, skipped),
		"imported": imported,
		"skipped":  skipped,
	})
}
