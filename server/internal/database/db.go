package database

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	appconfig "github.com/gobup/server/internal/config"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(dbPath string) error {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// 配置 SQLite 运行参数以支持并发读写。
	DB.Exec("PRAGMA journal_mode=WAL")
	DB.Exec("PRAGMA busy_timeout=5000")
	DB.Exec("PRAGMA synchronous=NORMAL")
	DB.Exec("PRAGMA cache_size=10000")

	// 配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// SQLite 只支持单个写入连接，但可以有多个读取连接
	sqlDB.SetMaxOpenConns(1)    // 限制最大打开连接数为 1，避免写入冲突
	sqlDB.SetMaxIdleConns(1)    // 空闲连接数
	sqlDB.SetConnMaxLifetime(0) // 连接可以一直重用

	// 在 AutoMigrate 之前修复历史遗留的 live_msgs 表结构。
	// 旧版本用匿名 uint 作为主键，实际列名为 "uint"；新版本改为具名 ID，列名为 "id"。
	// SQLite 不支持 ALTER TABLE ADD PRIMARY KEY，AutoMigrate 会直接报错，必须手动重建表。
	if err := migrateLiveMsgsTable(); err != nil {
		return fmt.Errorf("迁移 live_msgs 表失败: %w", err)
	}

	// 自动迁移
	err = DB.AutoMigrate(
		&models.RecordRoom{},
		&models.RecordHistory{},
		&models.RecordHistoryPart{},
		&models.BiliBiliUser{},
		&models.LiveMsg{},
		&models.VideoSyncTask{},
		&models.SystemConfig{},
	)
	if err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 添加额外的索引和约束
	// 为 RecordHistory 添加组合索引
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_history_room_time ON record_histories(room_id, end_time DESC)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_history_session_room ON record_histories(session_id, room_id)")

	// 为 RecordHistoryPart 添加组合索引
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_part_history_time ON record_history_parts(history_id, start_time)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_part_file_path ON record_history_parts(file_path)")
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_part_room_time ON record_history_parts(room_id, end_time)")

	// 数据补填：对已存在的已审核通过记录，补填 approved_at 字段
	// 新字段 approved_at 上线前已经通过审核的记录为 NULL，导致弹幕回补定时任务扫不到。
	// 用 COALESCE(synced_at, updated_at) 作为近似的审核通过时间，保证历史数据立即参与回补扫描。
	DB.Exec(`UPDATE record_histories SET approved_at = COALESCE(synced_at, updated_at) WHERE video_state = 1 AND approved_at IS NULL`)

	// 数据迁移：将旧的 delete_type 单字段策略迁移到新的 file_op_* 四字段体系。
	// 仅对 delete_type != 0 且 file_op_action = 0 的房间执行（幂等，不重复执行）。
	// 迁移映射关系（原 delete_type → 新字段）：
	//   0=不处理, 1=上传前删除(全量), 2=上传前移动, 3=上传后删除, 4=上传后移动,
	//   5=上传前复制, 6=上传后复制, 7=上传后立即删除, 8=N天后删除,
	//   9=投稿后删除(仅视频), 10=投稿后移动, 11=审核后复制, 12=审核后延迟删除(仅视频,3天)
	DB.Exec(`UPDATE record_rooms SET
		file_op_trigger = CASE delete_type
			WHEN 1 THEN 2 WHEN 2 THEN 2 WHEN 5 THEN 2
			WHEN 3 THEN 1 WHEN 4 THEN 1 WHEN 6 THEN 1 WHEN 7 THEN 1 WHEN 8 THEN 1
			WHEN 9 THEN 3 WHEN 10 THEN 3
			WHEN 11 THEN 4 WHEN 12 THEN 4
			ELSE 0 END,
		file_op_action = CASE delete_type
			WHEN 1 THEN 1 WHEN 3 THEN 1 WHEN 7 THEN 1 WHEN 8 THEN 1
			WHEN 9 THEN 1 WHEN 12 THEN 1
			WHEN 2 THEN 2 WHEN 4 THEN 2 WHEN 10 THEN 2
			WHEN 5 THEN 3 WHEN 6 THEN 3 WHEN 11 THEN 3
			ELSE 0 END,
		file_op_scope = CASE delete_type
			WHEN 9 THEN 1 WHEN 12 THEN 1
			ELSE 7 END,
		file_op_delay = CASE delete_type
			WHEN 8 THEN delete_day
			WHEN 12 THEN 3
			ELSE 0 END
		WHERE delete_type != 0 AND file_op_action = 0`)

	// 初始化系统配置（如果不存在）
	var config models.SystemConfig
	if err := DB.First(&config).Error; err != nil {
		// 创建默认配置
		config = models.SystemConfig{
			AutoFileScan:       true,
			EnableFileWatcher:  true,
			FileScanInterval:   60,
			FileScanMinAge:     12,
			FileScanMinSize:    1048576, // 1MB
			FileScanMaxAge:     720,     // 30天
			WorkPath:           appconfig.AppConfig.WorkPath,
			CustomScanPaths:    "", // 默认为空
			EnableOrphanScan:   true,
			OrphanScanInterval: 360,  // 6小时
			AutoDataRepair:     true, // 默认开启数据修复
		}
		DB.Create(&config)
	} else if config.WorkPath == "" && appconfig.AppConfig.WorkPath != "" {
		config.WorkPath = appconfig.AppConfig.WorkPath
		DB.Save(&config)
	}

	return nil
}

func CloseDB() {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
}

func GetDB() *gorm.DB {
	return DB
}

// WithRetry 执行数据库操作并在遇到 database is locked 错误时自动重试
func WithRetry(fn func() error, maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}

		// 检查是否是数据库锁定错误
		if err.Error() == "database is locked" {
			// 等待一段时间后重试，使用指数退避
			waitTime := time.Duration(50*(i+1)) * time.Millisecond
			time.Sleep(waitTime)
			continue
		}

		// 其他错误直接返回
		return err
	}
	return err
}

// migrateLiveMsgsTable 处理 live_msgs 表的历史结构兼容迁移。
// 旧版本将主键列命名为 "uint"（匿名嵌入 uint），新版本改为 "id"（具名字段 ID）。
// SQLite 不支持直接 ALTER TABLE ADD PRIMARY KEY，因此采用"重建"策略：
//  1. 检查旧表是否存在且包含 "uint" 列
//  2. 如果是旧表：重命名 → 建新表 → 复制数据 → 删旧表
func migrateLiveMsgsTable() error {
	// 检查 live_msgs 表是否存在
	var tableCount int
	DB.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='live_msgs'").Scan(&tableCount)
	if tableCount == 0 {
		// 表不存在，无需迁移，AutoMigrate 会自动创建
		return nil
	}

	// 检查是否存在旧版 "uint" 列（匿名嵌入主键）
	type ColInfo struct {
		Name string `gorm:"column:name"`
	}
	var cols []ColInfo
	DB.Raw("PRAGMA table_info(live_msgs)").Scan(&cols)

	hasUintCol := false
	hasIDCol := false
	for _, c := range cols {
		if c.Name == "uint" {
			hasUintCol = true
		}
		if c.Name == "id" {
			hasIDCol = true
		}
	}

	if !hasUintCol || hasIDCol {
		// 已经是新结构，无需迁移
		return nil
	}

	// 旧表：重建流程
	// Step 1: 将旧表重命名为备份表
	if err := DB.Exec("ALTER TABLE live_msgs RENAME TO live_msgs_old").Error; err != nil {
		return fmt.Errorf("重命名旧表失败: %w", err)
	}

	// Step 2: 用新结构创建 live_msgs 表（AutoMigrate 前手动建表）
	createSQL := `CREATE TABLE live_msgs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME,
		deleted_at DATETIME,
		bv_id TEXT,
		room_id TEXT,
		session_id TEXT,
		timestamp INTEGER,
		type INTEGER,
		message TEXT,
		user_name TEXT,
		uid INTEGER,
		u_level INTEGER DEFAULT 0,
		medal_name TEXT,
		medal_level INTEGER DEFAULT 0,
		medal_room_id TEXT,
		sent INTEGER DEFAULT 0,
		c_id INTEGER,
		progress INTEGER,
		mode INTEGER DEFAULT 1,
		font_size INTEGER DEFAULT 25,
		color INTEGER DEFAULT 16777215
	)`
	if err := DB.Exec(createSQL).Error; err != nil {
		// 建表失败，回滚重命名
		DB.Exec("ALTER TABLE live_msgs_old RENAME TO live_msgs")
		return fmt.Errorf("创建新表失败: %w", err)
	}

	// Step 3: 将旧数据复制到新表（旧 "uint" 列映射到新 "id"）
	copySQL := `INSERT INTO live_msgs (id, created_at, deleted_at, bv_id, room_id, session_id,
		timestamp, type, message, user_name, uid, u_level, medal_name, medal_level,
		medal_room_id, sent, c_id, progress, mode, font_size, color)
	SELECT "uint", created_at, deleted_at, bv_id, room_id, session_id,
		timestamp, type, message, user_name, uid, u_level, medal_name, medal_level,
		medal_room_id, sent, c_id, progress, mode, font_size, color
	FROM live_msgs_old`
	if err := DB.Exec(copySQL).Error; err != nil {
		// 复制失败，回滚：删除新表，恢复旧表名
		DB.Exec("DROP TABLE IF EXISTS live_msgs")
		DB.Exec("ALTER TABLE live_msgs_old RENAME TO live_msgs")
		return fmt.Errorf("迁移数据失败: %w", err)
	}

	// Step 4: 删除旧备份表
	DB.Exec("DROP TABLE live_msgs_old")

	return nil
}
