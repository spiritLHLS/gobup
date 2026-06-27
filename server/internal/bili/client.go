package bili

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gobup/server/internal/ratelimit"
	"github.com/imroc/req/v3"
	"github.com/wzshiming/socks5"
)

type BiliClient struct {
	AccessKey         string
	AccessToken       string
	Cookies           string
	Mid               int64
	Line              string // 上传线路，如 cs_txa, cs_bda2
	ReqClient         *req.Client
	UploadRateLimiter *ratelimit.RateLimiter
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
		SetTimeout(uploadRequestTimeout()).            // 适配大文件上传，可通过 GOBUP_UPLOAD_TIMEOUT_MINUTES 调整
		SetCommonRetryCount(0).                        // 禁用自动重试，由业务层控制
		EnableKeepAlives().                            // 启用连接保持
		SetTLSHandshakeTimeout(tlsHandshakeTimeout()). // TLS握手超时
		SetCommonRetryCondition(func(_ *req.Response, _ error) bool {
			return false // 禁用自动重试，避免并发冲突
		}).
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
		SetTimeout(uploadRequestTimeout()).            // 适配大文件上传，可通过 GOBUP_UPLOAD_TIMEOUT_MINUTES 调整
		SetTLSHandshakeTimeout(tlsHandshakeTimeout()). // TLS握手超时
		ImpersonateChrome().
		DisableKeepAlives(). // 禁用连接复用，避免EOF错误
		SetCommonRetryCondition(func(_ *req.Response, _ error) bool {
			return false // 禁用自动重试，避免并发时代理失效 (ref: https://github.com/imroc/req/issues/445)
		})

	if cookies != "" {
		client.SetCommonHeader("Cookie", cookies)
	}

	// 如果提供了代理URL，使用wzshiming/socks5处理socks5代理，避免EOF错误
	// (ref: https://github.com/imroc/req/issues/473, https://github.com/imroc/req/issues/272)
	if proxyURL != "" {
		if strings.HasPrefix(proxyURL, "socks5://") {
			// 使用 wzshiming/socks5 库处理 socks5 代理
			socks5Dialer, err := socks5.NewDialer(proxyURL)
			if err == nil {
				// 设置自定义拨号器
				client.SetDial(func(ctx context.Context, network, addr string) (net.Conn, error) {
					return socks5Dialer.DialContext(ctx, network, addr)
				})
			}
		} else {
			// 非socks5代理使用原生SetProxyURL
			client.SetProxyURL(proxyURL)
		}
	}

	return &BiliClient{
		AccessKey: accessKey,
		Cookies:   cookies,
		Mid:       mid,
		ReqClient: client,
	}
}

func uploadRequestTimeout() time.Duration {
	return envDuration("GOBUP_UPLOAD_TIMEOUT_MINUTES", 30*time.Minute, time.Minute, 1, 24*60)
}

func tlsHandshakeTimeout() time.Duration {
	return envDuration("GOBUP_TLS_HANDSHAKE_TIMEOUT_SECONDS", 30*time.Second, time.Second, 1, 300)
}

func envDuration(name string, fallback, unit time.Duration, minValue, maxValue int64) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < minValue {
		return fallback
	}
	if maxValue > 0 && parsed > maxValue {
		parsed = maxValue
	}
	return time.Duration(parsed) * unit
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

	publishReq := PublishVideoRequest{
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

	fullCookie := c.cookieWithBuvID()

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	apiURL := ""
	var lastResp *req.Response
	err := WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitPublish(); err != nil {
			return err
		}

		// 构建URL，添加时间戳和csrf参数（参考biliupforjava）
		apiURL = fmt.Sprintf("https://member.bilibili.com/x/vu/web/add/v3?t=%d&csrf=%s",
			time.Now().UnixMilli(), csrf)

		r, err := c.ReqClient.R().
			SetHeader("Cookie", fullCookie).
			SetHeader("Content-Type", "application/json").
			SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame").
			SetBodyJsonMarshal(publishReq).
			SetSuccessResult(&resp).
			Post(apiURL)
		lastResp = r
		if err != nil {
			logBiliRequestError("投稿", "POST", apiURL, err)
			return err
		}
		if !r.IsSuccessState() {
			logBiliHTTPError("投稿", "POST", apiURL, r)
			return wrapRetryAfterError(fmt.Errorf("投稿HTTP错误: status=%d", r.GetStatusCode()), r.GetHeader("Retry-After"), time.Now())
		}
		return nil
	})

	if err != nil {
		return 0, "", fmt.Errorf("投稿请求失败: %w", err)
	}

	if resp.Code != 0 {
		logBiliAPIError("投稿", "POST", apiURL, resp.Code, resp.Msg, lastResp)
		// 检查是否是"稿件已成功投稿，请勿重新提交"的错误
		// 这种情况说明投稿实际上已经成功了，需要从用户投稿列表中查找视频信息
		if strings.Contains(resp.Msg, "稿件已成功投稿") || strings.Contains(resp.Msg, "请勿重新提交") {
			log.Printf("[投稿] 检测到重复投稿错误，尝试从投稿列表中查找视频: %s", resp.Msg)

			// 从错误信息中提取稿件名
			// 格式: "稿件已成功投稿，请勿重新提交哦～\n提交时间：03:54 稿件名《xxx》"
			titleFromError := extractTitleFromDuplicateError(resp.Msg)
			if titleFromError == "" {
				// 如果无法从错误信息提取标题，使用请求的标题
				titleFromError = title
			}
			log.Printf("[投稿] 从错误信息中提取的稿件名: %s", titleFromError)

			// 等待一下让B站处理完成
			time.Sleep(2 * time.Second)

			// 获取用户投稿列表，查找匹配的视频
			archives, err := c.GetUserArchiveList(c.Mid, 1, 20)
			if err != nil {
				log.Printf("[投稿] 获取用户投稿列表失败: %v，返回原始错误", err)
				return 0, "", fmt.Errorf("投稿失败: %s", resp.Msg)
			}

			// 查找标题匹配的视频（从最新的开始查找）
			for _, archive := range archives {
				if archive.Title == titleFromError || archive.Title == title {
					log.Printf("[投稿] ✓ 找到匹配的视频: AID=%d, BVID=%s, Title=%s",
						archive.Aid, archive.Bvid, archive.Title)
					return archive.Aid, archive.Bvid, nil
				}
			}

			log.Printf("[投稿] 未找到匹配的视频，返回原始错误")
		}

		return 0, "", fmt.Errorf("投稿失败: %s", resp.Msg)
	}

	// 返回AID和BvID
	return resp.Data.Aid, resp.Data.Bvid, nil
}

const (
	creativeSeasonListURL  = "https://member.bilibili.com/x2/creative/web/seasons"
	seasonListPageSize     = 50
	seasonListMaxPageCount = 20
)

type creativeSeasonListResult struct {
	Code   int    `json:"code"`
	Msg    string `json:"message"`
	AltMsg string `json:"msg"`
	Data   struct {
		Seasons []creativeSeasonListItem `json:"seasons"`
		Total   int                      `json:"total"`
		Items   []legacySeasonListItem   `json:"items"`
	} `json:"data"`
}

type creativeSeasonListItem struct {
	Season struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Name   string `json:"name"`
		EpNum  int    `json:"ep_num"`
		State  int    `json:"state"`
		Forbid int    `json:"forbid"`
	} `json:"season"`
	Sections struct {
		Sections []creativeSeasonSection `json:"sections"`
	} `json:"sections"`
	PartEpisodes []struct{} `json:"part_episodes"`
}

type creativeSeasonSection struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	State     int    `json:"state"`
	PartState int    `json:"partState"`
	EpCount   int    `json:"epCount"`
}

type legacySeasonListItem struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Total int    `json:"total"`
	Meta  struct {
		Name  string `json:"name"`
		Total int    `json:"total"`
	} `json:"meta"`
	Sections []struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"sections"`
}

// GetSeasons 获取合集列表（使用创作中心 API，需要登录态）
func (c *BiliClient) GetSeasons() ([]Season, error) {
	fullCookie := c.cookieWithBuvID()
	var seasons []Season
	seen := make(map[int64]struct{})
	total := 0

	for page := 1; page <= seasonListMaxPageCount; page++ {
		pageItems, pageTotal, err := c.getSeasonPage(page, seasonListPageSize, fullCookie)
		if err != nil {
			return nil, err
		}
		if pageTotal > 0 {
			total = pageTotal
		}
		before := len(seasons)
		for _, item := range pageItems {
			if item.ID == 0 {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			seasons = append(seasons, item)
		}
		if len(pageItems) == 0 || len(seasons) == before {
			break
		}
		if total > 0 && len(seasons) >= total {
			break
		}
		if len(pageItems) < seasonListPageSize {
			break
		}
	}
	return seasons, nil
}

func (c *BiliClient) getSeasonPage(page, pageSize int, fullCookie string) ([]Season, int, error) {
	apiURL := fmt.Sprintf("%s?pn=%d&ps=%d&order=mtime&sort=desc", creativeSeasonListURL, page, pageSize)
	resp, err := c.ReqClient.R().
		SetHeader("Cookie", fullCookie).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Origin", "https://member.bilibili.com").
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame?page_from=creative_home_top_upload").
		SetHeader("X-Requested-With", "XMLHttpRequest").
		Get(apiURL)
	if err != nil {
		logBiliRequestError("获取合集", "GET", apiURL, err)
		return nil, 0, err
	}
	if !resp.IsSuccessState() {
		logBiliHTTPError("获取合集", "GET", apiURL, resp)
		return nil, 0, fmt.Errorf("获取合集失败: HTTP %d", resp.GetStatusCode())
	}
	seasons, total, err := parseSeasonList(resp.Bytes())
	if err != nil {
		log.Printf("[B站接口] 获取合集响应解析失败: %v", err)
		return nil, 0, err
	}
	return seasons, total, nil
}

func parseSeasonList(body []byte) ([]Season, int, error) {
	var result creativeSeasonListResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, 0, fmt.Errorf("获取合集失败: 响应不是有效JSON: %w", err)
	}
	if result.Code != 0 {
		msg := result.Msg
		if msg == "" {
			msg = result.AltMsg
		}
		if msg == "" {
			msg = "请求错误"
		}
		return nil, 0, fmt.Errorf("获取合集失败: code=%d, msg=%s", result.Code, msg)
	}

	seasons := parseCreativeSeasons(result.Data.Seasons)
	if len(seasons) == 0 && len(result.Data.Items) > 0 {
		seasons = parseLegacySeasons(result.Data.Items)
	}
	return seasons, result.Data.Total, nil
}

func parseCreativeSeasons(items []creativeSeasonListItem) []Season {
	seasons := make([]Season, 0, len(items))
	for _, item := range items {
		id := item.Season.ID
		name := strings.TrimSpace(item.Season.Title)
		if name == "" {
			name = strings.TrimSpace(item.Season.Name)
		}
		if id == 0 || name == "" {
			continue
		}

		count := item.Season.EpNum
		if count == 0 {
			for _, section := range item.Sections.Sections {
				count += section.EpCount
			}
		}
		if count == 0 && item.PartEpisodes != nil {
			count = len(item.PartEpisodes)
		}

		seasons = append(seasons, Season{
			ID:        id,
			Name:      name,
			Count:     count,
			SectionID: preferredSeasonSectionID(item.Sections.Sections),
		})
	}
	return seasons
}

func parseLegacySeasons(items []legacySeasonListItem) []Season {
	seasons := make([]Season, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Meta.Name)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		total := item.Meta.Total
		if total == 0 {
			total = item.Total
		}
		if item.ID == 0 || name == "" {
			continue
		}
		season := Season{
			ID:    item.ID,
			Name:  name,
			Count: total,
		}
		for _, section := range item.Sections {
			if section.ID > 0 {
				season.SectionID = section.ID
				break
			}
		}
		seasons = append(seasons, season)
	}
	return seasons
}

func preferredSeasonSectionID(sections []creativeSeasonSection) int64 {
	var fallback int64
	for _, section := range sections {
		if section.ID <= 0 {
			continue
		}
		if fallback == 0 {
			fallback = section.ID
		}
		if section.State == 0 && section.PartState == 0 {
			return section.ID
		}
	}
	return fallback
}

func (c *BiliClient) ResolveSeasonSectionID(rawID int64) (int64, error) {
	if rawID <= 0 {
		return 0, nil
	}
	seasons, err := c.GetSeasons()
	if err != nil {
		return rawID, err
	}
	for _, season := range seasons {
		if season.SectionID == rawID {
			return rawID, nil
		}
		if season.ID == rawID {
			if season.SectionID <= 0 {
				return 0, fmt.Errorf("合集 %d 未返回可用小节ID", rawID)
			}
			return season.SectionID, nil
		}
	}
	return rawID, nil
}

// CreateSeason 创建合集，并返回可用于后续加入投稿的小节信息。
func (c *BiliClient) CreateSeason(title, desc, cover string) (*Season, error) {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return nil, fmt.Errorf("未找到CSRF token")
	}

	title = strings.TrimSpace(title)
	desc = strings.TrimSpace(desc)
	cover = strings.TrimSpace(cover)
	if title == "" {
		return nil, fmt.Errorf("合集标题不能为空")
	}

	if existing, err := c.findSeasonByTitle(title); err == nil && existing != nil {
		return existing, nil
	}
	if cover == "" {
		coverData, coverErr := defaultSeasonCoverPNG()
		if coverErr != nil {
			return nil, fmt.Errorf("生成默认合集封面失败: %w", coverErr)
		}
		uploadedCover, uploadErr := c.UploadCover(coverData)
		if uploadErr != nil {
			return nil, fmt.Errorf("合集封面为空，默认封面上传失败: %w", uploadErr)
		}
		cover = uploadedCover
		log.Printf("[合集] 未提供封面，已自动上传默认封面: %s", cover)
	}

	var result struct {
		Code    int             `json:"code"`
		Msg     string          `json:"message"`
		AltMsg  string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
		TraceID string          `json:"trace_id"`
	}

	form := url.Values{}
	form.Set("title", title)
	form.Set("desc", desc)
	form.Set("cover", cover)
	form.Set("season_price", "0")
	form.Set("csrf", csrf)

	apiURL := "https://member.bilibili.com/x2/creative/web/season/add"
	resp, err := c.ReqClient.R().
		SetHeader("Cookie", c.cookieWithBuvID()).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Origin", "https://member.bilibili.com").
		SetHeader("Referer", "https://member.bilibili.com/platform/upload-manager/collection").
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetHeader("X-Requested-With", "XMLHttpRequest").
		SetFormDataFromValues(form).
		SetSuccessResult(&result).
		Post(apiURL)
	if err != nil {
		logBiliRequestError("创建合集", "POST", apiURL, err)
		return nil, fmt.Errorf("创建合集请求失败: %w", err)
	}
	if !resp.IsSuccessState() {
		logBiliHTTPError("创建合集", "POST", apiURL, resp)
		return nil, fmt.Errorf("创建合集HTTP错误: status=%d", resp.GetStatusCode())
	}
	if result.Code != 0 {
		msg := result.Msg
		if msg == "" {
			msg = result.AltMsg
		}
		if msg == "" {
			msg = "请求错误"
		}
		logBiliAPIError("创建合集", "POST", apiURL, result.Code, msg, resp)
		if existing, findErr := c.findSeasonByTitle(title); findErr == nil && existing != nil {
			return existing, nil
		}
		if result.TraceID != "" {
			return nil, fmt.Errorf("创建合集失败: code=%d, msg=%s, trace_id=%s", result.Code, msg, result.TraceID)
		}
		return nil, fmt.Errorf("创建合集失败: code=%d, msg=%s", result.Code, msg)
	}
	seasonID, parseErr := parseSeasonID(result.Data)
	if parseErr != nil {
		if existing, findErr := c.findSeasonByTitle(title); findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("创建合集失败: 响应数据异常: %w", parseErr)
	}
	if seasonID == 0 {
		if existing, findErr := c.findSeasonByTitle(title); findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("创建合集失败: 响应未返回合集ID")
	}

	season := &Season{
		ID:   seasonID,
		Name: title,
	}

	// 创建接口只返回合集ID；再拉取一次列表，补齐小节ID，供 AddToSeason 使用。
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		seasons, err := c.GetSeasons()
		if err != nil {
			continue
		}
		for _, item := range seasons {
			if item.ID == seasonID {
				season = &item
				return season, nil
			}
		}
	}
	return season, nil
}

func (c *BiliClient) findSeasonByTitle(title string) (*Season, error) {
	seasons, err := c.GetSeasons()
	if err != nil {
		return nil, err
	}
	normalizedTitle := strings.TrimSpace(title)
	for _, season := range seasons {
		if strings.TrimSpace(season.Name) == normalizedTitle {
			item := season
			return &item, nil
		}
	}
	return nil, nil
}

func parseSeasonID(data json.RawMessage) (int64, error) {
	if len(data) == 0 || string(data) == "null" {
		return 0, nil
	}
	var id int64
	if err := json.Unmarshal(data, &id); err == nil {
		return id, nil
	}
	var dataObj struct {
		ID       int64 `json:"id"`
		SeasonID int64 `json:"season_id"`
	}
	if err := json.Unmarshal(data, &dataObj); err != nil {
		return 0, err
	}
	if dataObj.ID != 0 {
		return dataObj.ID, nil
	}
	return dataObj.SeasonID, nil
}

// AddToSeason 将视频加入合集
func (c *BiliClient) AddToSeason(sectionID int64, aid, cid int64, title string) error {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return fmt.Errorf("未找到CSRF token")
	}
	if sectionID <= 0 {
		return fmt.Errorf("合集小节ID不能为空")
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
		"csrf":       csrf,
		"section_id": sectionID,
		"sectionId":  sectionID,
		"episode":    []map[string]interface{}{episode},
		"episodes":   []map[string]interface{}{episode},
	}

	var result struct {
		Code   int    `json:"code"`
		Msg    string `json:"message"`
		AltMsg string `json:"msg"`
	}

	apiURL := fmt.Sprintf("https://member.bilibili.com/x2/creative/web/season/section/episodes/add?t=%d&csrf=%s",
		time.Now().UnixMilli(), csrf)

	resp, err := c.ReqClient.R().
		SetHeader("Cookie", c.cookieWithBuvID()).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Origin", "https://member.bilibili.com").
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame?page_from=creative_home_top_upload").
		SetHeader("Content-Type", "application/json").
		SetHeader("X-Requested-With", "XMLHttpRequest").
		SetBodyJsonMarshal(requestBody).
		SetSuccessResult(&result).
		Post(apiURL)

	if err != nil {
		logBiliRequestError("加入合集", "POST", apiURL, err)
		return fmt.Errorf("加入合集失败: %w", err)
	}
	if !resp.IsSuccessState() {
		logBiliHTTPError("加入合集", "POST", apiURL, resp)
		return fmt.Errorf("加入合集失败: HTTP %d", resp.GetStatusCode())
	}

	if result.Code != 0 {
		msg := result.Msg
		if msg == "" {
			msg = result.AltMsg
		}
		if msg == "" {
			msg = "请求错误"
		}
		logBiliAPIError("加入合集", "POST", apiURL, result.Code, msg, resp)
		return fmt.Errorf("加入合集失败: %s", msg)
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

	resp, err := c.ReqClient.R().
		SetHeader("Referer", "https://member.bilibili.com/platform/upload/video/frame").
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"cover": dataURI,
		}).
		SetSuccessResult(&result).
		Post(apiURL)
	if err != nil {
		logBiliRequestError("上传封面", "POST", apiURL, err)
		return "", fmt.Errorf("请求错误: %w", err)
	}
	if !resp.IsSuccessState() {
		logBiliHTTPError("上传封面", "POST", apiURL, resp)
		return "", fmt.Errorf("上传封面失败: HTTP %d", resp.GetStatusCode())
	}

	if result.Code != 0 {
		logBiliAPIError("上传封面", "POST", apiURL, result.Code, result.Msg, resp)
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
	var result BuvIdResponse
	apiURL := "https://api.bilibili.com/x/frontend/finger/spi"
	resp, err := c.ReqClient.R().
		SetSuccessResult(&result).
		Get(apiURL)
	if err != nil {
		logBiliRequestError("获取buvid", "GET", apiURL, err)
		return nil, fmt.Errorf("获取buvid失败: %w", err)
	}
	if !resp.IsSuccessState() {
		logBiliHTTPError("获取buvid", "GET", apiURL, resp)
		return nil, fmt.Errorf("获取buvid失败: HTTP %d", resp.GetStatusCode())
	}
	if result.Code != 0 {
		logBiliAPIError("获取buvid", "GET", apiURL, result.Code, result.Msg, resp)
		return nil, fmt.Errorf("获取buvid失败: %s", result.Msg)
	}
	return &result, nil
}

func (c *BiliClient) cookieWithBuvID() string {
	fullCookie := strings.TrimSpace(c.Cookies)
	buvResp, err := c.GetBuvId()
	if err != nil {
		log.Printf("[B站接口] 获取buvid失败，继续使用原Cookie: %v", err)
		return fullCookie
	}
	if buvResp == nil || buvResp.Data.B3 == "" || buvResp.Data.B4 == "" {
		return fullCookie
	}
	fullCookie = appendCookieValueIfMissing(fullCookie, "buvid3", buvResp.Data.B3)
	fullCookie = appendCookieValueIfMissing(fullCookie, "buvid4", buvResp.Data.B4)
	return fullCookie
}

func appendCookieValueIfMissing(cookie, name, value string) string {
	if strings.TrimSpace(value) == "" || cookieHasKey(cookie, name) {
		return cookie
	}
	if strings.TrimSpace(cookie) == "" {
		return fmt.Sprintf("%s=%s", name, value)
	}
	return cookie + fmt.Sprintf("; %s=%s", name, value)
}

func cookieHasKey(cookie, name string) bool {
	for _, part := range strings.Split(cookie, ";") {
		key, _, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			return true
		}
	}
	return false
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
		logBiliRequestError("发送动态", "POST", apiURL, err)
		return err
	}

	if !resp.IsSuccessState() || result.Code != 0 {
		if !resp.IsSuccessState() {
			logBiliHTTPError("发送动态", "POST", apiURL, resp)
		} else {
			logBiliAPIError("发送动态", "POST", apiURL, result.Code, result.Msg, resp)
		}
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
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Count     int    `json:"count"`
	SectionID int64  `json:"sectionId"` // 用于 AddToSeason 的节ID
}

// extractTitleFromDuplicateError 从"稿件已成功投稿"错误信息中提取稿件名
// 错误格式示例: "稿件已成功投稿，请勿重新提交哦～\n提交时间：03:54 稿件名《新年玩新游 2026年02月18日01点13分》"
func extractTitleFromDuplicateError(errMsg string) string {
	// 使用正则表达式提取《》中的内容
	re := regexp.MustCompile(`稿件名[《〈]([^》〉]+)[》〉]`)
	matches := re.FindStringSubmatch(errMsg)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
