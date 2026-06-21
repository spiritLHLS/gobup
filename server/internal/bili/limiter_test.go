package bili

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterDelaySeconds(t *testing.T) {
	delay, ok := parseRetryAfterDelay("15", time.Now())
	if !ok {
		t.Fatal("expected numeric Retry-After header to parse")
	}
	if delay != 15*time.Second {
		t.Fatalf("expected 15s, got %v", delay)
	}
}

func TestRateLimitRetryConfigStartsAtOneMinute(t *testing.T) {
	if RateLimitRetryConfig.InitialDelay != time.Minute {
		t.Fatalf("rate-limit initial delay = %s, want 1m", RateLimitRetryConfig.InitialDelay)
	}
}

func TestParseRetryAfterDelayHTTPDate(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	header := now.Add(45 * time.Second).Format(http.TimeFormat)
	delay, ok := parseRetryAfterDelay(header, now)
	if !ok {
		t.Fatal("expected HTTP date Retry-After header to parse")
	}
	if delay != 45*time.Second {
		t.Fatalf("expected 45s, got %v", delay)
	}
}

func TestParseRetryAfterDelayRejectsInvalidValues(t *testing.T) {
	for _, header := range []string{"", "abc", "-1"} {
		if delay, ok := parseRetryAfterDelay(header, time.Now()); ok {
			t.Fatalf("expected %q to be rejected, got %v", header, delay)
		}
	}
}

func TestWrapRetryAfterErrorExposesRetryDelay(t *testing.T) {
	err := wrapRetryAfterError(errors.New("HTTP 429"), "7", time.Now())
	var retryErr interface {
		RetryDelay() time.Duration
	}
	if !errors.As(err, &retryErr) {
		t.Fatal("expected wrapped error to expose RetryDelay")
	}
	if retryErr.RetryDelay() != 7*time.Second {
		t.Fatalf("expected 7s retry delay, got %v", retryErr.RetryDelay())
	}
}

func TestUploadRequestTimeoutUsesEnvironment(t *testing.T) {
	t.Setenv("GOBUP_UPLOAD_TIMEOUT_MINUTES", "45")
	if got := uploadRequestTimeout(); got != 45*time.Minute {
		t.Fatalf("expected 45m timeout, got %v", got)
	}
}

func TestUploadRequestTimeoutCapsExcessiveValues(t *testing.T) {
	t.Setenv("GOBUP_UPLOAD_TIMEOUT_MINUTES", "999999")
	if got := uploadRequestTimeout(); got != 24*time.Hour {
		t.Fatalf("expected timeout cap at 24h, got %v", got)
	}
}

func TestTLSHandshakeTimeoutFallback(t *testing.T) {
	t.Setenv("GOBUP_TLS_HANDSHAKE_TIMEOUT_SECONDS", "bad")
	if got := tlsHandshakeTimeout(); got != 30*time.Second {
		t.Fatalf("expected fallback TLS timeout, got %v", got)
	}
}
