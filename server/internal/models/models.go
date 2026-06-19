package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	TranscodeVideoCodecH264 = "h264"
	TranscodeVideoCodecH265 = "h265"
)

func NormalizeTranscodeVideoCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case TranscodeVideoCodecH265, "hevc", "x265":
		return TranscodeVideoCodecH265
	default:
		return TranscodeVideoCodecH264
	}
}

// RecordRoom 直播间配置
type RecordRoom struct {
	ID                   uint           `gorm:"primarykey" json:"id"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
	RoomID               string         `gorm:"uniqueIndex:idx_room_id;not null" json:"roomId"`
	Uname                string         `gorm:"index" json:"uname"`
	Title                string         `json:"title"`
	AreaName             string         `json:"areaName"`
	AreaNameParent       string         `json:"areaNameParent"`
	AreaNameChild        string         `json:"areaNameChild"`
	HistoryID            uint           `json:"historyId"`
	UploadUserID         uint           `gorm:"index" json:"uploadUserId"`
	Priority             int            `gorm:"default:50;index" json:"priority"`      // 自动上传调度优先级，0-100，数值越大越优先
	UploadSpeedLimitMBps float64        `gorm:"default:0" json:"uploadSpeedLimitMbps"` // 房间级上传限速，MB/s，0=使用全局/不限制
	Upload               bool           `gorm:"default:true;index" json:"upload"`      // 启用上传功能（总开关）
	AutoUpload           bool           `gorm:"default:true" json:"autoUpload"`        // 录制完成后自动上传分P
	AutoPublish          bool           `gorm:"default:false" json:"autoPublish"`      // 所有分P上传完成后自动投稿
	AutoParseDanmaku     bool           `gorm:"default:false" json:"autoParseDanmaku"` // 自动解析弹幕
	AutoSyncInfo         bool           `gorm:"default:false" json:"autoSyncInfo"`     // 定时同步视频信息（每30分钟）
	LastSyncTime         *time.Time     `json:"lastSyncTime"`                          // 最后同步时间
	TitleTemplate        string         `gorm:"type:text" json:"titleTemplate"`
	PartTitleTemplate    string         `gorm:"type:text" json:"partTitleTemplate"`
	DescTemplate         string         `gorm:"type:text" json:"descTemplate"`
	DynamicTemplate      string         `gorm:"type:text" json:"dynamicTemplate"`
	FileSizeLimit        int64          `gorm:"default:0" json:"fileSizeLimit"`
	DurationLimit        int            `gorm:"default:60" json:"durationLimit"`
	Tags                 string         `json:"tags"`
	TID                  int            `gorm:"default:171" json:"tid"`
	Copyright            int            `gorm:"default:1" json:"copyright"`
	SourceTemplate       string         `gorm:"type:text;default:'直播间: https://live.bilibili.com/${roomId}  稿件直播源'" json:"sourceTemplate"` // 转载来源模板，支持变量替换
	PercentileRank       float64        `gorm:"default:0.95" json:"percentileRank"`
	HighEnergyCut        bool           `gorm:"default:false" json:"highEnergyCut"`
	WindowSize           int            `gorm:"default:60" json:"windowSize"`         // 高能剪辑窗口大小(秒)
	MinSegmentDuration   int            `gorm:"default:10" json:"minSegmentDuration"` // 最小片段时长(秒)
	IsOnlySelf           bool           `gorm:"default:false" json:"isOnlySelf"`
	NoDisturbance        bool           `gorm:"default:false" json:"noDisturbance"`
	Line                 string         `gorm:"default:cs_bda2" json:"line"`
	AvailableLines       string         `gorm:"type:text" json:"availableLines"`         // 可用线路列表，逗号分隔，用于自动切换
	UploadUserStrategy   string         `gorm:"default:fixed" json:"uploadUserStrategy"` // fixed, round_robin, least_queue
	UploadWindowEnabled  bool           `gorm:"default:false" json:"uploadWindowEnabled"`
	UploadWindowStart    string         `gorm:"default:00:00" json:"uploadWindowStart"` // HH:MM
	UploadWindowEnd      string         `gorm:"default:23:59" json:"uploadWindowEnd"`   // HH:MM
	CoverURL             string         `json:"coverUrl"`
	CoverType            string         `gorm:"default:default" json:"coverType"` // default, live, frame, diy
	AutoExtractCover     bool           `gorm:"default:false" json:"autoExtractCover"`
	CoverFrameSecond     int            `gorm:"default:5" json:"coverFrameSecond"`
	Wxuid                string         `json:"wxuid"`
	PushMsgTags          string         `json:"pushMsgTags"`
	DeleteType           int            `gorm:"column:delete_type" json:"-"` // deprecated: 请使用 FileOpTrigger/Action/Scope/Delay
	DeleteDay            int            `gorm:"column:delete_day"  json:"-"` // deprecated: 请使用 FileOpDelay
	MoveDir              string         `json:"moveDir"`
	// 文件操作配置（替代旧版 DeleteType/DeleteDay）
	// FileOpTrigger: 0=不处理, 1=分P上传完成后, 2=全部分P上传完成后（投稿前）, 3=投稿成功后, 4=审核通过后
	FileOpTrigger int `gorm:"default:0" json:"fileOpTrigger"`
	// FileOpAction: 0=不处理, 1=删除, 2=移动, 3=复制
	FileOpAction int `gorm:"default:0" json:"fileOpAction"`
	// FileOpScope bitmask: 1=视频文件, 2=弹幕(.xml), 4=封面(.jpg/.png)
	FileOpScope int `gorm:"default:1" json:"fileOpScope"`
	// FileOpDelay: 0=立即执行, N=N天后执行
	FileOpDelay           int        `gorm:"default:0" json:"fileOpDelay"`
	SendDm                bool       `gorm:"default:false" json:"sendDm"`
	DmDistinct            bool       `gorm:"default:false" json:"dmDistinct"`          // 弹幕去重
	DmUlLevel             int        `gorm:"default:0" json:"dmUlLevel"`               // 用户等级过滤
	DmMedalLevel          int        `gorm:"default:0" json:"dmMedalLevel"`            // 粉丝勋章过滤 0-不过滤 1-佩戴粉丝勋章 2-佩戴主播粉丝勋章
	DmKeywordBlacklist    string     `gorm:"type:text" json:"dmKeywordBlacklist"`      // 关键词屏蔽，一行一个
	EnableDanmakuBurn     bool       `gorm:"default:false" json:"enableDanmakuBurn"`   // 启用弹幕烧录（生成带弹幕版本）
	AutoUpdatePublished   bool       `gorm:"default:false" json:"autoUpdatePublished"` // 弹幕版上传后自动更新已投稿视频
	DanmakuBurnStyle      string     `gorm:"default:default" json:"danmakuBurnStyle"`  // 弹幕样式：default, compact, large
	DanmakuFontSize       int        `gorm:"default:0" json:"danmakuFontSize"`         // 弹幕字号，0=跟随样式预设
	DanmakuFontColor      string     `json:"danmakuFontColor"`                         // 弹幕统一颜色，空=保留原色，支持 #RRGGBB
	DanmakuScrollArea     float64    `gorm:"default:0.75" json:"danmakuScrollArea"`    // 滚动弹幕区域比例
	DanmakuDisplayArea    float64    `gorm:"default:0.8" json:"danmakuDisplayArea"`    // 弹幕显示区域比例
	EnablePreTranscode    bool       `gorm:"default:false" json:"enablePreTranscode"`  // 上传前使用 FFmpeg 转码/压缩
	TranscodePreset       string     `gorm:"default:veryfast" json:"transcodePreset"`
	TranscodeCRF          int        `gorm:"default:23" json:"transcodeCrf"`
	TranscodeMaxWidth     int        `gorm:"default:0" json:"transcodeMaxWidth"` // 0=保持原宽度
	TranscodeAudioBitrate string     `gorm:"default:160k" json:"transcodeAudioBitrate"`
	TranscodeVideoCodec   string     `gorm:"default:h264" json:"transcodeVideoCodec"` // h264, h265
	Recording             bool       `gorm:"default:false;index" json:"recording"`
	Streaming             bool       `gorm:"default:false;index" json:"streaming"`
	SessionID             string     `gorm:"index" json:"sessionId"`
	SeasonID              int64      `json:"seasonId"`
	MergeBySession        bool       `gorm:"default:true" json:"mergeBySession"` // 同SessionID合并到一个投稿
	LiveStatus            int        `gorm:"default:0;index" json:"liveStatus"`  // 直播状态: 0未开播 1正在直播 2轮播中
	LastCheckTime         *time.Time `json:"lastCheckTime"`                      // 最后检查时间
}

// RecordHistory 录制历史
type RecordHistory struct {
	ID                 uint           `gorm:"primarykey" json:"id"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
	EventID            string         `gorm:"index" json:"eventId"`
	RoomID             string         `gorm:"index;not null" json:"roomId"`
	SessionID          string         `gorm:"uniqueIndex:idx_session" json:"sessionId"`
	Uname              string         `json:"uname"`
	Title              string         `json:"title"`
	AreaName           string         `json:"areaName"`
	StartTime          time.Time      `gorm:"index" json:"startTime"`
	EndTime            time.Time      `gorm:"index" json:"endTime"`
	Recording          bool           `gorm:"default:false;index" json:"recording"`
	Streaming          bool           `gorm:"default:false" json:"streaming"`
	Upload             bool           `gorm:"default:true;index" json:"upload"`
	Publish            bool           `gorm:"default:false;index" json:"publish"`
	BvID               string         `gorm:"index" json:"bvId"`
	AvID               string         `gorm:"index" json:"avId"`
	Code               int            `gorm:"default:-1" json:"code"`
	Message            string         `json:"message"`
	FilePath           string         `json:"filePath"`
	FileSize           int64          `gorm:"default:0" json:"fileSize"`
	UploadRetryCount   int            `gorm:"default:0" json:"uploadRetryCount"`
	UploadStatus       int            `gorm:"default:0;index" json:"uploadStatus"`     // 上传状态: 0未上传, 1上传中, 2已上传
	PublishErrorType   string         `gorm:"index" json:"publishErrorType"`           // 投稿错误分类：rate_limit/auth/network/file/unknown
	PublishCooldownAt  *time.Time     `gorm:"index" json:"publishCooldownAt"`          // 投稿风控冷却时间
	PublishRetryCount  int            `gorm:"default:0" json:"publishRetryCount"`      // 投稿失败重试计数
	VideoState         int            `gorm:"default:-1;index" json:"videoState"`      // 视频状态: -1未知, 0审核中, 1已通过, -2未通过, 2已下架, 3仅自己可见
	VideoStateDesc     string         `json:"videoStateDesc"`                          // 视频状态描述
	DanmakuSent        bool           `gorm:"default:false;index" json:"danmakuSent"`  // 弹幕是否已发送
	DanmakuCount       int            `gorm:"default:0" json:"danmakuCount"`           // 弹幕总数
	FilesMoved         bool           `gorm:"default:false;index" json:"filesMoved"`   // 文件是否已移动
	SyncedAt           *time.Time     `json:"syncedAt"`                                // 最后同步时间
	ApprovedAt         *time.Time     `gorm:"index" json:"approvedAt"`                 // 审核通过时间（video_state 首次变为 1 时记录）
	CoverURL           string         `json:"coverUrl"`                                // 封面URL
	IsHighlight        bool           `gorm:"default:false;index" json:"isHighlight"`  // 是否为自动生成的高能剪辑稿件
	GuideCommentRPID   int64          `gorm:"default:0" json:"guideCommentRpid"`       // 同场次BV索引评论ID
	GuideCommentPinned bool           `gorm:"default:false" json:"guideCommentPinned"` // 同场次BV索引评论是否已置顶
	ScheduledDeleteAt  *time.Time     `gorm:"index" json:"scheduledDeleteAt"`          // 计划执行时间（用于延迟操作策略）
	ScheduledOpAction  int            `gorm:"default:0" json:"scheduledOpAction"`      // 计划时记录的操作类型(1=删除,2=移动,3=复制)
	ScheduledOpScope   int            `gorm:"default:1" json:"scheduledOpScope"`       // 计划时记录的操作范围 bitmask
	RoomName           string         `gorm:"-" json:"roomName"`
	PartCount          int            `gorm:"-" json:"partCount"`
	PartDuration       float64        `gorm:"-" json:"partDuration"`
	UploadPartCount    int            `gorm:"-" json:"uploadPartCount"`
	RecordPartCount    int            `gorm:"-" json:"recordPartCount"`
	MsgCount           int            `gorm:"-" json:"msgCount"`
}

// RecordHistoryPart 录制分P
type RecordHistoryPart struct {
	ID                  uint       `gorm:"primarykey" json:"id"`
	CreatedAt           time.Time  `gorm:"index" json:"createdAt"`
	HistoryID           uint       `gorm:"index;not null" json:"historyId"`
	RoomID              string     `gorm:"index" json:"roomId"`
	SessionID           string     `gorm:"index" json:"sessionId"`
	Title               string     `json:"title"`
	LiveTitle           string     `json:"liveTitle"`
	AreaName            string     `json:"areaName"`
	FilePath            string     `gorm:"uniqueIndex:idx_file_path" json:"filePath"`
	FileName            string     `json:"fileName"`
	FileSize            int64      `gorm:"default:0" json:"fileSize"`
	Duration            int        `gorm:"default:0" json:"duration"`
	StartTime           time.Time  `gorm:"index" json:"startTime"`
	EndTime             time.Time  `json:"endTime"`
	Recording           bool       `gorm:"default:false;index" json:"recording"`
	Upload              bool       `gorm:"default:false;index" json:"upload"`
	Uploading           bool       `gorm:"default:false" json:"uploading"`
	UploadPaused        bool       `gorm:"default:false;index" json:"uploadPaused"`
	UploadCancelled     bool       `gorm:"default:false;index" json:"uploadCancelled"`
	CID                 int64      `gorm:"column:c_id" json:"cid"`
	FileDelete          bool       `gorm:"default:false" json:"fileDelete"`
	FileMoved           bool       `gorm:"default:false" json:"fileMoved"`
	Page                int        `gorm:"default:0" json:"page"`                      // 分P序号
	XcodeState          int        `gorm:"default:0" json:"xcodeState"`                // 转码状态
	UploadUserID        uint       `gorm:"index" json:"uploadUserId"`                  // 实际执行上传的B站账号
	UploadedAt          *time.Time `gorm:"index" json:"uploadedAt"`                    // 上传完成时间，用于账号每日配额统计
	UploadRetryCount    int        `gorm:"default:0" json:"uploadRetryCount"`          // 上传重试次数
	UploadErrorMsg      string     `gorm:"type:text" json:"uploadErrorMsg"`            // 上传错误信息
	UploadErrorType     string     `gorm:"index" json:"uploadErrorType"`               // 错误分类：network/rate_limit/auth/file/transcode/window/user/unknown
	UploadLine          string     `json:"uploadLine"`                                 // 实际上传使用的线路
	RateLimitCooldownAt *time.Time `gorm:"index" json:"rateLimitCooldownAt"`           // 速率限制冷却时间（24小时后恢复）
	RateLimitRetryCount int        `gorm:"default:0" json:"rateLimitRetryCount"`       // 406速率限制失败次数
	IsTempFile          bool       `gorm:"default:false;index" json:"isTempFile"`      // 是否为临时文件（自动切分、弹幕烧录等生成）
	SourcePartID        uint       `gorm:"index" json:"sourcePartId"`                  // 源Part ID（如果是从其他Part派生）
	TempFileType        string     `json:"tempFileType"`                               // 临时文件类型：split（切分）, danmaku_burn（弹幕烧录）, high_energy（高能剪辑）
	AppendedToVideo     bool       `gorm:"default:false;index" json:"appendedToVideo"` // 弹幕烧录版是否已通过 EditVideo 成功追加到已投稿视频
}

// BiliBiliUser B站用户
type BiliBiliUser struct {
	ID               uint           `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	UID              int64          `gorm:"uniqueIndex;not null" json:"uid"`
	Uname            string         `gorm:"index" json:"uname"`
	Face             string         `json:"face"`
	Cookies          string         `gorm:"type:text" json:"cookies"`
	AccessKey        string         `json:"accessKey"`
	RefreshToken     string         `json:"refreshToken"`
	Login            bool           `gorm:"default:false;index" json:"login"`
	Enabled          bool           `gorm:"default:true;index" json:"enabled"`
	Level            int            `json:"level"`
	VipType          int            `json:"vipType"`
	VipStatus        int            `json:"vipStatus"`
	Moral            int            `json:"moral"`
	CookieInfo       string         `gorm:"type:text" json:"cookieInfo"`
	LoginTime        *time.Time     `json:"loginTime"`
	ExpireTime       *time.Time     `json:"expireTime"`
	LastCheckTime    *time.Time     `json:"lastCheckTime"`
	LastCheckError   string         `gorm:"type:text" json:"lastCheckError"`
	WxPushToken      string         `json:"wxPushToken"`                       // 用户的WxPusher token
	DailyUploadQuota int            `gorm:"default:0" json:"dailyUploadQuota"` // 每日上传分P配额，0表示不限额
}

type LiveMsg struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"createdAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	BvID        string         `gorm:"index" json:"bvid"`
	RoomID      string         `gorm:"index" json:"roomId"`
	SessionID   string         `gorm:"index" json:"sessionId"`
	Timestamp   int64          `gorm:"index" json:"timestamp"` // 相对于直播开始的时间戳（毫秒）
	Type        int            `json:"type"`                   // 1=文字弹幕
	Message     string         `gorm:"type:text" json:"message"`
	UserName    string         `json:"userName"`
	UID         int64          `json:"uid"`
	ULevel      int            `gorm:"column:u_level;default:0" json:"ulevel"`        // 用户等级
	MedalName   string         `json:"medalName"`                                     // 粉丝勋章名称
	MedalLevel  int            `gorm:"default:0" json:"medalLevel"`                   // 粉丝勋章等级
	MedalRoomID string         `gorm:"column:medal_room_id;index" json:"medalRoomId"` // 粉丝勋章所属的房间ID
	Sent        bool           `gorm:"default:false;index" json:"sent"`               // 是否已发送到视频
	CID         int64          `gorm:"column:c_id;index" json:"cid"`                  // 发送到哪个CID
	Progress    int            `json:"progress"`                                      // 视频中的位置（毫秒）
	Mode        int            `gorm:"default:1" json:"mode"`                         // 弹幕模式: 1滚动 4底部 5顶部
	FontSize    int            `gorm:"default:25" json:"fontSize"`                    // 字号
	Color       int            `gorm:"default:16777215" json:"color"`                 // 颜色
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
	NextRunAt  *time.Time     `gorm:"index" json:"nextRunAt"`
	Message    string         `gorm:"type:text" json:"message"`
	UserName   string         `json:"userName"`
	UID        int64          `json:"uid"`
}

// SystemConfig 系统全局配置
type SystemConfig struct {
	ID                    uint      `gorm:"primarykey" json:"id"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
	AutoFileScan          bool      `gorm:"default:true" json:"autoFileScan"`           // 自动扫盘录入
	EnableFileWatcher     bool      `gorm:"default:true" json:"enableFileWatcher"`      // 启用目录事件监控，作为扫盘的即时触发器
	FileScanInterval      int       `gorm:"default:60" json:"fileScanInterval"`         // 文件扫描间隔（分钟）
	FileScanMinAge        int       `gorm:"default:12" json:"fileScanMinAge"`           // 文件最小年龄（小时），避免扫描正在写入的文件
	FileScanMinSize       int64     `gorm:"default:1048576" json:"fileScanMinSize"`     // 文件最小大小（字节）
	FileScanMaxAge        int       `gorm:"default:720" json:"fileScanMaxAge"`          // 文件最大年龄（小时），30天
	WorkPath              string    `gorm:"type:text" json:"workPath"`                  // 录制文件工作目录
	CustomScanPaths       string    `gorm:"type:text" json:"customScanPaths"`           // 自定义扫盘目录，逗号分隔，优先扫描
	EnableOrphanScan      bool      `gorm:"default:true" json:"enableOrphanScan"`       // 启用孤儿文件扫描
	OrphanScanInterval    int       `gorm:"default:360" json:"orphanScanInterval"`      // 孤儿文件扫描间隔（分钟）
	EnableDanmakuProxy    bool      `gorm:"default:false" json:"enableDanmakuProxy"`    // 启用弹幕代理池（全局配置）
	DanmakuProxyList      string    `gorm:"type:text" json:"danmakuProxyList"`          // 代理列表，每行一个，格式: socks5://ip:port 或 http://user:pass@ip:port
	AutoDataRepair        bool      `gorm:"default:true" json:"autoDataRepair"`         // 开启每日凌晨进行数据一致性修复
	UploadSpeedLimitMBps  float64   `gorm:"default:0" json:"uploadSpeedLimitMbps"`      // 全局上传限速，MB/s，0=不限制
	UploadWhileRecording  bool      `gorm:"default:false" json:"uploadWhileRecording"`  // 文件稳定后允许直播仍在进行时预上传
	PublishWhileRecording bool      `gorm:"default:false" json:"publishWhileRecording"` // 分P上传完后允许直播仍在进行时投稿，后续分P自动追加
	PublishMode           string    `gorm:"default:local" json:"publishMode"`           // local, remote
	PublishAgentEndpoint  string    `gorm:"type:text" json:"publishAgentEndpoint"`      // 远程投稿 Agent 地址
	PublishAgentToken     string    `gorm:"type:text" json:"publishAgentToken"`         // 远程投稿 Agent 访问令牌
	PublishAgentTimeout   int       `gorm:"default:30" json:"publishAgentTimeout"`      // 远程投稿请求超时（秒）
	DanmakuBurnStyle      string    `gorm:"default:default" json:"danmakuBurnStyle"`    // 全局弹幕烧录样式默认值
	DanmakuFontSize       int       `gorm:"default:0" json:"danmakuFontSize"`           // 全局弹幕字号默认值，0=跟随样式
	DanmakuFontColor      string    `json:"danmakuFontColor"`                           // 全局弹幕颜色默认值
	DanmakuScrollArea     float64   `gorm:"default:0.75" json:"danmakuScrollArea"`      // 全局滚动弹幕区域比例
	DanmakuDisplayArea    float64   `gorm:"default:0.8" json:"danmakuDisplayArea"`      // 全局弹幕显示区域比例
}
