package models

import (
	"time"

	"gorm.io/gorm"
)

// RecordRoom 直播间配置
type RecordRoom struct {
	ID                 uint           `gorm:"primarykey" json:"id"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
	RoomID             string         `gorm:"uniqueIndex:idx_room_id;not null" json:"roomId"`
	Uname              string         `gorm:"index" json:"uname"`
	Title              string         `json:"title"`
	AreaName           string         `json:"areaName"`
	AreaNameParent     string         `json:"areaNameParent"`
	AreaNameChild      string         `json:"areaNameChild"`
	HistoryID          uint           `json:"historyId"`
	UploadUserID       uint           `gorm:"index" json:"uploadUserId"`
	Upload             bool           `gorm:"default:true;index" json:"upload"`
	AutoUpload         bool           `gorm:"default:true" json:"autoUpload"`
	TitleTemplate      string         `gorm:"type:text" json:"titleTemplate"`
	PartTitleTemplate  string         `gorm:"type:text" json:"partTitleTemplate"`
	DescTemplate       string         `gorm:"type:text" json:"descTemplate"`
	DynamicTemplate    string         `gorm:"type:text" json:"dynamicTemplate"`
	FileSizeLimit      int64          `gorm:"default:0" json:"fileSizeLimit"`
	DurationLimit      int            `gorm:"default:60" json:"durationLimit"`
	Tags               string         `json:"tags"`
	TID                int            `gorm:"default:171" json:"tid"`
	Copyright          int            `gorm:"default:1" json:"copyright"`
	PercentileRank     float64        `gorm:"default:0.95" json:"percentileRank"`
	HighEnergyCut      bool           `gorm:"default:false" json:"highEnergyCut"`
	WindowSize         int            `gorm:"default:60" json:"windowSize"`         // 高能剪辑窗口大小(秒)
	MinSegmentDuration int            `gorm:"default:10" json:"minSegmentDuration"` // 最小片段时长(秒)
	IsOnlySelf         bool           `gorm:"default:false" json:"isOnlySelf"`
	NoDisturbance      bool           `gorm:"default:false" json:"noDisturbance"`
	Line               string         `gorm:"default:cs_bda2" json:"line"`
	CoverURL           string         `json:"coverUrl"`
	CoverType          string         `gorm:"default:default" json:"coverType"` // default, live, diy
	Wxuid              string         `json:"wxuid"`
	PushMsgTags        string         `json:"pushMsgTags"`
	DeleteType         int            `gorm:"default:0" json:"deleteType"` // 0-不处理 1-上传前删除 2-上传前移动 3-上传后删除 4-上传后移动 5-上传前复制 6-上传后复制 7-上传完成后立即删除 8-N天后删除移动 9-投稿成功后删除 10-投稿成功后移动 11-审核通过后复制
	DeleteDay          int            `gorm:"default:5" json:"deleteDay"`
	MoveDir            string         `json:"moveDir"`
	SendDm             bool           `gorm:"default:false" json:"sendDm"`
	DmDistinct         bool           `gorm:"default:false" json:"dmDistinct"`     // 弹幕去重
	DmUlLevel          int            `gorm:"default:0" json:"dmUlLevel"`          // 用户等级过滤
	DmMedalLevel       int            `gorm:"default:0" json:"dmMedalLevel"`       // 粉丝勋章过滤 0-不过滤 1-佩戴粉丝勋章 2-佩戴主播粉丝勋章
	DmKeywordBlacklist string         `gorm:"type:text" json:"dmKeywordBlacklist"` // 关键词屏蔽，一行一个
	Recording          bool           `gorm:"default:false;index" json:"recording"`
	Streaming          bool           `gorm:"default:false;index" json:"streaming"`
	SessionID          string         `gorm:"index" json:"sessionId"`
	SeasonID           int64          `json:"seasonId"`
}

// RecordHistory 录制历史
type RecordHistory struct {
	ID               uint           `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	EventID          string         `gorm:"index" json:"eventId"`
	RoomID           string         `gorm:"index;not null" json:"roomId"`
	SessionID        string         `gorm:"uniqueIndex:idx_session" json:"sessionId"`
	Uname            string         `json:"uname"`
	Title            string         `json:"title"`
	AreaName         string         `json:"areaName"`
	StartTime        time.Time      `gorm:"index" json:"startTime"`
	EndTime          time.Time      `gorm:"index" json:"endTime"`
	Recording        bool           `gorm:"default:false;index" json:"recording"`
	Streaming        bool           `gorm:"default:false" json:"streaming"`
	Upload           bool           `gorm:"default:true;index" json:"upload"`
	Publish          bool           `gorm:"default:false;index" json:"publish"`
	BvID             string         `gorm:"index" json:"bvId"`
	AvID             string         `gorm:"index" json:"avId"`
	Code             int            `gorm:"default:-1" json:"code"`
	Message          string         `json:"message"`
	FilePath         string         `json:"filePath"`
	FileSize         int64          `gorm:"default:0" json:"fileSize"`
	UploadRetryCount int            `gorm:"default:0" json:"uploadRetryCount"`
	VideoState       int            `gorm:"default:-1;index" json:"videoState"`     // 视频状态: -1未知, 0审核中, 1已通过, 2未通过
	VideoStateDesc   string         `json:"videoStateDesc"`                         // 视频状态描述
	DanmakuSent      bool           `gorm:"default:false;index" json:"danmakuSent"` // 弹幕是否已发送
	DanmakuCount     int            `gorm:"default:0" json:"danmakuCount"`          // 弹幕总数
	FilesMoved       bool           `gorm:"default:false;index" json:"filesMoved"`  // 文件是否已移动
	SyncedAt         *time.Time     `json:"syncedAt"`                               // 最后同步时间
	CoverURL         string         `json:"coverUrl"`                               // 封面URL
	RoomName         string         `gorm:"-" json:"roomName"`
	PartCount        int            `gorm:"-" json:"partCount"`
	PartDuration     float64        `gorm:"-" json:"partDuration"`
	UploadPartCount  int            `gorm:"-" json:"uploadPartCount"`
	RecordPartCount  int            `gorm:"-" json:"recordPartCount"`
	MsgCount         int            `gorm:"-" json:"msgCount"`
}

// RecordHistoryPart 录制分P
type RecordHistoryPart struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
	HistoryID  uint      `gorm:"index;not null" json:"historyId"`
	RoomID     string    `gorm:"index" json:"roomId"`
	SessionID  string    `gorm:"index" json:"sessionId"`
	Title      string    `json:"title"`
	LiveTitle  string    `json:"liveTitle"`
	AreaName   string    `json:"areaName"`
	FilePath   string    `gorm:"uniqueIndex:idx_file_path" json:"filePath"`
	FileName   string    `json:"fileName"`
	FileSize   int64     `gorm:"default:0" json:"fileSize"`
	Duration   int       `gorm:"default:0" json:"duration"`
	StartTime  time.Time `gorm:"index" json:"startTime"`
	EndTime    time.Time `json:"endTime"`
	Recording  bool      `gorm:"default:false;index" json:"recording"`
	Upload     bool      `gorm:"default:false;index" json:"upload"`
	Uploading  bool      `gorm:"default:false" json:"uploading"`
	CID        int64     `json:"cid"`
	FileDelete bool      `gorm:"default:false" json:"fileDelete"`
	FileMoved  bool      `gorm:"default:false" json:"fileMoved"`
	Page       int       `gorm:"default:0" json:"page"`       // 分P序号
	XcodeState int       `gorm:"default:0" json:"xcodeState"` // 转码状态
}

// BiliBiliUser B站用户
type BiliBiliUser struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	UID          int64          `gorm:"uniqueIndex;not null" json:"uid"`
	Uname        string         `gorm:"index" json:"uname"`
	Face         string         `json:"face"`
	Cookies      string         `gorm:"type:text" json:"cookies"`
	AccessKey    string         `json:"accessKey"`
	RefreshToken string         `json:"refreshToken"`
	Login        bool           `gorm:"default:false;index" json:"login"`
	Level        int            `json:"level"`
	VipType      int            `json:"vipType"`
	VipStatus    int            `json:"vipStatus"`
	Moral        int            `json:"moral"`
	CookieInfo   string         `gorm:"type:text" json:"cookieInfo"`
	LoginTime    *time.Time     `json:"loginTime"`
	ExpireTime   *time.Time     `json:"expireTime"`
	WxPushToken  string         `json:"wxPushToken"` // 用户的WxPusher token
}

type LiveMsg struct {
	uint       `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time      `json:"createdAt"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	BvID       string         `gorm:"index" json:"bvid"`
	RoomID     string         `gorm:"index" json:"roomId"`
	SessionID  string         `gorm:"index" json:"sessionId"`
	Timestamp  int64          `gorm:"index" json:"timestamp"` // 相对于直播开始的时间戳（毫秒）
	Type       int            `json:"type"`                   // 1=文字弹幕
	Message    string         `gorm:"type:text" json:"message"`
	UserName   string         `json:"userName"`
	UID        int64          `json:"uid"`
	ULevel     int            `gorm:"default:0" json:"ulevel"`         // 用户等级
	MedalName  string         `json:"medalName"`                       // 粉丝勋章名称
	MedalLevel int            `gorm:"default:0" json:"medalLevel"`     // 粉丝勋章等级
	Sent       bool           `gorm:"default:false;index" json:"sent"` // 是否已发送到视频
	CID        int64          `gorm:"index" json:"cid"`                // 发送到哪个CID
	Progress   int            `json:"progress"`                        // 视频中的位置（毫秒）
	Mode       int            `gorm:"default:1" json:"mode"`           // 弹幕模式: 1滚动 4底部 5顶部
	FontSize   int            `gorm:"default:25" json:"fontSize"`      // 字号
	Color      int            `gorm:"default:16777215" json:"color"`   // 颜色
}

// VideoSyncTask 视频同步任务
type VideoSyncTask struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	HistoryID  uint           `gorm:"uniqueIndex;not null" json:"historyId"`
	BvID       string         `gorm:"index" json:"bvid"`
	Status     string         `gorm:"default:pending;index" json:"status"` // pending, running, completed, failed
	RetryCount int            `gorm:"default:0" json:"retryCount"`
	LastError  string         `json:"lastError"`
	NextRunAt  *time.Time     `gorm:"index" json:"nextRunAte"`
	Message    string         `gorm:"type:text" json:"message"`
	UserName   string         `json:"userName"`
	UID        int64          `json:"uid"`
}
