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
	)
	if err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
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
