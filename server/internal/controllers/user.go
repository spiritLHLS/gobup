package controllers

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

// QRCodeCache 二维码缓存
var qrCodeCache = make(map[string]*bili.QRCodeResponse)

// ListBiliUsers 获取用户列表
func ListBiliUsers(c *gin.Context) {
	db := database.GetDB()
	var users []models.BiliBiliUser
	db.Select("id", "created_at", "updated_at", "uid", "uname", "face", "login", "level", "vip_type", "vip_status", "login_time", "expire_time").
		Order("created_at DESC").
		Find(&users)

	c.JSON(http.StatusOK, users)
}

// LoginUser 生成登录二维码
func LoginUser(c *gin.Context) {
	// 生成二维码
	qrResp, err := bili.GenerateTVQRCode()
	if err != nil {
		log.Printf("生成二维码失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "生成二维码失败: " + err.Error()})
		return
	}

	// 缓存二维码信息
	qrCodeCache[qrResp.Data.AuthCode] = qrResp

	c.JSON(http.StatusOK, gin.H{
		"type":      "success",
		"authCode":  qrResp.Data.AuthCode,
		"qrcodeUrl": qrResp.Data.URL,
	})
}

// LoginReturn 轮询登录状态
func LoginReturn(c *gin.Context) {
	authCode := c.Query("authCode")
	if authCode == "" {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "缺少authCode"})
		return
	}

	// 轮询登录状态
	pollResp, err := bili.PollQRCodeStatus(authCode)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "轮询失败: " + err.Error()})
		return
	}

	// 处理不同的状态码
	switch pollResp.Data.Code {
	case 0: // 登录成功
		// 从响应中提取Cookie
		cookieStr := extractCookiesFromResponse(pollResp)
		if cookieStr == "" {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "获取Cookie失败"})
			return
		}

		// 获取用户信息
		userInfo, err := bili.GetUserInfo(cookieStr)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "获取用户信息失败: " + err.Error()})
			return
		}

		// 保存用户
		db := database.GetDB()
		var user models.BiliBiliUser

		now := time.Now()
		expireTime := now.Add(30 * 24 * time.Hour) // 30天过期

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
			c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存用户失败"})
			return
		}

		// 清除缓存
		delete(qrCodeCache, authCode)

		c.JSON(http.StatusOK, gin.H{
			"type": "success",
			"msg":  "登录成功",
			"user": user,
		})

	case 86038: // 二维码已失效
		delete(qrCodeCache, authCode)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "二维码已失效，请重新获取"})

	case 86090: // 已扫码未确认
		c.JSON(http.StatusOK, gin.H{"type": "waiting", "msg": "已扫码，等待确认"})

	case 86101: // 未扫码
		c.JSON(http.StatusOK, gin.H{"type": "waiting", "msg": "等待扫码"})

	default:
		c.JSON(http.StatusOK, gin.H{"type": "waiting", "msg": "等待扫码"})
	}
}

// UpdateBiliUser 更新用户
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

// DeleteBiliUser 删除用户
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

// extractCookiesFromResponse 从响应中提取Cookie
func extractCookiesFromResponse(resp *bili.QRCodePollResponse) string {
	// 这里需要根据实际API响应解析Cookie
	// TV端登录会在URL中返回参数，需要构建Cookie字符串
	// 简化实现，实际需要解析URL参数
	return resp.Data.URL // 需要进一步解析
}
