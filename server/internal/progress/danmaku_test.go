package progress

import "testing"

func TestDanmakuTaskProgress(t *testing.T) {
	danmakuProgressMu.Lock()
	danmakuProgressMap = make(map[int64]*DanmakuProgress)
	danmakuProgressMu.Unlock()

	SetDanmakuTaskProgress(7, 2, 5, true, false, false, "burn_encode", "烧录弹幕到视频")

	got := GetDanmakuProgress(7)
	if got.Stage != "burn_encode" || got.Message != "烧录弹幕到视频" {
		t.Fatalf("unexpected stage progress: %+v", got)
	}
	if !got.Sending || got.Completed || got.Failed {
		t.Fatalf("unexpected flags: %+v", got)
	}
	if got.UpdateAtMs <= 0 {
		t.Fatalf("expected update timestamp: %+v", got)
	}

	got.Stage = "mutated"
	fresh := GetDanmakuProgress(7)
	if fresh.Stage != "burn_encode" {
		t.Fatal("GetDanmakuProgress should return a copy")
	}
}

func TestSetDanmakuProgressKeepsLegacySendingStage(t *testing.T) {
	danmakuProgressMu.Lock()
	danmakuProgressMap = make(map[int64]*DanmakuProgress)
	danmakuProgressMu.Unlock()

	SetDanmakuProgress(8, 3, 10, true, false)
	got := GetDanmakuProgress(8)
	if got.Stage != "sending" {
		t.Fatalf("stage = %q, want sending", got.Stage)
	}
	if got.Current != 3 || got.Total != 10 || !got.Sending {
		t.Fatalf("unexpected legacy progress: %+v", got)
	}
}
