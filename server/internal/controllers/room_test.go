package controllers

import (
	"testing"

	"github.com/gobup/server/internal/models"
)

func TestNormalizeRoomConfigDefaultsTranscodeCodec(t *testing.T) {
	room := models.RecordRoom{
		TranscodeVideoCodec:   "x265",
		TranscodePreset:       "veryfast",
		TranscodeCRF:          23,
		TranscodeAudioBitrate: "160k",
	}

	normalizeRoomConfig(&room)

	if room.TranscodeVideoCodec != models.TranscodeVideoCodecH265 {
		t.Fatalf("TranscodeVideoCodec = %q, want h265", room.TranscodeVideoCodec)
	}

	room.TranscodeVideoCodec = "vp9"
	normalizeRoomConfig(&room)

	if room.TranscodeVideoCodec != models.TranscodeVideoCodecH264 {
		t.Fatalf("TranscodeVideoCodec = %q, want h264", room.TranscodeVideoCodec)
	}
}

func TestNormalizeRoomConfigDanmakuBurnOptions(t *testing.T) {
	room := models.RecordRoom{
		DanmakuBurnStyle:   "invalid",
		DanmakuFontSize:    100,
		DanmakuFontColor:   "  #3366cc  ",
		DanmakuScrollArea:  2,
		DanmakuDisplayArea: -1,
	}

	normalizeRoomConfig(&room)

	if room.DanmakuBurnStyle != "default" {
		t.Fatalf("DanmakuBurnStyle = %q, want default", room.DanmakuBurnStyle)
	}
	if room.DanmakuFontSize != 72 {
		t.Fatalf("DanmakuFontSize = %d, want 72", room.DanmakuFontSize)
	}
	if room.DanmakuFontColor != "#3366cc" {
		t.Fatalf("DanmakuFontColor = %q, want trimmed value", room.DanmakuFontColor)
	}
	if room.DanmakuScrollArea != 1 {
		t.Fatalf("DanmakuScrollArea = %v, want 1", room.DanmakuScrollArea)
	}
	if room.DanmakuDisplayArea != 0.8 {
		t.Fatalf("DanmakuDisplayArea = %v, want 0.8", room.DanmakuDisplayArea)
	}
}
