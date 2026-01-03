package routes

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/gobup/server/internal/controllers"
	"github.com/gobup/server/internal/middleware"
)

func SetupRoutes(router *gin.Engine) {
	// 设置静态文件路由（根据编译标签决定是嵌入模式还是独立模式）
	if err := setupStaticRoutes(router); err != nil {
		log.Printf("设置静态文件路由失败: %v", err)
	}

	api := router.Group("/api")
	{
		auth := api.Group("")
		auth.Use(middleware.BasicAuth())
		{
			rooms := auth.Group("/room")
			{
				rooms.POST("", controllers.ListRooms)
				rooms.POST("/add", controllers.AddRoom)
				rooms.POST("/update", controllers.UpdateRoom)
				rooms.GET("/delete/:id", controllers.DeleteRoom)

				rooms.GET("/lines", controllers.GetUploadLines)
				rooms.GET("/recommendedLines", controllers.GetRecommendedLines)
				rooms.GET("/testLines", controllers.TestAllLines)
				rooms.GET("/testSpeed", controllers.TestLineSpeed)
				rooms.GET("/seasons/:roomId", controllers.GetSeasons)
				rooms.GET("/verification", controllers.VerifyTemplate)
			}

			users := auth.Group("/biliUser")
			{
				users.GET("/list", controllers.ListBiliUsers)
				users.POST("/loginByCookie", controllers.LoginByCookie)
				users.GET("/refresh/:id", controllers.RefreshUserCookie)
				users.GET("/checkStatus/:id", controllers.CheckUserStatus)
				users.GET("/login", controllers.LoginUser)
				users.GET("/loginCheck", controllers.LoginCheck)
				users.GET("/loginCancel", controllers.LoginCancel)

			}

			histories := auth.Group("/history")
			{
				histories.POST("/list", controllers.ListHistories)
				histories.POST("/update", controllers.UpdateHistory)
				histories.GET("/delete/:id", controllers.DeleteHistory)
				histories.POST("/deleteWithFiles/:id", controllers.DeleteHistoryWithFiles)
				histories.POST("/resetStatus/:id", controllers.ResetHistoryStatus)
				histories.POST("/upload/:id", controllers.UploadHistory)
				histories.POST("/publish/:id", controllers.RePublishHistory)
				histories.GET("/updatePublishStatus/:id", controllers.UpdatePublishStatus)

				// 批量操作
				histories.POST("/batchUpdate", controllers.BatchUpdateStatus)
				histories.POST("/batchDelete", controllers.BatchDelete)
				histories.POST("/batchUpload", controllers.BatchUploadHistory)
				histories.POST("/batchPublish", controllers.BatchPublishHistory)
				histories.POST("/batchResetStatus", controllers.BatchResetStatus)
				histories.POST("/batchDeleteWithFiles", controllers.BatchDeleteWithFiles)
				histories.POST("/cleanOld", controllers.CleanOldHistories)

				// 弹幕相关
				histories.POST("/sendDanmaku/:id", controllers.SendDanmaku)
				histories.POST("/batchSendDanmaku", controllers.BatchSendDanmaku)
				histories.GET("/danmakuStats/:id", controllers.GetDanmakuStats)
				histories.POST("/parseDanmaku/:id", controllers.ParseDanmaku)
				histories.POST("/batchParseDanmaku", controllers.BatchParseDanmaku)

				// 文件移动
				histories.POST("/moveFiles/:id", controllers.MoveFiles)
				histories.POST("/batchMoveFiles", controllers.BatchMoveFiles)

				// 视频同步
				histories.POST("/syncVideo/:id", controllers.SyncVideoInfo)
				histories.POST("/batchSyncVideo", controllers.BatchSyncVideo)
				histories.POST("/createSyncTask/:id", controllers.CreateSyncTask)
			}

			// 视频同步任务
			syncTasks := auth.Group("/syncTasks")
			{
				syncTasks.GET("/list", controllers.ListSyncTasks)
				syncTasks.POST("/retryFailed", controllers.RetryFailedSyncTasks)
			}

			// 分P操作
			parts := auth.Group("/part")
			{
				parts.POST("/list/:id", controllers.ListParts)
				parts.GET("/uploadEditor/:id", controllers.UploadToEditor)
			}

			// 上传限速配置
			ratelimit := auth.Group("/ratelimit")
			{
				ratelimit.GET("/config", controllers.GetRateLimitConfig)
				ratelimit.POST("/config", controllers.SetRateLimitConfig)
			}

			// 验证码相关（参考biliupforjava）
			captcha := auth.Group("/captcha")
			{
				captcha.GET("/status", controllers.GetCaptchaStatus)
				captcha.POST("/submit", controllers.SubmitCaptchaResult)
				captcha.POST("/clear", controllers.ClearCaptcha)
			}

			// 队列状态
			queue := auth.Group("/queue")
			{
				queue.GET("/upload/status", controllers.GetUploadQueueStatus)
				queue.GET("/danmaku/status", controllers.GetDanmakuQueueStatus)
				queue.GET("/parse/status", controllers.GetParseQueueStatus)
			}

			// 配置导入导出
			config := auth.Group("/config")
			{
				config.POST("/export", controllers.ExportConfig)
				config.POST("/import", controllers.ImportConfig)

				// 系统配置管理
				config.GET("/system", controllers.GetSystemConfig)
				config.PUT("/system", controllers.UpdateSystemConfig)
				config.POST("/toggle", controllers.ToggleSystemConfig)
				config.GET("/stats", controllers.GetSystemStats)
			}

			// 日志API
			logs := auth.Group("/logs")
			{
				logs.GET("", controllers.GetLogs)
				logs.POST("/clear", controllers.ClearLogs)
			}

			// 文件扫描API
			filescan := auth.Group("/filescan")
			{
				filescan.POST("/trigger", controllers.TriggerFileScan)
				filescan.GET("/preview", controllers.PreviewFileScan)
				filescan.POST("/import", controllers.ImportSelectedFiles)
				filescan.POST("/cleanCompleted", controllers.CleanCompletedFiles)
			}

			// 数据修复API
			datarepair := auth.Group("/datarepair")
			{
				datarepair.GET("/check", controllers.CheckDataConsistency)
				datarepair.POST("/repair", controllers.RepairDataConsistency)
			}
		}
	}

	// WebSocket路由
	ws := router.Group("/ws")
	{
		ws.GET("/log", controllers.WSLog)
	}

	// 进度查询API
	progress := api.Group("/progress")
	{
		progress.GET("/part/:partId", controllers.GetPartProgress)
		progress.GET("/history/:historyId", controllers.GetHistoryProgress)
		progress.GET("/danmaku/:historyId", controllers.GetDanmakuProgress)
	}
}
