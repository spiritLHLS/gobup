package bili

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
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

// parseLineParams 从线路名称解析 zone 和 upcdn 参数
// 例如: cs_txa -> zone=cs, upcdn=txa
//
//	jd_bd -> zone=jd, upcdn=bd
func parseLineParams(line string) (zone, upcdn string) {
	// 默认值
	zone = "cs"
	upcdn = "ws"

	if line == "" {
		return
	}

	// 特殊处理 app
	if line == "app" {
		return
	}

	// 格式: zone_upcdn，例如 cs_txa, jd_bd
	parts := []string{}
	start := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '_' {
			parts = append(parts, line[start:i])
			start = i + 1
		}
	}
	parts = append(parts, line[start:])

	if len(parts) >= 2 {
		zone = parts[0]
		upcdn = parts[1]
	}

	return
}

// getUpUrl 从upos_uri提取上传路径
// 从第一个/之后开始提取，匹配Java版本实现：upos_uri.substring(upos_uri.indexOf("/") + 1)
// 例如: upos:/ugcever/xxx.flv -> ugcever/xxx.flv
//
//	/ugcbup/xxx.mp4 -> ugcbup/xxx.mp4
func getUpUrl(uposURI string) string {
	if uposURI == "" {
		return ""
	}
	// 查找第一个/的位置，从其后开始提取
	idx := strings.Index(uposURI, "/")
	if idx >= 0 && idx < len(uposURI)-1 {
		return uposURI[idx+1:]
	}
	// 如果没有/或/在最后，返回原字符串
	return uposURI
}

// CalculateChunkCount 计算文件的分片数量
func CalculateChunkCount(fileSize int64, chunkSize int64) int64 {
	return (fileSize + chunkSize - 1) / chunkSize
}

// ShouldSplitFile 判断文件是否需要分割（分片数超过10000）
func ShouldSplitFile(fileSize int64, chunkSize int64) bool {
	chunkCount := CalculateChunkCount(fileSize, chunkSize)
	return chunkCount > 10000
}
