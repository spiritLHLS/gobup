package services

import (
	"strings"
	"testing"

	"github.com/gobup/server/internal/models"
)

func TestParseFFmpegProgressPercent(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		durationMs int64
		want       int
		wantOK     bool
	}{
		{
			name:       "out_time_ms microseconds",
			line:       "out_time_ms=5000000",
			durationMs: 10000,
			want:       50,
			wantOK:     true,
		},
		{
			name:       "out_time_us microseconds",
			line:       "out_time_us=2500000",
			durationMs: 10000,
			want:       25,
			wantOK:     true,
		},
		{
			name:       "out_time clock",
			line:       "out_time=00:00:07.500000",
			durationMs: 10000,
			want:       75,
			wantOK:     true,
		},
		{
			name:       "clamped to 100",
			line:       "out_time=00:00:12.000000",
			durationMs: 10000,
			want:       100,
			wantOK:     true,
		},
		{
			name:       "ignored progress status line",
			line:       "progress=continue",
			durationMs: 10000,
			wantOK:     false,
		},
		{
			name:   "unknown duration",
			line:   "out_time=00:00:05.000000",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseFFmpegProgressPercent(tt.line, tt.durationMs)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("percent = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseFFmpegClockMs(t *testing.T) {
	got, ok := parseFFmpegClockMs("01:02:03.500000")
	if !ok {
		t.Fatal("expected parse success")
	}
	want := int64((1*3600+2*60+3)*1000 + 500)
	if got != want {
		t.Fatalf("parseFFmpegClockMs() = %d, want %d", got, want)
	}
}

func TestResolveTranscodeSettingsDefaultsToH264(t *testing.T) {
	settings := resolveTranscodeSettings(&models.RecordRoom{
		TranscodePreset:       "unknown",
		TranscodeCRF:          8,
		TranscodeAudioBitrate: " ",
		TranscodeVideoCodec:   "vp9",
	})

	if settings.VideoCodec != models.TranscodeVideoCodecH264 {
		t.Fatalf("VideoCodec = %q, want h264", settings.VideoCodec)
	}
	if settings.VideoEncoder != "libx264" {
		t.Fatalf("VideoEncoder = %q, want libx264", settings.VideoEncoder)
	}
	if settings.Preset != "veryfast" {
		t.Fatalf("Preset = %q, want veryfast", settings.Preset)
	}
	if settings.CRF != 23 {
		t.Fatalf("CRF = %d, want 23", settings.CRF)
	}
	if settings.AudioBitrate != "160k" {
		t.Fatalf("AudioBitrate = %q, want 160k", settings.AudioBitrate)
	}
}

func TestBuildTranscodeArgsSupportsH265MP4(t *testing.T) {
	settings := resolveTranscodeSettings(&models.RecordRoom{
		TranscodePreset:       "slow",
		TranscodeCRF:          28,
		TranscodeMaxWidth:     1920,
		TranscodeAudioBitrate: "128k",
		TranscodeVideoCodec:   "hevc",
	})
	args := buildTranscodeArgs("/tmp/input.flv", "/tmp/output.mp4", settings)
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"-c:v libx265",
		"-tag:v hvc1",
		"-preset slow",
		"-crf 28",
		"-vf scale=w='min(1920,iw)':h=-2",
		"-b:a 128k",
		"/tmp/output.mp4",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}
