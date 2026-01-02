package bili

import (
	"fmt"
	"time"

	"github.com/imroc/req/v3"
)

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
	Pages    []struct {
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

	r, err := req.Get("https://api.bilibili.com/x/web-interface/view")

	if err != nil {
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	if !r.IsSuccessState() {
		return nil, fmt.Errorf("获取视频信息失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("获取视频信息失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return &resp.Data, nil
}

// GetVideoInfoByAid 通过aid获取视频信息
func (c *BiliClient) GetVideoInfoByAid(aid int64) (*VideoInfo, error) {
	var resp VideoInfoResponse

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

	r, err := req.Get("https://api.bilibili.com/x/web-interface/view")

	if err != nil {
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	if !r.IsSuccessState() {
		return nil, fmt.Errorf("获取视频信息失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("获取视频信息失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return &resp.Data, nil
}

// VideoPartInfo 分P详细信息
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
	r, err := c.ReqClient.R().
		SetQueryParams(map[string]string{
			"bvid":       bvid,
			"topic_grey": "1",
		}).
		SetSuccessResult(&resp).
		Get("https://member.bilibili.com/x/vupre/web/archive/view")

	if err != nil {
		return nil, fmt.Errorf("获取分P信息失败: %w", err)
	}

	if !r.IsSuccessState() {
		return nil, fmt.Errorf("获取分P信息失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
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
func (c *BiliClient) EditVideo(aid int64, title, desc, tags string, tid int, cover string, videos []PublishVideoPartRequest) error {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return fmt.Errorf("未找到CSRF token")
	}

	req := EditVideoRequest{
		Aid:       aid,
		Copyright: 1,
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
		return fmt.Errorf("编辑视频失败: %w", err)
	}

	if !r.IsSuccessState() {
		return fmt.Errorf("编辑视频失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
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

	r, err := c.ReqClient.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame").
		SetFormData(map[string]string{
			"aid":          fmt.Sprintf("%d", aid),
			"is_only_self": fmt.Sprintf("%d", onlySelfValue),
			"csrf":         csrf,
		}).
		SetSuccessResult(&resp).
		Post("https://member.bilibili.com/x/vu/web/edit/visibility")

	if err != nil {
		return fmt.Errorf("更新可见性失败: %w", err)
	}

	if !r.IsSuccessState() {
		return fmt.Errorf("更新可见性失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
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

// GetUserArchiveList 获取用户投稿列表
func (c *BiliClient) GetUserArchiveList(mid int64, pn, ps int) ([]UserArchive, error) {
	var resp UserArchiveListResponse

	r, err := c.ReqClient.R().
		SetQueryParams(map[string]string{
			"mid": fmt.Sprintf("%d", mid),
			"pn":  fmt.Sprintf("%d", pn),
			"ps":  fmt.Sprintf("%d", ps),
		}).
		SetSuccessResult(&resp).
		Get("https://api.bilibili.com/x/space/wbi/arc/search")

	if err != nil {
		return nil, fmt.Errorf("获取用户投稿列表失败: %w", err)
	}

	if !r.IsSuccessState() {
		return nil, fmt.Errorf("获取用户投稿列表失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("获取用户投稿列表失败: %s (code=%d)", resp.Message, resp.Code)
	}

	return resp.Data.List.Vlist, nil
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

// GetBuvid 获取buvid
func GetBuvid() (*BuvIdResponse, error) {
	var resp BuvIdResponse

	// 创建新的req客户端
	client := req.C().
		SetCommonHeader("Referer", "https://live.bilibili.com/").
		SetCommonHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	r, err := client.R().
		SetSuccessResult(&resp).
		Get("https://api.bilibili.com/x/frontend/finger/spi")

	if err != nil {
		return nil, err
	}

	if !r.IsSuccessState() {
		return nil, fmt.Errorf("获取buvid失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("获取buvid失败: code=%d", resp.Code)
	}

	return &resp, nil
}
