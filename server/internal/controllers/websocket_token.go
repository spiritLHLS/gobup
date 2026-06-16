package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const websocketTokenTTL = 5 * time.Minute

var websocketTokenStore = struct {
	sync.Mutex
	tokens map[string]time.Time
}{
	tokens: make(map[string]time.Time),
}

// IssueWebSocketToken 签发短期 WebSocket token。
//
// @Summary Issue WebSocket progress token
// @Description Issues a short-lived token for connecting to /ws/progress after BasicAuth succeeds.
// @Tags progress
// @Security BasicAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /progress/ws-token [get]
func IssueWebSocketToken(c *gin.Context) {
	token, err := newWebSocketToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成WebSocket token失败"})
		return
	}

	storeWebSocketToken(token)
	c.JSON(http.StatusOK, gin.H{
		"token":     token,
		"expiresIn": int(websocketTokenTTL.Seconds()),
	})
}

func newWebSocketToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func storeWebSocketToken(token string) {
	websocketTokenStore.Lock()
	defer websocketTokenStore.Unlock()

	now := time.Now()
	cleanupExpiredWebSocketTokensLocked(now)
	websocketTokenStore.tokens[token] = now.Add(websocketTokenTTL)
}

func validateWebSocketToken(token string) bool {
	if token == "" {
		return false
	}

	websocketTokenStore.Lock()
	defer websocketTokenStore.Unlock()

	now := time.Now()
	cleanupExpiredWebSocketTokensLocked(now)
	expiresAt, ok := websocketTokenStore.tokens[token]
	if !ok || now.After(expiresAt) {
		delete(websocketTokenStore.tokens, token)
		return false
	}
	return true
}

func cleanupExpiredWebSocketTokensLocked(now time.Time) {
	for token, expiresAt := range websocketTokenStore.tokens {
		if now.After(expiresAt) {
			delete(websocketTokenStore.tokens, token)
		}
	}
}
