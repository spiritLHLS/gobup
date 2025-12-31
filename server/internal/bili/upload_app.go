package bili

import (
	"fmt"
	"log"
	"path/filepath"
)

// AppUploader APP端上传器
type AppUploader struct {
	client           *BiliClient
	progressCallback ProgressCallback
}

// NewAppUploader 创建APP端上传器
func NewAppUploader(client *BiliClient) *AppUploader {
	return &AppUploader{client: client}
}

// SetProgressCallback 设置进度回调
func (u *AppUploader) SetProgressCallback(callback ProgressCallback) {
	u.progressCallback = callback
}

// Upload 上传文件
func (u *AppUploader) Upload(filePath string) (*UploadResult, error) {
	fileInfo, file, err := getFileInfo(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileName := filepath.Base(filePath)

	// 预上传
	log.Printf("[APP] 开始预上传: file=%s, size=%d", fileName, fileInfo.Size)
	preResp, err := u.preUpload(fileName, fileInfo.Size)
	if err != nil {
		return nil, fmt.Errorf("APP预上传失败: %w", err)
	}
	log.Printf("[APP] 预上传成功: biz_id=%d", preResp.BizID)

	// 计算文件MD5
	md5Hash, err := calculateFileMD5(file)
	if err != nil {
		return nil, err
	}

	// APP分片上传
	chunkSize := int64(2 * 1024 * 1024) // 2MB
	totalChunks := (fileInfo.Size + chunkSize - 1) / chunkSize
	log.Printf("[APP] 开始分片上传: total_chunks=%d, chunk_size=%dMB", totalChunks, chunkSize/(1024*1024))

	chunkDone := 0
	err = readFileChunks(file, chunkSize, func(chunk FileChunk) error {
		err := u.uploadChunk(preResp.Endpoint, chunk.Data, int(chunk.Index), int(totalChunks), fileName)
		if err != nil {
			return err
		}
		chunkDone++
		if u.progressCallback != nil {
			u.progressCallback(chunkDone, int(totalChunks))
		}
		log.Printf("[APP] 上传进度: %d/%d (%.1f%%)", chunkDone, totalChunks, float64(chunkDone)*100/float64(totalChunks))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("APP上传分片失败: %w", err)
	}

	// 完成上传
	log.Printf("[APP] 开始合并分片: total_chunks=%d", totalChunks)
	if err := u.completeUpload(preResp.Endpoint, int(totalChunks), fileInfo.Size, md5Hash, fileName); err != nil {
		return nil, fmt.Errorf("APP完成上传失败: %w", err)
	}
	log.Printf("[APP] 上传完成: file=%s, biz_id=%d", fileName, preResp.BizID)

	return &UploadResult{
		FileName: fileName,
		BizID:    preResp.BizID,
	}, nil
}

func (u *AppUploader) preUpload(filename string, filesize int64) (*PreUploadResp, error) {
	params := map[string]string{
		"name":    filename,
		"size":    fmt.Sprintf("%d", filesize),
		"r":       "ugcfr/pc3",
		"profile": "ugcfr/pc3",
		"ssl":     "0",
		"version": "2.3.0",
		"build":   "2030000",
	}

	apiURL := "https://member.bilibili.com/preupload?" + buildQueryString(params)

	var preResp PreUploadResp
	_, err := u.client.ReqClient.R().
		SetSuccessResult(&preResp).
		Get(apiURL)
	if err != nil {
		return nil, err
	}

	if preResp.OK != 1 {
		return nil, fmt.Errorf("APP预上传失败")
	}

	return &preResp, nil
}

func (u *AppUploader) uploadChunk(endpoint string, chunk []byte, chunkIndex, totalChunks int, filename string) error {
	uploadURL := fmt.Sprintf("%s?chunk=%d&chunks=%d&name=%s", endpoint, chunkIndex, totalChunks, filename)

	// 计算分片MD5
	chunkMD5 := calculateChunkMD5(chunk)

	resp, err := u.client.ReqClient.R().
		SetHeader("Content-Type", "application/octet-stream").
		SetHeader("Content-MD5", chunkMD5).
		SetBody(chunk).
		Post(uploadURL)
	if err != nil {
		return err
	}

	if !resp.IsSuccessState() {
		return fmt.Errorf("APP上传分片失败: %s", resp.String())
	}

	return nil
}

func (u *AppUploader) completeUpload(endpoint string, chunks int, filesize int64, md5Hash, filename string) error {
	uploadURL := fmt.Sprintf("%s?chunks=%d&filesize=%d&md5=%s&name=%s&version=2.3.0",
		endpoint, chunks, filesize, md5Hash, filename)

	var result map[string]interface{}
	resp, err := u.client.ReqClient.R().
		SetSuccessResult(&result).
		Post(uploadURL)
	if err != nil {
		return err
	}

	if !resp.IsSuccessState() {
		return fmt.Errorf("APP完成上传失败: %s", resp.String())
	}

	if ok, exists := result["OK"].(float64); !exists || ok != 1 {
		return fmt.Errorf("APP上传未成功")
	}

	return nil
}
