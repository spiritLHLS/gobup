package services

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"time"
)

// FLVMetadata FLV文件元数据
type FLVMetadata struct {
	SessionID  string    // 本次直播的SessionID
	RoomID     string    // 直播间号
	Uname      string    // 主播名
	Title      string    // 标题
	StartTime  time.Time // 开始时间
	Duration   float64   // 时长（秒）
	FileSize   int64     // 文件大小
	VideoCodec string
	AudioCodec string
	Width      int
	Height     int
	FrameRate  float64
	BitRate    int
}

// FLVParser FLV文件解析器
type FLVParser struct{}

// NewFLVParser 创建FLV解析器
func NewFLVParser() *FLVParser {
	return &FLVParser{}
}

// ParseFLVFile 解析FLV文件以获取元数据
func (p *FLVParser) ParseFLVFile(filePath string) (*FLVMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	// 读取FLV文件头
	header := make([]byte, 13)
	if _, err := file.Read(header); err != nil {
		return nil, fmt.Errorf("读取FLV文件头失败: %w", err)
	}

	// 验证FLV文件签名
	if string(header[0:3]) != "FLV" {
		return nil, fmt.Errorf("不是有效的FLV文件")
	}

	metadata := &FLVMetadata{
		FileSize:  fileInfo.Size(),
		StartTime: fileInfo.ModTime(),
	}

	// 查找并解析元数据标签
	if err := p.findAndParseMetadata(file, metadata); err != nil {
		log.Printf("[FLVParser] 解析FLV元数据失败: %v，使用默认值", err)
		// 不返回错误，继续使用已有的默认值
	}

	return metadata, nil
}

// findAndParseMetadata 查找并解析FLV中的元数据标签
func (p *FLVParser) findAndParseMetadata(file *os.File, metadata *FLVMetadata) error {
	// FLV格式: FLV文件头(13字节) -> 数据标签列表
	// 每个标签: 类型(1字节) + 大小(3字节) + 时间戳(3字节) + 时间戳扩展(1字节) + StreamID(3字节) + 数据
	// 标签类型: 0x08=音频, 0x09=视频, 0x12=脚本数据

	// 跳过DataOffset后的数据
	file.Seek(13, io.SeekStart) // 跳过FLV文件头

	// 读取数据偏移量（前4字节后面的部分已经读过了，重新定位）
	file.Seek(4, io.SeekStart) // 重新到文件开头第4字节处
	dataOffsetBytes := make([]byte, 4)
	if _, err := file.Read(dataOffsetBytes); err != nil {
		return fmt.Errorf("读取数据偏移失败: %w", err)
	}
	dataOffset := binary.BigEndian.Uint32(dataOffsetBytes)

	// 定位到第一个标签
	file.Seek(int64(dataOffset), io.SeekStart)

	// 扫描标签，找到脚本数据标签（0x12）
	for {
		// 读取标签头（11字节）
		tagHeader := make([]byte, 11)
		n, err := file.Read(tagHeader)
		if err != nil || n < 11 {
			break // 到达文件末尾或读取不足
		}

		tagType := tagHeader[0]
		tagSize := uint32(tagHeader[1])<<16 | uint32(tagHeader[2])<<8 | uint32(tagHeader[3])

		// 脚本数据标签类型为0x12
		if tagType == 0x12 {
			// 读取标签数据
			tagData := make([]byte, tagSize)
			if _, err := file.Read(tagData); err != nil {
				break
			}

			// 解析脚本数据
			p.parseScriptData(tagData, metadata)
			return nil // 只需要找第一个脚本数据标签
		} else {
			// 跳过其他类型的标签
			if _, err := file.Seek(int64(tagSize), io.SeekCurrent); err != nil {
				break
			}
		}

		// 读取标签大小（前一个标签的大小）
		prevTagSizeBytes := make([]byte, 4)
		if _, err := file.Read(prevTagSizeBytes); err != nil {
			break // 到达文件末尾
		}
	}

	return nil
}

// parseScriptData 解析FLV脚本数据
func (p *FLVParser) parseScriptData(data []byte, metadata *FLVMetadata) {
	reader := bytes.NewReader(data)

	// 脚本数据通常以AMF0格式存储
	// 第一个值通常是事件名称字符串（如 "onMetaData"）
	if v, err := p.readAMF0Value(reader); err == nil {
		// 只用于跳过第一个值，实际内容不需要使用
		_ = v
	}

	// 第二个值通常是包含元数据的对象
	if eventObj, err := p.readAMF0Value(reader); err == nil {
		if obj, ok := eventObj.(map[string]interface{}); ok {
			p.extractMetadataFromObject(obj, metadata)
		}
	}
}

// readAMF0Value 读取AMF0格式的值
func (p *FLVParser) readAMF0Value(reader *bytes.Reader) (interface{}, error) {
	typeBytes := make([]byte, 1)
	if _, err := reader.Read(typeBytes); err != nil {
		return nil, err
	}

	valueType := typeBytes[0]

	switch valueType {
	case 0x00: // Number (double)
		numBytes := make([]byte, 8)
		if _, err := reader.Read(numBytes); err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(numBytes)), nil

	case 0x01: // Boolean
		boolBytes := make([]byte, 1)
		if _, err := reader.Read(boolBytes); err != nil {
			return nil, err
		}
		return boolBytes[0] != 0, nil

	case 0x02: // String
		lenBytes := make([]byte, 2)
		if _, err := reader.Read(lenBytes); err != nil {
			return nil, err
		}
		strlen := binary.BigEndian.Uint16(lenBytes)
		strBytes := make([]byte, strlen)
		if _, err := reader.Read(strBytes); err != nil {
			return nil, err
		}
		return string(strBytes), nil

	case 0x03: // Object
		obj := make(map[string]interface{})
		for {
			// 读取属性名
			lenBytes := make([]byte, 2)
			if _, err := reader.Read(lenBytes); err != nil {
				break
			}
			strlen := binary.BigEndian.Uint16(lenBytes)

			if strlen == 0 {
				// 对象结束标记
				endMarker := make([]byte, 1)
				reader.Read(endMarker)
				break
			}

			keyBytes := make([]byte, strlen)
			if _, err := reader.Read(keyBytes); err != nil {
				break
			}
			key := string(keyBytes)

			// 读取属性值
			if value, err := p.readAMF0Value(reader); err == nil {
				obj[key] = value
			}
		}
		return obj, nil

	case 0x08: // Associative array
		lenBytes := make([]byte, 4)
		if _, err := reader.Read(lenBytes); err != nil {
			return nil, err
		}
		arrayLen := binary.BigEndian.Uint32(lenBytes)

		obj := make(map[string]interface{})
		for i := 0; i < int(arrayLen); i++ {
			// 读取键
			lenBytes := make([]byte, 2)
			if _, err := reader.Read(lenBytes); err != nil {
				break
			}
			strlen := binary.BigEndian.Uint16(lenBytes)
			keyBytes := make([]byte, strlen)
			if _, err := reader.Read(keyBytes); err != nil {
				break
			}
			key := string(keyBytes)

			// 读取值
			if value, err := p.readAMF0Value(reader); err == nil {
				obj[key] = value
			}
		}
		return obj, nil

	case 0x09: // Strict array
		lenBytes := make([]byte, 4)
		if _, err := reader.Read(lenBytes); err != nil {
			return nil, err
		}
		arrayLen := binary.BigEndian.Uint32(lenBytes)

		arr := make([]interface{}, 0)
		for i := 0; i < int(arrayLen); i++ {
			if value, err := p.readAMF0Value(reader); err == nil {
				arr = append(arr, value)
			}
		}
		return arr, nil

	case 0x0A: // Null
		return nil, nil

	case 0x0B: // Undefined
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown AMF0 type: %x", valueType)
	}
}

// extractMetadataFromObject 从对象中提取元数据
func (p *FLVParser) extractMetadataFromObject(obj map[string]interface{}, metadata *FLVMetadata) {
	// 标准FLV元数据字段
	if duration, ok := obj["duration"].(float64); ok {
		metadata.Duration = duration
	}

	if width, ok := obj["width"].(float64); ok {
		metadata.Width = int(width)
	}

	if height, ok := obj["height"].(float64); ok {
		metadata.Height = int(height)
	}

	if frameRate, ok := obj["framerate"].(float64); ok {
		metadata.FrameRate = frameRate
	}

	if bitRate, ok := obj["bitrate"].(float64); ok {
		metadata.BitRate = int(bitRate)
	}

	// 寻找自定义字段（录播姬特定）
	// 录播姬可能添加自定义字段：sessionID, roomId, uid, uname, title等
	if sessionID, ok := obj["sessionID"].(string); ok && sessionID != "" {
		metadata.SessionID = sessionID
		log.Printf("[FLVParser] 从FLV元数据读取SessionID: %s", sessionID)
	}

	if roomID, ok := obj["roomId"].(string); ok && roomID != "" {
		metadata.RoomID = roomID
	}

	if uname, ok := obj["uname"].(string); ok && uname != "" {
		metadata.Uname = uname
	}

	if title, ok := obj["title"].(string); ok && title != "" {
		metadata.Title = title
	}

	// 尝试从自定义JSON字段读取（某些录播工具可能使用这种方式）
	if customStr, ok := obj["custom"].(string); ok {
		var customData map[string]interface{}
		if err := json.Unmarshal([]byte(customStr), &customData); err == nil {
			if sessionID, ok := customData["sessionID"].(string); ok && sessionID != "" {
				metadata.SessionID = sessionID
			}
		}
	}

	// 尝试从其他可能的字段名读取（兼容不同的录播工具）
	for key, value := range obj {
		keyLower := strings.ToLower(key)

		// 匹配SessionID相关字段名
		if (keyLower == "sessionid" || keyLower == "session_id" || keyLower == "session") && metadata.SessionID == "" {
			if sessionID, ok := value.(string); ok && sessionID != "" {
				metadata.SessionID = sessionID
				log.Printf("[FLVParser] 从FLV元数据读取SessionID (字段名:%s): %s", key, sessionID)
			}
		}

		// 匹配RoomID相关字段名
		if (keyLower == "roomid" || keyLower == "room_id" || keyLower == "room") && metadata.RoomID == "" {
			if roomID, ok := value.(string); ok && roomID != "" {
				metadata.RoomID = roomID
			} else if roomID, ok := value.(float64); ok {
				metadata.RoomID = fmt.Sprintf("%.0f", roomID)
			}
		}
	}
}
