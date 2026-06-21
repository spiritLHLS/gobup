package bili

import (
	"fmt"
	"strconv"
	"strings"
)

const biliVideoReplyType = "1"

type replyActionResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RPID int64 `json:"rpid"`
	} `json:"data"`
}

// AddVideoComment posts a normal comment to a video and returns the new reply id.
func (c *BiliClient) AddVideoComment(aid int64, bvid, message string) (int64, error) {
	message = strings.TrimSpace(message)
	if aid <= 0 {
		return 0, fmt.Errorf("无效AID")
	}
	if message == "" {
		return 0, fmt.Errorf("评论内容为空")
	}

	csrf := c.GetCSRF()
	if csrf == "" {
		return 0, fmt.Errorf("未找到CSRF token")
	}

	var result replyActionResponse
	apiURL := "https://api.bilibili.com/x/v2/reply/add"
	resp, err := c.ReqClient.R().
		SetHeader("Referer", videoReferer(aid, bvid)).
		SetFormData(map[string]string{
			"oid":        strconv.FormatInt(aid, 10),
			"type":       biliVideoReplyType,
			"message":    message,
			"plat":       "1",
			"ordering":   "heat",
			"csrf":       csrf,
			"csrf_token": csrf,
		}).
		SetSuccessResult(&result).
		Post(apiURL)
	if err != nil {
		logBiliRequestError("发送评论", "POST", apiURL, err)
		return 0, fmt.Errorf("发送评论失败: %w", err)
	}
	if resp.IsErrorState() {
		logBiliHTTPError("发送评论", "POST", apiURL, resp)
		return 0, fmt.Errorf("发送评论HTTP错误: %s", resp.Status)
	}
	if result.Code != 0 {
		logBiliAPIError("发送评论", "POST", apiURL, result.Code, result.Message, resp)
		return 0, fmt.Errorf("发送评论失败(code=%d): %s", result.Code, result.Message)
	}
	if result.Data.RPID <= 0 {
		return 0, fmt.Errorf("发送评论成功但返回RPID为空")
	}
	return result.Data.RPID, nil
}

// PinVideoComment best-effort pins a comment for the current uploader account.
func (c *BiliClient) PinVideoComment(aid int64, bvid string, rpid int64) error {
	if aid <= 0 || rpid <= 0 {
		return fmt.Errorf("无效AID或RPID")
	}

	csrf := c.GetCSRF()
	if csrf == "" {
		return fmt.Errorf("未找到CSRF token")
	}

	var result replyActionResponse
	apiURL := "https://api.bilibili.com/x/v2/reply/top"
	resp, err := c.ReqClient.R().
		SetHeader("Referer", videoReferer(aid, bvid)).
		SetFormData(map[string]string{
			"oid":        strconv.FormatInt(aid, 10),
			"type":       biliVideoReplyType,
			"rpid":       strconv.FormatInt(rpid, 10),
			"action":     "1",
			"csrf":       csrf,
			"csrf_token": csrf,
		}).
		SetSuccessResult(&result).
		Post(apiURL)
	if err != nil {
		logBiliRequestError("置顶评论", "POST", apiURL, err)
		return fmt.Errorf("置顶评论失败: %w", err)
	}
	if resp.IsErrorState() {
		logBiliHTTPError("置顶评论", "POST", apiURL, resp)
		return fmt.Errorf("置顶评论HTTP错误: %s", resp.Status)
	}
	if result.Code != 0 {
		logBiliAPIError("置顶评论", "POST", apiURL, result.Code, result.Message, resp)
		return fmt.Errorf("置顶评论失败(code=%d): %s", result.Code, result.Message)
	}
	return nil
}

func videoReferer(aid int64, bvid string) string {
	if strings.HasPrefix(bvid, "BV") {
		return "https://www.bilibili.com/video/" + bvid
	}
	return "https://www.bilibili.com/video/av" + strconv.FormatInt(aid, 10)
}
