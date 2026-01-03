package bili

import (
	"fmt"
	"log"
	"strconv"
	"strings"
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

	// 控制发送速率，避免被限制 - 成功后等待较长时间
	time.Sleep(25 * time.Second)

	return nil
}

// BatchSendDanmaku 批量发送弹幕（自动限速）
func (c *BiliClient) BatchSendDanmaku(danmakus []DanmakuItem) (int, error) {
	successCount := 0
	failedCount := 0
	for i, dm := range danmakus {
		err := c.SendDanmaku(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color)
		if err != nil {
			failedCount++
			// 检查错误类型
			if strings.Contains(err.Error(), "36703") {
				// 发送频率过快，等待120秒后继续
				log.Printf("[弹幕发送] ⚠️  发送频率过快，等待120秒... (已发送%d/%d)", i+1, len(danmakus))
				time.Sleep(120 * time.Second)
				// 重试当前弹幕
				err = c.SendDanmaku(dm.CID, dm.BvID, dm.Progress, dm.Message, dm.Mode, dm.FontSize, dm.Color)
				if err == nil {
					successCount++
					failedCount--
				}
			} else if strings.Contains(err.Error(), "36704") {
				// 视频未审核通过，停止发送
				log.Printf("[弹幕发送] ❌ 视频未审核通过，停止发送 (已发送%d/%d)", successCount, len(danmakus))
				return successCount, fmt.Errorf("视频未审核通过: %w", err)
			} else if strings.Contains(err.Error(), "36701") || strings.Contains(err.Error(), "36702") || strings.Contains(err.Error(), "36714") {
				// 弹幕内容/长度/时间问题，跳过这条弹幕
				log.Printf("[弹幕发送] ⚠️  跳过问题弹幕: %v", err)
				continue
			} else if strings.Contains(err.Error(), "-101") || strings.Contains(err.Error(), "-102") || strings.Contains(err.Error(), "-111") {
				// 账号问题，停止发送
				log.Printf("[弹幕发送] ❌ 账号异常，停止发送: %v", err)
				return successCount, err
			} else {
				log.Printf("[弹幕发送] ⚠️  发送失败: %v", err)
			}
		} else {
			successCount++
		}

		// 每10条额外休息一下
		if (i+1)%10 == 0 {
			log.Printf("[弹幕发送] ⏸️  已发送10条，休息5秒... (进度: %d/%d)", i+1, len(danmakus))
			time.Sleep(5 * time.Second)
		}
	}

	if failedCount > 0 {
		log.Printf("[弹幕发送] ⚠️  批量发送完成: 成功%d条, 失败%d条", successCount, failedCount)
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
