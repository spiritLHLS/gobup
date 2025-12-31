package bili

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"

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

// GenerateWebQRCode 生成Web端二维码（使用旧版API，参考Java项目实现）
func GenerateWebQRCode() (*QRCodeResponse, error) {
	// 参考: BiliUserController.java loginUser()
	apiURL := "https://passport.bilibili.com/qrcode/getLoginUrl"

	fmt.Printf("[WEB_QR] 请求URL: %s\n", apiURL)

	var qrResp QRCodeResponse
	client := req.C().ImpersonateChrome()
	resp, err := client.R().
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36").
		SetHeader("Referer", "https://www.bilibili.com/").
		Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求二维码失败: %w", err)
	}

	// 打印原始响应用于调试
	rawBody := resp.String()
	fmt.Printf("[WEB_QR_DEBUG] 原始响应: %s\n", rawBody)

	// 解析JSON
	if err := resp.UnmarshalJson(&qrResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if qrResp.Code != 0 {
		return nil, fmt.Errorf("生成二维码失败: %s", qrResp.Message)
	}

	// Web端使用oauthKey作为authCode用于轮询
	qrResp.Data.AuthCode = qrResp.Data.OauthKey

	fmt.Printf("[WEB_QR] 生成成功 - url: %s, oauthKey: %s\n", qrResp.Data.URL, qrResp.Data.OauthKey)

	return &qrResp, nil
}

// GenerateTVQRCode 生成TV端二维码（参考Java项目BiliApi.generateQRUrlTV()）
func GenerateTVQRCode() (*QRCodeResponse, error) {
	// 参考: BiliApi.java generateQRUrlTV()
	// 使用完全相同的参数和签名方式
	params := map[string]string{
		"appkey":   AppKey,
		"local_id": "0",
		"ts":       "0", // 注意：参考项目使用"0"而不是当前时间戳
	}

	signedURL := signParams(params)
	apiURL := "https://passport.bilibili.com/x/passport-tv-login/qrcode/auth_code?" + signedURL

	fmt.Printf("[TV_QR] 请求URL: %s\n", apiURL)

	var qrResp QRCodeResponse
	client := req.C().ImpersonateChrome()
	resp, err := client.R().
		Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求二维码失败: %w", err)
	}

	// 打印原始响应用于调试
	rawBody := resp.String()
	fmt.Printf("[TV_QR_DEBUG] 原始响应: %s\n", rawBody)

	// 解析JSON
	if err := resp.UnmarshalJson(&qrResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if qrResp.Code != 0 {
		return nil, fmt.Errorf("生成TV端二维码失败 code=%d msg=%s", qrResp.Code, qrResp.Message)
	}

	fmt.Printf("[TV_QR] 生成成功 - url: %s, auth_code: %s\n", qrResp.Data.URL, qrResp.Data.AuthCode)

	return &qrResp, nil
}

// PollWebQRCodeStatus 轮询Web端二维码状态（参考Python项目main.py save_ck()函数实现）
func PollWebQRCodeStatus(oauthKey string) (*QRCodePollResponse, error) {
	// 参考: main.py save_ck() 和 BiliApi.java loginOnWeb()
	tokenurl := "https://passport.bilibili.com/qrcode/getLoginInfo"

	fmt.Printf("[WEB_POLL] 请求URL: %s\n", tokenurl)

	var pollResp QRCodePollResponse
	client := req.C().ImpersonateChrome()
	resp, err := client.R().
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36").
		SetHeader("Host", "passport.bilibili.com").
		SetHeader("Referer", "https://passport.bilibili.com/login").
		SetFormData(map[string]string{
			"oauthKey": oauthKey,
			"gourl":    "https://www.bilibili.com/",
		}).
		Post(tokenurl)

	if err != nil {
		return nil, fmt.Errorf("轮询状态失败: %w", err)
	}

	// 打印原始响应用于调试
	rawBody := resp.String()
	fmt.Printf("[WEB_POLL_DEBUG] 原始响应: %s\n", rawBody)

	// 解析JSON
	if err := resp.UnmarshalJson(&pollResp); err != nil {
		return nil, fmt.Errorf("解析轮询响应失败: %w", err)
	}

	// 参考Python项目的状态码处理逻辑：
	// '-4' in str(qrcodedata['data']): 二维码未失效，请扫码
	// '-5' in str(qrcodedata['data']): 已扫码，请确认
	// '-2' in str(qrcodedata['data']): 二维码已失效
	// 'True' in str(qrcodedata['status']): 已确认，登入成功
	fmt.Printf("[WEB_POLL] 原始响应 - status: %v, data.code: %d, data.message: %s\n",
		pollResp.Status, pollResp.Data.Code, pollResp.Data.Message)

	// 优先判断status字段（Python项目用'True' in str(qrcodedata['status'])）
	if pollResp.Status {
		// 登录成功
		pollResp.Data.Code = 0
		fmt.Printf("[WEB_POLL] 登录成功 - url=%s\n", pollResp.Data.URL)
	} else {
		// 根据data.code字段判断状态
		switch pollResp.Data.Code {
		case -4:
			// 二维码未失效，等待扫码
			pollResp.Data.Code = 86101
			fmt.Printf("[WEB_POLL] 等待扫码\n")
		case -5:
			// 已扫码，等待确认
			pollResp.Data.Code = 86090
			fmt.Printf("[WEB_POLL] 已扫码，等待确认\n")
		case -2:
			// 二维码已失效
			pollResp.Data.Code = 86038
			fmt.Printf("[WEB_POLL] 二维码已过期\n")
		default:
			// 其他未知状态，默认为等待扫码
			pollResp.Data.Code = 86101
			fmt.Printf("[WEB_POLL] 未知状态 code=%d，默认等待扫码\n", pollResp.Data.Code)
		}
	}

	return &pollResp, nil
}

// PollTVQRCodeStatus 轮询TV端二维码状态（参考Java项目BiliApi.loginOnTV()）
func PollTVQRCodeStatus(authCode string) (*QRCodePollResponse, error) {
	// 参考: BiliApi.java loginOnTV()
	// 使用完全相同的参数和签名方式
	params := map[string]string{
		"appkey":    AppKey,
		"auth_code": authCode,
		"local_id":  "0",
		"ts":        "0", // 注意：参考项目使用"0"而不是当前时间戳
	}

	signedURL := signParams(params)
	apiURL := "https://passport.bilibili.com/x/passport-tv-login/qrcode/poll?" + signedURL

	fmt.Printf("[TV_POLL] 轮询URL: %s\n", apiURL)

	var pollResp QRCodePollResponse
	client := req.C().ImpersonateChrome()
	resp, err := client.R().
		Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("轮询状态失败: %w", err)
	}

	// 打印原始响应用于调试
	rawBody := resp.String()
	fmt.Printf("[TV_POLL_DEBUG] 原始响应: %s\n", rawBody)

	// 解析JSON
	if err := resp.UnmarshalJson(&pollResp); err != nil {
		return nil, fmt.Errorf("解析轮询响应失败: %w", err)
	}

	// TV端返回的状态码映射（参考Java项目）
	// code=0: 登录成功
	// code=86038: 二维码已失效
	// code=86090: 已扫码未确认
	// code=86101: 未扫码
	fmt.Printf("[TV_POLL] 原始响应 - code=%d, message=%s, data.code=%d, url=%s, hasRefreshToken=%v\n",
		pollResp.Code, pollResp.Message, pollResp.Data.Code, pollResp.Data.URL, pollResp.Data.RefreshToken != "")

	// TV端的状态码在顶层code字段，不是data.code
	if pollResp.Code == 0 {
		// 登录成功
		pollResp.Data.Code = 0
		fmt.Printf("[TV_POLL] 登录成功\n")
	} else {
		// 将顶层code映射到data.code以保持统一接口
		pollResp.Data.Code = pollResp.Code
		switch pollResp.Code {
		case 86038:
			fmt.Printf("[TV_POLL] 二维码已过期\n")
		case 86090:
			fmt.Printf("[TV_POLL] 已扫码，等待确认\n")
		case 86101:
			fmt.Printf("[TV_POLL] 等待扫码\n")
		default:
			fmt.Printf("[TV_POLL] 未知状态 code=%d\n", pollResp.Code)
		}
	}

	return &pollResp, nil
}

// GetUserInfo 获取用户信息
func GetUserInfo(cookies string) (*UserInfoResponse, error) {
	apiURL := "https://api.bilibili.com/x/space/myinfo"
	fmt.Printf("[USER_INFO] 请求URL: %s\n", apiURL)

	var userInfo UserInfoResponse
	client := req.C().ImpersonateChrome()
	resp, err := client.R().
		SetHeader("Cookie", cookies).
		Get(apiURL)
	if err != nil {
		return nil, err
	}

	// 打印原始响应用于调试
	rawBody := resp.String()
	fmt.Printf("[USER_INFO_DEBUG] 原始响应: %s\n", rawBody)

	// 解析JSON
	if err := resp.UnmarshalJson(&userInfo); err != nil {
		return nil, fmt.Errorf("解析用户信息失败: %w", err)
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

// ExtractCookiesFromWebPollResponse 从Web端轮询响应中提取Cookie（参考Java项目BiliApi.loginOnWeb()）
func ExtractCookiesFromWebPollResponse(pollResp *QRCodePollResponse, client *req.Client) string {
	// 参考: BiliApi.java loginOnWeb()
	// String url2 = webLoginDto.getData().getUrl();
	// String SESSDATA = getParameterValueFromUrl(url2, "SESSDATA");
	// String bili_jct = getParameterValueFromUrl(url2, "bili_jct");
	// String DedeUserID = getParameterValueFromUrl(url2, "DedeUserID");
	// String DedeUserID__ckMd5 = getParameterValueFromUrl(url2, "DedeUserID__ckMd5");
	// String sid = getParameterValueFromUrl(url2, "sid");

	if pollResp == nil || pollResp.Data.Code != 0 {
		fmt.Printf("[WEB_COOKIE] 登录未完成，跳过Cookie提取 - code=%d\n", pollResp.Data.Code)
		return ""
	}

	if pollResp.Data.URL == "" {
		fmt.Printf("[WEB_COOKIE] 错误：URL为空\n")
		return ""
	}

	// Web端登录成功后，URL中包含Cookie参数
	// 格式: https://passport.biligame.com/crossDomain?...&DedeUserID=xxx&SESSDATA=xxx&bili_jct=xxx&...
	fmt.Printf("[WEB_COOKIE] 解析登录URL: %s\n", pollResp.Data.URL[:min(100, len(pollResp.Data.URL))])

	parsedURL, err := url.Parse(pollResp.Data.URL)
	if err != nil {
		fmt.Printf("[WEB_COOKIE] URL解析失败: %v\n", err)
		return ""
	}

	// 从URL查询参数中提取Cookie（完全按照Java项目实现）
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

	// 构建Cookie字符串（格式与Java项目完全一致）
	// webLoginDto.setCookie("bili_jct=" + bili_jct + ";SESSDATA=" + SESSDATA + ";DedeUserID=" + DedeUserID + ";DedeUserID__ckMd5=" + DedeUserID__ckMd5 + ";sid+" + sid + ";");
	cookieStrs := []string{
		fmt.Sprintf("bili_jct=%s", biliJct),
		fmt.Sprintf("SESSDATA=%s", sessdata),
		fmt.Sprintf("DedeUserID=%s", dedeUserID),
	}

	if dedeUserIDCkMd5 != "" {
		cookieStrs = append(cookieStrs, fmt.Sprintf("DedeUserID__ckMd5=%s", dedeUserIDCkMd5))
	}
	if sid != "" {
		cookieStrs = append(cookieStrs, fmt.Sprintf("sid=%s", sid))
	}

	result := strings.Join(cookieStrs, "; ")
	fmt.Printf("[WEB_COOKIE] 提取成功 - DedeUserID: %s, SESSDATA长度: %d, bili_jct长度: %d\n",
		dedeUserID, len(sessdata), len(biliJct))

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
