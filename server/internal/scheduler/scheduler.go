package scheduler

import (
	"log"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
	"github.com/robfig/cron/v3"
)

var cronJob *cron.Cron

func InitScheduler() {
	cronJob = cron.New()

	// 视频同步任务 - 每5分钟执行一次
	cronJob.AddFunc("*/5 * * * *", func() {
		// 检查是否启用自动上传
		if !isFeatureEnabled("AutoUpload") {
			return
		}

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

	// 文件扫描任务 - 每小时执行一次，扫描未入库的录制文件
	cronJob.AddFunc("30 * * * *", func() {
		// 检查是否启用自动扫盘
		if !isFeatureEnabled("AutoFileScan") {
			return
		}

		log.Println("执行定时任务: 文件扫描")
		scanService := services.NewFileScanService()
		config := services.LoadConfigFromDB()

		result, err := scanService.ScanAndImport(config)
		if err != nil {
			log.Printf("文件扫描任务失败: %v", err)
			return
		}

		if result.NewFiles > 0 || result.FailedFiles > 0 {
			log.Printf("文件扫描完成: 总文件=%d, 新导入=%d, 跳过=%d, 失败=%d",
				result.TotalFiles, result.NewFiles, result.SkippedFiles, result.FailedFiles)
		}
	})

	// 孤儿文件扫描 - 每6小时执行一次
	cronJob.AddFunc("0 */6 * * *", func() {
		// 检查是否启用孤儿文件扫描
		if !isFeatureEnabled("EnableOrphanScan") {
			return
		}

		log.Println("执行定时任务: 孤儿文件扫描")
		scanService := services.NewFileScanService()
		if err := scanService.ScanOrphanFiles(); err != nil {
			log.Printf("孤儿文件扫描失败: %v", err)
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

// isFeatureEnabled 检查功能是否启用
func isFeatureEnabled(feature string) bool {
	db := database.GetDB()
	var config models.SystemConfig

	if err := db.First(&config).Error; err != nil {
		log.Printf("[Scheduler] 获取系统配置失败，默认启用功能: %v", err)
		return true // 如果获取配置失败，默认启用
	}

	switch feature {
	case "AutoUpload":
		return config.AutoUpload
	case "AutoPublish":
		return config.AutoPublish
	case "AutoDelete":
		return config.AutoDelete
	case "AutoSendDanmaku":
		return config.AutoSendDanmaku
	case "AutoFileScan":
		return config.AutoFileScan
	case "EnableOrphanScan":
		return config.EnableOrphanScan
	default:
		return true
	}
}

func StopScheduler() {
	if cronJob != nil {
		cronJob.Stop()
	}
}
