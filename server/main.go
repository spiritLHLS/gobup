package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/config"
	"github.com/gobup/server/internal/controllers"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/logging"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/routes"
	"github.com/gobup/server/internal/scheduler"
	"github.com/gobup/server/internal/upload"
)

// initAdminUser 初始化管理员用户（仅用于首次启动）
func initAdminUser() {
	// 如果没有提供初始用户名和密码，则跳过
	if config.AppConfig.InitUsername == "" || config.AppConfig.InitPassword == "" {
		log.Println("未提供初始管理员账号，跳过创建")
		return
	}

	db := database.GetDB()

	// 检查是否已存在管理员用户（通过特殊UID标识）
	var adminUser models.BiliBiliUser
	result := db.Where("uid = ?", -1).First(&adminUser)

	if result.Error != nil {
		// 创建管理员账号
		now := time.Now()
		expireTime := now.Add(365 * 24 * time.Hour) // 1年过期

		adminUser = models.BiliBiliUser{
			UID:        -1, // 使用特殊UID标识管理员账号
			Uname:      config.AppConfig.InitUsername,
			Login:      true,
			LoginTime:  &now,
			ExpireTime: &expireTime,
			// 实际的认证会通过middleware实现
		}

		if err := db.Create(&adminUser).Error; err != nil {
			log.Printf("创建管理员账号失败: %v", err)
		} else {
			log.Printf("管理员账号创建成功: %s", config.AppConfig.InitUsername)
		}
	} else {
		log.Println("管理员账号已存在")
	}
}

func main() {
	// 命令行参数
	port := flag.Int("port", 12380, "HTTP服务端口")
	workPath := flag.String("work-path", "", "录播文件工作目录")
	username := flag.String("username", "", "初始管理员用户名")
	password := flag.String("password", "", "初始管理员密码")
	dataPath := flag.String("data-path", "./data", "数据目录")
	flag.Parse()

	// 从环境变量获取用户名和密码（命令行参数优先）
	if *username == "" {
		*username = os.Getenv("USERNAME")
	}
	if *password == "" {
		*password = os.Getenv("PASSWORD")
	}

	// 初始化配置
	config.Init(*port, *workPath, *username, *password, *dataPath)

	// 设置日志拦截器，将日志推送到WebSocket
	logging.SetupLogInterceptor()

	// 创建必要的目录
	if config.AppConfig.WorkPath != "" {
		if err := os.MkdirAll(config.AppConfig.WorkPath, 0755); err != nil {
			log.Fatalf("创建工作目录失败: %v", err)
		}
	}

	if err := os.MkdirAll(config.AppConfig.DataPath, 0755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	// 初始化数据库
	dbPath := filepath.Join(config.AppConfig.DataPath, "gobup.db")
	if err := database.InitDB(dbPath); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer database.CloseDB()

	// 初始化管理员用户
	initAdminUser()

	// 初始化定时任务
	scheduler.InitScheduler()
	defer scheduler.StopScheduler()

	// 初始化上传服务
	uploadSvc := upload.NewService()
	controllers.SetUploadService(uploadSvc)
	controllers.SetHistoryUploadService(uploadSvc)

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)

	// 创建并设置路由
	router := gin.Default()
	routes.SetupRoutes(router)

	// 启动服务
	addr := fmt.Sprintf(":%d", config.AppConfig.Port)
	log.Printf("GoBup服务启动在端口 %d", config.AppConfig.Port)
	log.Printf("工作目录: %s", config.AppConfig.WorkPath)
	log.Printf("数据目录: %s", config.AppConfig.DataPath)

	if err := router.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
