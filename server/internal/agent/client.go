package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Type string          `json:"type"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data,omitempty"`
}

type PublishResult struct {
	Publish bool   `json:"publish"`
	BvID    string `json:"bvId"`
	AvID    string `json:"avId"`
	Message string `json:"message"`
}

type HealthData struct {
	Version         string   `json:"version"`
	Purpose         string   `json:"purpose"`
	Capabilities    []string `json:"capabilities"`
	Listen          string   `json:"listen"`
	WorkPath        string   `json:"workPath"`
	UpstreamBaseURL string   `json:"upstreamBaseUrl"`
	Time            string   `json:"time"`
}

type FileCheckRequest struct {
	Paths      []string `json:"paths,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	MinSize    *int64   `json:"minSize,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

type FileCheckItem struct {
	FilePath   string    `json:"filePath"`
	FileName   string    `json:"fileName"`
	FileSize   int64     `json:"fileSize"`
	ModTime    time.Time `json:"modTime"`
	InDatabase *bool     `json:"inDatabase,omitempty"`
	RoomID     string    `json:"roomId,omitempty"`
	Uname      string    `json:"uname,omitempty"`
	Title      string    `json:"title,omitempty"`
}

type FileCheckResult struct {
	Purpose         string          `json:"purpose,omitempty"`
	WorkPath        string          `json:"workPath,omitempty"`
	TotalFiles      int             `json:"totalFiles"`
	TotalSize       int64           `json:"totalSize"`
	DatabaseAware   bool            `json:"databaseAware,omitempty"`
	InDatabaseCount int             `json:"inDatabaseCount,omitempty"`
	NewFiles        int             `json:"newFiles,omitempty"`
	SampleLimit     int             `json:"sampleLimit,omitempty"`
	Files           []FileCheckItem `json:"files"`
	Errors          []string        `json:"errors,omitempty"`
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

func (c *Client) Health() (*HealthData, error) {
	req, err := http.NewRequest(http.MethodGet, c.url("/health"), nil)
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result, decodeErr := decodeResponse(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Msg != "" {
			return nil, fmt.Errorf("agent health returned HTTP %d: %s", resp.StatusCode, result.Msg)
		}
		return nil, fmt.Errorf("agent health returned HTTP %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	if result.Type == "error" {
		if result.Msg == "" {
			result.Msg = "远程 Agent 不可用"
		}
		return nil, errors.New(result.Msg)
	}
	if len(result.Data) == 0 {
		return &HealthData{}, nil
	}
	var data HealthData
	if err := json.Unmarshal(result.Data, &data); err != nil {
		return nil, fmt.Errorf("解析 Agent 健康响应失败: %w", err)
	}
	return &data, nil
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

	result, _ := decodeResponse(resp.Body)
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
	if len(result.Data) == 0 {
		return nil, nil
	}

	var publishResult PublishResult
	var nested Response
	if err := json.Unmarshal(result.Data, &nested); err == nil && (nested.Type != "" || len(nested.Data) > 0) {
		if nested.Type == "error" {
			if nested.Msg == "" {
				nested.Msg = "远程 Agent 投稿失败"
			}
			return nil, errors.New(nested.Msg)
		}
		if err := json.Unmarshal(nested.Data, &publishResult); err == nil {
			return &publishResult, nil
		}
	}
	if err := json.Unmarshal(result.Data, &publishResult); err == nil {
		return &publishResult, nil
	}
	return nil, fmt.Errorf("解析 Agent 投稿响应失败")
}

func (c *Client) CheckFiles(reqData FileCheckRequest) (*FileCheckResult, error) {
	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.url("/files/check"), bytes.NewReader(body))
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

	result, decodeErr := decodeResponse(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Msg != "" {
			return nil, fmt.Errorf("agent file check returned HTTP %d: %s", resp.StatusCode, result.Msg)
		}
		return nil, fmt.Errorf("agent file check returned HTTP %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	if result.Type == "error" {
		if result.Msg == "" {
			result.Msg = "远程 Agent 文件检查失败"
		}
		return nil, errors.New(result.Msg)
	}
	var checkResult FileCheckResult
	if len(result.Data) > 0 {
		if err := json.Unmarshal(result.Data, &checkResult); err != nil {
			return nil, fmt.Errorf("解析 Agent 文件检查响应失败: %w", err)
		}
	}
	return &checkResult, nil
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

func decodeResponse(body io.Reader) (Response, error) {
	var result Response
	err := json.NewDecoder(body).Decode(&result)
	return result, err
}
