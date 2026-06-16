package upload

import (
	"strings"
	"testing"
	"time"
)

func TestProgressTrackerTranscodingState(t *testing.T) {
	tracker := NewProgressTracker()
	tracker.MarkTranscoding(1, 10, 2, 42, "转码中 42%")

	progress := tracker.GetByPartID(1)
	if progress == nil {
		t.Fatal("expected progress entry")
	}
	if progress.State != StateTranscoding {
		t.Fatalf("state = %s, want %s", progress.State, StateTranscoding)
	}
	if !progress.IsActive() {
		t.Fatal("transcoding progress should be active")
	}
	if progress.Percent != 42 || progress.ChunkDone != 42 || progress.ChunkTotal != 100 {
		t.Fatalf("unexpected transcode progress: %+v", progress)
	}

	tracker.Start(1, 10, 2, 8)
	progress = tracker.GetByPartID(1)
	if progress.State != StateUploading {
		t.Fatalf("state = %s, want %s", progress.State, StateUploading)
	}
	if progress.Percent != 0 || progress.ChunkDone != 0 || progress.ChunkTotal != 8 {
		t.Fatalf("upload start should reset chunk progress: %+v", progress)
	}
}

func TestProgressTrackerRetryWaitState(t *testing.T) {
	tracker := NewProgressTracker()
	tracker.Start(2, 20, 1, 10)
	tracker.UpdateChunkDone(2, 20, 1, 4, 10)
	tracker.MarkRetryWait(2, "等待 5秒 后重试")

	progress := tracker.GetByPartID(2)
	if progress == nil {
		t.Fatal("expected progress entry")
	}
	if progress.State != StateRetryWait {
		t.Fatalf("state = %s, want %s", progress.State, StateRetryWait)
	}
	if !progress.IsActive() {
		t.Fatal("retry-wait progress should remain active")
	}
	if progress.StateMsg != "等待 5秒 后重试" {
		t.Fatalf("StateMsg = %q", progress.StateMsg)
	}
	if progress.ChunkDone != 4 || progress.ChunkTotal != 10 {
		t.Fatalf("retry wait should preserve chunk progress: %+v", progress)
	}
}

func TestFormatUploadRetryWaitMessage(t *testing.T) {
	got := formatUploadRetryWaitMessage(2, 5, 5*time.Second, 3, 10)
	for _, want := range []string{"等待 1分钟 后重试", "(2/5)", "已完成分片 3/10"} {
		if !strings.Contains(got, want) {
			t.Fatalf("message %q missing %q", got, want)
		}
	}
}
