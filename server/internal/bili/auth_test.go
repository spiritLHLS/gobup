package bili

import "testing"

func TestGetBiliAppCredentialsRequiresEnv(t *testing.T) {
	t.Setenv(biliAppKeyEnv, "")
	t.Setenv(biliAppSecretEnv, "")

	if _, _, err := getBiliAppCredentials(); err == nil {
		t.Fatal("expected missing credentials to fail")
	}
}

func TestSignParamsUsesProvidedSecret(t *testing.T) {
	params := map[string]string{
		"appkey": "test-key",
		"ts":     "0",
	}

	signed := signParams(params, "test-secret")
	if signed["sign"] == "" {
		t.Fatal("expected sign to be set")
	}
	if signed["sign"] != md5Sign("appkey=test-key&ts=0test-secret") {
		t.Fatalf("unexpected sign: %s", signed["sign"])
	}
}
