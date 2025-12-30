package bili

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
)

// FileChunk 文件分片信息
type FileChunk struct {
	Data   []byte
	Index  int64
	Size   int64
	Offset int64
}

// UploadConfig 上传配置
type UploadConfig struct {
	ChunkSize int64 // 分片大小
}

// buildQueryString 构建查询字符串
func buildQueryString(params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		values.Add(k, v)
	}
	return values.Encode()
}

// calculateFileMD5 计算文件MD5
func calculateFileMD5(file *os.File) (string, error) {
	file.Seek(0, 0)
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算MD5失败: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// calculateChunkMD5 计算分片MD5
func calculateChunkMD5(chunk []byte) string {
	hash := md5.New()
	hash.Write(chunk)
	return hex.EncodeToString(hash.Sum(nil))
}

// readFileChunks 读取文件分片
func readFileChunks(file *os.File, chunkSize int64, handler func(chunk FileChunk) error) error {
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	fileSize := stat.Size()
	totalChunks := (fileSize + chunkSize - 1) / chunkSize

	file.Seek(0, 0)
	for i := int64(0); i < totalChunks; i++ {
		offset := i * chunkSize
		size := chunkSize
		if offset+size > fileSize {
			size = fileSize - offset
		}

		chunk := make([]byte, size)
		if _, err := file.ReadAt(chunk, offset); err != nil && err != io.EOF {
			return fmt.Errorf("读取文件分片失败: %w", err)
		}

		if err := handler(FileChunk{
			Data:   chunk,
			Index:  i,
			Size:   size,
			Offset: offset,
		}); err != nil {
			return err
		}
	}

	return nil
}

// getFileInfo 获取文件信息
type FileInfo struct {
	Size int64
	Name string
}

func getFileInfo(filePath string) (*FileInfo, *os.File, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开文件失败: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	return &FileInfo{
		Size: stat.Size(),
		Name: stat.Name(),
	}, file, nil
}
