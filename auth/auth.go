package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

type Session struct {
	Token     string
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type AuthManager struct {
	username string
	password string
	sessions map[string]*Session
	mu       sync.RWMutex
	enabled  bool
}

func NewAuthManager(username, password string) *AuthManager {
	return &AuthManager{
		username: username,
		password: password,
		sessions: make(map[string]*Session),
		enabled:  username != "" && password != "",
	}
}

func (am *AuthManager) IsEnabled() bool {
	return am.enabled
}

func (am *AuthManager) Login(username, password string) (string, bool) {
	if !am.enabled {
		return "", false
	}

	if username != am.username || password != am.password {
		return "", false
	}

	// Generate session token
	token := generateToken()

	session := &Session{
		Token:     token,
		Username:  username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	am.mu.Lock()
	am.sessions[token] = session
	am.mu.Unlock()

	return token, true
}

func (am *AuthManager) Logout(token string) {
	am.mu.Lock()
	delete(am.sessions, token)
	am.mu.Unlock()
}

func (am *AuthManager) ValidateSession(token string) bool {
	if !am.enabled {
		return false
	}

	am.mu.RLock()
	session, exists := am.sessions[token]
	am.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(session.ExpiresAt) {
		am.Logout(token)
		return false
	}

	return true
}

func (am *AuthManager) GetSession(token string) *Session {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.sessions[token]
}

func (am *AuthManager) Middleware(next http.HandlerFunc, requireAuth bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If auth is not enabled, proceed but mark as not authenticated
		if !am.enabled {
			r.Header.Set("X-Authenticated", "false")
			next(w, r)
			return
		}

		// Get token from cookie or header
		token := ""
		if cookie, err := r.Cookie("session"); err == nil {
			token = cookie.Value
		}
		if token == "" {
			token = r.Header.Get("Authorization")
		}

		isAuthenticated := am.ValidateSession(token)

		if requireAuth && !isAuthenticated {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if isAuthenticated {
			r.Header.Set("X-Authenticated", "true")
		} else {
			r.Header.Set("X-Authenticated", "false")
		}

		next(w, r)
	}
}

func generateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CleanupExpiredSessions removes expired sessions
func (am *AuthManager) CleanupExpiredSessions() {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	for token, session := range am.sessions {
		if now.After(session.ExpiresAt) {
			delete(am.sessions, token)
		}
	}
}

// StartCleanupRoutine starts a goroutine that periodically cleans up expired sessions
func (am *AuthManager) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			am.CleanupExpiredSessions()
		}
	}()
}
