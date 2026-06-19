package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

type PublishRequest struct {
	HistoryID uint `json:"historyId"`
	UserID    uint `json:"userId"`
}

type Response struct {
	Type string         `json:"type"`
	Msg  string         `json:"msg"`
	Data *PublishResult `json:"data,omitempty"`
}

type PublishResult struct {
	Publish bool   `json:"publish"`
	BvID    string `json:"bvId"`
	AvID    string `json:"avId"`
	Message string `json:"message"`
}

func NewClient(endpoint, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		token:    strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Health() error {
	req, err := http.NewRequest(http.MethodGet, c.url("/health"), nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("agent health returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) PublishHistory(historyID, userID uint) (*PublishResult, error) {
	body, err := json.Marshal(PublishRequest{HistoryID: historyID, UserID: userID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.url("/publish"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result Response
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Msg != "" {
			return nil, fmt.Errorf("agent publish returned HTTP %d: %s", resp.StatusCode, result.Msg)
		}
		return nil, fmt.Errorf("agent publish returned HTTP %d", resp.StatusCode)
	}
	if result.Type == "error" {
		if result.Msg == "" {
			result.Msg = "远程 Agent 投稿失败"
		}
		return nil, errors.New(result.Msg)
	}
	return result.Data, nil
}

func (c *Client) url(path string) string {
	if strings.HasSuffix(c.endpoint, "/agent/v1") {
		return c.endpoint + path
	}
	return c.endpoint + "/agent/v1" + path
}

func (c *Client) authorize(req *http.Request) {
	if c.token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Agent-Token", c.token)
}
