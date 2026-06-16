package controllers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gobup/server/internal/models"
)

func TestSafeBiliUserResponseRedactsSensitiveFields(t *testing.T) {
	now := time.Now()
	user := models.BiliBiliUser{
		ID:               12,
		UID:              345,
		Uname:            "tester",
		Cookies:          "SESSDATA=secret",
		AccessKey:        "access-secret",
		RefreshToken:     "refresh-secret",
		CookieInfo:       "cookie-info-secret",
		WxPushToken:      "wxpusher-secret",
		Login:            true,
		Enabled:          true,
		LoginTime:        &now,
		ExpireTime:       &now,
		DailyUploadQuota: 9,
	}

	resp := safeBiliUserResponse(user)
	if resp.ID != user.ID || resp.UID != user.UID || resp.Uname != user.Uname {
		t.Fatalf("basic identity fields were not preserved: %#v", resp)
	}
	if !resp.HasWxPushToken {
		t.Fatal("expected WxPusher presence flag")
	}
	if resp.DailyUploadQuota != user.DailyUploadQuota {
		t.Fatalf("daily quota = %d, want %d", resp.DailyUploadQuota, user.DailyUploadQuota)
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"SESSDATA=secret", "access-secret", "refresh-secret", "cookie-info-secret", "wxpusher-secret"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("safe response leaked secret %q in %s", secret, payload)
		}
	}
}
