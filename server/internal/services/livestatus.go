package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"github.com/imroc/req/v3"
)

// LiveRoomInfo B站直播间信息
type LiveRoomInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		UID            int64  `json:"uid"`
		RoomID         int64  `json:"room_id"`
		ShortID        int    `json:"short_id"`
		LiveStatus     int    `json:"live_status"` // 0:未开播 1:正在直播 2:轮播中
		RoomStatus     int    `json:"room_status"` // 0:房间封禁 1:房间正常
		Title          string `json:"title"`       // 直播标题
		UserCover      string `json:"user_cover"`  // 直播封面
		ParentAreaID   int    `json:"parent_area_id"`
		AreaID         int    `json:"area_id"`
		AreaName       string `json:"area_name"`
		ParentAreaName string `json:"parent_area_name"`
	} `json:"data"`
}

// UserInfo B站主播信息（直播相关）
type UserInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
	Data    struct {
		Info struct {
			UID    int64  `json:"uid"`
			Uname  string `json:"uname"`
			Face   string `json:"face"`
			Gender int    `json:"gender"`
		} `json:"info"`
		RoomID     int64  `json:"room_id"`
		MedalName  string `json:"medal_name"`
		GloryCount int    `json:"glory_count"`
	} `json:"data"`
}

// LiveStatusService 直播状态服务
type LiveStatusService struct {
	client *req.Client
}

// NewLiveStatusService 创建直播状态服务实例
func NewLiveStatusService() *LiveStatusService {
	client := req.C().
		SetTimeout(10 * time.Second).
		SetCommonRetryCount(2)

	return &LiveStatusService{
		client: client,
	}
}

// GetRoomInfo 获取直播间信息
func (s *LiveStatusService) GetRoomInfo(roomID string) (*LiveRoomInfo, error) {
	url := fmt.Sprintf("https://api.live.bilibili.com/room/v1/Room/get_info?room_id=%s", roomID)

	var roomInfo LiveRoomInfo
	resp, err := s.client.R().
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36").
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("请求直播间信息失败: %w", err)
	}

	if err := json.Unmarshal(resp.Bytes(), &roomInfo); err != nil {
		return nil, fmt.Errorf("解析直播间信息失败: %w", err)
	}

	if roomInfo.Code != 0 {
		return nil, fmt.Errorf("获取直播间信息失败: %s", roomInfo.Message)
	}

	return &roomInfo, nil
}

// GetUserInfo 获取主播信息（直播相关API）
func (s *LiveStatusService) GetUserInfo(uid int64) (*UserInfo, error) {
	url := fmt.Sprintf("https://api.live.bilibili.com/live_user/v1/Master/info?uid=%d", uid)

	var userInfo UserInfo
	resp, err := s.client.R().
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36").
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("请求主播信息失败: %w", err)
	}

	if err := json.Unmarshal(resp.Bytes(), &userInfo); err != nil {
		return nil, fmt.Errorf("解析主播信息失败: %w", err)
	}

	if userInfo.Code != 0 {
		return nil, fmt.Errorf("获取主播信息失败: %s", userInfo.Message)
	}

	return &userInfo, nil
}

// UpdateRoomLiveStatus 更新房间的直播状态
func (s *LiveStatusService) UpdateRoomLiveStatus(room *models.RecordRoom) error {
	roomInfo, err := s.GetRoomInfo(room.RoomID)
	if err != nil {
		log.Printf("[LiveStatus] 获取房间 %s 的直播状态失败: %v", room.RoomID, err)
		return err
	}

	db := database.GetDB()

	// 记录之前的状态
	wasStreaming := room.Streaming

	// 获取主播信息
	uname := room.Uname // 默认保持原有名称
	if roomInfo.Data.UID > 0 {
		userInfo, err := s.GetUserInfo(roomInfo.Data.UID)
		if err == nil && userInfo.Data.Info.Uname != "" {
			uname = userInfo.Data.Info.Uname
			log.Printf("[LiveStatus] 获取到主播名称: %s (UID=%d)", uname, roomInfo.Data.UID)
		} else {
			log.Printf("[LiveStatus] 获取主播名称失败，保持原名称: %v", err)
		}
	}

	// 更新房间状态
	updates := map[string]interface{}{
		"streaming":        roomInfo.Data.LiveStatus == 1,
		"title":            roomInfo.Data.Title,
		"uname":            uname,
		"area_name":        roomInfo.Data.AreaName,
		"area_name_parent": roomInfo.Data.ParentAreaName,
		"live_status":      roomInfo.Data.LiveStatus,
		"last_check_time":  time.Now(),
	}

	if err := db.Model(room).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新房间状态失败: %w", err)
	}

	// 如果主播名称已更新，同步更新该房间的所有历史记录
	if uname != room.Uname && uname != "" {
		result := db.Model(&models.RecordHistory{}).
			Where("room_id = ?", room.RoomID).
			Update("uname", uname)

		if result.Error != nil {
			log.Printf("[LiveStatus] 更新历史记录主播名失败: %v", result.Error)
		} else if result.RowsAffected > 0 {
			log.Printf("[LiveStatus] 已同步更新 %d 条历史记录的主播名: %s -> %s",
				result.RowsAffected, room.Uname, uname)
		}
	}

	// 检测直播状态变化
	isStreaming := roomInfo.Data.LiveStatus == 1

	if wasStreaming && !isStreaming {
		log.Printf("[LiveStatus] 房间 %s 直播结束，可以处理录播文件", room.RoomID)
		// 这里可以触发文件扫描或其他处理逻辑
	} else if !wasStreaming && isStreaming {
		log.Printf("[LiveStatus] 房间 %s 开始直播: %s", room.RoomID, roomInfo.Data.Title)
	}

	log.Printf("[LiveStatus] 房间 %s 状态更新: live_status=%d, streaming=%v, title=%s",
		room.RoomID, roomInfo.Data.LiveStatus, isStreaming, roomInfo.Data.Title)

	return nil
}

// UpdateAllRoomsStatus 更新所有房间的直播状态
func (s *LiveStatusService) UpdateAllRoomsStatus() error {
	db := database.GetDB()

	var rooms []models.RecordRoom
	if err := db.Where("upload = ?", true).Find(&rooms).Error; err != nil {
		return fmt.Errorf("查询房间列表失败: %w", err)
	}

	log.Printf("[LiveStatus] 开始更新 %d 个房间的直播状态", len(rooms))

	successCount := 0
	failCount := 0

	for i := range rooms {
		if err := s.UpdateRoomLiveStatus(&rooms[i]); err != nil {
			log.Printf("[LiveStatus] 更新房间 %s 状态失败: %v", rooms[i].RoomID, err)
			failCount++
		} else {
			successCount++
		}

		// 避免请求过快，每个请求间隔一下
		if i < len(rooms)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	log.Printf("[LiveStatus] 状态更新完成: 成功=%d, 失败=%d", successCount, failCount)

	return nil
}

// IsRoomRecordingFinished 判断房间的录播是否已完成（直播已结束且录制文件稳定）
// 返回值: (是否可以处理, 是否使用了保底逻辑, 错误)
func (s *LiveStatusService) IsRoomRecordingFinished(roomID string, fileModTime time.Time, fileTitle string) (bool, bool, error) {
	// 保底逻辑：文件修改时间超过1小时才处理
	const fallbackDuration = 1 * time.Hour
	timeSinceModified := time.Since(fileModTime)

	roomInfo, err := s.GetRoomInfo(roomID)
	if err != nil {
		// 如果无法获取直播状态，使用保底逻辑：1小时时间判断
		log.Printf("[LiveStatus] 无法获取房间 %s 状态，使用保底逻辑（1小时）: %v", roomID, err)
		canProcess := timeSinceModified >= fallbackDuration
		if canProcess {
			log.Printf("[LiveStatus] 文件修改时间 %v >= 1小时，允许处理", timeSinceModified)
		} else {
			log.Printf("[LiveStatus] 文件修改时间 %v < 1小时，跳过处理", timeSinceModified)
		}
		return canProcess, true, nil
	}

	// 检查响应数据是否有效
	if roomInfo.Data.RoomID == 0 {
		log.Printf("[LiveStatus] 房间 %s 返回数据无效，使用保底逻辑（1小时）", roomID)
		canProcess := timeSinceModified >= fallbackDuration
		return canProcess, true, nil
	}

	isLive := roomInfo.Data.LiveStatus == 1

	// 定义文件稳定的时间阈值
	const liveFileThreshold = 1 * time.Hour // 直播中的房间，文件修改超过1小时视为旧直播
	const endedFileBuffer = 5 * time.Minute // 直播结束后，文件修改超过5分钟才处理

	if isLive {
		// 房间正在直播，需要区分是当前这场直播还是之前的录播
		// 判断1：文件修改时间
		if timeSinceModified < liveFileThreshold {
			// 文件最近修改过（不到1小时），可能是当前这场直播的文件，跳过
			log.Printf("[LiveStatus] 房间 %s 正在直播（live_status=%d），文件修改时间过近（%v），跳过文件处理",
				roomID, roomInfo.Data.LiveStatus, timeSinceModified)
			return false, false, nil
		}

		// 判断2：标题匹配（即使文件修改时间>1小时，但如果标题与当前直播一致，可能是分P）
		if fileTitle != "" && roomInfo.Data.Title != "" {
			// 简单的标题匹配：去除空格后比较，或者检查是否包含
			fileT := strings.TrimSpace(fileTitle)
			liveT := strings.TrimSpace(roomInfo.Data.Title)
			if fileT == liveT || strings.Contains(fileT, liveT) || strings.Contains(liveT, fileT) {
				log.Printf("[LiveStatus] 房间 %s 正在直播（live_status=%d），文件标题与当前直播匹配（文件:%s, 直播:%s），可能是当前直播的分P，跳过处理",
					roomID, roomInfo.Data.LiveStatus, fileTitle, roomInfo.Data.Title)
				return false, false, nil
			}
		}

		// 文件很久没修改了（超过1小时）且标题不匹配，这是之前那场直播的文件，可以处理
		log.Printf("[LiveStatus] 房间 %s 正在直播（live_status=%d），但文件修改时间较久（%v）且标题不匹配（文件:%s, 直播:%s），判定为旧直播文件，可以处理",
			roomID, roomInfo.Data.LiveStatus, timeSinceModified, fileTitle, roomInfo.Data.Title)
		return true, false, nil
	}

	// 直播已结束，检查文件修改时间
	// 给予一定的缓冲时间（如5分钟），确保录播软件已完成文件写入
	if timeSinceModified < endedFileBuffer {
		log.Printf("[LiveStatus] 房间 %s 直播已结束（live_status=%d），但文件修改时间过近（%v），等待稳定",
			roomID, roomInfo.Data.LiveStatus, timeSinceModified)
		return false, false, nil
	}

	log.Printf("[LiveStatus] 房间 %s 直播已结束（live_status=%d）且文件已稳定（%v），可以处理",
		roomID, roomInfo.Data.LiveStatus, timeSinceModified)
	return true, false, nil
}
