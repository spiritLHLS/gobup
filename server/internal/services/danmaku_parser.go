package services

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

// DanmakuXMLParser 弹幕XML解析器
type DanmakuXMLParser struct{}

func NewDanmakuXMLParser() *DanmakuXMLParser {
	return &DanmakuXMLParser{}
}

// XML结构定义（兼容录播姬和blrec）
type DanmakuXML struct {
	XMLName xml.Name `xml:"i"`
	D       []D      `xml:"d"`     // 普通弹幕
	SC      []SC     `xml:"sc"`    // SC留言
	Gift    []Gift   `xml:"gift"`  // 礼物
	Guard   []Guard  `xml:"guard"` // 上舰
}

// D 普通弹幕
type D struct {
	P    string `xml:"p,attr"`    // 参数: 时间戳,模式,字号,颜色,发送时间,弹幕池,用户ID,弹幕ID
	Text string `xml:",chardata"` // 弹幕内容
	Raw  string `xml:"raw,attr"`  // 原始数据（blrec格式，包含更多信息）
}

// SC Super Chat 留言
type SC struct {
	TS    string `xml:"ts,attr"`    // 时间戳
	User  string `xml:"user,attr"`  // 用户名
	Price string `xml:"price,attr"` // 价格
	Text  string `xml:",chardata"`  // 留言内容
}

// Gift 礼物
type Gift struct {
	TS       string `xml:"ts,attr"`       // 时间戳
	User     string `xml:"user,attr"`     // 用户名
	GiftName string `xml:"giftName,attr"` // 礼物名称
	Num      string `xml:"num,attr"`      // 数量
}

// Guard 上舰/续费
type Guard struct {
	TS    string `xml:"ts,attr"`    // 时间戳
	User  string `xml:"user,attr"`  // 用户名
	Level string `xml:"level,attr"` // 等级 (1=总督, 2=提督, 3=舰长)
	Count string `xml:"count,attr"` // 数量/月数
}

// ParseDanmakuFile 解析弹幕XML文件
func (p *DanmakuXMLParser) ParseDanmakuFile(xmlPath string, sessionID string) (int, error) {
	log.Printf("[弹幕解析] 开始解析文件: %s (session_id=%s)", xmlPath, sessionID)

	// 打开XML文件
	file, err := os.Open(xmlPath)
	if err != nil {
		return 0, fmt.Errorf("无法打开文件: %w", err)
	}
	defer file.Close()

	// 解析XML
	data, err := io.ReadAll(file)
	if err != nil {
		return 0, fmt.Errorf("读取文件失败: %w", err)
	}

	var dmXML DanmakuXML
	if err := xml.Unmarshal(data, &dmXML); err != nil {
		return 0, fmt.Errorf("解析XML失败: %w", err)
	}

	db := database.GetDB()
	count := 0

	log.Printf("[弹幕解析] 解析到: 普通弹幕=%d, SC=%d, 礼物=%d, 上舰=%d",
		len(dmXML.D), len(dmXML.SC), len(dmXML.Gift), len(dmXML.Guard))

	// 收集所有需要保存的弹幕
	var msgsToSave []*models.LiveMsg

	// 解析普通弹幕
	for _, d := range dmXML.D {
		msg, err := p.parseDanmaku(d, sessionID)
		if err != nil {
			log.Printf("[弹幕解析] ⚠️  解析弹幕失败: %v", err)
			continue
		}
		msgsToSave = append(msgsToSave, msg)
	}

	// 解析SC留言
	for _, sc := range dmXML.SC {
		msg, err := p.parseSC(sc, sessionID)
		if err != nil {
			log.Printf("[弹幕解析] ⚠️  解析SC失败: %v", err)
			continue
		}
		msgsToSave = append(msgsToSave, msg)
	}

	// 解析上舰（转换为特殊弹幕）
	for _, guard := range dmXML.Guard {
		msg, err := p.parseGuard(guard, sessionID)
		if err != nil {
			log.Printf("[弹幕解析] ⚠️  解析上舰失败: %v", err)
			continue
		}
		msgsToSave = append(msgsToSave, msg)
	}

	// 使用事务批量保存，减少数据库锁定时间
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, msg := range msgsToSave {
			// 检查是否已存在（去重）
			var existing models.LiveMsg
			if err := tx.Where("session_id = ? AND timestamp = ? AND message = ?",
				msg.SessionID, msg.Timestamp, msg.Message).First(&existing).Error; err == nil {
				// 已存在，跳过
				continue
			}

			// 保存到数据库
			if err := tx.Create(msg).Error; err != nil {
				log.Printf("[弹幕解析] ❌ 保存弹幕失败: %v", err)
				// 不中断事务，继续处理下一条
				continue
			}
			count++
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("保存弹幕事务失败: %w", err)
	}

	log.Printf("[弹幕解析] ✅ 解析完成: 成功导入 %d 条弹幕", count)
	return count, nil
}

// parseDanmaku 解析普通弹幕
func (p *DanmakuXMLParser) parseDanmaku(d D, sessionID string) (*models.LiveMsg, error) {
	// 解析p属性: 时间戳,模式,字号,颜色,发送时间,弹幕池,用户ID,弹幕ID
	parts := strings.Split(d.P, ",")
	if len(parts) < 8 {
		return nil, fmt.Errorf("弹幕格式错误: p属性字段不足")
	}

	// 时间戳（秒 -> 毫秒）
	timestamp, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, fmt.Errorf("解析时间戳失败: %w", err)
	}
	timestampMs := int64(timestamp * 1000)

	// 模式
	mode, _ := strconv.Atoi(parts[1])

	// 字号
	fontSize, _ := strconv.Atoi(parts[2])

	// 颜色
	color, _ := strconv.Atoi(parts[3])

	// 用户ID（可能为空）
	uid, _ := strconv.ParseInt(parts[6], 10, 64)

	msg := &models.LiveMsg{
		SessionID: sessionID,
		Timestamp: timestampMs,
		Message:   strings.TrimSpace(d.Text),
		Mode:      mode,
		FontSize:  fontSize,
		Color:     color,
		UID:       uid,
		ULevel:    0, // 默认值，如果有raw数据可以进一步解析
		Sent:      false,
	}

	// 如果有raw数据（blrec格式），可以解析更多信息
	if d.Raw != "" {
		p.parseRawData(d.Raw, msg)
		// 检查是否为抽奖弹幕（parseRawData会将抽奖弹幕的message设为空）
		if msg.Message == "" {
			return nil, fmt.Errorf("抽奖弹幕，已过滤")
		}
	}

	return msg, nil
}

// parseSC 解析SC留言
func (p *DanmakuXMLParser) parseSC(sc SC, sessionID string) (*models.LiveMsg, error) {
	// 时间戳（秒 -> 毫秒）
	timestamp, err := strconv.ParseFloat(sc.TS, 64)
	if err != nil {
		return nil, fmt.Errorf("解析时间戳失败: %w", err)
	}
	timestampMs := int64(timestamp * 1000)

	// 价格
	price, _ := strconv.Atoi(sc.Price)

	// blrec的金额需要除以1000
	if price > 1000 {
		price = price / 1000
	}

	// 构建SC消息
	message := fmt.Sprintf("%s发送了%d元留言：%s", sc.User, price, sc.Text)
	if len(message) > 100 {
		message = message[:99]
	}

	return &models.LiveMsg{
		SessionID: sessionID,
		Timestamp: timestampMs,
		Message:   message,
		Mode:      5,        // 顶部弹幕
		FontSize:  64,       // 大字号
		Color:     16776960, // 金色
		Sent:      false,
	}, nil
}

// parseGuard 解析上舰信息
func (p *DanmakuXMLParser) parseGuard(guard Guard, sessionID string) (*models.LiveMsg, error) {
	// 时间戳（秒 -> 毫秒）
	timestamp, err := strconv.ParseFloat(guard.TS, 64)
	if err != nil {
		return nil, fmt.Errorf("解析时间戳失败: %w", err)
	}
	timestampMs := int64(timestamp * 1000)

	// 上舰等级
	level, _ := strconv.Atoi(guard.Level)
	levelName := "舰长"
	switch level {
	case 1:
		levelName = "总督"
	case 2:
		levelName = "提督"
	case 3:
		levelName = "舰长"
	}

	count, _ := strconv.Atoi(guard.Count)

	// 构建上舰消息
	message := fmt.Sprintf("%s开通了%d个月%s", guard.User, count, levelName)
	if len(message) > 100 {
		message = message[:99]
	}

	return &models.LiveMsg{
		SessionID: sessionID,
		Timestamp: timestampMs,
		Message:   message,
		Mode:      5,        // 顶部弹幕
		FontSize:  64,       // 大字号
		Color:     16776960, // 金色
		Sent:      false,
	}, nil
}

// parseRawData 解析原始数据（blrec格式）
func (p *DanmakuXMLParser) parseRawData(raw string, msg *models.LiveMsg) {
	// raw格式: [[时间,模式,字号,颜色,时间戳,弹幕池,用户ID,弹幕ID,权重,抽奖标志],[...],[粉丝勋章信息],[用户等级,...],...]
	var rawData []interface{}
	if err := json.Unmarshal([]byte(raw), &rawData); err != nil {
		return
	}

	if len(rawData) < 5 {
		return
	}

	// 解析基本信息 rawData[0]
	if basicInfo, ok := rawData[0].([]interface{}); ok && len(basicInfo) >= 10 {
		// 检查是否为抽奖弹幕（索引9）
		if lottery, ok := basicInfo[9].(float64); ok && int(lottery) != 0 {
			msg.Message = "" // 标记为需要过滤的抽奖弹幕
			return
		}
	}

	// 解析粉丝勋章信息 rawData[3]
	if medalInfo, ok := rawData[3].([]interface{}); ok && len(medalInfo) >= 4 {
		// [勋章等级, 勋章名称, 主播名称, 房间ID]
		if level, ok := medalInfo[0].(float64); ok {
			msg.MedalLevel = int(level)
		}
		if name, ok := medalInfo[1].(string); ok {
			msg.MedalName = name
		}
	}

	// 解析用户等级信息 rawData[4]
	if userInfo, ok := rawData[4].([]interface{}); ok && len(userInfo) >= 1 {
		if ulLevel, ok := userInfo[0].(float64); ok {
			msg.ULevel = int(ulLevel)
		}
	}

	// 解析用户名 rawData[2] 可能包含用户名
	if userNameInfo, ok := rawData[2].([]interface{}); ok && len(userNameInfo) >= 2 {
		if userName, ok := userNameInfo[1].(string); ok {
			msg.UserName = userName
		}
	}
}

// ParseDanmakuForHistory 为历史记录解析弹幕XML文件
func (p *DanmakuXMLParser) ParseDanmakuForHistory(historyID uint) (int, error) {
	db := database.GetDB()

	// 获取历史记录
	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return 0, fmt.Errorf("历史记录不存在: %w", err)
	}

	// 获取所有分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ?", historyID).Find(&parts).Error; err != nil {
		return 0, fmt.Errorf("查询分P失败: %w", err)
	}

	if len(parts) == 0 {
		return 0, fmt.Errorf("没有找到分P记录")
	}

	totalCount := 0

	// 对每个分P查找对应的XML文件
	for _, part := range parts {
		// 尝试多种可能的XML文件路径
		xmlPaths := []string{
			strings.TrimSuffix(part.FilePath, filepath.Ext(part.FilePath)) + ".xml",
			filepath.Join(filepath.Dir(part.FilePath), strings.TrimSuffix(filepath.Base(part.FilePath), filepath.Ext(part.FilePath))+".xml"),
		}

		var xmlPath string
		for _, path := range xmlPaths {
			if _, err := os.Stat(path); err == nil {
				xmlPath = path
				break
			}
		}

		if xmlPath == "" {
			log.Printf("[弹幕解析] ⚠️  未找到弹幕XML文件: %s", part.FilePath)
			continue
		}

		// 解析XML文件
		count, err := p.ParseDanmakuFile(xmlPath, history.SessionID)
		if err != nil {
			log.Printf("[弹幕解析] ❌ 解析失败: %s, error: %v", xmlPath, err)
			continue
		}

		totalCount += count
	}

	if totalCount == 0 {
		return 0, fmt.Errorf("没有解析到任何弹幕")
	}

	// 更新历史记录的弹幕统计
	history.DanmakuCount = totalCount
	db.Save(&history)

	log.Printf("[弹幕解析] ✅ 历史记录%d解析完成: 共导入 %d 条弹幕", historyID, totalCount)

	return totalCount, nil
}
