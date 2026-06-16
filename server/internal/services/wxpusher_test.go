package services

import "testing"

func TestWxPusherMessageKeyIgnoresDynamicTimeLine(t *testing.T) {
	msgA := PushMessage{
		AppToken: "token",
		Content: `❌ 上传失败
房间: room
文件: video.mp4
原因: HTTP 406
时间: 2026-06-03 10:00:00`,
		ContentType: 1,
		UIDs:        []string{"uid"},
	}
	msgB := msgA
	msgB.Content = `❌ 上传失败
房间: room
文件: video.mp4
原因: HTTP 406
时间: 2026-06-03 10:05:00`

	if wxPusherMessageKey(msgA) != wxPusherMessageKey(msgB) {
		t.Fatal("expected dynamic time line to be ignored for dedupe")
	}
}
