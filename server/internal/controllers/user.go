package controllers

import (
	"encoding/base64"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/skip2/go-qrcode"
)

// LoginSession 登录会话
type LoginSession struct {
	AuthCode   string
	QRCodeURL  string
	CreateTime int64
	Status     string // pending, success, failed, expired
	Message    string
}

var loginSessions = make(map[string]*LoginSession)

const sessionExpireTime = 5 * 60 // 5分钟过期

// ListBiliUsers 获取B站用户列表（不包括管理员）
func ListBiliUsers(c *gin.Context) {
	db := database.GetDB()
	var users []models.BiliBiliUser
	db.Select("id", "created_at", "updated_at", "uid", "uname", "face", "login", "level", "vip_type", "vip_status", "login_time", "expire_time", "wx_push_token").
		Order("created_at DESC").
		Find(&users)

	c.JSON(http.StatusOK, users)
}

// LoginUser 生成B站登录二维码
func LoginUser(c *gin.Context) {
	// 生成TV端二维码（参考biliupforjava实现）
	qrResp, err := bili.GenerateTVQRCode()
	if err != nil {
		log.Printf("生成二维码失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"error": "生成二维码失败: " + err.Error()})
		return
	}

	// 生成二维码图片
	qrCode, err := qrcode.New(qrResp.Data.URL, qrcode.Medium)
	if err != nil {
		log.Printf("生成二维码图片失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"error": "生成二维码图片失败"})
		return
	}

	qrCode.DisableBorder = true
	pngBytes, err := qrCode.PNG(256)
	if err != nil {
		log.Printf("生成PNG失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"error": "生成PNG失败"})
		return
	}

	// Base64编码
	imageBase64 := base64.StdEncoding.EncodeToString(pngBytes)

	// 使用图片的最后100个字符作为session key
	sessionKey := imageBase64[len(imageBase64)-100:]

	// 创建登录会话
	session := &LoginSession{
		AuthCode:   qrResp.Data.AuthCode,
		QRCodeURL:  qrResp.Data.URL,
		CreateTime: time.Now().Unix(),
		Status:     "pending",
		Message:    "等待扫码",
	}
	loginSessions[sessionKey] = session

	c.JSON(http.StatusOK, gin.H{
		"image": imageBase64,
		"key":   sessionKey,
	})
}

// LoginCheck 检查登录状态（轮询）
func LoginCheck(c *gin.Context) {
	sessionKey := c.Query("key")
	if sessionKey == "" {
		c.JSON(http.StatusOK, gin.H{
			"status":  "failed",
			"message": "缺少key参数",
		})
		return
	}

	session, exists := loginSessions[sessionKey]
	if !exists {
		c.JSON(http.StatusOK, gin.H{
			"status":  "failed",
			"message": "会话不存在或已过期",
		})
		return
	}

	// 检查会话是否过期
	if time.Now().Unix()-session.CreateTime > sessionExpireTime {
		delete(loginSessions, sessionKey)
		c.JSON(http.StatusOK, gin.H{
			"status":  "expired",
			"message": "二维码已过期，请刷新",
		})
		return
	}

	// 如果已有状态，直接返回
	if session.Status != "pending" {
		if session.Status == "success" {
			delete(loginSessions, sessionKey)
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  session.Status,
			"message": session.Message,
		})
		return
	}

	// 轮询登录状态
	pollResp, err := bili.PollQRCodeStatus(session.AuthCode)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "pending",
			"message": "检查中...",
		})
		return
	}

	switch pollResp.Data.Code {
	case 0: // 登录成功
		// 解析Cookie
		cookieStr := bili.ExtractCookiesFromPollResponse(pollResp)
		if cookieStr == "" {
			session.Status = "failed"
			session.Message = "获取Cookie失败"
			c.JSON(http.StatusOK, gin.H{
				"status":  "failed",
				"message": "获取Cookie失败",
			})
			return
		}

		// 获取用户信息
		userInfo, err := bili.GetUserInfo(cookieStr)
		if err != nil {
			session.Status = "failed"
			session.Message = "获取用户信息失败"
			c.JSON(http.StatusOK, gin.H{
				"status":  "failed",
				"message": "获取用户信息失败: " + err.Error(),
			})
			return
		}

		// 保存用户到数据库
		db := database.GetDB()
		var user models.BiliBiliUser

		now := time.Now()
		expireTime := now.Add(30 * 24 * time.Hour)

		result := db.Where("uid = ?", userInfo.Data.Mid).First(&user)
		if result.Error != nil {
			// 新用户
			user = models.BiliBiliUser{
				UID:          userInfo.Data.Mid,
				Uname:        userInfo.Data.Uname,
				Face:         userInfo.Data.Face,
				Cookies:      cookieStr,
				RefreshToken: pollResp.Data.RefreshToken,
				Login:        true,
				Level:        userInfo.Data.Level,
				VipType:      userInfo.Data.VipType,
				VipStatus:    userInfo.Data.VipStatus,
				LoginTime:    &now,
				ExpireTime:   &expireTime,
			}
		} else {
			// 更新现有用户
			user.Uname = userInfo.Data.Uname
			user.Face = userInfo.Data.Face
			user.Cookies = cookieStr
			user.RefreshToken = pollResp.Data.RefreshToken
			user.Login = true
			user.Level = userInfo.Data.Level
			user.VipType = userInfo.Data.VipType
			user.VipStatus = userInfo.Data.VipStatus
			user.LoginTime = &now
			user.ExpireTime = &expireTime
		}

		if err := db.Save(&user).Error; err != nil {
			log.Printf("保存用户失败: %v", err)
			c.JSON(http.StatusOK, gin.H{
				"status":  "failed",
				"message": "保存用户失败",
			})
			return
		}

		log.Printf("[INFO] B站用户登录成功: UID=%d, Uname=%s", user.UID, user.Uname)

		session.Status = "success"
		session.Message = "登录成功"
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "登录成功",
		})

	case 86038: // 二维码已失效
		session.Status = "expired"
		session.Message = "二维码已过期"
		c.JSON(http.StatusOK, gin.H{
			"status":  "expired",
			"message": "二维码已过期，请刷新",
		})

	case 86090: // 已扫码未确认
		c.JSON(http.StatusOK, gin.H{
			"status":  "scanned",
			"message": "已扫码，请在手机上确认",
		})

	case 86101: // 未扫码
		c.JSON(http.StatusOK, gin.H{
			"status":  "pending",
			"message": "等待扫码",
		})

	default:
		c.JSON(http.StatusOK, gin.H{
			"status":  "pending",
			"message": "等待扫码",
		})
	}
}

// LoginCancel 取消登录
func LoginCancel(c *gin.Context) {
	sessionKey := c.Query("key")
	if sessionKey != "" {
		delete(loginSessions, sessionKey)
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "cancelled",
		"message": "已取消",
	})
}

// UpdateBiliUser 更新B站用户信息
func UpdateBiliUser(c *gin.Context) {
	var user models.BiliBiliUser
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()

	// 只更新允许更新的字段
	if err := db.Model(&user).Updates(map[string]interface{}{
		"uname":         user.Uname,
		"face":          user.Face,
		"level":         user.Level,
		"wx_push_token": user.WxPushToken,
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "更新成功"})
}

// DeleteBiliUser 删除B站用户
func DeleteBiliUser(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	// 软删除
	if err := db.Delete(&models.BiliBiliUser{}, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "删除成功"})
}

// RefreshUserCookie 刷新用户Cookie
func RefreshUserCookie(c *gin.Context) {
	id := c.Param("id")

	db := database.GetDB()
	var user models.BiliBiliUser

	if err := db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "用户不存在"})
		return
	}

	// 验证Cookie是否有效
	valid, err := bili.ValidateCookie(user.Cookies)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "验证失败: " + err.Error()})
		return
	}

	if !valid {
		user.Login = false
		db.Save(&user)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Cookie已失效，请重新登录"})
		return
	}

	// 获取最新用户信息
	userInfo, err := bili.GetUserInfo(user.Cookies)
	if err == nil {
		user.Uname = userInfo.Data.Uname
		user.Face = userInfo.Data.Face
		user.Level = userInfo.Data.Level
		user.Login = true
		db.Save(&user)
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Cookie有效", "user": user})
}
