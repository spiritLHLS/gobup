package upload

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gobup/server/internal/bili"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/gobup/server/internal/services"
)

func (s *Service) PublishHistory(historyID uint, userID uint) error {
	db := database.GetDB()

	var history models.RecordHistory
	if err := db.First(&history, historyID).Error; err != nil {
		return fmt.Errorf("历史记录不存在: %w", err)
	}

	var room models.RecordRoom
	if err := db.Where("room_id = ?", history.RoomID).First(&room).Error; err != nil {
		return fmt.Errorf("房间不存在: %w", err)
	}

	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return fmt.Errorf("用户未登录")
	}

	// 验证Cookie
	valid, err := bili.ValidateCookie(user.Cookies)
	if err != nil || !valid {
		user.Login = false
		db.Save(&user)
		return fmt.Errorf("用户Cookie已失效，请重新登录")
	}

	// 获取所有已上传的分P
	var parts []models.RecordHistoryPart
	if err := db.Where("history_id = ? AND upload = ?", historyID, true).
		Order("start_time ASC").
		Find(&parts).Error; err != nil {
		return fmt.Errorf("查询分P失败: %w", err)
	}

	if len(parts) == 0 {
		return fmt.Errorf("没有已上传的分P")
	}

	// 构建模板数据
	templateData := map[string]interface{}{
		"uname":     room.Uname,
		"title":     history.Title,
		"roomId":    history.RoomID,
		"areaName":  history.AreaName,
		"startTime": history.StartTime,
		"uid":       user.UID,
	}

	// 使用模板服务渲染
	title := s.templateSvc.RenderTitle(room.TitleTemplate, templateData)
	desc := s.templateSvc.RenderDescription(room.DescTemplate, templateData)
	dynamic := s.templateSvc.RenderDynamic(room.DynamicTemplate, templateData) // 动态模板
	tags := s.templateSvc.BuildTags(room.Tags, templateData)
	tagsStr := strings.Join(tags, ",")

	tid := room.TID
	if tid == 0 {
		tid = 171 // 默认分区：电子竞技
	}

	// 创建客户端
	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)

	// 获取封面
	coverURL := room.CoverURL
	if coverURL == "live" && len(parts) > 0 {
		// 从录制文件路径查找封面文件
		lastPartPath := parts[len(parts)-1].FilePath
		basePath := strings.TrimSuffix(lastPartPath, filepath.Ext(lastPartPath))

		// 尝试多种封面文件格式
		coverPaths := []string{
			basePath + ".cover.jpg",
			basePath + ".jpg",
			basePath + ".cover.png",
			basePath + ".png",
		}

		for _, coverPath := range coverPaths {
			if _, err := os.Stat(coverPath); err == nil {
				// 找到封面文件，上传到B站
				coverData, err := os.ReadFile(coverPath)
				if err == nil {
					log.Printf("找到封面文件: %s", coverPath)
					uploadedURL, err := client.UploadCover(coverData)
					if err == nil {
						coverURL = uploadedURL
						log.Printf("封面上传成功: %s", coverURL)
						break
					} else {
						log.Printf("封面上传失败: %v", err)
					}
				}
			}
		}

		if coverURL == "live" {
			// 如果没找到封面文件，使用默认或从视频截取
			coverURL = ""
			log.Printf("未找到封面文件，将使用默认封面或从视频截取")
		}
	} else if coverURL == "" {
		coverURL = "" // 使用默认封面或从视频截取
	}

	// 构建分P信息
	var videoParts []bili.PublishVideoPartRequest
	for i, part := range parts {
		partTemplateData := map[string]interface{}{
			"index":     i + 1,
			"startTime": part.StartTime,
			"areaName":  part.AreaName,
		}
		partTitle := s.templateSvc.RenderPartTitle(room.PartTitleTemplate, partTemplateData)

		// 获取文件名：优先使用数据库中的 FileName（从上传响应获取），如果为空则从 FilePath 提取
		filename := part.FileName
		if filename == "" {
			// 兼容旧数据：从文件路径提取文件名（不含扩展名）
			baseName := filepath.Base(part.FilePath)
			if ext := filepath.Ext(baseName); ext != "" {
				filename = baseName[:len(baseName)-len(ext)]
			} else {
				filename = baseName
			}
			log.Printf("警告: 分P[%d]的FileName为空，从FilePath提取: %s", i, filename)
		}

		// 调试日志：检查关键参数
		log.Printf("构建分P[%d]: filename=%s, cid=%d", i, filename, part.CID)

		// 只有CID大于0时才传递（参考biliupforjava实现）
		var cid int64
		if part.CID > 0 {
			cid = int64(part.CID)
		} else {
			log.Printf("警告: 分P[%d]的CID为0，可能导致投稿失败", i)
		}

		videoParts = append(videoParts, bili.PublishVideoPartRequest{
			Title:    partTitle,
			Desc:     "",
			Filename: filename,
			Cid:      cid,
		})
	}

	// 投稿，同时获取AID和BV号
	avID, bvid, err := client.PublishVideo(title, desc, tagsStr, tid, room.Copyright, coverURL, videoParts)
	if err != nil {
		// 检查是否是验证码错误
		captchaService := services.NewCaptchaService()
		if captchaService.IsCaptchaError(err.Error()) {
			log.Printf("检测到验证码错误: %v", err)
			history.Message = "投稿失败: 需要验证码验证"
			db.Save(&history)

			// 加入重试队列
			captchaService.HandleCaptchaError(historyID, userID, err.Error())
			return fmt.Errorf("需要验证码验证，已加入重试队列")
		}

		history.Message = fmt.Sprintf("投稿失败: %v", err)
		db.Save(&history)
		return fmt.Errorf("投稿失败: %w", err)
	}

	// 更新历史记录，使用API直接返回的BV号
	history.AvID = fmt.Sprintf("%d", avID)
	// 检查BV号格式，如果不是BV开头或格式错误，则通过aid转换
	if !strings.HasPrefix(bvid, "BV") || len(bvid) != 12 {
		log.Printf("警告: API返回的BV号格式错误: %s, 使用AID=%d重新转换", bvid, avID)
		bvid = Av2Bv(avID)
		log.Printf("转换后的正确BV号: %s", bvid)
	}
	history.BvID = bvid
	history.Publish = true
	history.Message = "投稿成功"
	// 注意：投稿后不修改UploadStatus，保持为2（已上传）
	db.Save(&history)

	log.Printf("投稿成功: AV%d, BV%s", avID, bvid)

	// 加入合集
	if room.SeasonID > 0 && len(videoParts) > 0 {
		// 使用第一个分P的CID
		cid := videoParts[0].Cid
		if err := client.AddToSeason(room.SeasonID, avID, cid, title); err != nil {
			log.Printf("加入合集失败: %v", err)
		} else {
			log.Printf("加入合集成功: SeasonID=%d, AID=%d", room.SeasonID, avID)
		}
	}

	// 创建视频同步任务
	syncService := services.NewVideoSyncService()
	if err := syncService.CreateSyncTask(historyID); err != nil {
		log.Printf("创建同步任务失败: %v", err)
	}

	// 推送通知
	if room.Wxuid != "" && containsTag(room.PushMsgTags, "投稿") {
		s.wxPusher.NotifyPublishSuccess(room.UploadUserID, room.Wxuid, room.Uname, title, history.BvID)
	}

	// 发送动态
	if dynamic != "" {
		// 替换动态中的bvid变量
		dynamicWithBv := strings.ReplaceAll(dynamic, "${bvid}", history.BvID)
		if err := client.SendDynamic(dynamicWithBv); err != nil {
			log.Printf("发送动态失败: %v", err)
		} else {
			log.Printf("发送动态成功: %s", dynamicWithBv)
		}
	}

	// 处理文件策略：9-投稿成功后删除, 10-投稿成功后移动
	if room.DeleteType == 9 || room.DeleteType == 10 {
		fileMoverSvc := services.NewFileMoverService()
		if err := fileMoverSvc.ProcessFilesByStrategy(historyID, room.DeleteType); err != nil {
			log.Printf("文件处理失败: %v", err)
		}
	}

	// 如果启用高能剪辑，创建高能剪辑任务
	if room.HighEnergyCut {
		go func() {
			log.Printf("开始高能剪辑: history_id=%d", historyID)
			highEnergySvc := services.NewHighEnergyCutService()
			outputFile, err := highEnergySvc.CutHighEnergySegments(historyID)
			if err != nil {
				log.Printf("高能剪辑失败: %v", err)
				return
			}
			log.Printf("高能剪辑完成: %s", outputFile)
			// TODO: 自动上传高能剪辑版本
		}()
	}

	return nil
}

// Av2Bv 将AV号转换为BV号
// 算法参考: https://github.com/SocialSisterYi/bilibili-API-collect
func Av2Bv(av int64) string {
	const (
		xorCode  = int64(23442827791579)
		maskCode = int64(2251799813685247)
		maxAid   = int64(1) << 51
		base     = 58
		alphabet = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	)

	bytes := []byte{'B', 'V', '1', '0', '0', '0', '0', '0', '0', '0', '0', '0'}
	bvIndex := len(bytes) - 1
	tmp := (maxAid | av) ^ xorCode

	for tmp > 0 {
		bytes[bvIndex] = alphabet[tmp%base]
		tmp /= base
		bvIndex--
	}

	// 交换特定位置的字符
	bytes[3], bytes[9] = bytes[9], bytes[3]
	bytes[4], bytes[7] = bytes[7], bytes[4]

	return string(bytes)
}

// GetSeasons 获取合集列表
func (s *Service) GetSeasons(userID uint) ([]bili.Season, error) {
	db := database.GetDB()

	var user models.BiliBiliUser
	if err := db.First(&user, userID).Error; err != nil {
		return nil, fmt.Errorf("用户不存在: %w", err)
	}

	if !user.Login {
		return nil, fmt.Errorf("用户未登录")
	}

	client := bili.NewBiliClient(user.AccessKey, user.Cookies, user.UID)
	return client.GetSeasons(user.UID)
}
