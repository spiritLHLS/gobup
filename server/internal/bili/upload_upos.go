package bili

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
)

// ProgressCallback 进度回调函数
type ProgressCallback func(chunkDone, chunkTotal int)

// UposUploader UPOS上传器
type UposUploader struct {
	client           *BiliClient
	progressCallback ProgressCallback
}

// NewUposUploader 创建UPOS上传器
func NewUposUploader(client *BiliClient) *UposUploader {
	return &UposUploader{client: client}
}

// SetProgressCallback 设置进度回调
func (u *UposUploader) SetProgressCallback(callback ProgressCallback) {
	u.progressCallback = callback
}

// Upload 上传文件
func (u *UposUploader) Upload(filePath string) (*UploadResult, error) {
	fileInfo, file, err := getFileInfo(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileName := filepath.Base(filePath)
	fileSize := fileInfo.Size
	chunkSize := int64(5 * 1024 * 1024) // 5MB
	totalParts := (fileSize + chunkSize - 1) / chunkSize

	// 1. 预上传
	log.Printf("[UPOS] 开始预上传: file=%s, size=%d", fileName, fileSize)
	preResp, err := u.preUpload(fileName, fileSize)
	if err != nil {
		return nil, fmt.Errorf("预上传失败: %w", err)
	}
	log.Printf("[UPOS] 预上传成功: biz_id=%d, endpoint=%s", preResp.BizID, preResp.Endpoint)

	// 2. 检查并选择正确的endpoint（根据线路选择）
	zone, upcdn := parseLineParams(u.client.Line)
	if zone != "" && upcdn != "" {
		// 如果返回的endpoint不包含指定的upcdn，从endpoints列表中选择
		if !strings.Contains(preResp.Endpoint, "upcdn"+upcdn) {
			log.Printf("[UPOS] 默认endpoint不匹配，尝试从备选线路选择: target_upcdn=%s", upcdn)
			for _, endpoint := range preResp.Endpoints {
				if strings.Contains(endpoint, "upcdn"+upcdn) {
					preResp.Endpoint = endpoint
					log.Printf("[UPOS] 已切换endpoint: %s", endpoint)
					break
				}
			}
		}
	}

	// 3. 线路上传（初始化分片上传，获取upload_id）
	log.Printf("[UPOS] 开始线路上传初始化")
	lineResp, err := u.lineUpload(preResp)
	if err != nil {
		return nil, fmt.Errorf("线路上传初始化失败: %w", err)
	}
	log.Printf("[UPOS] 线路上传初始化成功: upload_id=%s", lineResp.UploadID)

	// 4. 分片上传
	log.Printf("[UPOS] 开始分片上传: total_parts=%d, chunk_size=%dMB", totalParts, chunkSize/(1024*1024))
	chunkDone := 0
	err = readFileChunks(file, chunkSize, func(chunk FileChunk) error {
		// UPOS使用从1开始的分片编号
		partNum := int(chunk.Index + 1)
		err := u.uploadChunk(preResp, lineResp, chunk.Data, partNum, int(totalParts), fileSize)
		if err != nil {
			return err
		}
		chunkDone++
		// 更新进度
		if u.progressCallback != nil {
			u.progressCallback(chunkDone, int(totalParts))
		}
		log.Printf("[UPOS] 上传进度: %d/%d (%.1f%%)", chunkDone, totalParts, float64(chunkDone)*100/float64(totalParts))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("上传分片失败: %w", err)
	}

	// 5. 完成上传
	log.Printf("[UPOS] 开始合并分片: total_parts=%d", totalParts)
	if err := u.completeUpload(preResp, lineResp, int(totalParts)); err != nil {
		return nil, fmt.Errorf("完成上传失败: %w", err)
	}
	log.Printf("[UPOS] 上传完成: file=%s, biz_id=%d", fileName, preResp.BizID)

	return &UploadResult{
		FileName: fileName,
		BizID:    preResp.BizID,
	}, nil
}

func (u *UposUploader) preUpload(filename string, filesize int64) (*PreUploadResp, error) {
	// 解析线路参数
	zone, upcdn := parseLineParams(u.client.Line)

	log.Printf("[UPOS] 线路配置: line=%s, zone=%s, upcdn=%s", u.client.Line, zone, upcdn)

	params := map[string]string{
		"name":          filename,
		"size":          fmt.Sprintf("%d", filesize),
		"r":             "upos",
		"profile":       "ugcupos/bup",
		"ssl":           "0",
		"version":       "2.14.0",
		"build":         "2140000",
		"zone":          zone,
		"upcdn":         upcdn,
		"probe_version": "20221109",
	}

	apiURL := "https://member.bilibili.com/preupload?" + buildQueryString(params)

	// 设置referer header（用于线路选择）
	lineQuery := fmt.Sprintf("?os=upos&zone=%s&upcdn=%s", zone, upcdn)

	var preResp PreUploadResp

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	err := WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitPreUpload(); err != nil {
			return err
		}

		resp, err := u.client.ReqClient.R().
			SetHeader("referer", lineQuery).
			SetSuccessResult(&preResp).
			Get(apiURL)

		if err != nil {
			log.Printf("[UPOS] 预上传请求失败: err=%v", err)
			return err
		}

		if !resp.IsSuccessState() {
			log.Printf("[UPOS] 预上传HTTP错误: status=%d, body=%s", resp.GetStatusCode(), resp.String())
			return fmt.Errorf("HTTP错误: status=%d", resp.GetStatusCode())
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	if preResp.OK != 1 {
		log.Printf("[UPOS] 预上传返回失败: OK=%d", preResp.OK)
		return nil, fmt.Errorf("预上传返回失败: OK=%d", preResp.OK)
	}

	log.Printf("[UPOS] 预上传响应: endpoint=%s", preResp.Endpoint)

	return &preResp, nil
}

func (u *UposUploader) lineUpload(pre *PreUploadResp) (*LineUploadResp, error) {
	// 构建URL: https:{endpoint}/{upUrl}?uploads&output=json
	// upUrl 从 upos_uri 提取（去掉开头的/）
	upUrl := getUpUrl(pre.UposURI)
	if upUrl == "" {
		return nil, fmt.Errorf("upos_uri为空或无效")
	}

	// 确保endpoint以https:开头
	endpoint := pre.Endpoint
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https:" + endpoint
	}

	uploadURL := endpoint + "/" + upUrl + "?uploads&output=json"
	var lineResp LineUploadResp

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	err := WithRetry(DefaultRetryConfig, func() error {
		if err := limiter.WaitGeneral(); err != nil {
			return err
		}

		resp, err := u.client.ReqClient.R().
			SetHeader("X-Upos-Auth", pre.Auth).
			SetSuccessResult(&lineResp).
			Post(uploadURL)
		if err != nil {
			log.Printf("[UPOS] 线路上传请求失败: err=%v", err)
			return err
		}

		if !resp.IsSuccessState() {
			log.Printf("[UPOS] 线路上传HTTP错误: status=%d, body=%s", resp.GetStatusCode(), resp.String())
			return fmt.Errorf("HTTP错误: status=%d", resp.GetStatusCode())
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	if lineResp.OK != 1 {
		log.Printf("[UPOS] 线路上传返回失败: OK=%d", lineResp.OK)
		return nil, fmt.Errorf("线路上传返回失败: OK=%d", lineResp.OK)
	}

	return &lineResp, nil
}

func (u *UposUploader) uploadChunk(pre *PreUploadResp, line *LineUploadResp, chunk []byte, partNum, totalParts int, fileSize int64) error {
	chunkSize := int64(len(chunk))
	// 标准分片大小
	standardChunkSize := int64(5 * 1024 * 1024)
	start := int64(partNum-1) * standardChunkSize
	end := start + chunkSize - 1

	params := map[string]string{
		"partNumber": fmt.Sprintf("%d", partNum),
		"uploadId":   line.UploadID, // 使用lineUpload返回的upload_id
		"chunk":      fmt.Sprintf("%d", partNum-1),
		"chunks":     fmt.Sprintf("%d", totalParts),
		"size":       fmt.Sprintf("%d", chunkSize),
		"start":      fmt.Sprintf("%d", start),
		"end":        fmt.Sprintf("%d", end),
		"total":      fmt.Sprintf("%d", fileSize),
	}

	// 确保endpoint格式正确
	endpoint := pre.Endpoint
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https:" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	// 构建完整URL
	uposURI := pre.UposURI
	if !strings.HasPrefix(uposURI, "/") {
		uposURI = "/" + uposURI
	}
	uploadURL := endpoint + uposURI + "?" + buildQueryString(params)

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	return WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitChunkUpload(); err != nil {
			return err
		}

		resp, err := u.client.ReqClient.R().
			SetHeader("X-Upos-Auth", pre.Auth).
			SetBody(chunk).
			Put(uploadURL)
		if err != nil {
			return err
		}

		if !resp.IsSuccessState() {
			return fmt.Errorf("上传分片失败: %s", resp.String())
		}

		return nil
	})
}

func (u *UposUploader) completeUpload(pre *PreUploadResp, line *LineUploadResp, totalParts int) error {
	parts := make([]map[string]interface{}, totalParts)
	for i := 0; i < totalParts; i++ {
		parts[i] = map[string]interface{}{
			"partNumber": i + 1,
			"eTag":       "etag",
		}
	}

	body := map[string]interface{}{
		"parts": parts,
	}

	params := map[string]string{
		"output":   "json",
		"name":     pre.BiliFilename,
		"profile":  "ugcupos/bup",
		"uploadId": line.UploadID,
		"biz_id":   fmt.Sprintf("%d", pre.BizID),
	}

	// 确保endpoint格式正确
	endpoint := pre.Endpoint
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https:" + endpoint
	}

	// 构建完整URL
	upUrl := getUpUrl(pre.UposURI)
	uploadURL := endpoint + "/" + upUrl + "?" + buildQueryString(params)
	limiter := GetAPILimiter()
	var result map[string]interface{}
	err := WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitGeneral(); err != nil {
			return err
		}

		resp, err := u.client.ReqClient.R().
			SetHeader("X-Upos-Auth", pre.Auth).
			SetBody(body).
			SetSuccessResult(&result).
			Post(uploadURL)
		if err != nil {
			return err
		}

		if !resp.IsSuccessState() {
			return fmt.Errorf("完成上传失败: %s", resp.String())
		}

		return nil
	})

	if err != nil {
		return err
	}

	if ok, exists := result["OK"].(float64); !exists || ok != 1 {
		return fmt.Errorf("上传未成功")
	}

	return nil
}
