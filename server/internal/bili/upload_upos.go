package bili

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
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

	// 分片上传最多重试3次整个流程（如果某个分片持续失败）
	maxUploadRetries := 3
	for uploadRetry := 0; uploadRetry < maxUploadRetries; uploadRetry++ {
		if uploadRetry > 0 {
			log.Printf("[UPOS] ⚠️ 检测到分片上传失败，开始断点续传 (重试 %d/%d)，从分片 %d/%d 继续", uploadRetry+1, maxUploadRetries, chunkDone+1, totalParts)
		}

		err = readFileChunks(file, chunkSize, func(chunk FileChunk) error {
			// 如果是重试，跳过已上传的分片
			if int(chunk.Index) < chunkDone {
				// 静默跳过已上传的分片，避免日志过多
				return nil
			}

			// UPOS使用从1开始的分片编号
			partNum := int(chunk.Index + 1)
			err := u.uploadChunk(preResp, lineResp, chunk.Data, partNum, int(totalParts), fileSize)
			if err != nil {
				log.Printf("[UPOS] ❌ 分片 %d/%d 上传失败: %v", partNum, totalParts, err)
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

		if err == nil {
			break
		}

		// 如果是最后一次重试仍然失败，返回错误
		if uploadRetry == maxUploadRetries-1 {
			return nil, fmt.Errorf("上传分片失败: %w", err)
		}
	}

	// 5. 完成上传
	log.Printf("[UPOS] 开始合并分片: total_parts=%d", totalParts)
	if err := u.completeUpload(preResp, lineResp, int(totalParts)); err != nil {
		return nil, fmt.Errorf("完成上传失败: %w", err)
	}

	// 从 lineResp.Key 中提取文件名（参考 biliupforjava 的 LineUploadBean.getFileName()）
	// Key 格式类似: "/upos/xxx.flv" 或 "xxx.flv"
	resultFileName := extractFileNameFromKey(lineResp.Key)
	if resultFileName == "" {
		log.Printf("[UPOS] 警告: 无法从Key提取文件名，使用原始文件名: key=%s, file=%s", lineResp.Key, fileName)
		resultFileName = fileName
	}
	log.Printf("[UPOS] 上传完成: file=%s, biz_id=%d, server_filename=%s", fileName, preResp.BizID, resultFileName)

	return &UploadResult{
		FileName: resultFileName,
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
	var isRateLimited bool

	// 使用限流器和重试机制
	limiter := GetAPILimiter()

	// 首先尝试使用默认配置
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
			body := resp.String()
			log.Printf("[UPOS] 预上传HTTP错误: status=%d, body=%s", resp.GetStatusCode(), body)

			// 检测是否为B站限流错误
			if resp.GetStatusCode() == 406 || contains(body, "601") || contains(body, "上传视频过快") {
				isRateLimited = true
				log.Printf("[UPOS] 检测到B站限流，将使用更长的重试间隔")
			}

			return fmt.Errorf("HTTP错误: status=%d", resp.GetStatusCode())
		}

		return nil
	})

	// 如果检测到限流，使用限流专用重试配置再试一次
	if err != nil && isRateLimited {
		log.Printf("[UPOS] 使用限流重试配置重新尝试，首次等待15秒...")
		err = WithRetry(RateLimitRetryConfig, func() error {
			if err := limiter.WaitPreUpload(); err != nil {
				return err
			}

			resp, err := u.client.ReqClient.R().
				SetHeader("referer", lineQuery).
				SetSuccessResult(&preResp).
				Get(apiURL)

			if err != nil {
				return err
			}

			if !resp.IsSuccessState() {
				log.Printf("[UPOS] 预上传仍然被限流: status=%d", resp.GetStatusCode())
				return fmt.Errorf("HTTP错误: status=%d", resp.GetStatusCode())
			}

			return nil
		})
	}
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	if preResp.OK != 1 {
		log.Printf("[UPOS] 预上传返回失败: OK=%d", preResp.OK)
		return nil, fmt.Errorf("预上传返回失败: OK=%d", preResp.OK)
	}

	log.Printf("[UPOS] 预上传响应: endpoint=%s, biz_id=%d, bili_filename=%s, upos_uri=%s",
		preResp.Endpoint, preResp.BizID, preResp.BiliFilename, preResp.UposURI)

	return &preResp, nil
}

func (u *UposUploader) lineUpload(pre *PreUploadResp) (*LineUploadResp, error) {
	// 构建URL: https:{endpoint}/{upUrl}?uploads&output=json
	// 参考Java实现: "https:" + preUploadBean.getEndpoint() + preUploadBean.getUpUrl() + "?uploads&output=json"
	upUrl := getUpUrl(pre.UposURI)
	if upUrl == "" {
		return nil, fmt.Errorf("upos_uri为空或无效")
	}

	// 确保endpoint以https:开头
	endpoint := pre.Endpoint
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https:" + endpoint
	}
	// 移除endpoint末尾的斜杠（如果有）
	endpoint = strings.TrimSuffix(endpoint, "/")

	// upUrl已经不包含开头的/，直接拼接
	uploadURL := endpoint + "/" + upUrl + "?uploads&output=json"
	log.Printf("[UPOS] 线路上传URL: %s", uploadURL)
	log.Printf("[UPOS] upos_uri: %s, upUrl: %s", pre.UposURI, upUrl)

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

	// 构建完整URL - 参考Java实现: "https:" + preUploadBean.getEndpoint() + preUploadBean.getUpUrl()
	upUrl := getUpUrl(pre.UposURI)
	uploadURL := endpoint + "/" + upUrl + "?" + buildQueryString(params)

	// 使用限流器和重试机制
	limiter := GetAPILimiter()

	var lastErr error
	for attempt := 0; attempt <= DefaultRetryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// 网络错误重试前等待
			delay := time.Duration(float64(DefaultRetryConfig.InitialDelay) * float64(attempt))
			if delay > DefaultRetryConfig.MaxDelay {
				delay = DefaultRetryConfig.MaxDelay
			}
			log.Printf("[UPOS] 分片%d上传失败，等待%v后重试 (%d/%d): %v", partNum, delay, attempt, DefaultRetryConfig.MaxRetries, lastErr)
			time.Sleep(delay)
		}

		// 等待限流器允许
		if err := limiter.WaitChunkUpload(); err != nil {
			return err
		}

		resp, err := u.client.ReqClient.R().
			SetHeader("X-Upos-Auth", pre.Auth).
			SetHeader("Content-Type", "application/octet-stream").
			SetBody(chunk).
			Put(uploadURL)
		if err != nil {
			lastErr = err
			// 网络错误可以重试
			continue
		}

		statusCode := resp.GetStatusCode()
		if !resp.IsSuccessState() {
			log.Printf("[UPOS] 上传分片%d HTTP错误: status=%d, body=%s", partNum, statusCode, resp.String())
			// 明确返回HTTP状态码，方便上层判断
			if statusCode == 406 {
				return fmt.Errorf("HTTP 406: 速率限制 - %s", resp.String())
			} else if statusCode == 601 {
				return fmt.Errorf("HTTP 601: 上传视频过快 - %s", resp.String())
			}
			lastErr = fmt.Errorf("HTTP %d: 上传分片失败 - %s", statusCode, resp.String())
			// HTTP错误也可以重试（除了406/601）
			continue
		}

		// 成功
		return nil
	}

	// 所有重试都失败
	if lastErr != nil {
		log.Printf("[UPOS] 分片%d上传失败，已重试%d次: %v", partNum, DefaultRetryConfig.MaxRetries, lastErr)
		return lastErr
	}
	return fmt.Errorf("上传分片%d失败: 未知错误", partNum)
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
	endpoint = strings.TrimSuffix(endpoint, "/")

	// 构建完整URL
	upUrl := getUpUrl(pre.UposURI)
	uploadURL := endpoint + "/" + upUrl + "?" + buildQueryString(params)
	log.Printf("[UPOS] 完成上传URL: %s", uploadURL)

	limiter := GetAPILimiter()
	var result map[string]interface{}
	err := WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitGeneral(); err != nil {
			return err
		}

		resp, err := u.client.ReqClient.R().
			SetHeader("X-Upos-Auth", pre.Auth).
			SetHeader("Content-Type", "application/json").
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

// extractFileNameFromKey 从 Key 中提取文件名
// 参考 biliupforjava 的 LineUploadBean.getFileName()
// Key 格式类似: "/upos/xxx.flv" 或 "xxx.flv"
// 返回去掉路径和扩展名的文件名，例如: "/upos/n123456.flv" -> "n123456"
func extractFileNameFromKey(key string) string {
	if key == "" {
		return ""
	}

	// 去掉开头的 /
	key = strings.TrimPrefix(key, "/")

	// 查找最后一个 / 后的部分（文件名）
	lastSlash := strings.LastIndex(key, "/")
	if lastSlash >= 0 {
		key = key[lastSlash+1:]
	}

	// 查找第一个 . 的位置（扩展名开始）
	dotIndex := strings.Index(key, ".")
	if dotIndex > 0 {
		return key[:dotIndex]
	}

	return key
}
