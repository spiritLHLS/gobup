package services

import (
	"fmt"
	"strings"
	"time"
)

// TemplateService 模板服务
type TemplateService struct{}

// NewTemplateService 创建模板服务
func NewTemplateService() *TemplateService {
	return &TemplateService{}
}

// RenderTitle 渲染标题模板
func (s *TemplateService) RenderTitle(template string, data map[string]interface{}) string {
	if template == "" {
		return s.getDefault(data)
	}
	return s.render(template, data)
}

// RenderDescription 渲染描述模板
func (s *TemplateService) RenderDescription(template string, data map[string]interface{}) string {
	if template == "" {
		return s.getDefaultDesc(data)
	}
	return s.render(template, data)
}

// RenderPartTitle 渲染分P标题模板
func (s *TemplateService) RenderPartTitle(template string, data map[string]interface{}) string {
	if template == "" {
		return fmt.Sprintf("P%d", data["index"])
	}
	return s.render(template, data)
}

// RenderDynamic 渲染动态模板
func (s *TemplateService) RenderDynamic(template string, data map[string]interface{}) string {
	if template == "" {
		return ""
	}
	return s.render(template, data)
}

// render 渲染模板
func (s *TemplateService) render(template string, data map[string]interface{}) string {
	result := template

	// 替换基础变量
	if uname, ok := data["uname"].(string); ok {
		result = strings.ReplaceAll(result, "${uname}", uname)
	}
	if title, ok := data["title"].(string); ok {
		result = strings.ReplaceAll(result, "${title}", title)
	}
	if roomId, ok := data["roomId"].(string); ok {
		result = strings.ReplaceAll(result, "${roomId}", roomId)
	}
	if areaName, ok := data["areaName"].(string); ok {
		result = strings.ReplaceAll(result, "${areaName}", areaName)
	}
	if uid, ok := data["uid"].(int64); ok {
		result = strings.ReplaceAll(result, "${@uid}", fmt.Sprintf("%d", uid))
	}
	if index, ok := data["index"].(int); ok {
		result = strings.ReplaceAll(result, "${index}", fmt.Sprintf("%d", index))
	}
	if fileName, ok := data["fileName"].(string); ok {
		result = strings.ReplaceAll(result, "${fileName}", fileName)
	}

	// 替换时间变量
	var startTime time.Time
	if t, ok := data["startTime"].(time.Time); ok {
		startTime = t
	} else {
		startTime = time.Now()
	}

	// 常用时间格式
	result = strings.ReplaceAll(result, "${yyyy年MM月dd日HH点mm分}", startTime.Format("2006年01月02日15点04分"))
	result = strings.ReplaceAll(result, "${yyyy-MM-dd HH:mm}", startTime.Format("2006-01-02 15:04"))
	result = strings.ReplaceAll(result, "${yyyy-MM-dd}", startTime.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "${MM月dd日HH点mm分}", startTime.Format("01月02日15点04分"))
	result = strings.ReplaceAll(result, "${HH:mm}", startTime.Format("15:04"))

	// 处理自定义日期格式 ${date:format}
	if strings.Contains(result, "${date:") {
		start := strings.Index(result, "${date:")
		if start >= 0 {
			end := strings.Index(result[start:], "}")
			if end > 0 {
				formatStr := result[start+7 : start+end]
				formatted := s.formatDate(startTime, formatStr)
				result = result[:start] + formatted + result[start+end+1:]
			}
		}
	}

	return result
}

// formatDate 格式化日期
func (s *TemplateService) formatDate(t time.Time, format string) string {
	// 转换常见格式
	format = strings.ReplaceAll(format, "yyyy", "2006")
	format = strings.ReplaceAll(format, "MM", "01")
	format = strings.ReplaceAll(format, "dd", "02")
	format = strings.ReplaceAll(format, "HH", "15")
	format = strings.ReplaceAll(format, "mm", "04")
	format = strings.ReplaceAll(format, "ss", "05")

	return t.Format(format)
}

// getDefault 获取默认标题
func (s *TemplateService) getDefault(data map[string]interface{}) string {
	uname := s.getString(data, "uname", "未知主播")
	startTime := s.getTime(data, "startTime", time.Now())
	return fmt.Sprintf("%s %s 直播录像", uname, startTime.Format("2006-01-02 15:04"))
}

// getDefaultDesc 获取默认描述
func (s *TemplateService) getDefaultDesc(data map[string]interface{}) string {
	uname := s.getString(data, "uname", "未知主播")
	roomId := s.getString(data, "roomId", "")
	title := s.getString(data, "title", "")
	startTime := s.getTime(data, "startTime", time.Now())

	desc := fmt.Sprintf("主播: %s\n", uname)
	if roomId != "" {
		desc += fmt.Sprintf("房间号: %s\n", roomId)
	}
	if title != "" {
		desc += fmt.Sprintf("直播标题: %s\n", title)
	}
	desc += fmt.Sprintf("录制时间: %s\n", startTime.Format("2006-01-02 15:04:05"))
	desc += "\n本视频由GoBup自动录制并上传"

	return desc
}

func (s *TemplateService) getString(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return defaultValue
}

func (s *TemplateService) getTime(data map[string]interface{}, key string, defaultValue time.Time) time.Time {
	if val, ok := data[key].(time.Time); ok {
		return val
	}
	return defaultValue
}

// BuildTags 构建标签
func (s *TemplateService) BuildTags(tagStr string, data map[string]interface{}) []string {
	if tagStr == "" {
		return []string{}
	}

	// 渲染模板
	rendered := s.render(tagStr, data)

	// 分割标签
	tags := strings.Split(rendered, ",")
	var result []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			result = append(result, tag)
		}
	}

	return result
}
