package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

func TestLoadPublishablePartCounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RecordHistoryPart{}); err != nil {
		t.Fatal(err)
	}

	parts := []models.RecordHistoryPart{
		{HistoryID: 1, FilePath: "/tmp/original-uploaded.mp4", Upload: true, IsTempFile: false},
		{HistoryID: 1, FilePath: "/tmp/original-pending.mp4", Upload: false, IsTempFile: false},
		{HistoryID: 1, FilePath: "/tmp/danmaku-burn.mp4", Upload: true, IsTempFile: true, TempFileType: "danmaku_burn"},
		{HistoryID: 2, FilePath: "/tmp/split-parent.mp4", Upload: false, FileDelete: true, IsTempFile: false},
		{HistoryID: 2, FilePath: "/tmp/split-1.mp4", Upload: true, FileDelete: true, IsTempFile: true, TempFileType: "split"},
		{HistoryID: 2, FilePath: "/tmp/split-2.mp4", Upload: true, FileDelete: true, IsTempFile: true, TempFileType: "split"},
		{HistoryID: 3, FilePath: "/tmp/recording.mp4", Upload: false, IsTempFile: false, Recording: true},
	}
	if err := db.Create(&parts).Error; err != nil {
		t.Fatal(err)
	}

	counts := LoadPublishablePartCounts(db, []uint{1, 2, 3, 4})

	if got := counts[1]; got.Uploaded != 1 || got.Total != 2 || got.Recording != 0 {
		t.Fatalf("history 1 counts = %+v, want uploaded=1 total=2 recording=0", got)
	}
	if got := counts[2]; got.Uploaded != 2 || got.Total != 2 || got.Recording != 0 {
		t.Fatalf("history 2 counts = %+v, want uploaded=2 total=2 recording=0", got)
	}
	if got := counts[3]; got.Uploaded != 0 || got.Total != 1 || got.Recording != 1 {
		t.Fatalf("history 3 counts = %+v, want uploaded=0 total=1 recording=1", got)
	}
	if got := counts[4]; got.Uploaded != 0 || got.Total != 0 || got.Recording != 0 {
		t.Fatalf("history 4 counts = %+v, want zero counts", got)
	}
}
