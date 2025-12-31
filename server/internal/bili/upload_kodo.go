package bili

import (
	"fmt"
	"log"
)

// KodoUploader 七牛云KODO上传器
type KodoUploader struct {
	client           *BiliClient
	progressCallback ProgressCallback
}

// NewKodoUploader 创建KODO上传器
func NewKodoUploader(client *BiliClient) *KodoUploader {
	return &KodoUploader{client: client}
}

// SetProgressCallback 设置进度回调
func (u *KodoUploader) SetProgressCallback(callback ProgressCallback) {
	u.progressCallback = callback
}

// Upload 上传文件
func (u *KodoUploader) Upload(filePath string) (*UploadResult, error) {
	fileInfo, file, err := getFileInfo(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 预上传
	log.Printf("[KODO] 开始预上传: file=%s, size=%d", fileInfo.Name, fileInfo.Size)
	preResp, err := u.preUpload(fileInfo.Name, fileInfo.Size)
	if err != nil {
		return nil, fmt.Errorf("KODO预上传失败: %w", err)
	}
	log.Printf("[KODO] 预上传成功: biz_id=%d", preResp.BizID)

	// 分片上传
	chunkSize := int64(4 * 1024 * 1024) // 4MB
	var ctxs []string
	totalChunks := (fileInfo.Size + chunkSize - 1) / chunkSize
	log.Printf("[KODO] 开始分片上传: total_chunks=%d, chunk_size=%dMB", totalChunks, chunkSize/(1024*1024))

	chunkDone := 0
	err = readFileChunks(file, chunkSize, func(chunk FileChunk) error {
		ctx, err := u.uploadChunk(preResp, chunk.Data, int(chunk.Index))
		if err != nil {
			return fmt.Errorf("KODO上传分片%d失败: %w", chunk.Index, err)
		}
		ctxs = append(ctxs, ctx)
		chunkDone++
		if u.progressCallback != nil {
			u.progressCallback(chunkDone, int(totalChunks))
		}
		log.Printf("[KODO] 上传进度: %d/%d (%.1f%%)", chunkDone, totalChunks, float64(chunkDone)*100/float64(totalChunks))
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 完成上传
	log.Printf("[KODO] 开始合并分片: total_chunks=%d", totalChunks)
	if err := u.completeUpload(preResp, ctxs, fileInfo.Size); err != nil {
		return nil, fmt.Errorf("KODO完成上传失败: %w", err)
	}
	log.Printf("[KODO] 上传完成: file=%s, biz_id=%d", fileInfo.Name, preResp.BizID)

	return &UploadResult{
		FileName: fileInfo.Name,
		BizID:    preResp.BizID,
	}, nil
}

func (u *KodoUploader) preUpload(filename string, filesize int64) (*PreUploadResp, error) {
	params := map[string]string{
		"name":    filename,
		"size":    fmt.Sprintf("%d", filesize),
		"r":       "kodo",
		"profile": "ugcupos/bupfetch",
		"ssl":     "0",
		"version": "2.14.0",
		"build":   "2140000",
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
		return nil, fmt.Errorf("KODO预上传失败")
	}

	return &preResp, nil
}

func (u *KodoUploader) uploadChunk(pre *PreUploadResp, chunk []byte, index int) (string, error) {
	uploadURL := fmt.Sprintf("%s/mkblk/%d", pre.Endpoint, len(chunk))

	var result struct {
		Ctx string `json:"ctx"`
	}

	// 使用限流器和重试机制
	limiter := GetAPILimiter()
	err := WithRetry(DefaultRetryConfig, func() error {
		// 等待限流器允许
		if err := limiter.WaitChunkUpload(); err != nil {
			return err
		}

		resp, err := u.client.ReqClient.R().
			SetHeader("Authorization", "UpToken "+pre.Auth).
			SetBody(chunk).
			SetSuccessResult(&result).
			Post(uploadURL)
		if err != nil {
			return err
		}

		if !resp.IsSuccessState() {
			return fmt.Errorf("KODO上传分片失败: %s", resp.String())
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return result.Ctx, nil
}

func (u *KodoUploader) completeUpload(pre *PreUploadResp, ctxs []string, fileSize int64) error {
	uploadURL := fmt.Sprintf("%s/mkfile/%d/key/%s", pre.Endpoint, fileSize, pre.BiliFilename)

	body := ""
	for _, ctx := range ctxs {
		if body != "" {
			body += ","
		}
		body += ctx
	}

	var result map[string]interface{}
	resp, err := u.client.ReqClient.R().
		SetHeader("Authorization", "UpToken "+pre.Auth).
		SetBodyString(body).
		SetSuccessResult(&result).
		Post(uploadURL)
	if err != nil {
		return err
	}

	if !resp.IsSuccessState() {
		return fmt.Errorf("KODO完成上传失败: %s", resp.String())
	}

	return nil
}
