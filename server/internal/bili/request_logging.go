package bili

import (
	"log"
	"net/url"
	"strings"

	"github.com/imroc/req/v3"
)

func logBiliRequestError(action, method, rawURL string, err error) {
	if err == nil {
		return
	}
	log.Printf("[B站接口] %s请求失败: method=%s path=%s err=%v", action, method, redactBiliURL(rawURL), err)
}

func logBiliHTTPError(action, method, rawURL string, resp *req.Response) {
	if resp == nil {
		log.Printf("[B站接口] %sHTTP错误: method=%s path=%s status=<nil> body=<nil>", action, method, redactBiliURL(rawURL))
		return
	}
	log.Printf("[B站接口] %sHTTP错误: method=%s path=%s status=%d body=%s",
		action, method, redactBiliURL(rawURL), resp.GetStatusCode(), resp.String())
}

func logBiliAPIError(action, method, rawURL string, code int, message string, resp *req.Response) {
	body := ""
	if resp != nil {
		body = resp.String()
	}
	log.Printf("[B站接口] %s业务错误: method=%s path=%s code=%d msg=%s body=%s",
		action, method, redactBiliURL(rawURL), code, message, body)
}

func redactBiliURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	for _, key := range []string{"csrf", "csrf_token", "access_key", "token", "SESSDATA", "bili_jct"} {
		if query.Has(key) {
			query.Set(key, "[redacted]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
