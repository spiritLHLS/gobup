package controllers

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/imroc/req/v3"
	"github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
	"gorm.io/gorm/clause"
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

var (
	loginSessions   = make(map[string]*LoginSession)
	loginSessionsMu sync.RWMutex
)

const sessionExpireTime = 3 * 60 // 3分钟过期

type BiliUserResponse struct {
	ID               uint       `json:"id"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	UID              int64      `json:"uid"`
	Uname            string     `json:"uname"`
	Face             string     `json:"face"`
	Login            bool       `json:"login"`
	Enabled          bool       `json:"enabled"`
	Level            int        `json:"level"`
	VipType          int        `json:"vipType"`
	VipStatus        int        `json:"vipStatus"`
	LoginTime        *time.Time `json:"loginTime"`
	ExpireTime       *time.Time `json:"expireTime"`
	LastCheckTime    *time.Time `json:"lastCheckTime"`
	LastCheckError   string     `json:"lastCheckError"`
	HasWxPushToken   bool       `json:"hasWxPushToken"`
	DailyUploadQuota int        `json:"dailyUploadQuota"`
}

func safeBiliUserResponse(user models.BiliBiliUser) BiliUserResponse {
	return BiliUserResponse{
		ID:               user.ID,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
		UID:              user.UID,
		Uname:            user.Uname,
		Face:             user.Face,
		Login:            user.Login,
		Enabled:          user.Enabled,
		Level:            user.Level,
		VipType:          user.VipType,
		VipStatus:        user.VipStatus,
		LoginTime:        user.LoginTime,
		ExpireTime:       user.ExpireTime,
		LastCheckTime:    user.LastCheckTime,
		LastCheckError:   user.LastCheckError,
		HasWxPushToken:   strings.TrimSpace(user.WxPushToken) != "",
		DailyUploadQuota: user.DailyUploadQuota,
	}
}

func safeBiliUserResponses(users []models.BiliBiliUser) []BiliUserResponse {
	responses := make([]BiliUserResponse, 0, len(users))
	for _, user := range users {
		responses = append(responses, safeBiliUserResponse(user))
	}
	return responses
}

// cleanupExpiredLoginSessions 清理已过期的登录会话，防止内存泄漏
func cleanupExpiredLoginSessions() {
	now := time.Now().Unix()
	loginSessionsMu.Lock()
	for key, session := range loginSessions {
		// 保留双倍过期时间的缓冲，避免已成功但还未返回的会话被过早清除
		if now-session.CreateTime > sessionExpireTime*2 {
			delete(loginSessions, key)
		}
	}
	loginSessionsMu.Unlock()
}

// ListBiliUsers 获取B站用户列表（不包括管理员）
func ListBiliUsers(c *gin.Context) {
	db := database.GetDB()
	var users []models.BiliBiliUser
	// 过滤掉UID=-1的root管理员用户
	db.Select("id", "created_at", "updated_at", "uid", "uname", "face", "login", "enabled", "level", "vip_type", "vip_status", "login_time", "expire_time", "last_check_time", "last_check_error", "wx_push_token", "daily_upload_quota").
		Where("uid != ?", -1).
		Order("created_at DESC").
		Find(&users)

	c.JSON(http.StatusOK, safeBiliUserResponses(users))
}

// LoginUser 生成B站登录二维码
func LoginUser(c *gin.Context) {
	// 每次创建新会话时顺手清理已过期的遗留会话，防止 loginSessions map 无限增长
	cleanupExpiredLoginSessions()

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
	log.Printf("%s端二维码已生成: url长度=%d, authCode长度=%d", loginType, len(qrResp.Data.URL), len(qrResp.Data.AuthCode))

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

	// 使用图片的最后100个字符作为session key，并转为URL安全格式（替换掉+/=避免URL编码问题）
	rawKey := imageBase64[len(imageBase64)-100:]
	sessionKey := strings.NewReplacer("+", "-", "/", "_", "=", "").Replace(rawKey)

	// 创建登录会话
	session := &LoginSession{
		AuthCode:   qrResp.Data.AuthCode,
		QRCodeURL:  qrResp.Data.URL,
		CreateTime: time.Now().Unix(),
		Status:     "pending",
		Message:    "等待扫码",
		LoginType:  loginType, // 正确保存登录类型
	}
	loginSessionsMu.Lock()
	loginSessions[sessionKey] = session
	loginSessionsMu.Unlock()
	log.Printf("[SUCCESS] 登录会话已创建 - keyLength: %d, type: %s, authCodeLength: %d", len(sessionKey), loginType, len(qrResp.Data.AuthCode))

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
	log.Printf("[CHECK] 收到登录检查请求 - keyLength: %d", len(sessionKey))
	if sessionKey == "" {
		log.Printf("[CHECK] 缺少key参数")
		c.JSON(http.StatusOK, gin.H{
			"status":  "failed",
			"message": "缺少key参数",
		})
		return
	}

	loginSessionsMu.RLock()
	session, exists := loginSessions[sessionKey]
	sessionCount := len(loginSessions)
	loginSessionsMu.RUnlock()

	if !exists {
		log.Printf("[CHECK] 会话不存在 - keyLength: %d, 当前会话数: %d", len(sessionKey), sessionCount)
		c.JSON(http.StatusOK, gin.H{
			"status":  "failed",
			"message": "会话不存在或已过期",
		})
		return
	}

	// 检查会话是否过期
	if time.Now().Unix()-session.CreateTime > sessionExpireTime {
		log.Printf("[CHECK] 会话已过期 - type: %s, authCodeLength: %d", session.LoginType, len(session.AuthCode))
		loginSessionsMu.Lock()
		delete(loginSessions, sessionKey)
		loginSessionsMu.Unlock()
		c.JSON(http.StatusOK, gin.H{
			"status":  "expired",
			"message": "二维码已过期，请刷新",
		})
		return
	}

	// 如果已有状态，直接返回（success 状态不立即删除，让会话自然过期，避免并发请求误返回"会话不存在"）
	if session.Status != "pending" {
		log.Printf("[CHECK] 返回已有状态 - status: %s, type: %s", session.Status, session.LoginType)
		c.JSON(http.StatusOK, gin.H{
			"status":  session.Status,
			"message": session.Message,
		})
		return
	}

	// 根据登录类型轮询登录状态
	var pollResp *bili.QRCodePollResponse
	var err error

	log.Printf("[POLL] 开始轮询 - type: %s, authCodeLength: %d", session.LoginType, len(session.AuthCode))

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

		log.Printf("[%s] Cookie提取完成，长度=%d（已隐藏敏感内容）", session.LoginType, len(cookieStr))
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

		// 保存用户到数据库（使用 Upsert，防止两次扫码同时成功时 UNIQUE 冲突）
		db := database.GetDB()

		now := time.Now()
		expireTime := now.Add(30 * 24 * time.Hour)

		user := models.BiliBiliUser{
			UID:          userInfo.Data.Mid,
			Uname:        userInfo.Data.Uname,
			Face:         userInfo.Data.Face,
			Cookies:      cookieStr,
			RefreshToken: pollResp.Data.RefreshToken,
			Login:        true,
			Enabled:      true,
			Level:        userInfo.Data.Level,
			VipType:      userInfo.Data.VipType,
			VipStatus:    userInfo.Data.VipStatus,
			LoginTime:    &now,
			ExpireTime:   &expireTime,
		}

		upsertCols := []string{"uname", "face", "cookies", "refresh_token", "login",
			"enabled", "level", "vip_type", "vip_status", "login_time", "expire_time", "updated_at", "deleted_at"}
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "uid"}},
			DoUpdates: clause.AssignmentColumns(upsertCols),
		}).Create(&user).Error; err != nil {
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
		loginSessionsMu.Lock()
		delete(loginSessions, sessionKey)
		loginSessionsMu.Unlock()
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

	if user.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户ID不能为空"})
		return
	}

	db := database.GetDB()

	// 只更新允许更新的字段
	if err := db.Model(&user).Updates(map[string]interface{}{
		"uname":              user.Uname,
		"face":               user.Face,
		"level":              user.Level,
		"wx_push_token":      user.WxPushToken,
		"daily_upload_quota": user.DailyUploadQuota,
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "更新成功"})
}

// SetBiliUserEnabled 启用或禁用B站账号。
func SetBiliUserEnabled(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	db := database.GetDB()
	var user models.BiliBiliUser
	if err := db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "用户不存在"})
		return
	}
	if user.UID == -1 {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "管理员账号不能禁用"})
		return
	}

	if err := db.Model(&user).Update("enabled", req.Enabled).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "状态更新失败"})
		return
	}

	statusText := "启用"
	if !req.Enabled {
		statusText = "禁用"
	}
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "账号已" + statusText})
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
		"user":       safeBiliUserResponse(user),
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

	now := time.Now()
	user.LastCheckTime = &now

	// 验证Cookie是否有效
	valid, err := bili.ValidateCookie(user.Cookies)
	if err != nil {
		user.Login = false
		user.LastCheckError = err.Error()
		db.Save(&user)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "验证失败: " + err.Error(), "user": safeBiliUserResponse(user)})
		return
	}

	if !valid {
		user.Login = false
		user.LastCheckError = "Cookie已失效"
		db.Save(&user)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "Cookie已失效，请重新登录", "user": safeBiliUserResponse(user)})
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
	user.LastCheckError = ""
	db.Save(&user)

	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "Cookie有效，用户状态正常", "user": safeBiliUserResponse(user)})
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

	// 保存用户到数据库（使用 Upsert，防止并发登录时 UNIQUE 冲突）
	db := database.GetDB()

	now := time.Now()
	expireTime := now.Add(30 * 24 * time.Hour) // 30天过期

	user := models.BiliBiliUser{
		UID:        userInfo.Data.Mid,
		Uname:      userInfo.Data.Uname,
		Face:       userInfo.Data.Face,
		Cookies:    cookieStr,
		Login:      true,
		Enabled:    true,
		Level:      userInfo.Data.Level,
		VipType:    userInfo.Data.VipType,
		VipStatus:  userInfo.Data.VipStatus,
		LoginTime:  &now,
		ExpireTime: &expireTime,
	}

	// 使用 Upsert，防止并发登录时 UNIQUE 冲突
	upsertCols := []string{"uname", "face", "cookies", "login", "enabled",
		"level", "vip_type", "vip_status", "login_time", "expire_time", "updated_at", "deleted_at"}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "uid"}},
		DoUpdates: clause.AssignmentColumns(upsertCols),
	}).Create(&user).Error; err != nil {
		log.Printf("保存用户失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": "保存用户失败"})
		return
	}

	log.Printf("[INFO] B站用户通过Cookie登录成功: UID=%d, Uname=%s", user.UID, user.Uname)

	c.JSON(http.StatusOK, gin.H{
		"type": "success",
		"msg":  "登录成功",
		"user": safeBiliUserResponse(user),
	})
}
