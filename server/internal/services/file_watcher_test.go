package services

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestIsFileWatcherVideoChangeEvent(t *testing.T) {
	scanSvc := NewFileScanService()
	tests := []struct {
		name  string
		event fsnotify.Event
		want  bool
	}{
		{
			name: "create mp4",
			event: fsnotify.Event{
				Name: "/rec/room/video.mp4",
				Op:   fsnotify.Create,
			},
			want: true,
		},
		{
			name: "write uppercase flv",
			event: fsnotify.Event{
				Name: "/rec/room/video.FLV",
				Op:   fsnotify.Write,
			},
			want: true,
		},
		{
			name: "rename mkv",
			event: fsnotify.Event{
				Name: "/rec/room/video.mkv",
				Op:   fsnotify.Rename,
			},
			want: true,
		},
		{
			name: "remove video ignored",
			event: fsnotify.Event{
				Name: "/rec/room/video.mp4",
				Op:   fsnotify.Remove,
			},
			want: false,
		},
		{
			name: "non video ignored",
			event: fsnotify.Event{
				Name: "/rec/room/comment.xml",
				Op:   fsnotify.Write,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFileWatcherVideoChangeEvent(scanSvc, tt.event); got != tt.want {
				t.Fatalf("isFileWatcherVideoChangeEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}
