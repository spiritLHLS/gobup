package controllers

import (
	"strings"
	"testing"

	"github.com/gobup/server/internal/models"
)

func TestClampAgentPriority(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{input: -1, want: 0},
		{input: 0, want: 0},
		{input: 50, want: 50},
		{input: 101, want: 100},
	}

	for _, tt := range tests {
		if got := clampAgentPriority(tt.input); got != tt.want {
			t.Fatalf("clampAgentPriority(%d)=%d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestAgentUploadTargetAvailability(t *testing.T) {
	okNode := &models.AgentNode{
		Enabled:          true,
		Purpose:          models.AgentPurposeUpload,
		LastHealthStatus: "success",
	}
	if ok, reason := agentUploadTargetAvailability(okNode); !ok || reason != "" {
		t.Fatalf("available node=(%v,%q), want available", ok, reason)
	}

	blockedNode := &models.AgentNode{
		Enabled:     true,
		Blocked:     true,
		BlockReason: "maintenance",
		Purpose:     models.AgentPurposeBoth,
	}
	if ok, reason := agentUploadTargetAvailability(blockedNode); ok || !strings.Contains(reason, "maintenance") {
		t.Fatalf("blocked node=(%v,%q), want blocked reason", ok, reason)
	}

	filescanNode := &models.AgentNode{
		Enabled:          true,
		Purpose:          models.AgentPurposeFilescan,
		LastHealthStatus: "success",
	}
	if ok, reason := agentUploadTargetAvailability(filescanNode); ok || !strings.Contains(reason, "用途") {
		t.Fatalf("filescan node=(%v,%q), want purpose rejection", ok, reason)
	}

	uncheckedNode := &models.AgentNode{
		Enabled: true,
		Purpose: models.AgentPurposeBoth,
	}
	if ok, reason := agentUploadTargetAvailability(uncheckedNode); ok || reason == "" {
		t.Fatalf("unchecked node=(%v,%q), want unavailable reason", ok, reason)
	}
}
