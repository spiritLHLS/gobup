package upload

import (
	"errors"
	"testing"
	"time"

	"github.com/gobup/server/internal/models"
)

func TestIsWithinUploadWindowOvernight(t *testing.T) {
	room := &models.RecordRoom{
		UploadWindowEnabled: true,
		UploadWindowStart:   "23:00",
		UploadWindowEnd:     "06:00",
	}

	tests := []struct {
		name string
		hour int
		min  int
		want bool
	}{
		{name: "before midnight", hour: 23, min: 30, want: true},
		{name: "after midnight", hour: 5, min: 59, want: true},
		{name: "outside day", hour: 12, min: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 6, 3, tt.hour, tt.min, 0, 0, time.Local)
			got, _ := isWithinUploadWindow(room, now)
			if got != tt.want {
				t.Fatalf("isWithinUploadWindow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeUntilNextUploadWindow(t *testing.T) {
	location := time.FixedZone("test", 8*3600)
	tests := []struct {
		name  string
		room  *models.RecordRoom
		now   time.Time
		want  time.Duration
		allow time.Duration
	}{
		{
			name: "same day before window",
			room: &models.RecordRoom{
				UploadWindowEnabled: true,
				UploadWindowStart:   "09:00",
				UploadWindowEnd:     "18:00",
			},
			now:   time.Date(2026, 6, 3, 8, 30, 0, 0, location),
			want:  30 * time.Minute,
			allow: time.Second,
		},
		{
			name: "next day after daytime window",
			room: &models.RecordRoom{
				UploadWindowEnabled: true,
				UploadWindowStart:   "09:00",
				UploadWindowEnd:     "18:00",
			},
			now:   time.Date(2026, 6, 3, 19, 0, 0, 0, location),
			want:  14 * time.Hour,
			allow: time.Second,
		},
		{
			name: "overnight window during daytime closure",
			room: &models.RecordRoom{
				UploadWindowEnabled: true,
				UploadWindowStart:   "23:00",
				UploadWindowEnd:     "06:00",
			},
			now:   time.Date(2026, 6, 3, 12, 0, 0, 0, location),
			want:  11 * time.Hour,
			allow: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeUntilNextUploadWindow(tt.room, tt.now)
			if got < tt.want-tt.allow || got > tt.want+tt.allow {
				t.Fatalf("timeUntilNextUploadWindow() = %s, want around %s", got, tt.want)
			}
		})
	}
}

func TestValidateUploadVideoPath(t *testing.T) {
	if err := validateUploadVideoPath("/rec/part01.FLV"); err != nil {
		t.Fatalf("expected FLV to be accepted: %v", err)
	}
	if err := validateUploadVideoPath("/rec/not-video.txt"); err == nil {
		t.Fatal("expected txt file to be rejected")
	}
}

func TestClassifyUploadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "window", err: &UploadWindowClosedError{Window: "09:00-18:00", RetryAfter: time.Hour}, want: UploadErrorTypeWindow},
		{name: "rate limit", err: errors.New("HTTP 429 Retry-After: 60"), want: UploadErrorTypeRateLimit},
		{name: "auth", err: errors.New("用户Cookie已失效"), want: UploadErrorTypeAuth},
		{name: "file", err: errors.New("文件不存在: /rec/a.flv"), want: UploadErrorTypeFile},
		{name: "transcode", err: errors.New("上传前转码失败: ffmpeg exited"), want: UploadErrorTypeTranscode},
		{name: "network", err: errors.New("connection reset by peer"), want: UploadErrorTypeNetwork},
		{name: "user", err: errors.New("用户取消上传"), want: UploadErrorTypeUser},
		{name: "unknown", err: errors.New("unexpected response"), want: UploadErrorTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyUploadError(tt.err); got != tt.want {
				t.Fatalf("classifyUploadError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChooseDailyQuotaUser(t *testing.T) {
	tests := []struct {
		name       string
		candidates []dailyQuotaCandidate
		wantID     uint
		wantOK     bool
	}{
		{
			name: "largest remaining quota wins",
			candidates: []dailyQuotaCandidate{
				{ID: 1, Quota: 10, Used: 4, QueueLen: 1},
				{ID: 2, Quota: 10, Used: 2, QueueLen: 1},
			},
			wantID: 2,
			wantOK: true,
		},
		{
			name: "exhausted users are skipped",
			candidates: []dailyQuotaCandidate{
				{ID: 1, Quota: 3, Used: 3, QueueLen: 0},
				{ID: 2, Quota: 3, Used: 1, QueueLen: 1},
			},
			wantID: 2,
			wantOK: true,
		},
		{
			name: "unlimited users prefer shortest queue",
			candidates: []dailyQuotaCandidate{
				{ID: 1, Quota: 0, Used: 20, QueueLen: 5},
				{ID: 2, Quota: 0, Used: 2, QueueLen: 1},
			},
			wantID: 2,
			wantOK: true,
		},
		{
			name: "all limited users exhausted",
			candidates: []dailyQuotaCandidate{
				{ID: 1, Quota: 2, Used: 2, QueueLen: 0},
				{ID: 2, Quota: 3, Used: 2, QueueLen: 1},
			},
			wantID: 0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := chooseDailyQuotaUser(tt.candidates)
			if gotOK != tt.wantOK || gotID != tt.wantID {
				t.Fatalf("chooseDailyQuotaUser() = (%d, %v), want (%d, %v)", gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}
