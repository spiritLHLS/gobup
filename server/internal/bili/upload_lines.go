package bili

// UploadLine 上传线路配置
type UploadLine struct {
	Name        string // 线路名称
	DisplayName string // 显示名称
	UploadType  string // 上传类型: upos, kodo, app
	CDN         string // CDN类型
	Profile     string // profile参数
	Description string // 描述信息
}

// GetAllUploadLines 获取所有可用的上传线路
func GetAllUploadLines() []UploadLine {
	return []UploadLine{
		// UPOS线路 - 百度云系列
		{Name: "cs_bda2", DisplayName: "百度云-BDA2", UploadType: "upos", CDN: "bda2", Profile: "ugcupos/bup", Description: "百度云BDA2线路（推荐）"},
		{Name: "cs_bldsa", DisplayName: "百度云-BLDSA", UploadType: "upos", CDN: "bldsa", Profile: "ugcupos/bup", Description: "百度云BLDSA线路"},

		// UPOS线路 - 腾讯云系列
		{Name: "cs_tx", DisplayName: "腾讯云-TX", UploadType: "upos", CDN: "tx", Profile: "ugcupos/bup", Description: "腾讯云TX线路"},
		{Name: "cs_estx", DisplayName: "腾讯云-ESTX", UploadType: "upos", CDN: "estx", Profile: "ugcupos/bup", Description: "腾讯云ESTX线路（新）"},
		{Name: "cs_txa", DisplayName: "腾讯云-TXA", UploadType: "upos", CDN: "txa", Profile: "ugcupos/bup", Description: "腾讯云TXA线路"},

		// UPOS线路 - 阿里云系列
		{Name: "cs_alia", DisplayName: "阿里云-ALIA", UploadType: "upos", CDN: "alia", Profile: "ugcupos/bup", Description: "阿里云ALIA线路"},

		// UPOS线路 - 中国大陆系列
		{Name: "cs_cnbldsa", DisplayName: "中国大陆-B站自建", UploadType: "upos", CDN: "cnbldsa", Profile: "ugcupos/bup", Description: "B站自建线路"},
		{Name: "cs_cnbd", DisplayName: "中国大陆-百度云", UploadType: "upos", CDN: "cnbd", Profile: "ugcupos/bup", Description: "中国大陆百度云"},

		// Kodo线路
		{Name: "kodo", DisplayName: "七牛云Kodo", UploadType: "kodo", CDN: "", Profile: "ugcupos/bup", Description: "七牛云Kodo上传"},

		// App线路
		{Name: "app", DisplayName: "App上传", UploadType: "app", CDN: "", Profile: "", Description: "App端上传（小文件）"},
	}
}

// GetUploadLine 根据名称获取上传线路
func GetUploadLine(name string) *UploadLine {
	lines := GetAllUploadLines()
	for _, line := range lines {
		if line.Name == name {
			return &line
		}
	}
	return nil
}

// GetDefaultLines 获取默认推荐的线路列表
func GetDefaultLines() []string {
	return []string{
		"cs_bda2",    // 百度云BDA2（最常用）
		"cs_tx",      // 腾讯云TX
		"cs_bldsa",   // 百度云BLDSA
		"cs_estx",    // 腾讯云ESTX（新）
		"cs_cnbldsa", // B站自建
		"kodo",       // 七牛云Kodo（备用）
	}
}

// ParseLineConfig 从配置字符串解析线路列表
func ParseLineConfig(config string) []string {
	if config == "" {
		return GetDefaultLines()
	}

	// 逗号分隔的线路列表
	lines := []string{}
	for _, line := range splitAndTrim(config, ",") {
		if line != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return GetDefaultLines()
	}

	return lines
}

// splitAndTrim 分割字符串并去除空格
func splitAndTrim(s, sep string) []string {
	parts := []string{}
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
