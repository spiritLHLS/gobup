package controllers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
	"github.com/imroc/req/v3"
)

// ListRooms 列出全部房间配置。
//
// @Summary List rooms
// @Description Lists recording rooms and their upload settings.
// @Tags rooms
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {array} models.RecordRoom
// @Router /room [post]
func ListRooms(c *gin.Context) {
	db := database.GetDB()
	var rooms []models.RecordRoom
	db.Find(&rooms)
	c.JSON(http.StatusOK, rooms)
}

func AddRoom(c *gin.Context) {
	var req struct {
		RoomID string `json:"roomId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}

	db := database.GetDB()
	var existing models.RecordRoom
	if err := db.Where("room_id = ?", req.RoomID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"type": "warning", "msg": "房间已存在"})
		return
	}

	room := models.RecordRoom{RoomID: req.RoomID}
	normalizeRoomConfig(&room)
	db.Create(&room)
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "添加成功"})
}

func UpdateRoom(c *gin.Context) {
	var room models.RecordRoom
	if err := c.ShouldBindJSON(&room); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 确保转载来源模板有默认值
	if room.Copyright == 2 && room.SourceTemplate == "" {
		room.SourceTemplate = "直播间: https://live.bilibili.com/${roomId}  稿件直播源"
	}
	normalizeRoomConfig(&room)

	db := database.GetDB()
	db.Save(&room)
	c.JSON(http.StatusOK, true)
}

func normalizeRoomConfig(room *models.RecordRoom) {
	if room.UploadUserStrategy == "" {
		room.UploadUserStrategy = "fixed"
	}
	if room.UploadWindowStart == "" {
		room.UploadWindowStart = "00:00"
	}
	if room.UploadWindowEnd == "" {
		room.UploadWindowEnd = "23:59"
	}
	if room.Priority < 0 {
		room.Priority = 0
	}
	if room.ID == 0 && room.Priority == 0 {
		room.Priority = 50
	}
	if room.Priority > 100 {
		room.Priority = 100
	}
	if room.UploadSpeedLimitMBps < 0 {
		room.UploadSpeedLimitMBps = 0
	}
	if room.CoverFrameSecond < 0 {
		room.CoverFrameSecond = 5
	}
	switch strings.TrimSpace(room.DanmakuBurnStyle) {
	case "compact", "large":
		room.DanmakuBurnStyle = strings.TrimSpace(room.DanmakuBurnStyle)
	default:
		room.DanmakuBurnStyle = "default"
	}
	if room.DanmakuFontSize < 0 {
		room.DanmakuFontSize = 0
	}
	if room.DanmakuFontSize > 0 && room.DanmakuFontSize < 12 {
		room.DanmakuFontSize = 12
	}
	if room.DanmakuFontSize > 72 {
		room.DanmakuFontSize = 72
	}
	room.DanmakuFontColor = strings.TrimSpace(room.DanmakuFontColor)
	if room.DanmakuScrollArea <= 0 {
		room.DanmakuScrollArea = 0.75
	}
	if room.DanmakuScrollArea < 0.1 {
		room.DanmakuScrollArea = 0.1
	}
	if room.DanmakuScrollArea > 1 {
		room.DanmakuScrollArea = 1
	}
	if room.DanmakuDisplayArea <= 0 {
		room.DanmakuDisplayArea = 0.8
	}
	if room.DanmakuDisplayArea < 0.1 {
		room.DanmakuDisplayArea = 0.1
	}
	if room.DanmakuDisplayArea > 1 {
		room.DanmakuDisplayArea = 1
	}
	if room.TranscodePreset == "" {
		room.TranscodePreset = "veryfast"
	}
	if room.TranscodeCRF == 0 {
		room.TranscodeCRF = 23
	}
	if room.TranscodeCRF < 18 {
		room.TranscodeCRF = 18
	}
	if room.TranscodeCRF > 35 {
		room.TranscodeCRF = 35
	}
	if room.TranscodeAudioBitrate == "" {
		room.TranscodeAudioBitrate = "160k"
	}
	room.TranscodeVideoCodec = models.NormalizeTranscodeVideoCodec(room.TranscodeVideoCodec)
}

func DeleteRoom(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()
	db.Delete(&models.RecordRoom{}, id)
	c.JSON(http.StatusOK, gin.H{"type": "success", "msg": "删除成功"})
}

// UploadLine 上传线路
type UploadLine struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Region      string `json:"region"`      // 地区分类：cn, an, at, ak
	Continent   string `json:"continent"`   // 大洲：asia, america, etc
	Provider    string `json:"provider"`    // 服务商：bilibili, baidu, tencent, aliyun, qiniu
	Recommended bool   `json:"recommended"` // 是否推荐
	LineQuery   string `json:"lineQuery"`   // 线路查询参数，用于测速
}

// getAllUploadLines 返回所有上传线路（内部使用）
func getAllUploadLines() []UploadLine {
	return []UploadLine{
		// 默认线路
		{Value: "cs_bda2", Label: "CS_BDA2", Description: "默认线路", Region: "cs", Continent: "asia", Provider: "bilibili", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=bda2"},
		{Value: "cs_bldsa", Label: "CS_BLDSA", Description: "默认线路", Region: "cs", Continent: "asia", Provider: "bilibili", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=bldsa"},
		{Value: "cs_tx", Label: "CS_TX", Description: "腾讯云", Region: "cs", Continent: "asia", Provider: "tencent", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=tx"},
		{Value: "cs_estx", Label: "CS_ESTX", Description: "腾讯云(新增)", Region: "cs", Continent: "asia", Provider: "tencent", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=estx"},
		{Value: "cs_txa", Label: "CS_TXA", Description: "腾讯云A", Region: "cs", Continent: "asia", Provider: "tencent", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=txa"},
		{Value: "cs_alia", Label: "CS_ALIA", Description: "阿里云", Region: "cs", Continent: "asia", Provider: "aliyun", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=alia"},
		{Value: "jd_bd", Label: "JD_BD", Description: "百度云", Region: "jd", Continent: "asia", Provider: "baidu", Recommended: false, LineQuery: "?os=upos&zone=jd&upcdn=bd"},
		{Value: "jd_bldsa", Label: "JD_BLDSA", Description: "B站自建", Region: "jd", Continent: "asia", Provider: "bilibili", Recommended: false, LineQuery: "?os=upos&zone=jd&upcdn=bldsa"},
		{Value: "jd_tx", Label: "JD_TX", Description: "腾讯云", Region: "jd", Continent: "asia", Provider: "tencent", Recommended: false, LineQuery: "?os=upos&zone=jd&upcdn=tx"},
		{Value: "jd_txa", Label: "JD_TXA", Description: "腾讯云A", Region: "jd", Continent: "asia", Provider: "tencent", Recommended: false, LineQuery: "?os=upos&zone=jd&upcdn=txa"},
		{Value: "jd_alia", Label: "JD_ALIA", Description: "阿里云", Region: "jd", Continent: "asia", Provider: "aliyun", Recommended: false, LineQuery: "?os=upos&zone=jd&upcdn=alia"},

		// 中国大陆(cn)
		{Value: "cs_cnbldsa", Label: "CS_CNBLDSA", Description: "中国大陆-B站", Region: "cn", Continent: "asia", Provider: "bilibili", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=cnbldsa"},
		{Value: "cs_cnbd", Label: "CS_CNBD", Description: "中国大陆-百度", Region: "cn", Continent: "asia", Provider: "baidu", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=cnbd"},
		{Value: "cs_cntx", Label: "CS_CNTX", Description: "中国大陆-腾讯", Region: "cn", Continent: "asia", Provider: "tencent", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=cntx"},

		// 北美(an)
		{Value: "cs_andsa", Label: "CS_ANDSA", Description: "北美-B站", Region: "an", Continent: "america", Provider: "bilibili", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=andsa"},
		{Value: "cs_anbd", Label: "CS_ANBD", Description: "北美-百度", Region: "an", Continent: "america", Provider: "baidu", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=anbd"},
		{Value: "cs_antx", Label: "CS_ANTX", Description: "北美-腾讯", Region: "an", Continent: "america", Provider: "tencent", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=antx"},

		// 台湾(at)
		{Value: "cs_atdsa", Label: "CS_ATDSA", Description: "台湾-B站", Region: "at", Continent: "asia", Provider: "bilibili", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=atdsa"},
		{Value: "cs_atbd", Label: "CS_ATBD", Description: "台湾-百度", Region: "at", Continent: "asia", Provider: "baidu", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=atbd"},
		{Value: "cs_attx", Label: "CS_ATTX", Description: "台湾-腾讯", Region: "at", Continent: "asia", Provider: "tencent", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=attx"},

		// 香港(ak)
		{Value: "cs_akbd", Label: "CS_AKBD", Description: "香港-百度", Region: "ak", Continent: "asia", Provider: "baidu", Recommended: true, LineQuery: "?os=upos&zone=cs&upcdn=akbd"},

		// 其他
		{Value: "upos", Label: "UPOS", Description: "UPOS默认", Region: "", Continent: "", Provider: "bilibili", Recommended: false, LineQuery: "?os=upos"},
		{Value: "app", Label: "APP", Description: "APP上传（小文件适用）", Region: "", Continent: "", Provider: "bilibili", Recommended: false, LineQuery: "?os=app"},

		// 废弃线路（兼容性）
		{Value: "cs_qn", Label: "CS_QN_废弃", Description: "七牛云(废弃)", Region: "", Continent: "", Provider: "qiniu", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=qn"},
		{Value: "cs_qnhk", Label: "CS_QNHK_废弃", Description: "七牛香港(废弃)", Region: "", Continent: "", Provider: "qiniu", Recommended: false, LineQuery: "?os=upos&zone=cs&upcdn=qnhk"},
		{Value: "sz_ws", Label: "SZ_WS_废弃", Description: "网宿(废弃)", Region: "", Continent: "", Provider: "bilibili", Recommended: false, LineQuery: "?os=upos&zone=sz&upcdn=ws"},
	}
}

func GetUploadLines(c *gin.Context) {
	lines := getAllUploadLines()
	c.JSON(http.StatusOK, lines)
}

func GetSeasons(c *gin.Context) {
	userIDStr := c.Query("userId")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 userId 参数"})
		return
	}

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId 格式错误"})
		return
	}

	db := database.GetDB()
	var user models.BiliBiliUser
	if err := db.First(&user, uint(userID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	if !user.Login {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未登录"})
		return
	}
	if !user.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "用户已禁用"})
		return
	}

	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	seasons, err := client.GetSeasons()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if seasons == nil {
		seasons = []bili.Season{}
	}
	c.JSON(http.StatusOK, seasons)
}

func CreateSeason(c *gin.Context) {
	var req struct {
		UserID uint   `json:"userId"`
		Title  string `json:"title"`
		Desc   string `json:"desc"`
		Cover  string `json:"cover"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "参数错误"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Desc = strings.TrimSpace(req.Desc)
	req.Cover = strings.TrimSpace(req.Cover)
	if req.UserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "缺少用户ID"})
		return
	}
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "msg": "合集标题不能为空"})
		return
	}

	db := database.GetDB()
	var user models.BiliBiliUser
	if err := db.First(&user, req.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"type": "error", "msg": "用户不存在"})
		return
	}
	if !user.Login {
		c.JSON(http.StatusUnauthorized, gin.H{"type": "error", "msg": "用户未登录"})
		return
	}
	if !user.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "msg": "用户已禁用"})
		return
	}

	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	season, err := client.CreateSeason(req.Title, req.Desc, req.Cover)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"type": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, season)
}

// GetRecommendedLines 获取推荐线路
func GetRecommendedLines(c *gin.Context) {
	continent := c.Query("continent") // asia, america, europe等
	region := c.Query("region")       // cn, an, at, ak等
	provider := c.Query("provider")   // bilibili, tencent, baidu, aliyun, qiniu

	allLines := getAllUploadLines()

	// 筛选
	var filtered []UploadLine
	for _, line := range allLines {
		if continent != "" && line.Continent != continent {
			continue
		}
		if region != "" && line.Region != region {
			continue
		}
		if provider != "" && line.Provider != provider {
			continue
		}
		filtered = append(filtered, line)
	}

	c.JSON(http.StatusOK, filtered)
}

// OfficialLine B站官方线路定义
type OfficialLine struct {
	Query string `json:"query"`
	URL   string `json:"url"`
}

// TestAllLines 批量测试所有线路的可用性（延迟）- 全并发策略
func TestAllLines(c *gin.Context) {
	result := make(map[string]string)
	var mu sync.Mutex

	// 1. 获取官方线路列表（5s超时）
	client := req.C().SetTimeout(10 * time.Second).ImpersonateChrome()
	resp, err := client.R().Get("https://member.bilibili.com/preupload?r=ping&file=lines.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取官方线路失败: " + err.Error()})
		return
	}

	var officialLines []OfficialLine
	if err := json.Unmarshal(resp.Bytes(), &officialLines); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析线路数据失败: " + err.Error()})
		return
	}

	// 2. 构建 query -> url 映射
	queryToURL := make(map[string]string)
	for _, line := range officialLines {
		queryToURL[line.Query] = line.URL
	}

	// 3. 全并发测试：所有线路同时发起，用信号量限制最大并发数为 20
	allLines := getAllUploadLines()
	sem := make(chan struct{}, 20) // 最大并发 20
	var wg sync.WaitGroup

	for _, uploadLine := range allLines {
		wg.Add(1)
		go func(line UploadLine) {
			defer wg.Done()
			sem <- struct{}{}        // 占用并发槽
			defer func() { <-sem }() // 释放并发槽

			// 构建查询 key
			queryKey := line.LineQuery
			if len(queryKey) > 0 && queryKey[0] == '?' {
				queryKey = queryKey[1:]
			}

			testURLStr, exists := queryToURL[queryKey]
			if !exists {
				mu.Lock()
				result[line.Value] = "Unknown"
				mu.Unlock()
				return
			}
			if strings.HasPrefix(testURLStr, "//") {
				testURLStr = "https:" + testURLStr
			}

			start := time.Now()
			testClient := req.C().SetTimeout(5 * time.Second).ImpersonateChrome()
			testResp, testErr := testClient.R().Head(testURLStr)
			if testErr != nil {
				mu.Lock()
				result[line.Value] = "Timeout"
				mu.Unlock()
				return
			}

			cost := time.Since(start).Milliseconds()
			mu.Lock()
			if testResp.StatusCode == 200 || testResp.StatusCode == 405 {
				// 405 Method Not Allowed 也代表服务器可达
				result[line.Value] = fmt.Sprintf("%dms", cost)
			} else {
				result[line.Value] = fmt.Sprintf("Error %d", testResp.StatusCode)
			}
			mu.Unlock()
		}(uploadLine)
	}

	wg.Wait()
	c.JSON(http.StatusOK, result)
}

// TestLineSpeed 测试单个线路的真实上传速度（通过 preupload API）
func TestLineSpeed(c *gin.Context) {
	line := c.Query("line")
	if line == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "line参数不能为空"})
		return
	}

	result := map[string]interface{}{
		"success": false,
	}

	// 1. 查找线路配置
	var targetLine *UploadLine
	allLines := getAllUploadLines()
	for _, l := range allLines {
		if l.Value == line {
			targetLine = &l
			break
		}
	}

	if targetLine == nil {
		result["msg"] = "Unknown Line"
		c.JSON(http.StatusOK, result)
		return
	}

	// 2. 获取官方线路列表（5s超时）
	client := req.C().SetTimeout(10 * time.Second).ImpersonateChrome()
	resp, err := client.R().Get("https://member.bilibili.com/preupload?r=ping&file=lines.json")
	if err != nil {
		result["msg"] = "获取官方线路失败: " + err.Error()
		c.JSON(http.StatusOK, result)
		return
	}

	var officialLines []OfficialLine
	if err := json.Unmarshal(resp.Bytes(), &officialLines); err != nil {
		result["msg"] = "解析线路数据失败"
		c.JSON(http.StatusOK, result)
		return
	}

	// 3. 查找对应的测速 URL
	queryKey := targetLine.LineQuery
	if len(queryKey) > 0 && queryKey[0] == '?' {
		queryKey = queryKey[1:]
	}

	var testURLStr string
	for _, officialLine := range officialLines {
		if officialLine.Query == queryKey {
			testURLStr = officialLine.URL
			break
		}
	}

	if testURLStr == "" {
		result["msg"] = "未找到对应的测速URL"
		c.JSON(http.StatusOK, result)
		return
	}

	if strings.HasPrefix(testURLStr, "//") {
		testURLStr = "https:" + testURLStr
	}

	// 4. 生成 2MB 随机数据进行真实上传测速
	size := 2 * 1024 * 1024 // 2MB
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		result["msg"] = "生成测试数据失败"
		c.JSON(http.StatusOK, result)
		return
	}

	// 5. 执行上传测速（PUT，模拟分片上传，15s超时）
	start := time.Now()
	testClient := req.C().SetTimeout(15 * time.Second).ImpersonateChrome()
	testResp, testErr := testClient.R().
		SetBodyBytes(data).
		Put(testURLStr)

	if testErr != nil {
		// 尝试 POST
		start = time.Now()
		testResp, testErr = testClient.R().
			SetBodyBytes(data).
			Post(testURLStr)
	}

	if testErr != nil {
		result["msg"] = "上传测试失败: " + testErr.Error()
		c.JSON(http.StatusOK, result)
		return
	}

	cost := time.Since(start).Milliseconds()
	// 4xx/5xx 也计入速度（已到达服务器说明连接成功），只要不是超时
	if testResp.StatusCode < 600 {
		speedMBps := float64(size) / 1024.0 / 1024.0 / (float64(cost) / 1000.0)
		result["success"] = true
		result["speed"] = fmt.Sprintf("%.2f MB/s", speedMBps)
		result["cost"] = cost
	} else {
		result["msg"] = fmt.Sprintf("Error %d", testResp.StatusCode)
	}

	c.JSON(http.StatusOK, result)
}

// VerifyTemplate 验证/预览模板（支持GET query参数和POST JSON体）
func VerifyTemplate(c *gin.Context) {
	var template string
	var roomIdStr string

	if c.Request.Method == "POST" {
		// POST: JSON body
		var body struct {
			Template string `json:"template"`
			RoomId   string `json:"roomId"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		template = body.Template
		roomIdStr = body.RoomId
	} else {
		// GET: query param
		template = c.Query("template")
		roomIdStr = c.Query("roomId")
	}

	if template == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template参数不能为空"})
		return
	}

	// 构建示例数据
	now := time.Now()
	data := map[string]interface{}{
		"uname":     "主播名称",
		"title":     "直播标题",
		"roomId":    roomIdStr,
		"areaName":  "游戏",
		"index":     1,
		"fileName":  "example_file_20241230.flv",
		"uid":       int64(987654321),
		"startTime": now,
	}
	if roomIdStr == "" {
		data["roomId"] = "123456"
	}

	// 渲染模板
	templateSvc := NewTemplateService()
	result := templateSvc.RenderTitle(template, data)

	c.JSON(http.StatusOK, gin.H{"result": result})
}

// NewTemplateService 临时创建模板服务（避免循环依赖）
func NewTemplateService() *services.TemplateService {
	return services.NewTemplateService()
}
