package bili

import (
	"fmt"
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

type PublishVideoRequest struct {
	Copyright    int                       `json:"copyright"`
	Cover        string                    `json:"cover"`
	Desc         string                    `json:"desc"`
	DescFormatID int                       `json:"desc_format_id"`
	Dynamic      string                    `json:"dynamic"`
	Interactive  int                       `json:"interactive"`
	NoReprint    int                       `json:"no_reprint"`
	OpenElec     int                       `json:"open_elec"`
	Source       string                    `json:"source"`
	Tag          string                    `json:"tag"`
	Tid          int                       `json:"tid"`
	Title        string                    `json:"title"`
	Videos       []PublishVideoPartRequest `json:"videos"`
	CSRF         string                    `json:"csrf"`
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

// PreUpload 预上传
func (c *BiliClient) PreUpload(filename string, filesize int64) (*PreUploadResp, error) {
	uploader := NewUposUploader(c)
	return uploader.preUpload(filename, filesize)
}

// PublishVideo 投稿视频
func (c *BiliClient) PublishVideo(title, desc, tags string, tid, copyright int, cover string) (int64, error) {
	csrf := GetCookieValue(c.Cookies, "bili_jct")

	req := PublishVideoRequest{
		Copyright: copyright,
		Cover:     cover,
		Desc:      desc,
		Tag:       tags,
		Tid:       tid,
		Title:     title,
		CSRF:      csrf,
		NoReprint: 1,
		OpenElec:  1,
	}

	var resp PublishResponse

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	err := WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitPublish(); err != nil {
			return err
		}

		_, err := c.ReqClient.R().
			SetBody(req).
			SetSuccessResult(&resp).
			Post("https://member.bilibili.com/x/vu/web/add/v3")
		return err
	})

	if err != nil {
		return 0, fmt.Errorf("投稿请求失败: %w", err)
	}

	if resp.Code != 0 {
		return 0, fmt.Errorf("投稿失败: %s", resp.Msg)
	}

	return resp.Data.Aid, nil
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

// UploadCover 上传封面
func (c *BiliClient) UploadCover(imageData []byte) (string, error) {
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	_, err := c.ReqClient.R().
		SetFileBytes("file", "cover.jpg", imageData).
		SetSuccessResult(&result).
		Post("https://member.bilibili.com/x/vu/web/cover/up")
	if err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("上传封面失败: %s", result.Msg)
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
