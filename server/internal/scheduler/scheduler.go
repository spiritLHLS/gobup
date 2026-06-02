package scheduler

import (
	"log"
	"time"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
	"github.com/gobup/server/internal/upload"
	"github.com/robfig/cron/v3"
)

var cronJob *cron.Cron
var uploadService *upload.Service
var fileWatcherService *services.FileWatcherService

func InitScheduler(svc *upload.Service) {
	cronJob = cron.New()

	// 复用 main 中创建的上传服务，避免自动上传队列和 API 查询队列分裂
	uploadService = svc
	if uploadService == nil {
		uploadService = upload.NewService()
	}

	// Bug4修复: 向 services 包注入投稿回调，使 room_auto_tasks.go 能触发投稿而不产生循环依赖
	services.TriggerPublish = func(historyID uint, userID uint) error {
		return uploadService.PublishHistory(historyID, userID)
	}

	// 服务启动后立即恢复因重启而滞留的临时分P（弹幕烧录版等）
	go func() {
		// 稍等5秒让DB连接和其他服务完全就绪后再执行
		time.Sleep(5 * time.Second)
		// 1. 先重置上次崩溃时卡在 uploading=true 的分P，避免永远不被重试
		log.Println("[启动恢复] 重置崩溃时卡在 uploading 状态的分P")
		uploadService.ResetStuckUploadingParts()
		// 2. 重新入队滞留但文件仍存在的临时分P（Stage D/E：烧录完成但未入队）
		log.Println("[启动恢复] 检查并重新入队滞留的临时分P")
		uploadService.RequeueStuckTempParts()
		// 3. 对 flv 已上传但烧录被中断的分P重新触发烧录（Stage C：ffmpeg 被 kill）
		log.Println("[启动恢复] 检查并重新触发被中断的弹幕烧录")
		uploadService.RequeueInterruptedBurns()
		// 4. 对全部分P已上传但投稿被中断的历史记录重新触发 checkAndPublish（Stage F）
		log.Println("[启动恢复] 检查并恢复被中断的自动投稿")
		uploadService.RecoverUnpublishedHistories()
		// 5. 检查已审核通过满1小时但缺少弹幕烧录版的历史记录，触发补录追加
		// 等额外30秒，确保步骤1-4的入队工作稳定后再扫描，避免重复烧录竞争
		time.Sleep(30 * time.Second)
		log.Println("[启动恢复] 检查已审核通过视频中缺失的弹幕烧录版分P")
		if err := uploadService.AppendDanmakuBurnedPartsToApprovedVideos(); err != nil {
			log.Printf("[启动恢复] 弹幕回补检查失败: %v", err)
		}
		// 6. 对存量历史记录补执行文件处理操作（版本升级入口）
		// 确保旧规则应用到升级前已完成但未处理的历史记录上。
		// 已处理的记录（files_moved=true 或 scheduled_delete_at 已设置）会被自动跳过。
		log.Println("[启动恢复] 对存量历史记录补执行文件操作")
		moverSvc := services.NewFileMoverService()
		if err := moverSvc.BackfillFileOps(); err != nil {
			log.Printf("[启动恢复] 文件操作回填失败: %v", err)
		}
	}()

	// 视频同步任务 - 每10分钟执行一次
	cronJob.AddFunc("*/10 * * * *", func() {
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

	// 计划删除任务 - 每小时执行一次（处理 strategy 8/12 的延迟删除）
	cronJob.AddFunc("15 * * * *", func() {
		log.Println("执行定时任务: 计划删除检查")
		moverService := services.NewFileMoverService()
		if err := moverService.ProcessScheduledDeletes(); err != nil {
			log.Printf("计划删除任务失败: %v", err)
		}
	})

	// 文件扫描任务 - 每小时执行一次，扫描未入库的录制文件
	fileWatcherService = services.NewFileWatcherService()
	fileWatcherService.Start()

	// 文件扫描任务 - 每小时执行一次，作为文件事件监听的兜底
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
			if err == services.ErrScanAlreadyRunning {
				log.Println("[文件扫描] 上次扫描尚未完成，本次跳过")
			} else {
				log.Printf("文件扫描任务失败: %v", err)
			}
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

	// 直播状态监控 - 每10分钟执行一次
	cronJob.AddFunc("*/10 * * * *", func() {
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

	// 数据一致性检查与修复 - 每天凌晨4点执行
	cronJob.AddFunc("0 4 * * *", func() {
		// 检查是否启用自动数据修复
		if !isFeatureEnabled("AutoDataRepair") {
			return
		}

		log.Println("执行定时任务: 数据一致性检查与修复")
		repairService := services.NewDataRepairService()
		result, err := repairService.CheckAndRepairDataConsistency(false) // 自动执行实际修复
		if err != nil {
			log.Printf("数据一致性修复失败: %v", err)
			return
		}

		// 如果有问题被修复，记录日志
		if result.CreatedHistories > 0 || result.DeletedEmptyHistories > 0 ||
			result.ReassignedParts > 0 || result.UpdatedHistoryTimes > 0 {
			log.Printf("数据一致性修复完成: 孤儿分P=%d, 空历史=%d, 新建历史=%d, 删除空历史=%d, 更新时间=%d, 重新分配=%d",
				result.OrphanParts, result.EmptyHistories, result.CreatedHistories,
				result.DeletedEmptyHistories, result.UpdatedHistoryTimes, result.ReassignedParts)
		}
	})

	// Token自动刷新任务 - 每2天执行一次（参考biliupforjava的RefreshTokenJob）
	cronJob.AddFunc("0 */48 * * *", func() {
		log.Println("执行定时任务: Token自动刷新")
		if err := refreshAllUserTokens(); err != nil {
			log.Printf("Token刷新任务失败: %v", err)
		}
	})

	// 自动上传任务 - 每10分钟执行一次，检查并处理待上传的分P
	cronJob.AddFunc("*/10 * * * *", func() {
		log.Println("执行定时任务: 自动上传检查")
		if err := processAutoUpload(); err != nil {
			log.Printf("自动上传任务失败: %v", err)
		}
	})

	// 房间自动任务 - 每30分钟执行一次，处理房间级别的自动同步和弹幕任务
	cronJob.AddFunc("*/30 * * * *", func() {
		log.Println("执行定时任务: 房间自动任务")
		roomAutoTaskService := services.NewRoomAutoTaskService()
		if err := roomAutoTaskService.ProcessRoomAutoTasks(); err != nil {
			log.Printf("房间自动任务失败: %v", err)
		}
	})

	// 弹幕回补任务 - 每30分钟执行一次（在整点+20/50分执行，与其他30分钟任务错开）
	// 对已审核通过满1小时的投稿，检查是否还有缺失的弹幕烧录版分P，
	// 若原始视频文件和XML弹幕文件均存在则自动烧录、上传并追加到B站视频。
	cronJob.AddFunc("20,50 * * * *", func() {
		log.Println("执行定时任务: 弹幕回补检查")
		if err := uploadService.AppendDanmakuBurnedPartsToApprovedVideos(); err != nil {
			log.Printf("弹幕回补任务失败: %v", err)
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
	case "AutoDataRepair":
		return config.AutoDataRepair
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

// processAutoUpload 处理自动上传任务
func processAutoUpload() error {
	// 获取自动上传服务
	autoUploadSvc := services.NewAutoUploadService()

	// 先清理孤立的临时分P记录（文件不存在却卡在未上传状态的）
	autoUploadSvc.CleanOrphanedTempParts()

	// 恢复滞留的临时分P（文件存在但未入队，如服务重启导致的）
	uploadService.RequeueStuckTempParts()

	// 获取所有待上传的分P
	tasks, err := autoUploadSvc.GetPendingUploadParts()
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		log.Println("[自动上传] 没有待上传的分P")
		return nil
	}

	log.Printf("[自动上传] 发现 %d 个待上传的分P，开始加入上传队列", len(tasks))

	successCount := 0
	failCount := 0

	for _, task := range tasks {
		log.Printf("[自动上传] 加入队列: room=%s (%s), part_id=%d, file=%s",
			task.Room.RoomID, task.Room.Uname, task.Part.ID, task.Part.FileName)

		// 将分P加入上传队列
		if err := uploadService.UploadPart(&task.Part, &task.History, &task.Room); err != nil {
			log.Printf("[自动上传] 加入队列失败: part_id=%d, error=%v", task.Part.ID, err)
			failCount++
			continue
		}

		successCount++
	}

	log.Printf("[自动上传] 完成: 成功加入队列=%d, 失败=%d", successCount, failCount)
	return nil
}

func StopScheduler() {
	if fileWatcherService != nil {
		fileWatcherService.Stop()
		fileWatcherService = nil
	}
	if cronJob != nil {
		cronJob.Stop()
	}
}
