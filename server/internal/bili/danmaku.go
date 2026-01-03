package bili

import (
	"fmt"
	"strconv"
)

// SendDanmaku 发送弹幕到视频
type SendDanmakuRequest struct {
	Type     int    `json:"type"`     // 1滚动 4底部 5顶部
	OID      int64  `json:"oid"`      // cid
	Msg      string `json:"msg"`      // 弹幕内容
	BvID     string `json:"bvid"`     // bv号
	Progress int    `json:"progress"` // 时间(毫秒)
	Color    int    `json:"color"`    // 颜色
	FontSize int    `json:"fontsize"` // 字号
	Mode     int    `json:"mode"`     // 模式
	Plat     int    `json:"plat"`     // 平台 1
	CSRF     string `json:"csrf"`     // csrf token
}

type SendDanmakuResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		DmID    int64  `json:"dmid"`
		DmIDStr string `json:"dmid_str"`
		Visible bool   `json:"visible"`
		Action  string `json:"action"`
	} `json:"data"`
}

// SendDanmakuWithoutWait 发送弹幕
func (c *BiliClient) SendDanmakuWithoutWait(cid int64, bvid string, progress int, message string, mode, fontSize, color int) error {
	csrf := GetCookieValue(c.Cookies, "bili_jct")
	if csrf == "" {
		return fmt.Errorf("未找到CSRF token")
	}

	req := SendDanmakuRequest{
		Type:     1,
		OID:      cid,
		Msg:      message,
		BvID:     bvid,
		Progress: progress,
		Color:    color,
		FontSize: fontSize,
		Mode:     mode,
		Plat:     1,
		CSRF:     csrf,
	}

	var resp SendDanmakuResponse
	r, err := c.ReqClient.R().
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36").
		SetHeader("Referer", "https://www.bilibili.com/video/"+bvid).
		SetHeader("Cookie", c.Cookies).
		SetHeader("x-bili-aurora-eid", "UlMFQVcABlAH").
		SetHeader("x-bili-aurora-zone", "sh001").
		SetFormData(map[string]string{
			"type":     strconv.Itoa(req.Type),
			"oid":      strconv.FormatInt(req.OID, 10),
			"msg":      req.Msg,
			"bvid":     req.BvID,
			"progress": strconv.Itoa(req.Progress),
			"color":    strconv.Itoa(req.Color),
			"fontsize": strconv.Itoa(req.FontSize),
			"mode":     strconv.Itoa(req.Mode),
			"plat":     strconv.Itoa(req.Plat),
			"csrf":     req.CSRF,
		}).
		SetSuccessResult(&resp).
		Post("https://api.bilibili.com/x/v2/dm/post")

	if err != nil {
		return fmt.Errorf("发送弹幕失败: %w", err)
	}

	if !r.IsSuccessState() {
		return fmt.Errorf("发送弹幕失败: HTTP %d", r.StatusCode)
	}

	if resp.Code != 0 {
		// 详细的错误码处理
		switch resp.Code {
		case 36701:
			return fmt.Errorf("弹幕包含被禁止的内容 (code=%d)", resp.Code)
		case 36702:
			return fmt.Errorf("弹幕长度超过100字符 (code=%d)", resp.Code)
		case 36703:
			return fmt.Errorf("发送频率过快，需要等待 (code=%d)", resp.Code)
		case 36704:
			return fmt.Errorf("禁止向未审核的视频发送弹幕 (code=%d)", resp.Code)
		case 36714:
			return fmt.Errorf("弹幕发送时间不合法 (code=%d)", resp.Code)
		case -101:
			return fmt.Errorf("账号未登录 (code=%d)", resp.Code)
		case -102:
			return fmt.Errorf("账号被封停 (code=%d)", resp.Code)
		case -111:
			return fmt.Errorf("csrf校验失败 (code=%d)", resp.Code)
		default:
			return fmt.Errorf("发送弹幕失败: %s (code=%d)", resp.Message, resp.Code)
		}
	}

	return nil
}

type DanmakuItem struct {
	CID      int64
	BvID     string
	Progress int
	Message  string
	Mode     int
	FontSize int
	Color    int
}
