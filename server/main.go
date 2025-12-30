package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/config"
	"github.com/gobup/server/internal/controllers"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/routes"
	"github.com/gobup/server/internal/scheduler"
	"github.com/gobup/server/internal/upload"
)

func main() {
	// 命令行参数
	port := flag.Int("port", 12380, "HTTP服务端口")
	workPath := flag.String("work-path", "", "录播文件工作目录")
	username := flag.String("username", "", "登录用户名")
	password := flag.String("password", "", "登录密码")
	dataPath := flag.String("data-path", "./data", "数据目录")
	wxPushToken := flag.String("wxpush-token", "", "WxPusher AppToken")
	flag.Parse()

	// 从环境变量获取WxPusher token（命令行参数优先）
	if *wxPushToken == "" {
		*wxPushToken = os.Getenv("WXPUSH_TOKEN")
	}

	// 初始化配置
	config.Init(*port, *workPath, *username, *password, *dataPath, *wxPushToken)

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

	// 初始化定时任务
	scheduler.InitScheduler()
	defer scheduler.StopScheduler()

	// 初始化上传服务
	uploadSvc := upload.NewService()
	controllers.SetUploadService(uploadSvc)

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
