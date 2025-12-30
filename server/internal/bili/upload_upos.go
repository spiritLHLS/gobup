package bili

import (
	"fmt"
	"path/filepath"
)

// UposUploader UPOS上传器
type UposUploader struct {
	client *BiliClient
}

// NewUposUploader 创建UPOS上传器
func NewUposUploader(client *BiliClient) *UposUploader {
	return &UposUploader{client: client}
}

// Upload 上传文件
func (u *UposUploader) Upload(filePath string) (*UploadResult, error) {
	fileInfo, file, err := getFileInfo(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileName := filepath.Base(filePath)

	// 预上传
	preResp, err := u.preUpload(fileName, fileInfo.Size)
	if err != nil {
		return nil, fmt.Errorf("预上传失败: %w", err)
	}

	// 分片上传
	chunkSize := int64(5 * 1024 * 1024) // 5MB
	totalParts := (fileInfo.Size + chunkSize - 1) / chunkSize

	err = readFileChunks(file, chunkSize, func(chunk FileChunk) error {
		// UPOS使用从1开始的分片编号
		partNum := int(chunk.Index + 1)
		return u.uploadChunk(preResp, chunk.Data, partNum, int(totalParts))
	})
	if err != nil {
		return nil, fmt.Errorf("上传分片失败: %w", err)
	}

	// 完成上传
	if err := u.completeUpload(preResp, int(totalParts)); err != nil {
		return nil, fmt.Errorf("完成上传失败: %w", err)
	}

	return &UploadResult{
		FileName: fileName,
		BizID:    preResp.BizID,
	}, nil
}

func (u *UposUploader) preUpload(filename string, filesize int64) (*PreUploadResp, error) {
	params := map[string]string{
		"name":          filename,
		"size":          fmt.Sprintf("%d", filesize),
		"r":             "upos",
		"profile":       "ugcupos/bup",
		"ssl":           "0",
		"version":       "2.14.0",
		"build":         "2140000",
		"upcdn":         "ws",
		"probe_version": "20221109",
	}

	apiURL := "https://member.bilibili.com/preupload?" + buildQueryString(params)

	var preResp PreUploadResp

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	err := WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitPreUpload(); err != nil {
			return err
		}

		_, err := u.client.ReqClient.R().
			SetSuccessResult(&preResp).
			Get(apiURL)
		return err
	})

	if err != nil {
		return nil, err
	}

	if preResp.OK != 1 {
		return nil, fmt.Errorf("预上传失败")
	}

	return &preResp, nil
}

func (u *UposUploader) uploadChunk(pre *PreUploadResp, chunk []byte, partNum, totalParts int) error {
	params := map[string]string{
		"partNumber": fmt.Sprintf("%d", partNum),
		"uploadId":   pre.UploadID,
		"chunk":      fmt.Sprintf("%d", partNum-1),
		"chunks":     fmt.Sprintf("%d", totalParts),
		"size":       fmt.Sprintf("%d", len(chunk)),
		"start":      fmt.Sprintf("%d", (partNum-1)*len(chunk)),
		"end":        fmt.Sprintf("%d", partNum*len(chunk)-1),
		"total":      fmt.Sprintf("%d", len(chunk)*totalParts),
	}

	uploadURL := pre.Endpoint + "?" + buildQueryString(params)

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

func (u *UposUploader) completeUpload(pre *PreUploadResp, totalParts int) error {
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
		"uploadId": pre.UploadID,
		"biz_id":   fmt.Sprintf("%d", pre.BizID),
	}

	uploadURL := pre.Endpoint + "?" + buildQueryString(params)

	// 使用限流器和重试机制
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
