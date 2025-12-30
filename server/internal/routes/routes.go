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
		api.POST("/recordWebHook", controllers.RecordWebHook)
		api.GET("/recordWebHook", controllers.RecordWebHookGet)

		auth := api.Group("")
		auth.Use(middleware.BasicAuth())
		{
			rooms := auth.Group("/room")
			{
				rooms.POST("", controllers.ListRooms)
				rooms.POST("/add", controllers.AddRoom)
				rooms.POST("/update", controllers.UpdateRoom)
				rooms.GET("/delete/:id", controllers.DeleteRoom)
				rooms.POST("/uploadCover", controllers.UploadCover)
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
				users.GET("/login", controllers.LoginUser)
				users.GET("/loginCheck", controllers.LoginCheck)
				users.GET("/loginCancel", controllers.LoginCancel)

			}

			histories := auth.Group("/history")
			{
				histories.POST("/list", controllers.ListHistories)
				histories.POST("/update", controllers.UpdateHistory)
				histories.GET("/delete/:id", controllers.DeleteHistory)
				histories.GET("/rePublish/:id", controllers.RePublishHistory)
				histories.GET("/updatePublishStatus/:id", controllers.UpdatePublishStatus)

				// 批量操作
				histories.POST("/batchUpdate", controllers.BatchUpdateStatus)
				histories.POST("/batchDelete", controllers.BatchDelete)
				histories.POST("/cleanOld", controllers.CleanOldHistories)

				// 弹幕相关
				histories.POST("/sendDanmaku/:id", controllers.SendDanmaku)
				histories.GET("/danmakuStats/:id", controllers.GetDanmakuStats)

				// 文件移动
				histories.POST("/moveFiles/:id", controllers.MoveFiles)

				// 视频同步
				histories.POST("/syncVideo/:id", controllers.SyncVideoInfo)
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
	}
}
