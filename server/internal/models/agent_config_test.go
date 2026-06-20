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

func TestNormalizeAgentEndpoint(t *testing.T) {
	tests := map[string]string{
		"":                                     "",
		"192.0.2.10":                           "http://192.0.2.10:12381",
		"agent.example.com":                    "http://agent.example.com:12381",
		"http://agent.example.com":             "http://agent.example.com:12381",
		"https://agent.example.com:9443/path/": "https://agent.example.com:9443/path",
	}
	for input, expected := range tests {
		if actual := NormalizeAgentEndpoint(input); actual != expected {
			t.Fatalf("NormalizeAgentEndpoint(%q)=%q, want %q", input, actual, expected)
		}
	}
}

func TestNewAgentToken(t *testing.T) {
	token := NewAgentToken()
	if len(token) < 24 {
		t.Fatalf("NewAgentToken() length=%d, want at least 24", len(token))
	}
	if token == NewAgentToken() {
		t.Fatal("NewAgentToken() returned duplicate consecutive values")
	}
}
