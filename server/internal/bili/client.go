package bili

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

type BiliClient struct {
	AccessKey   string
	AccessToken string
	Cookies     string
	Mid         int64
	Line        string // 上传线路，如 cs_txa, cs_bda2
	ReqClient   *req.Client
}

type PreUploadResp struct {
	OK           int      `json:"OK"`
	Auth         string   `json:"auth"`
	Endpoint     string   `json:"endpoint"`
	Endpoints    []string `json:"endpoints"`
	BizID        int64    `json:"biz_id"`
	UploadID     string   `json:"upload_id"`
	UposURI      string   `json:"upos_uri"`
	BiliFilename string   `json:"bilifilename"`
}

// LineUploadResp 线路上传响应
type LineUploadResp struct {
	OK       int    `json:"OK"`
	UploadID string `json:"upload_id"`
	Key      string `json:"key"`
	Bucket   string `json:"bucket"`
}

type UploadResult struct {
	FileName string
	BizID    int64
}

type DescV2Item struct {
	BizID   string `json:"biz_id"`
	RawText string `json:"raw_text"`
	Type    int    `json:"type"`
}

type PublishVideoRequest struct {
	Copyright    int                       `json:"copyright"`
	Cover        string                    `json:"cover"`
	Desc         string                    `json:"desc"`
	DescFormatID int                       `json:"desc_format_id"`
	DescV2       []DescV2Item              `json:"desc_v2,omitempty"`
	Dynamic      string                    `json:"dynamic"`
	DynamicV2    []DescV2Item              `json:"dynamic_v2,omitempty"`
	Interactive  int                       `json:"interactive"`
	NoReprint    int                       `json:"no_reprint"`
	OpenElec     int                       `json:"open_elec"`
	Source       string                    `json:"source"`
	Tag          string                    `json:"tag"`
	Tid          int                       `json:"tid"`
	Title        string                    `json:"title"`
	Videos       []PublishVideoPartRequest `json:"videos"`
	CSRF         string                    `json:"csrf"`
	UpCloseReply bool                      `json:"up_close_reply"`
	UpCloseDanmu bool                      `json:"up_close_danmu"`
	WebOS        int                       `json:"web_os"`
}

type PublishVideoPartRequest struct {
	Desc     string `json:"desc"`
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Cid      int64  `json:"cid"`
}

type PublishResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"message"`
	Data struct {
		Aid  int64  `json:"aid"`
		Bvid string `json:"bvid"`
	} `json:"data"`
}

// BuvIdResponse 获取buvid响应
type BuvIdResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"message"`
	Data struct {
		B3 string `json:"b_3"`
		B4 string `json:"b_4"`
	} `json:"data"`
}

func NewBiliClient(accessKey, cookies string, mid int64) *BiliClient {
	client := req.C().
		SetTimeout(300 * time.Second).
		ImpersonateChrome()

	if cookies != "" {
		client.SetCommonHeader("Cookie", cookies)
	}

	return &BiliClient{
		AccessKey: accessKey,
		Cookies:   cookies,
		Mid:       mid,
		ReqClient: client,
	}
}

// NewBiliClientWithProxy 创建带代理的BiliClient
func NewBiliClientWithProxy(accessKey, cookies string, mid int64, proxyURL string) *BiliClient {
	client := req.C().
		SetTimeout(300 * time.Second).
		SetDial(func(ctx context.Context, network, addr string) (net.Conn, error) {
			// 为代理连接设置更短的拨号超时时间(30秒)
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, network, addr)
		}).
		ImpersonateChrome()

	if cookies != "" {
		client.SetCommonHeader("Cookie", cookies)
	}

	// 如果提供了代理URL，设置代理
	if proxyURL != "" {
		client.SetProxyURL(proxyURL)
	}

	return &BiliClient{
		AccessKey: accessKey,
		Cookies:   cookies,
		Mid:       mid,
		ReqClient: client,
	}
}

// PreUpload 预上传
func (c *BiliClient) PreUpload(filename string, filesize int64) (*PreUploadResp, error) {
	uploader := NewUposUploader(c)
	return uploader.preUpload(filename, filesize)
}

// PublishVideo 投稿视频
func (c *BiliClient) PublishVideo(title, desc, tags string, tid, copyright int, cover string, videos []PublishVideoPartRequest, source string) (int64, string, error) {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return 0, "", fmt.Errorf("未找到CSRF token (bili_jct)")
	}

	// 对于转载类型，source会由调用方提供（已经处理过模板）

	req := PublishVideoRequest{
		Copyright:    copyright,
		Cover:        cover,
		Desc:         desc,
		DescFormatID: 0,
		Tag:          tags,
		Tid:          tid,
		Title:        title,
		Videos:       videos,
		Source:       source,
		CSRF:         csrf,
		NoReprint:    1,
		OpenElec:     1,
		WebOS:        1,
	}

	// 调试日志：输出videos数组以检查CID
	fmt.Printf("投稿请求 - 视频数量: %d\n", len(videos))
	for i, v := range videos {
		fmt.Printf("  视频[%d]: filename=%s, cid=%d, title=%s\n", i, v.Filename, v.Cid, v.Title)
	}

	var resp PublishResponse

	// 获取buvid（参考biliupforjava的实现）
	buvResp, err := c.GetBuvId()
	if err != nil {
		// buvid获取失败不阻塞，记录日志继续
		fmt.Printf("警告: 获取buvid失败: %v\n", err)
	}

	// 构建完整的Cookie（包含buvid3和buvid4）
	fullCookie := c.Cookies
	if buvResp != nil && buvResp.Data.B3 != "" && buvResp.Data.B4 != "" {
		if !strings.Contains(c.Cookies, "buvid3=") {
			fullCookie += fmt.Sprintf("; buvid3=%s", buvResp.Data.B3)
		}
		if !strings.Contains(c.Cookies, "buvid4=") {
			fullCookie += fmt.Sprintf("; buvid4=%s", buvResp.Data.B4)
		}
	}

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	err = WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitPublish(); err != nil {
			return err
		}

		// 构建URL，添加时间戳和csrf参数（参考biliupforjava）
		apiURL := fmt.Sprintf("https://member.bilibili.com/x/vu/web/add/v3?t=%d&csrf=%s",
			time.Now().UnixMilli(), csrf)

		_, err := c.ReqClient.R().
			SetHeader("Cookie", fullCookie).
			SetHeader("Content-Type", "application/json").
			SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame").
			SetBodyJsonMarshal(req).
			SetSuccessResult(&resp).
			Post(apiURL)
		return err
	})

	if err != nil {
		return 0, "", fmt.Errorf("投稿请求失败: %w", err)
	}

	if resp.Code != 0 {
		return 0, "", fmt.Errorf("投稿失败: %s", resp.Msg)
	}

	// 返回AID和BvID
	return resp.Data.Aid, resp.Data.Bvid, nil
}

// GetSeasons 获取合集列表
func (c *BiliClient) GetSeasons(mid int64) ([]Season, error) {
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/polymer/space/seasons_series_list?mid=%d", mid)

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			ItemsList []Season `json:"items_lists"`
		} `json:"data"`
	}

	_, err := c.ReqClient.R().
		SetSuccessResult(&result).
		Get(apiURL)
	if err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("获取合集失败: %s", result.Msg)
	}

	return result.Data.ItemsList, nil
}

// AddToSeason 将视频加入合集
func (c *BiliClient) AddToSeason(sectionID int64, aid, cid int64, title string) error {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return fmt.Errorf("未找到CSRF token")
	}

	// 构建 episode 数据
	episode := map[string]interface{}{
		"aid":          aid,
		"cid":          cid,
		"title":        title,
		"charging_pay": 0,
	}

	// 构建请求体
	requestBody := map[string]interface{}{
		"csrf":      csrf,
		"sectionId": sectionID,
		"episodes":  []map[string]interface{}{episode},
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}

	apiURL := fmt.Sprintf("https://member.bilibili.com/x2/creative/web/season/section/episodes/add?t=%d&csrf=%s",
		time.Now().UnixMilli(), csrf)

	_, err := c.ReqClient.R().
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame?page_from=creative_home_top_upload").
		SetHeader("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/71.0.3578.98 Safari/537.36").
		SetBody(requestBody).
		SetSuccessResult(&result).
		Post(apiURL)

	if err != nil {
		return fmt.Errorf("加入合集失败: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("加入合集失败: %s", result.Msg)
	}

	return nil
}

// UploadCover 上传封面
// 参考 biliupforjava 实现：使用 base64 编码的 data URI 格式
func (c *BiliClient) UploadCover(imageData []byte) (string, error) {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return "", fmt.Errorf("未找到CSRF token")
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	// 添加csrf参数和适当的请求头
	apiURL := fmt.Sprintf("https://member.bilibili.com/x/vu/web/cover/up?csrf=%s", csrf)

	// 使用 base64 编码的 data URI 格式（参考 biliupforjava）
	// 检测图片类型
	imageType := "image/jpeg"
	if len(imageData) > 3 {
		// PNG: 89 50 4E 47
		if imageData[0] == 0x89 && imageData[1] == 0x50 && imageData[2] == 0x4E && imageData[3] == 0x47 {
			imageType = "image/png"
		}
	}

	// 使用 base64 标准库编码
	base64Data := base64.StdEncoding.EncodeToString(imageData)
	dataURI := fmt.Sprintf("data:%s;base64,%s", imageType, base64Data)

	_, err := c.ReqClient.R().
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame").
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"cover": dataURI,
		}).
		SetSuccessResult(&result).
		Post(apiURL)
	if err != nil {
		return "", fmt.Errorf("请求错误: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("%s", result.Msg)
	}

	return result.Data.URL, nil
}

// IsValidCookie 检查Cookie是否有效
func (c *BiliClient) IsValidCookie() bool {
	valid, _ := ValidateCookie(c.Cookies)
	return valid
}

// GetCSRF 获取CSRF Token
func (c *BiliClient) GetCSRF() string {
	return GetCookieValue(c.Cookies, "bili_jct")
}

// GetBuvId 获取buvid3和buvid4
func (c *BiliClient) GetBuvId() (*BuvIdResponse, error) {
	var resp BuvIdResponse
	_, err := c.ReqClient.R().
		SetSuccessResult(&resp).
		Get("https://api.bilibili.com/x/frontend/finger/spi")
	if err != nil {
		return nil, fmt.Errorf("获取buvid失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("获取buvid失败: %s", resp.Msg)
	}
	return &resp, nil
}

// SendDynamic 发送动态
func (c *BiliClient) SendDynamic(content string) error {
	// B站发送动态API（纯文字动态）
	apiURL := "https://api.vc.bilibili.com/dynamic_svr/v1/dynamic_svr/create"

	data := url.Values{}
	data.Set("dynamic_id", "0")
	data.Set("type", "4") // 4表示纯文字动态
	data.Set("rid", "0")
	data.Set("content", content)
	data.Set("csrf", c.GetCSRF())
	data.Set("csrf_token", c.GetCSRF())

	var result struct {
		Code int                    `json:"code"`
		Msg  string                 `json:"message"`
		Data map[string]interface{} `json:"data"`
	}

	resp, err := c.ReqClient.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetHeader("Referer", "https://t.bilibili.com/").
		SetBodyString(data.Encode()).
		SetSuccessResult(&result).
		Post(apiURL)

	if err != nil {
		return err
	}

	if !resp.IsSuccessState() || result.Code != 0 {
		return fmt.Errorf("发送动态失败: code=%d, msg=%s", result.Code, result.Msg)
	}

	return nil
}

// BuildCookieString 构建Cookie字符串
func BuildCookieString(cookieMap map[string]string) string {
	var parts []string
	for k, v := range cookieMap {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "; ")
}

// Season 合集信息
type Season struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}
