package database

import (
	"fmt"

	"github.com/gobup/server/internal/models"
	"gorm.io/driver/sqlite"
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
