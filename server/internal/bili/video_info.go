package bili

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

// StringSlice 可同时反序列化 B 站 API 中 tag 字段的两种格式：
//   - JSON array:  ["tag1","tag2"]
//   - JSON string: "tag1,tag2"（逗号分隔）
type StringSlice []string

func (s *StringSlice) UnmarshalJSON(data []byte) error {
	// 先尝试 []string
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}
	// 再尝试 string（逗号分隔）
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	if str == "" {
		*s = nil
		return nil
	}
	parts := strings.Split(str, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	*s = result
	return nil
}

// VideoInfo 视频基本信息
type VideoInfo struct {
	Aid      int64  `json:"aid"`
	Bvid     string `json:"bvid"`
	Videos   int    `json:"videos"`
	Tid      int    `json:"tid"`
	Title    string `json:"title"`
	Pic      string `json:"pic"`
	State    int    `json:"state"`    // 视频状态
	Duration int    `json:"duration"` // 总时长(秒)
	Owner    struct {
		Mid  int64  `json:"mid"`
		Name string `json:"name"`
		Face string `json:"face"`
	} `json:"owner"` // 视频所有者（UP主）信息
	Pages []struct {
		CID      int64  `json:"cid"`
		Page     int    `json:"page"`
		Part     string `json:"part"`
		Duration int    `json:"duration"`
	} `json:"pages"`
}

type VideoInfoResponse struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    VideoInfo `json:"data"`
}

// GetVideoInfo 获取视频信息
func (c *BiliClient) GetVideoInfo(bvid string) (*VideoInfo, error) {
	var resp VideoInfoResponse
	apiURL := "https://api.bilibili.com/x/web-interface/view"

	// 构建请求，带上Cookie获取更准确的状态信息
	req := c.ReqClient.R().
		SetQueryParam("bvid", bvid).
		SetSuccessResult(&resp)

	// 如果有Cookie，添加buvid以获取更准确的状态
	if c.Cookies != "" {
		// 获取buvid
		buvid, err := GetBuvid()
		if err == nil && buvid != nil {
			// 添加buvid到Cookie中
			cookieStr := c.Cookies
			if buvid.Data.B3 != "" {
				cookieStr += ";buvid3=" + buvid.Data.B3
			}
			if buvid.Data.B4 != "" {
				cookieStr += ";buvid4=" + buvid.Data.B4
			}
			req.SetHeader("Cookie", cookieStr)
		}
	}

	r, err := req.Get(apiURL)

	if err != nil {
		logBiliRequestError("获取视频信息", "GET", apiURL, err)
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("获取视频信息", "GET", apiURL, r)
		return nil, fmt.Errorf("获取视频信息失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		logBiliAPIError("获取视频信息", "GET", apiURL, resp.Code, resp.Message, r)
		return nil, fmt.Errorf("获取视频信息失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return &resp.Data, nil
}

// GetVideoInfoByAid 通过aid获取视频信息
func (c *BiliClient) GetVideoInfoByAid(aid int64) (*VideoInfo, error) {
	var resp VideoInfoResponse
	apiURL := "https://api.bilibili.com/x/web-interface/view"

	// 构建请求，带上Cookie获取更准确的状态信息
	req := c.ReqClient.R().
		SetQueryParam("aid", fmt.Sprintf("%d", aid)).
		SetSuccessResult(&resp)

	// 如果有Cookie，添加buvid以获取更准确的状态
	if c.Cookies != "" {
		// 获取buvid
		buvid, err := GetBuvid()
		if err == nil && buvid != nil {
			// 添加buvid到Cookie中
			cookieStr := c.Cookies
			if buvid.Data.B3 != "" {
				cookieStr += ";buvid3=" + buvid.Data.B3
			}
			if buvid.Data.B4 != "" {
				cookieStr += ";buvid4=" + buvid.Data.B4
			}
			req.SetHeader("Cookie", cookieStr)
		}
	}

	r, err := req.Get(apiURL)

	if err != nil {
		logBiliRequestError("获取视频信息", "GET", apiURL, err)
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("获取视频信息", "GET", apiURL, r)
		return nil, fmt.Errorf("获取视频信息失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		logBiliAPIError("获取视频信息", "GET", apiURL, resp.Code, resp.Message, r)
		return nil, fmt.Errorf("获取视频信息失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return &resp.Data, nil
}

// VideoArchiveDetail 视频稿件详细信息（member API）
type VideoArchiveDetail struct {
	Archive struct {
		Aid       int64       `json:"aid"`
		Bvid      string      `json:"bvid"`
		Title     string      `json:"title"`
		Desc      string      `json:"desc"`
		Copyright int         `json:"copyright"`
		Source    string      `json:"source"`
		Tid       int         `json:"tid"`
		Pic       string      `json:"cover"`
		Tag       StringSlice `json:"tag"`
	} `json:"archive"`
	Videos []struct {
		Aid        int64  `json:"aid"`
		Bvid       string `json:"bvid"`
		Title      string `json:"title"`
		Filename   string `json:"filename"`
		CID        int64  `json:"cid"`
		Ctime      int64  `json:"ctime"`
		FailCode   int    `json:"failCode"`
		XcodeState int    `json:"xcodeState"` // 转码状态
		FailDesc   string `json:"failDesc"`
		Page       int    `json:"page"`
		Part       string `json:"part"`
		Duration   int    `json:"duration"`
	} `json:"videos"`
}

type VideoArchiveDetailResponse struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    VideoArchiveDetail `json:"data"`
}

// GetArchiveDetailByAid 获取视频稿件详细信息（包含desc, tag, copyright, source等）
func (c *BiliClient) GetArchiveDetailByAid(aid int64) (*VideoArchiveDetail, error) {
	var resp VideoArchiveDetailResponse
	apiURL := "https://member.bilibili.com/x/vupre/web/archive/view"
	r, err := c.ReqClient.R().
		SetQueryParams(map[string]string{
			"aid":        fmt.Sprintf("%d", aid),
			"topic_grey": "1",
		}).
		SetSuccessResult(&resp).
		Get(apiURL)

	if err != nil {
		logBiliRequestError("获取稿件详细信息", "GET", apiURL, err)
		return nil, fmt.Errorf("获取稿件详细信息失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("获取稿件详细信息", "GET", apiURL, r)
		return nil, fmt.Errorf("获取稿件详细信息失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		logBiliAPIError("获取稿件详细信息", "GET", apiURL, resp.Code, resp.Message, r)
		return nil, fmt.Errorf("获取稿件详细信息失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return &resp.Data, nil
}

// VideoPartInfo 分P详细信息（兼容旧代码）
type VideoPartInfo struct {
	State  int `json:"state"`
	Videos []struct {
		Aid        int64  `json:"aid"`
		Bvid       string `json:"bvid"`
		Title      string `json:"title"`
		Filename   string `json:"filename"`
		CID        int64  `json:"cid"`
		Ctime      int64  `json:"ctime"`
		FailCode   int    `json:"failCode"`
		XcodeState int    `json:"xcodeState"` // 转码状态
		FailDesc   string `json:"failDesc"`
		Page       int    `json:"page"`
		Part       string `json:"part"`
		Duration   int    `json:"duration"`
	} `json:"videos"`
}

type VideoPartInfoResponse struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    VideoPartInfo `json:"data"`
}

// GetVideoPartInfo 获取视频分P详细信息（需要登录）
func (c *BiliClient) GetVideoPartInfo(bvid string) (*VideoPartInfo, error) {
	var resp VideoPartInfoResponse
	apiURL := "https://member.bilibili.com/x/vupre/web/archive/view"
	r, err := c.ReqClient.R().
		SetQueryParams(map[string]string{
			"bvid":       bvid,
			"topic_grey": "1",
		}).
		SetSuccessResult(&resp).
		Get(apiURL)

	if err != nil {
		logBiliRequestError("获取分P信息", "GET", apiURL, err)
		return nil, fmt.Errorf("获取分P信息失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("获取分P信息", "GET", apiURL, r)
		return nil, fmt.Errorf("获取分P信息失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		logBiliAPIError("获取分P信息", "GET", apiURL, resp.Code, resp.Message, r)
		return nil, fmt.Errorf("获取分P信息失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return &resp.Data, nil
}

// EditVideo 编辑视频信息
type EditVideoRequest struct {
	Aid        int64                     `json:"aid"`
	Copyright  int                       `json:"copyright"`
	Cover      string                    `json:"cover"`
	Desc       string                    `json:"desc"`
	Dynamic    string                    `json:"dynamic"`
	NoReprint  int                       `json:"no_reprint"`
	Source     string                    `json:"source"`
	Tag        string                    `json:"tag"`
	Tid        int                       `json:"tid"`
	Title      string                    `json:"title"`
	Videos     []PublishVideoPartRequest `json:"videos"`
	CSRF       string                    `json:"csrf"`
	IsOnlySelf int                       `json:"is_only_self"` // 是否仅自己可见
}

type EditVideoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// EditVideo 编辑已发布的视频
func (c *BiliClient) EditVideo(aid int64, title, desc, tags string, tid, copyright int, cover string, videos []PublishVideoPartRequest, source string) error {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return fmt.Errorf("未找到CSRF token")
	}

	req := EditVideoRequest{
		Aid:       aid,
		Copyright: copyright,
		Source:    source,
		Cover:     cover,
		Desc:      desc,
		Tag:       tags,
		Tid:       tid,
		Title:     title,
		Videos:    videos,
		CSRF:      csrf,
	}

	// 构建URL，添加时间戳和csrf参数（参考biliupforjava）
	apiURL := fmt.Sprintf("https://member.bilibili.com/x/vu/web/edit?t=%d&csrf=%s",
		time.Now().UnixMilli(), csrf)

	var resp EditVideoResponse
	r, err := c.ReqClient.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame").
		SetBodyJsonMarshal(req).
		SetSuccessResult(&resp).
		Post(apiURL)

	if err != nil {
		logBiliRequestError("编辑视频", "POST", apiURL, err)
		return fmt.Errorf("编辑视频失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("编辑视频", "POST", apiURL, r)
		return fmt.Errorf("编辑视频失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		logBiliAPIError("编辑视频", "POST", apiURL, resp.Code, resp.Message, r)
		return fmt.Errorf("编辑视频失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return nil
}

// UpdateVideoVisibility 更新视频可见性
func (c *BiliClient) UpdateVideoVisibility(aid int64, isOnlySelf bool) error {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return fmt.Errorf("未找到CSRF token")
	}

	onlySelfValue := 0
	if isOnlySelf {
		onlySelfValue = 1
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	apiURL := "https://member.bilibili.com/x/vu/web/edit/visibility"
	r, err := c.ReqClient.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame").
		SetFormData(map[string]string{
			"aid":          fmt.Sprintf("%d", aid),
			"is_only_self": fmt.Sprintf("%d", onlySelfValue),
			"csrf":         csrf,
		}).
		SetSuccessResult(&resp).
		Post(apiURL)

	if err != nil {
		logBiliRequestError("更新可见性", "POST", apiURL, err)
		return fmt.Errorf("更新可见性失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("更新可见性", "POST", apiURL, r)
		return fmt.Errorf("更新可见性失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		logBiliAPIError("更新可见性", "POST", apiURL, resp.Code, resp.Message, r)
		return fmt.Errorf("更新可见性失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return nil
}

// UserArchive 用户投稿视频
type UserArchive struct {
	Aid   int64  `json:"aid"`
	Bvid  string `json:"bvid"`
	Title string `json:"title"`
}

type UserArchiveListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		List struct {
			Vlist []UserArchive `json:"vlist"`
		} `json:"list"`
	} `json:"data"`
}

// UAPIArchive UAPI接口的投稿视频信息
type UAPIArchive struct {
	Aid         int64  `json:"aid"`
	Bvid        string `json:"bvid"`
	Title       string `json:"title"`
	Cover       string `json:"cover"`
	Duration    int    `json:"duration"`
	PlayCount   int    `json:"play_count"`
	PublishTime int64  `json:"publish_time"`
	CreateTime  int64  `json:"create_time"`
	State       int    `json:"state"`
	IsUgcPay    int    `json:"is_ugc_pay"`
}

// UAPIArchiveListResponse UAPI接口响应
type UAPIArchiveListResponse struct {
	Total  int           `json:"total"`
	Page   int           `json:"page"`
	Size   int           `json:"size"`
	Videos []UAPIArchive `json:"videos"`
}

// GetUserArchiveList 获取用户投稿列表（使用UAPI接口）
func (c *BiliClient) GetUserArchiveList(mid int64, pn, ps int) ([]UserArchive, error) {
	var resp UAPIArchiveListResponse
	apiURL := "https://uapis.cn/api/v1/social/bilibili/archives"

	r, err := c.ReqClient.R().
		SetQueryParams(map[string]string{
			"mid": fmt.Sprintf("%d", mid),
		}).
		SetSuccessResult(&resp).
		Get(apiURL)

	if err != nil {
		logBiliRequestError("获取用户投稿列表", "GET", apiURL, err)
		return nil, fmt.Errorf("获取用户投稿列表失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("获取用户投稿列表", "GET", apiURL, r)
		return nil, fmt.Errorf("获取用户投稿列表失败: HTTP %d", r.StatusCode)
	}

	// 转换为UserArchive格式
	var archives []UserArchive
	for _, v := range resp.Videos {
		archives = append(archives, UserArchive{
			Aid:   v.Aid,
			Bvid:  v.Bvid,
			Title: v.Title,
		})
	}

	return archives, nil
}

// GetBvidByAid 通过AID获取对应的BVID
func (c *BiliClient) GetBvidByAid(mid int64, aid int64) (string, error) {
	// 直接使用视频信息API获取BVID，无需查询投稿列表
	videoInfo, err := c.GetVideoInfoByAid(aid)
	if err != nil {
		return "", fmt.Errorf("获取视频信息失败: %w", err)
	}

	if videoInfo.Bvid == "" {
		return "", fmt.Errorf("视频信息中未包含BVID (AID=%d)", aid)
	}

	return videoInfo.Bvid, nil
}

// CheckVideoExistsInArchive 检查视频是否存在于用户投稿列表中（兜底检测）
// 用于在投稿后验证视频是否真的提交成功
func (c *BiliClient) CheckVideoExistsInArchive(mid int64, aid int64, bvid string) (bool, error) {
	var resp UAPIArchiveListResponse
	apiURL := "https://uapis.cn/api/v1/social/bilibili/archives"

	// 使用新的UAPI接口获取用户投稿列表
	r, err := c.ReqClient.R().
		SetQueryParams(map[string]string{
			"mid": fmt.Sprintf("%d", mid),
		}).
		SetSuccessResult(&resp).
		Get(apiURL)

	if err != nil {
		logBiliRequestError("获取用户投稿列表", "GET", apiURL, err)
		return false, fmt.Errorf("获取用户投稿列表失败: %w", err)
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("获取用户投稿列表", "GET", apiURL, r)
		return false, fmt.Errorf("获取用户投稿列表失败: HTTP %d", r.StatusCode)
	}

	// 检查视频是否在列表中
	for _, v := range resp.Videos {
		if (aid > 0 && v.Aid == aid) || (bvid != "" && v.Bvid == bvid) {
			return true, nil
		}
	}

	return false, nil
}

// GetBuvid 获取buvid
func GetBuvid() (*BuvIdResponse, error) {
	var resp BuvIdResponse
	apiURL := "https://api.bilibili.com/x/frontend/finger/spi"

	// 创建新的req客户端
	client := req.C().
		SetCommonHeader("Referer", "https://live.bilibili.com/").
		SetCommonHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	r, err := client.R().
		SetSuccessResult(&resp).
		Get(apiURL)

	if err != nil {
		logBiliRequestError("获取buvid", "GET", apiURL, err)
		return nil, err
	}

	if !r.IsSuccessState() {
		logBiliHTTPError("获取buvid", "GET", apiURL, r)
		return nil, fmt.Errorf("获取buvid失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		logBiliAPIError("获取buvid", "GET", apiURL, resp.Code, resp.Msg, r)
		return nil, fmt.Errorf("获取buvid失败: code=%d", resp.Code)
	}

	return &resp, nil
}
