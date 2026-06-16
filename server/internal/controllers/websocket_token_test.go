package controllers

import (
	"testing"
	"time"
)

func TestWebSocketTokenValidation(t *testing.T) {
	websocketTokenStore.Lock()
	websocketTokenStore.tokens = make(map[string]time.Time)
	websocketTokenStore.Unlock()

	token, err := newWebSocketToken()
	if err != nil {
		t.Fatalf("newWebSocketToken() error = %v", err)
	}
	if validateWebSocketToken(token) {
		t.Fatal("unstored token should be invalid")
	}

	storeWebSocketToken(token)
	if !validateWebSocketToken(token) {
		t.Fatal("stored token should be valid")
	}

	websocketTokenStore.Lock()
	websocketTokenStore.tokens[token] = time.Now().Add(-time.Second)
	websocketTokenStore.Unlock()

	if validateWebSocketToken(token) {
		t.Fatal("expired token should be invalid")
	}
}
