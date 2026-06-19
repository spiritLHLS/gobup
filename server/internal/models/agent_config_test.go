package models

import "testing"

func TestNormalizeAgentPurpose(t *testing.T) {
	tests := map[string]string{
		"":          AgentPurposeBoth,
		"upload":    AgentPurposeUpload,
		"publish":   AgentPurposeUpload,
		"filescan":  AgentPurposeFilescan,
		"filecheck": AgentPurposeFilescan,
		"both":      AgentPurposeBoth,
		"bad":       AgentPurposeBoth,
	}
	for input, expected := range tests {
		if actual := NormalizeAgentPurpose(input); actual != expected {
			t.Fatalf("NormalizeAgentPurpose(%q)=%q, want %q", input, actual, expected)
		}
	}
}

func TestAgentPurposeAllows(t *testing.T) {
	if !AgentPurposeAllows(AgentPurposeBoth, AgentPurposeUpload) || !AgentPurposeAllows(AgentPurposeBoth, AgentPurposeFilescan) {
		t.Fatal("both purpose should allow upload and filescan")
	}
	if AgentPurposeAllows(AgentPurposeFilescan, AgentPurposeUpload) {
		t.Fatal("filescan purpose must not allow upload")
	}
}
