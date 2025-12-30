package scheduler

import (
	"log"

	"github.com/gobup/server/internal/services"
	"github.com/robfig/cron/v3"
)

var cronJob *cron.Cron

func InitScheduler() {
	cronJob = cron.New()

	// 视频同步任务 - 每5分钟执行一次
	cronJob.AddFunc("*/5 * * * *", func() {
		log.Println("执行定时任务: 视频同步")
		syncService := services.NewVideoSyncService()
		if err := syncService.ProcessPendingTasks(); err != nil {
			log.Printf("视频同步任务失败: %v", err)
		}
	})

	// 文件移动任务 - 每小时执行一次
	cronJob.AddFunc("0 * * * *", func() {
		log.Println("执行定时任务: 文件移动")
		moverService := services.NewFileMoverService()
		if err := moverService.AutoMoveFiles(); err != nil {
			log.Printf("文件移动任务失败: %v", err)
		}
	})

	// 清理已完成的同步任务 - 每天凌晨3点执行
	cronJob.AddFunc("0 3 * * *", func() {
		log.Println("执行定时任务: 清理已完成的同步任务")
		syncService := services.NewVideoSyncService()
		if err := syncService.CleanCompletedTasks(); err != nil {
			log.Printf("清理任务失败: %v", err)
		}
	})

	cronJob.Start()
	log.Println("调度器已启动")
}

func StopScheduler() {
	if cronJob != nil {
		cronJob.Stop()
	}
}
