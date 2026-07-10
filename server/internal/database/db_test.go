package database

import (
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

func TestWithRetryDetectsSQLiteLockedError(t *testing.T) {
	calls := 0
	err := WithRetry(func() error {
		calls++
		if calls == 1 {
			return errors.New("SQLITE_BUSY: database is locked")
		}
		return nil
	}, 2)
	if err != nil {
		t.Fatalf("WithRetry() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRecordHistoryAllowsSameSessionAcrossDifferentDays(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RecordHistory{}); err != nil {
		t.Fatal(err)
	}

	first := models.RecordHistory{
		RoomID:    "5050",
		SessionID: "same-session",
		Title:     "same title",
		StartTime: time.Date(2026, 6, 20, 20, 0, 0, 0, time.Local),
		EndTime:   time.Date(2026, 6, 20, 22, 0, 0, 0, time.Local),
		Upload:    true,
	}
	second := first
	second.ID = 0
	second.StartTime = time.Date(2026, 6, 21, 20, 0, 0, 0, time.Local)
	second.EndTime = time.Date(2026, 6, 21, 22, 0, 0, 0, time.Local)

	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("same session id on a different day should be allowed: %v", err)
	}
}
