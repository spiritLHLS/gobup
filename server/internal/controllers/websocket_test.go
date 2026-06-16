package controllers

import (
	"net/http"
	"testing"
)

func TestIsAllowedWebSocketOrigin(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		origin  string
		env     string
		allowed bool
	}{
		{
			name:    "missing origin is allowed for non-browser clients",
			host:    "127.0.0.1:12380",
			allowed: true,
		},
		{
			name:    "same origin is allowed",
			host:    "example.com:12380",
			origin:  "http://example.com:12380",
			allowed: true,
		},
		{
			name:    "explicit allowlist origin is allowed",
			host:    "127.0.0.1:12380",
			origin:  "https://panel.example.com",
			env:     "https://panel.example.com",
			allowed: true,
		},
		{
			name:    "wildcard is ignored",
			host:    "127.0.0.1:12380",
			origin:  "https://evil.example.com",
			env:     "*",
			allowed: false,
		},
		{
			name:    "cross origin is rejected",
			host:    "127.0.0.1:12380",
			origin:  "https://evil.example.com",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOBUP_ALLOWED_ORIGINS", tt.env)
			req := &http.Request{
				Host:   tt.host,
				Header: http.Header{},
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			if got := isAllowedWebSocketOrigin(req); got != tt.allowed {
				t.Fatalf("isAllowedWebSocketOrigin() = %v, want %v", got, tt.allowed)
			}
		})
	}
}
