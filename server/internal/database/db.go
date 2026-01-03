package database

import (
	"fmt"
	"time"

	"github.com/gobup/server/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(dbPath string) error {
	var err error
	// 配置 SQLite 连接参数以支持并发
	// WAL 模式允许并发读写，busy_timeout 设置等待时间
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_timeout=5000&_synchronous=NORMAL&_cache_size=10000", dbPath)
	DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// 配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// SQLite 只支持单个写入连接，但可以有多个读取连接
	sqlDB.SetMaxOpenConns(1)    // 限制最大打开连接数为 1，避免写入冲突
	sqlDB.SetMaxIdleConns(1)    // 空闲连接数
	sqlDB.SetConnMaxLifetime(0) // 连接可以一直重用

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

	// 初始化系统配置（如果不存在）
	var config models.SystemConfig
	if err := DB.First(&config).Error; err != nil {
		// 创建默认配置
		config = models.SystemConfig{
			AutoFileScan:       true,
			FileScanInterval:   60,
			FileScanMinAge:     12,
			FileScanMinSize:    1048576, // 1MB
			FileScanMaxAge:     720,     // 30天
			CustomScanPaths:    "",      // 默认为空
			EnableOrphanScan:   true,
			OrphanScanInterval: 360, // 6小时
		}
		DB.Create(&config)
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
