package controllers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
	"github.com/imroc/req/v3"
)

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

	db := database.GetDB()
	db.Save(&room)
	c.JSON(http.StatusOK, true)
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
	c.JSON(http.StatusOK, []interface{}{})
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

// TestAllLines 批量测试所有线路的可用性（延迟）- 采用限流策略避免风控
func TestAllLines(c *gin.Context) {
	result := make(map[string]string)
	var mu sync.Mutex

	// 1. 获取官方线路列表
	client := req.C().SetTimeout(30 * time.Second).ImpersonateChrome()
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

	// 1.5. 优先快速检测所有官方线路可用性
	availableLines := make(map[string]bool)
	for _, line := range officialLines {
		testURL := line.URL
		if strings.HasPrefix(testURL, "//") {
			testURL = "https:" + testURL
		}
		// 快速HEAD请求检测可用性（2秒超时）
		testClient := req.C().SetTimeout(2 * time.Second).ImpersonateChrome()
		if _, err := testClient.R().Head(testURL); err == nil {
			availableLines[line.Query] = true
		}
	}

	// 2. 构建 query -> url 映射
	queryToURL := make(map[string]string)
	for _, line := range officialLines {
		queryToURL[line.Query] = line.URL
	}

	// 3. 限流并发测试 - 每批3条，避免风控
	allLines := getAllUploadLines()
	batchSize := 3              // 每批测试3条线路
	delayBetweenBatches := 1500 // 批次间延迟1.5秒
	delayBetweenRequests := 300 // 同批次内请求间延迟300ms

	for i := 0; i < len(allLines); i += batchSize {
		end := i + batchSize
		if end > len(allLines) {
			end = len(allLines)
		}
		batch := allLines[i:end]

		var wg sync.WaitGroup
		for idx, uploadLine := range batch {
			// 同批次内也加延迟，避免瞬间并发
			if idx > 0 {
				time.Sleep(time.Duration(delayBetweenRequests) * time.Millisecond)
			}

			wg.Add(1)
			go func(line UploadLine) {
				defer wg.Done()

				// 构建查询 key（去掉 LineQuery 的 ?）
				queryKey := ""
				if len(line.LineQuery) > 0 && line.LineQuery[0] == '?' {
					queryKey = line.LineQuery[1:]
				} else {
					queryKey = line.LineQuery
				}

				// 先检查官方线路是否可用
				if !availableLines[queryKey] {
					mu.Lock()
					result[line.Value] = "Unavailable"
					mu.Unlock()
					return
				}

				testURLStr, exists := queryToURL[queryKey]
				if !exists {
					mu.Lock()
					result[line.Value] = "Unknown"
					mu.Unlock()
					return
				}

				// 补全 URL
				if strings.HasPrefix(testURLStr, "//") {
					testURLStr = "https:" + testURLStr
				}

				// 测试延迟
				start := time.Now()
				testClient := req.C().SetTimeout(3 * time.Second).ImpersonateChrome()
				testResp, testErr := testClient.R().Get(testURLStr)

				if testErr != nil {
					mu.Lock()
					result[line.Value] = "Timeout"
					mu.Unlock()
					return
				}

				if testResp.StatusCode == 200 {
					cost := time.Since(start).Milliseconds()
					mu.Lock()
					result[line.Value] = fmt.Sprintf("%dms", cost)
					mu.Unlock()
				} else {
					mu.Lock()
					result[line.Value] = fmt.Sprintf("Error %d", testResp.StatusCode)
					mu.Unlock()
				}
			}(uploadLine)
		}

		wg.Wait()

		// 批次间延迟，避免风控
		if end < len(allLines) {
			time.Sleep(time.Duration(delayBetweenBatches) * time.Millisecond)
		}
	}

	c.JSON(http.StatusOK, result)
}

// TestLineSpeed 测试单个线路的真实上传速度
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

	// 2. 获取官方线路列表
	client := req.C().SetTimeout(30 * time.Second).ImpersonateChrome()
	resp, err := client.R().Get("https://member.bilibili.com/preupload?r=ping&file=lines.json")
	if err != nil {
		result["msg"] = "获取官方线路失败"
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
	queryKey := ""
	if len(targetLine.LineQuery) > 0 && targetLine.LineQuery[0] == '?' {
		queryKey = targetLine.LineQuery[1:]
	} else {
		queryKey = targetLine.LineQuery
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

	// 补全 URL
	if strings.HasPrefix(testURLStr, "//") {
		testURLStr = "https:" + testURLStr
	}

	// 4. 生成 1MB 随机数据进行上传测速
	size := 1024 * 1024 // 1MB
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		result["msg"] = "生成测试数据失败"
		c.JSON(http.StatusOK, result)
		return
	}

	// 5. 执行上传测速
	start := time.Now()
	testClient := req.C().SetTimeout(10 * time.Second).ImpersonateChrome()
	testResp, testErr := testClient.R().
		SetQueryParam("line", "1"). // line=1 表示 1MB
		SetBodyBytes(data).
		Post(testURLStr)

	if testErr != nil {
		result["msg"] = "上传测试失败: Timeout/Error"
		c.JSON(http.StatusOK, result)
		return
	}

	if testResp.StatusCode == 200 {
		cost := time.Since(start).Milliseconds()
		// 计算速度 MB/s
		speedMBps := float64(size) / 1024.0 / 1024.0 / (float64(cost) / 1000.0)
		result["success"] = true
		result["speed"] = fmt.Sprintf("%.2f MB/s", speedMBps)
		result["cost"] = cost
	} else {
		result["msg"] = fmt.Sprintf("Error %d", testResp.StatusCode)
	}

	c.JSON(http.StatusOK, result)
}

// VerifyTemplate 验证/预览模板
func VerifyTemplate(c *gin.Context) {
	template := c.Query("template")
	if template == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template参数不能为空"})
		return
	}

	// 构建示例数据
	now := time.Now()
	data := map[string]interface{}{
		"uname":     "主播名称",
		"title":     "直播标题",
		"roomId":    "123456",
		"areaName":  "游戏",
		"index":     1,
		"fileName":  "example_file_20241230.flv",
		"uid":       int64(987654321),
		"startTime": now,
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
