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

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

const (
	AppKey    = "4409e2ce8ffd12b8"
	AppSecret = "59b43e04ad6965f34319062b478f83dd"
)

// QRCodeResponse 二维码响应（通用）
type QRCodeResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		URL       string `json:"url"`
		QRcodeKey string `json:"qrcode_key"` // Web端使用
		AuthCode  string `json:"auth_code"`  // TV端使用
		OauthKey  string `json:"oauthKey"`   // Web端使用
	} `json:"data"`
}

// QRCodePollResponse 轮询响应（通用）
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
	Status bool `json:"status"` // Web端使用
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

// GenerateWebQRCode 生成Web端二维码（类似Python项目）
func GenerateWebQRCode() (*QRCodeResponse, error) {
	apiURL := "https://passport.bilibili.com/qrcode/getLoginUrl"

	var qrResp QRCodeResponse
	client := req.C().ImpersonateChrome()
	_, err := client.R().SetSuccessResult(&qrResp).Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求二维码失败: %w", err)
	}

	if qrResp.Code != 0 {
		return nil, fmt.Errorf("生成二维码失败: %s", qrResp.Message)
	}

	// Web端使用oauthKey作为authCode
	qrResp.Data.AuthCode = qrResp.Data.OauthKey

	return &qrResp, nil
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

// PollWebQRCodeStatus 轮询Web端二维码状态
func PollWebQRCodeStatus(oauthKey string) (*QRCodePollResponse, error) {
	apiURL := "https://passport.bilibili.com/qrcode/getLoginInfo"

	var pollResp QRCodePollResponse
	client := req.C().ImpersonateChrome()
	resp, err := client.R().
		SetFormData(map[string]string{
			"oauthKey": oauthKey,
			"gourl":    "https://www.bilibili.com/",
		}).
		SetSuccessResult(&pollResp).
		Post(apiURL)

	if err != nil {
		return nil, fmt.Errorf("轮询状态失败: %w", err)
	}

	// Web端特殊处理
	if pollResp.Data.Code == 0 || pollResp.Status {
		// 登录成功，设置状态码为0
		pollResp.Data.Code = 0
	} else if pollResp.Data.Code == -4 {
		// 二维码未失效
		pollResp.Data.Code = 86101
	} else if pollResp.Data.Code == -5 {
		// 已扫码未确认
		pollResp.Data.Code = 86090
	} else if pollResp.Data.Code == -2 {
		// 二维码已失效
		pollResp.Data.Code = 86038
	}

	// 记录响应用于调试
	if resp != nil {
		fmt.Printf("[WEB_POLL] 响应 - code=%d, status=%v, url=%s, hasURL=%v\n",
			pollResp.Data.Code, pollResp.Status, pollResp.Data.URL, pollResp.Data.URL != "")
	}

	return &pollResp, nil
}

// PollTVQRCodeStatus 轮询TV端二维码状态
func PollTVQRCodeStatus(authCode string) (*QRCodePollResponse, error) {
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

	fmt.Printf("[TV_POLL] 响应 - code=%d, url=%s, hasURL=%v, hasRefreshToken=%v\n",
		pollResp.Data.Code, pollResp.Data.URL, pollResp.Data.URL != "", pollResp.Data.RefreshToken != "")

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

// ExtractCookiesFromWebPollResponse 从Web端轮询响应中提取Cookie
func ExtractCookiesFromWebPollResponse(pollResp *QRCodePollResponse, client *req.Client) string {
	if pollResp == nil || (pollResp.Data.Code != 0 && !pollResp.Status) {
		return ""
	}

	if pollResp.Data.URL == "" {
		return ""
	}

	// Web端需要访问返回的URL来设置cookie
	resp, err := client.R().Get(pollResp.Data.URL)
	if err != nil {
		fmt.Printf("访问Web端登录URL失败: %v\n", err)
		return ""
	}

	// 从响应的cookies中提取
	cookies := resp.Cookies()
	var cookieStrs []string
	for _, cookie := range cookies {
		if cookie.Name == "SESSDATA" || cookie.Name == "bili_jct" ||
			cookie.Name == "DedeUserID" || cookie.Name == "DedeUserID__ckMd5" {
			cookieStrs = append(cookieStrs, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
			fmt.Printf("[WEB_COOKIE] 提取Cookie - %s=%s\n", cookie.Name, cookie.Value[:min(20, len(cookie.Value))])
		}
	}

	if len(cookieStrs) > 0 {
		result := strings.Join(cookieStrs, "; ")
		fmt.Printf("[WEB_COOKIE] 提取成功 - length: %d\n", len(result))
		return result
	}

	// 备选方案：从URL参数中提取
	parsedURL, err := url.Parse(pollResp.Data.URL)
	if err == nil {
		query := parsedURL.Query()
		cookieStrs = []string{
			fmt.Sprintf("DedeUserID=%s", query.Get("DedeUserID")),
			fmt.Sprintf("SESSDATA=%s", query.Get("SESSDATA")),
			fmt.Sprintf("bili_jct=%s", query.Get("bili_jct")),
			fmt.Sprintf("DedeUserID__ckMd5=%s", query.Get("DedeUserID__ckMd5")),
		}
		return strings.Join(cookieStrs, "; ")
	}

	return ""
}

// ExtractCookiesFromTVPollResponse 从TV端轮询响应中提取Cookie
func ExtractCookiesFromTVPollResponse(pollResp *QRCodePollResponse) string {
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
		fmt.Printf("解析TV端URL失败: %v\n", err)
		return ""
	}

	query := parsedURL.Query()

	// 提取关键Cookie字段
	cookies := []string{
		fmt.Sprintf("DedeUserID=%s", query.Get("DedeUserID")),
		fmt.Sprintf("SESSDATA=%s", query.Get("SESSDATA")),
		fmt.Sprintf("bili_jct=%s", query.Get("bili_jct")),
		fmt.Sprintf("DedeUserID__ckMd5=%s", query.Get("DedeUserID__ckMd5")),
		fmt.Sprintf("sid=%s", query.Get("sid")),
	}

	cookieStr := strings.Join(cookies, "; ")
	fmt.Printf("[TV_COOKIE] 提取成功 - length: %d, DedeUserID: %s, SESSDATA: %s\n",
		len(cookieStr), query.Get("DedeUserID"), query.Get("SESSDATA")[:min(20, len(query.Get("SESSDATA")))])
	return cookieStr
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
