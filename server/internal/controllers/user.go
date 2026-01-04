package controllers

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/imroc/req/v3"
	"github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
)

// nopCloser 包装 io.Writer 为 io.WriteCloser
type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LoginSession 登录会话
type LoginSession struct {
	AuthCode   string
	QRCodeURL  string
	CreateTime int64
	Status     string // pending, success, failed, expired
	Message    string
	LoginType  string // web or tv
}

var loginSessions = make(map[string]*LoginSession)

const sessionExpireTime = 3 * 60 // 3分钟过期（参考Python项目的180秒轮询时长）

// ListBiliUsers 获取B站用户列表（不包括管理员）
func ListBiliUsers(c *gin.Context) {
	db := database.GetDB()
	var users []models.BiliBiliUser
	// 过滤掉UID=-1的root管理员用户
	db.Select("id", "created_at", "updated_at", "uid", "uname", "face", "login", "level", "vip_type", "vip_status", "login_time", "expire_time", "wx_push_token").
		Where("uid != ?", -1).
		Order("created_at DESC").
		Find(&users)

	c.JSON(http.StatusOK, users)
}

// LoginUser 生成B站登录二维码
func LoginUser(c *gin.Context) {
	// 获取登录类型参数 (web/tv)，默认为tv
	loginType := c.DefaultQuery("type", "tv")
	log.Printf("开始生成%s端二维码...", loginType)

	var qrResp *bili.QRCodeResponse
	var err error

	if loginType == "web" {
		qrResp, err = bili.GenerateWebQRCode()
	} else {
		qrResp, err = bili.GenerateTVQRCode()
	}

	if err != nil {
		log.Printf("生成二维码失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"error": "生成二维码失败: " + err.Error()})
		return
	}
	log.Printf("%s端二维码URL: %s, AuthCode: %s", loginType, qrResp.Data.URL, qrResp.Data.AuthCode)

	// 生成二维码图片
	qrc, err := qrcode.NewWith(qrResp.Data.URL,
		qrcode.WithErrorCorrectionLevel(qrcode.ErrorCorrectionMedium),
	)
	if err != nil {
		log.Printf("创建二维码失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"error": "创建二维码失败"})
		return
	}

	buf := new(bytes.Buffer)
	w := nopCloser{buf}
	stdWriter := standard.NewWithWriter(w,
		standard.WithQRWidth(10),
		standard.WithBuiltinImageEncoder(standard.PNG_FORMAT),
	)
	if err = qrc.Save(stdWriter); err != nil {
		log.Printf("生成PNG失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"error": "生成PNG失败"})
		return
	}

	pngBytes := buf.Bytes()
	log.Printf("[INFO] 生成的PNG大小: %d bytes", len(pngBytes))

	// 验证PNG头部
	if len(pngBytes) < 8 || string(pngBytes[1:4]) != "PNG" {
		log.Printf("[ERROR] PNG格式无效，头部: %v", pngBytes[:min(8, len(pngBytes))])
		c.JSON(http.StatusOK, gin.H{"error": "生成的二维码图片格式无效"})
		return
	}

	// Base64编码
	imageBase64 := base64.StdEncoding.EncodeToString(pngBytes)
	log.Printf("[INFO] Base64编码长度: %d", len(imageBase64))
	log.Printf("[DEBUG] Base64前缀: %s", imageBase64[:min(50, len(imageBase64))])

	// 使用图片的最后100个字符作为session key
	sessionKey := imageBase64[len(imageBase64)-100:]

	// 创建登录会话
	session := &LoginSession{
		AuthCode:   qrResp.Data.AuthCode,
		QRCodeURL:  qrResp.Data.URL,
		CreateTime: time.Now().Unix(),
		Status:     "pending",
		Message:    "等待扫码",
		LoginType:  loginType, // 正确保存登录类型
	}
	loginSessions[sessionKey] = session
	log.Printf("[SUCCESS] 登录会话已创建 - sessionKey: %s, type: %s, authCode: %s", sessionKey, loginType, qrResp.Data.AuthCode)

	response := gin.H{
		"image": imageBase64,
		"key":   sessionKey,
		"type":  loginType,
	}
	log.Printf("[SUCCESS] 返回二维码数据 - type: %s, imageLength: %d, keyLength: %d", loginType, len(imageBase64), len(sessionKey))
	c.JSON(http.StatusOK, response)
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

	// 根据登录类型轮询登录状态
	var pollResp *bili.QRCodePollResponse
	var err error

	log.Printf("[POLL] 开始轮询 - type: %s, authCode: %s", session.LoginType, session.AuthCode)

	if session.LoginType == "web" {
		pollResp, err = bili.PollWebQRCodeStatus(session.AuthCode)
	} else {
		pollResp, err = bili.PollTVQRCodeStatus(session.AuthCode)
	}

	if err != nil {
		log.Printf("[ERROR] 轮询失败: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"status":  "pending",
			"message": "检查中...",
		})
		return
	}

	log.Printf("[POLL] 轮询响应 - code: %d, status: %s", pollResp.Data.Code, session.Status)

	switch pollResp.Data.Code {
	case 0: // 登录成功
		// 根据登录类型解析Cookie
		var cookieStr string
		if session.LoginType == "web" {
			client := req.C().ImpersonateChrome()
			cookieStr = bili.ExtractCookiesFromWebPollResponse(pollResp, client)
		} else {
			cookieStr = bili.ExtractCookiesFromTVPollResponse(pollResp)
		}

		log.Printf("[%s] 提取到的Cookie: %s", session.LoginType, cookieStr)
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

// RefreshUserCookie 刷新用户Token和Cookie（参考biliupforjava RefreshTokenJob）
func RefreshUserCookie(c *gin.Context) {
	id := c.Param("id")

	db := database.GetDB()
	var user models.BiliBiliUser

	if err := db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "用户不存在"})
		return
	}

	// 检查是否有RefreshToken
	if user.RefreshToken == "" {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "该用户无RefreshToken，请重新登录"})
		return
	}

	log.Printf("[INFO] 开始刷新用户Token: %s(%d)", user.Uname, user.UID)

	// 调用刷新Token API
	refreshResp, err := bili.RefreshToken(user.AccessKey, user.RefreshToken, user.Cookies)
	if err != nil {
		log.Printf("[ERROR] 刷新Token失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "刷新失败: " + err.Error()})
		return
	}

	// 提取新的Token和Cookie
	tokenInfo := bili.ExtractRefreshTokenInfo(refreshResp)
	if tokenInfo == nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "提取Token信息失败"})
		return
	}

	// 更新用户信息
	user.AccessKey = tokenInfo.AccessToken
	user.RefreshToken = tokenInfo.RefreshToken
	user.Cookies = tokenInfo.Cookies
	user.Login = true

	// 更新过期时间
	now := time.Now()
	expireTime := now.Add(time.Duration(tokenInfo.ExpiresIn) * time.Second)
	user.ExpireTime = &expireTime

	if err := db.Save(&user).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存用户失败"})
		return
	}

	log.Printf("[INFO] Token刷新成功: %s(%d), 新过期时间: %s",
		user.Uname, user.UID, expireTime.Format("2006-01-02 15:04:05"))

	c.JSON(http.StatusOK, gin.H{
		"type":       "success",
		"msg":        "Token刷新成功",
		"user":       user,
		"expireTime": expireTime.Format("2006-01-02 15:04:05"),
	})
}

// CheckUserStatus 检查用户Cookie状态
func CheckUserStatus(c *gin.Context) {
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
		user.Login = false
		db.Save(&user)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "验证失败: " + err.Error(), "user": user})
		return
	}

	if !valid {
		user.Login = false
		db.Save(&user)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Cookie已失效，请重新登录", "user": user})
		return
	}

	// 获取最新用户信息
	userInfo, err := bili.GetUserInfo(user.Cookies)
	if err == nil {
		user.Uname = userInfo.Data.Uname
		user.Face = userInfo.Data.Face
		user.Level = userInfo.Data.Level
		user.VipType = userInfo.Data.VipType
		user.VipStatus = userInfo.Data.VipStatus
	}

	user.Login = true
	db.Save(&user)

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Cookie有效，用户状态正常", "user": user})
}

// LoginByCookie 通过Cookie直接登录
func LoginByCookie(c *gin.Context) {
	var req struct {
		Cookies string `json:"cookies" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "请求参数错误"})
		return
	}

	// 去除首尾空格
	cookieStr := strings.TrimSpace(req.Cookies)
	if cookieStr == "" {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Cookie不能为空"})
		return
	}

	// 验证Cookie格式和有效性
	valid, err := bili.ValidateCookie(cookieStr)
	if err != nil {
		log.Printf("验证Cookie失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "验证Cookie失败: " + err.Error()})
		return
	}

	if !valid {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Cookie已失效或格式错误，请重新获取"})
		return
	}

	// 获取用户信息
	userInfo, err := bili.GetUserInfo(cookieStr)
	if err != nil {
		log.Printf("获取用户信息失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "获取用户信息失败，请检查Cookie是否正确"})
		return
	}

	// 保存用户到数据库
	db := database.GetDB()
	var user models.BiliBiliUser

	now := time.Now()
	expireTime := now.Add(30 * 24 * time.Hour) // 30天过期

	result := db.Where("uid = ?", userInfo.Data.Mid).First(&user)
	if result.Error != nil {
		// 新用户
		user = models.BiliBiliUser{
			UID:        userInfo.Data.Mid,
			Uname:      userInfo.Data.Uname,
			Face:       userInfo.Data.Face,
			Cookies:    cookieStr,
			Login:      true,
			Level:      userInfo.Data.Level,
			VipType:    userInfo.Data.VipType,
			VipStatus:  userInfo.Data.VipStatus,
			LoginTime:  &now,
			ExpireTime: &expireTime,
		}
	} else {
		// 更新现有用户
		user.Uname = userInfo.Data.Uname
		user.Face = userInfo.Data.Face
		user.Cookies = cookieStr
		user.Login = true
		user.Level = userInfo.Data.Level
		user.VipType = userInfo.Data.VipType
		user.VipStatus = userInfo.Data.VipStatus
		user.LoginTime = &now
		user.ExpireTime = &expireTime
	}

	if err := db.Save(&user).Error; err != nil {
		log.Printf("保存用户失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存用户失败"})
		return
	}

	log.Printf("[INFO] B站用户通过Cookie登录成功: UID=%d, Uname=%s", user.UID, user.Uname)

	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"msg":  "登录成功",
		"user": user,
	})
}
