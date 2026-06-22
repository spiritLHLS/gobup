package models

import (
	"testing"
	"time"
)

func TestLiveSessionDayRangeUsesRecordingLocalDay(t *testing.T) {
	location := time.FixedZone("test", 8*3600)
	start := time.Date(2026, 6, 21, 23, 59, 0, 0, location)

	dayStart, dayEnd, ok := LiveSessionDayRange(start)
	if !ok {
		t.Fatal("expected valid day range")
	}
	if want := time.Date(2026, 6, 21, 0, 0, 0, 0, location); !dayStart.Equal(want) {
		t.Fatalf("dayStart=%s, want %s", dayStart, want)
	}
	if want := time.Date(2026, 6, 22, 0, 0, 0, 0, location); !dayEnd.Equal(want) {
		t.Fatalf("dayEnd=%s, want %s", dayEnd, want)
	}
	if key := LiveSessionDayKey(start); key != "2026-06-21" {
		t.Fatalf("day key=%q, want 2026-06-21", key)
	}
}

func TestLiveSessionDayRangeRejectsZeroTime(t *testing.T) {
	if _, _, ok := LiveSessionDayRange(time.Time{}); ok {
		t.Fatal("zero time should not produce a day range")
	}
	if key := LiveSessionDayKey(time.Time{}); key != "" {
		t.Fatalf("zero time key=%q, want empty", key)
	}
}
