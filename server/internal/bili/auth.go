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

// GenerateWebQRCode 生成Web端二维码（使用旧版API）
func GenerateWebQRCode() (*QRCodeResponse, error) {
	apiURL := "https://passport.bilibili.com/qrcode/getLoginUrl"

	var qrResp QRCodeResponse
	client := req.C().ImpersonateChrome()
	_, err := client.R().
		SetHeader("Referer", "https://www.bilibili.com/").
		SetSuccessResult(&qrResp).
		Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求二维码失败: %w", err)
	}

	if qrResp.Code != 0 {
		return nil, fmt.Errorf("生成二维码失败: %s", qrResp.Message)
	}

	// Web端使用oauthKey作为authCode用于轮询
	qrResp.Data.AuthCode = qrResp.Data.OauthKey

	fmt.Printf("[WEB_QR] 生成成功 - url: %s, oauthKey: %s\n", qrResp.Data.URL, qrResp.Data.OauthKey)

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
	_, err := client.R().
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36").
		SetHeader("Host", "passport.bilibili.com").
		SetHeader("Referer", "https://passport.bilibili.com/login").
		SetFormData(map[string]string{
			"oauthKey": oauthKey,
			"gourl":    "https://www.bilibili.com/",
		}).
		SetSuccessResult(&pollResp).
		Post(apiURL)

	if err != nil {
		return nil, fmt.Errorf("轮询状态失败: %w", err)
	}

	// Web端轮询返回的状态码处理（参考Python项目）
	// status=True 表示登录成功
	// data.code: -4=未失效, -5=已扫码未确认, -2=已失效
	if pollResp.Status || pollResp.Data.Code == 0 {
		// 登录成功
		pollResp.Data.Code = 0
		fmt.Printf("[WEB_POLL] 登录成功 - url=%s\n", pollResp.Data.URL)
	} else if pollResp.Data.Code == -4 {
		// 二维码未失效，等待扫码
		pollResp.Data.Code = 86101
	} else if pollResp.Data.Code == -5 {
		// 已扫码未确认
		pollResp.Data.Code = 86090
		fmt.Printf("[WEB_POLL] 已扫码，等待确认\n")
	} else if pollResp.Data.Code == -2 {
		// 二维码已失效
		pollResp.Data.Code = 86038
		fmt.Printf("[WEB_POLL] 二维码已过期\n")
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

// ExtractCookiesFromWebPollResponse 从Web端轮询响应中提取Cookie（参考Python项目）
func ExtractCookiesFromWebPollResponse(pollResp *QRCodePollResponse, client *req.Client) string {
	if pollResp == nil || (pollResp.Data.Code != 0 && !pollResp.Status) {
		fmt.Printf("[WEB_COOKIE] 登录未完成，跳过Cookie提取\n")
		return ""
	}

	if pollResp.Data.URL == "" {
		fmt.Printf("[WEB_COOKIE] 错误：URL为空\n")
		return ""
	}

	// Web端登录成功后，URL中包含Cookie参数（参考Python实现）
	// 格式: https://passport.biligame.com/crossDomain?...&DedeUserID=xxx&SESSDATA=xxx&bili_jct=xxx&...
	fmt.Printf("[WEB_COOKIE] 解析登录URL: %s\n", pollResp.Data.URL[:min(100, len(pollResp.Data.URL))])

	parsedURL, err := url.Parse(pollResp.Data.URL)
	if err != nil {
		fmt.Printf("[WEB_COOKIE] URL解析失败: %v\n", err)
		return ""
	}

	// 从URL查询参数中提取Cookie（参考Python项目的实现）
	// txt = str(qrcodedata['data']['url'][42:-39])
	// DedeUserID = txt.split('&')[0]
	// SESSDATA = txt.split('&')[3]
	// bili_jct = txt.split('&')[4]
	query := parsedURL.Query()
	dedeUserID := query.Get("DedeUserID")
	sessdata := query.Get("SESSDATA")
	biliJct := query.Get("bili_jct")
	dedeUserIDCkMd5 := query.Get("DedeUserID__ckMd5")
	sid := query.Get("sid")

	if dedeUserID == "" || sessdata == "" || biliJct == "" {
		fmt.Printf("[WEB_COOKIE] 关键字段缺失 - DedeUserID: %v, SESSDATA: %v, bili_jct: %v\n",
			dedeUserID != "", sessdata != "", biliJct != "")
		return ""
	}

	// 构建Cookie字符串
	cookieStrs := []string{
		fmt.Sprintf("DedeUserID=%s", dedeUserID),
		fmt.Sprintf("SESSDATA=%s", sessdata),
		fmt.Sprintf("bili_jct=%s", biliJct),
	}

	if dedeUserIDCkMd5 != "" {
		cookieStrs = append(cookieStrs, fmt.Sprintf("DedeUserID__ckMd5=%s", dedeUserIDCkMd5))
	}
	if sid != "" {
		cookieStrs = append(cookieStrs, fmt.Sprintf("sid=%s", sid))
	}

	result := strings.Join(cookieStrs, "; ")
	fmt.Printf("[WEB_COOKIE] 提取成功 - DedeUserID: %s, SESSDATA长度: %d\n",
		dedeUserID, len(sessdata))

	return result
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
