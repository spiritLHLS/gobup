package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gobup/server/internal/agent"
	appconfig "github.com/gobup/server/internal/config"
	"github.com/gobup/server/internal/database"
)

func TestCheckRecordedFilesCountsVideoFiles(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	defer func() { database.DB = oldDB }()

	dir := t.TempDir()
	video := filepath.Join(dir, "record.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckRecordedFiles([]string{dir}, 1, 10, []string{".mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalFiles != 1 || result.TotalSize != 5 {
		t.Fatalf("unexpected totals: files=%d size=%d", result.TotalFiles, result.TotalSize)
	}
	if len(result.Files) != 1 || result.Files[0].FilePath != video {
		t.Fatalf("unexpected sample files: %#v", result.Files)
	}
	if result.DatabaseAware {
		t.Fatal("file check should not report database awareness before DB init")
	}
}

func TestCheckRecordedFilesFromRequestHonorsOverrides(t *testing.T) {
	oldDB := database.DB
	oldWorkPath := appconfig.AppConfig.WorkPath
	defer func() {
		if database.DB != nil && database.DB != oldDB {
			database.CloseDB()
		}
		database.DB = oldDB
		appconfig.AppConfig.WorkPath = oldWorkPath
	}()

	dir := t.TempDir()
	defaultDir := filepath.Join(dir, "default")
	requestDir := filepath.Join(dir, "request")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(requestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	appconfig.AppConfig.WorkPath = defaultDir
	if err := database.InitDB(filepath.Join(dir, "test.db")); err != nil {
		t.Fatal(err)
	}

	requestVideo := filepath.Join(requestDir, "small.mp4")
	if err := os.WriteFile(requestVideo, []byte("small"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultDir, "default.mp4"), []byte("default"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(requestDir, "other.flv"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	minSize := int64(1)
	result, err := CheckRecordedFilesFromRequest(agent.FileCheckRequest{
		Paths:      []string{requestDir},
		Limit:      10,
		MinSize:    &minSize,
		Extensions: []string{".mp4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalFiles != 1 || result.Files[0].FilePath != requestVideo {
		t.Fatalf("request overrides were not honored: %#v", result)
	}
	if !result.DatabaseAware {
		t.Fatal("file check should report database awareness after DB init")
	}
}
