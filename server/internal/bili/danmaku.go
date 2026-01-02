package bili

import (
	"fmt"
	"strconv"
	"time"
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

// SendDanmaku 发送弹幕
func (c *BiliClient) SendDanmaku(cid int64, bvid string, progress int, message string, mode, fontSize, color int) error {
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
		return fmt.Errorf("发送弹幕失败: %s (code=%d)", resp.Message, resp.Code)
	}

	// 控制发送速率，避免被限制
	time.Sleep(time.Second)

	return nil
}

// BatchSendDanmaku 批量发送弹幕（自动限速）
func (c *BiliClient) BatchSendDanmaku(danmakus []DanmakuItem) (int, error) {
	successCount := 0
	for i, dm := range danmakus {
		err := c.SendDanmaku(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color)
		if err != nil {
			return successCount, fmt.Errorf("发送第%d条弹幕失败: %w", i+1, err)
		}
		successCount++

		// 每10条休息一下
		if (i+1)%10 == 0 {
			time.Sleep(5 * time.Second)
		}
	}

	return successCount, nil
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
