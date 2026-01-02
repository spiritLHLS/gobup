package scheduler

import (
	"log"
	"time"

	"github.com/gobup/server/internal/bili"
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

	// 直播状态监控 - 每5分钟执行一次
	cronJob.AddFunc("*/5 * * * *", func() {
		log.Println("执行定时任务: 直播状态监控")
		liveStatusService := services.NewLiveStatusService()
		if err := liveStatusService.UpdateAllRoomsStatus(); err != nil {
			log.Printf("直播状态监控失败: %v", err)
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

	// Token自动刷新任务 - 每2天执行一次（参考biliupforjava的RefreshTokenJob）
	cronJob.AddFunc("0 */48 * * *", func() {
		log.Println("执行定时任务: Token自动刷新")
		if err := refreshAllUserTokens(); err != nil {
			log.Printf("Token刷新任务失败: %v", err)
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
	case "AutoFileScan":
		return config.AutoFileScan
	case "EnableOrphanScan":
		return config.EnableOrphanScan
	default:
		return true
	}
}

// refreshAllUserTokens 刷新所有用户的Token（参考biliupforjava的RefreshTokenJob）
func refreshAllUserTokens() error {
	db := database.GetDB()
	var users []models.BiliBiliUser

	// 查询所有已登录的用户（排除管理员账号 UID=-1）
	if err := db.Where("login = ? AND uid != ?", true, -1).Find(&users).Error; err != nil {
		return err
	}

	log.Printf("[TOKEN_REFRESH] 开始刷新%d个用户的Token", len(users))

	successCount := 0
	failCount := 0

	for _, user := range users {
		// 跳过没有RefreshToken的用户
		if user.RefreshToken == "" {
			log.Printf("[TOKEN_REFRESH] 跳过用户%s(%d): 无RefreshToken", user.Uname, user.UID)
			continue
		}

		// 避免请求过快，每次请求间隔5秒
		if successCount > 0 || failCount > 0 {
			time.Sleep(5 * time.Second)
		}

		log.Printf("[TOKEN_REFRESH] 刷新用户Token: %s(%d)", user.Uname, user.UID)

		// 调用刷新Token API
		refreshResp, err := bili.RefreshToken(user.AccessKey, user.RefreshToken, user.Cookies)
		if err != nil {
			log.Printf("[TOKEN_REFRESH] 刷新失败 %s(%d): %v", user.Uname, user.UID, err)
			failCount++
			continue
		}

		// 提取新的Token和Cookie
		tokenInfo := bili.ExtractRefreshTokenInfo(refreshResp)
		if tokenInfo == nil {
			log.Printf("[TOKEN_REFRESH] 提取Token信息失败 %s(%d)", user.Uname, user.UID)
			failCount++
			continue
		}

		// 更新用户信息
		user.AccessKey = tokenInfo.AccessToken
		user.RefreshToken = tokenInfo.RefreshToken
		user.Cookies = tokenInfo.Cookies

		// 更新过期时间
		now := time.Now()
		expireTime := now.Add(time.Duration(tokenInfo.ExpiresIn) * time.Second)
		user.ExpireTime = &expireTime

		if err := db.Save(&user).Error; err != nil {
			log.Printf("[TOKEN_REFRESH] 保存用户失败 %s(%d): %v", user.Uname, user.UID, err)
			failCount++
			continue
		}

		log.Printf("[TOKEN_REFRESH] 刷新成功 %s(%d), 新过期时间: %s",
			user.Uname, user.UID, expireTime.Format("2006-01-02 15:04:05"))
		successCount++
	}

	log.Printf("[TOKEN_REFRESH] 完成: 成功=%d, 失败=%d", successCount, failCount)
	return nil
}

func StopScheduler() {
	if cronJob != nil {
		cronJob.Stop()
	}
}
