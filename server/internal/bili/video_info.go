package bili

import (
	"fmt"
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
	r, err := c.ReqClient.R().
		SetQueryParam("bvid", bvid).
		SetSuccessResult(&resp).
		Get("https://api.bilibili.com/x/web-interface/view")

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

	var resp EditVideoResponse
	r, err := c.ReqClient.R().
		SetBodyJsonMarshal(req).
		SetSuccessResult(&resp).
		Post(fmt.Sprintf("https://member.bilibili.com/x/vu/web/edit?csrf=%s", csrf))

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
