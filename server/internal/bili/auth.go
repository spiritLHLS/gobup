package bili

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

const (
	AppKey    = "4409e2ce8ffd12b8"
	AppSecret = "59b43e04ad6965f34319062b478f83dd"
)

// QRCodeResponse 二维码响应
type QRCodeResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		URL       string `json:"url"`
		QRcodeKey string `json:"qrcode_key"`
		AuthCode  string `json:"auth_code"`
	} `json:"data"`
}

// QRCodePollResponse 轮询响应
type QRCodePollResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		URL          string `json:"url"`
		RefreshToken string `json:"refresh_token"`
		Timestamp    int64  `json:"timestamp"`
		Code         int    `json:"code"`
		Message      string `json:"message"`
	} `json:"data"`
}

// UserInfoResponse 用户信息响应
type UserInfoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Mid       int64  `json:"mid"`
		Uname     string `json:"uname"`
		Face      string `json:"face"`
		Level     int    `json:"level_info.current_level"`
		VipType   int    `json:"vip.type"`
		VipStatus int    `json:"vip.status"`
		Moral     int    `json:"moral"`
	} `json:"data"`
}

// GenerateTVQRCode 生成TV端二维码
func GenerateTVQRCode() (*QRCodeResponse, error) {
	params := map[string]string{
		"appkey":   AppKey,
		"local_id": "0",
		"ts":       fmt.Sprintf("%d", time.Now().Unix()),
	}

	signedURL := signParams(params)
	apiURL := "https://passport.bilibili.com/x/passport-tv-login/qrcode/auth_code?" + signedURL

	var qrResp QRCodeResponse
	client := req.C().ImpersonateChrome()
	_, err := client.R().SetSuccessResult(&qrResp).Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求二维码失败: %w", err)
	}

	if qrResp.Code != 0 {
		return nil, fmt.Errorf("生成二维码失败: %s", qrResp.Message)
	}

	return &qrResp, nil
}

// PollQRCodeStatus 轮询二维码状态
func PollQRCodeStatus(authCode string) (*QRCodePollResponse, error) {
	params := map[string]string{
		"appkey":    AppKey,
		"auth_code": authCode,
		"local_id":  "0",
		"ts":        fmt.Sprintf("%d", time.Now().Unix()),
	}

	signedURL := signParams(params)
	apiURL := "https://passport.bilibili.com/x/passport-tv-login/qrcode/poll?" + signedURL

	var pollResp QRCodePollResponse
	client := req.C().ImpersonateChrome()
	_, err := client.R().SetSuccessResult(&pollResp).Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("轮询状态失败: %w", err)
	}

	return &pollResp, nil
}

// GetUserInfo 获取用户信息
func GetUserInfo(cookies string) (*UserInfoResponse, error) {
	var userInfo UserInfoResponse
	client := req.C().ImpersonateChrome()
	_, err := client.R().
		SetHeader("Cookie", cookies).
		SetSuccessResult(&userInfo).
		Get("https://api.bilibili.com/x/space/myinfo")
	if err != nil {
		return nil, err
	}

	if userInfo.Code == -101 {
		return nil, fmt.Errorf("cookie已失效")
	}

	if userInfo.Code != 0 {
		return nil, fmt.Errorf("获取用户信息失败: %s", userInfo.Message)
	}

	return &userInfo, nil
}

// ValidateCookie 验证Cookie有效性
func ValidateCookie(cookies string) (bool, error) {
	_, err := GetUserInfo(cookies)
	if err != nil {
		if strings.Contains(err.Error(), "已失效") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ExtractCookiesFromPollResponse 从轮询响应中提取Cookie
// 参考biliupforjava的实现
func ExtractCookiesFromPollResponse(pollResp *QRCodePollResponse) string {
	if pollResp == nil || pollResp.Data.Code != 0 {
		return ""
	}

	// TV端登录会在返回的数据中包含cookie信息
	// 需要从URL中提取参数
	if pollResp.Data.URL == "" {
		return ""
	}

	parsedURL, err := url.Parse(pollResp.Data.URL)
	if err != nil {
		return ""
	}

	query := parsedURL.Query()

	// 提取关键Cookie字段
	cookies := []string{
		fmt.Sprintf("SESSDATA=%s", query.Get("SESSDATA")),
		fmt.Sprintf("bili_jct=%s", query.Get("bili_jct")),
		fmt.Sprintf("DedeUserID=%s", query.Get("DedeUserID")),
		fmt.Sprintf("DedeUserID__ckMd5=%s", query.Get("DedeUserID__ckMd5")),
		fmt.Sprintf("sid=%s", query.Get("sid")),
	}

	return strings.Join(cookies, "; ")
}

// ParseCookies 解析Cookie字符串
func ParseCookies(cookieStr string) map[string]string {
	cookies := make(map[string]string)
	parts := strings.Split(cookieStr, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "="); idx > 0 {
			key := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			cookies[key] = value
		}
	}
	return cookies
}

// GetCookieValue 获取Cookie值
func GetCookieValue(cookieStr, key string) string {
	cookies := ParseCookies(cookieStr)
	return cookies[key]
}

// signParams 签名参数
func signParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var query []string
	for _, k := range keys {
		query = append(query, fmt.Sprintf("%s=%s", k, url.QueryEscape(params[k])))
	}
	queryString := strings.Join(query, "&")

	sign := md5Sign(queryString + AppSecret)
	return queryString + "&sign=" + sign
}

// md5Sign 计算MD5签名
func md5Sign(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
