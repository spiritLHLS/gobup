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
		// UPOS线路 - 默认线路系列（推荐）
		{Name: "cs_bda2", DisplayName: "百度云-BDA2", UploadType: "upos", CDN: "bda2", Profile: "ugcupos/bup", Description: "百度云BDA2线路（推荐）"},
		{Name: "cs_bldsa", DisplayName: "百度云-BLDSA", UploadType: "upos", CDN: "bldsa", Profile: "ugcupos/bup", Description: "百度云BLDSA线路（推荐）"},
		{Name: "cs_tx", DisplayName: "腾讯云-TX", UploadType: "upos", CDN: "tx", Profile: "ugcupos/bup", Description: "腾讯云TX线路（推荐）"},
		{Name: "cs_estx", DisplayName: "腾讯云-ESTX", UploadType: "upos", CDN: "estx", Profile: "ugcupos/bup", Description: "腾讯云ESTX线路（新）"},
		{Name: "cs_txa", DisplayName: "腾讯云-TXA", UploadType: "upos", CDN: "txa", Profile: "ugcupos/bup", Description: "腾讯云TXA线路"},
		{Name: "cs_alia", DisplayName: "阿里云-ALIA", UploadType: "upos", CDN: "alia", Profile: "ugcupos/bup", Description: "阿里云ALIA线路"},

		// UPOS线路 - 京东云系列
		{Name: "jd_bd", DisplayName: "京东云-百度", UploadType: "upos", CDN: "bd", Profile: "ugcupos/bup", Description: "京东云百度线路"},
		{Name: "jd_bldsa", DisplayName: "京东云-B站", UploadType: "upos", CDN: "bldsa", Profile: "ugcupos/bup", Description: "京东云B站线路"},
		{Name: "jd_tx", DisplayName: "京东云-腾讯", UploadType: "upos", CDN: "tx", Profile: "ugcupos/bup", Description: "京东云腾讯线路"},
		{Name: "jd_txa", DisplayName: "京东云-腾讯A", UploadType: "upos", CDN: "txa", Profile: "ugcupos/bup", Description: "京东云腾讯A线路"},
		{Name: "jd_alia", DisplayName: "京东云-阿里", UploadType: "upos", CDN: "alia", Profile: "ugcupos/bup", Description: "京东云阿里线路"},

		// UPOS线路 - 中国大陆系列
		{Name: "cs_cnbldsa", DisplayName: "中国大陆-B站", UploadType: "upos", CDN: "cnbldsa", Profile: "ugcupos/bup", Description: "中国大陆B站线路（推荐）"},
		{Name: "cs_cnbd", DisplayName: "中国大陆-百度", UploadType: "upos", CDN: "cnbd", Profile: "ugcupos/bup", Description: "中国大陆百度线路"},
		{Name: "cs_cntx", DisplayName: "中国大陆-腾讯", UploadType: "upos", CDN: "cntx", Profile: "ugcupos/bup", Description: "中国大陆腾讯线路"},

		// UPOS线路 - 北美系列
		{Name: "cs_andsa", DisplayName: "北美-B站", UploadType: "upos", CDN: "andsa", Profile: "ugcupos/bup", Description: "北美B站线路"},
		{Name: "cs_anbd", DisplayName: "北美-百度", UploadType: "upos", CDN: "anbd", Profile: "ugcupos/bup", Description: "北美百度线路"},
		{Name: "cs_antx", DisplayName: "北美-腾讯", UploadType: "upos", CDN: "antx", Profile: "ugcupos/bup", Description: "北美腾讯线路"},

		// UPOS线路 - 台湾系列
		{Name: "cs_atdsa", DisplayName: "台湾-B站", UploadType: "upos", CDN: "atdsa", Profile: "ugcupos/bup", Description: "台湾B站线路"},
		{Name: "cs_atbd", DisplayName: "台湾-百度", UploadType: "upos", CDN: "atbd", Profile: "ugcupos/bup", Description: "台湾百度线路"},
		{Name: "cs_attx", DisplayName: "台湾-腾讯", UploadType: "upos", CDN: "attx", Profile: "ugcupos/bup", Description: "台湾腾讯线路"},

		// UPOS线路 - 香港系列
		{Name: "cs_akbd", DisplayName: "香港-百度", UploadType: "upos", CDN: "akbd", Profile: "ugcupos/bup", Description: "香港百度线路"},

		// UPOS线路 - 其他
		{Name: "upos", DisplayName: "UPOS默认", UploadType: "upos", CDN: "", Profile: "ugcupos/bup", Description: "UPOS默认线路"},

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
