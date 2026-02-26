package auth

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// HashPassword generates md5 hash of "syspeek_" + password
func HashPassword(password string) string {
	hash := md5.Sum([]byte("syspeek_" + password))
	return hex.EncodeToString(hash[:])
}

type Session struct {
	Token     string
	Username  string
	ReadWrite bool // true = can perform actions, false = read-only
	CreatedAt time.Time
	ExpiresAt time.Time
}

type AuthManager struct {
	// Read-write user (admin)
	username string
	password string
	// Read-only user
	readOnlyUsername string
	readOnlyPassword string
	// Sessions
	sessions map[string]*Session
	mu       sync.RWMutex
	// Flags
	hasReadWrite bool // Has read-write credentials configured
	hasReadOnly  bool // Has read-only credentials configured
	isPublic     bool // Public read-only access (no login required for viewing)
	isAdmin      bool // Full admin access without authentication
}

func NewAuthManager(username, password, readOnlyUsername, readOnlyPassword string, isPublic, isAdmin bool) *AuthManager {
	return &AuthManager{
		username:         username,
		password:         password,
		readOnlyUsername: readOnlyUsername,
		readOnlyPassword: readOnlyPassword,
		sessions:         make(map[string]*Session),
		hasReadWrite:     username != "" && password != "",
		hasReadOnly:      readOnlyUsername != "" && readOnlyPassword != "",
		isPublic:         isPublic,
		isAdmin:          isAdmin,
	}
}

// IsEnabled returns true if any form of authentication is configured
func (am *AuthManager) IsEnabled() bool {
	return am.hasReadWrite || am.hasReadOnly
}

// RequiresLoginForReadOnly returns true if login is required to view the app
func (am *AuthManager) RequiresLoginForReadOnly() bool {
	// If public mode, read-only doesn't require login
	if am.isPublic {
		return false
	}
	// If not public, requires login if any auth is configured
	return am.IsEnabled()
}

// HasReadWriteAuth returns true if read-write credentials are configured
func (am *AuthManager) HasReadWriteAuth() bool {
	return am.hasReadWrite
}

// HasReadOnlyAuth returns true if read-only credentials are configured
func (am *AuthManager) HasReadOnlyAuth() bool {
	return am.hasReadOnly
}

// IsPublic returns true if public read-only access is enabled
func (am *AuthManager) IsPublic() bool {
	return am.isPublic
}

// IsAdminMode returns true if full admin access is enabled without authentication
func (am *AuthManager) IsAdminMode() bool {
	return am.isAdmin
}

// Login attempts to authenticate and returns (token, readWrite, success)
// The password parameter is the plain-text password from the user.
// It gets hashed and compared against the stored hash in config.
func (am *AuthManager) Login(username, password string) (string, bool, bool) {
	hashedPassword := HashPassword(password)

	// Try read-write credentials first
	if am.hasReadWrite && username == am.username && hashedPassword == am.password {
		token := generateToken()
		session := &Session{
			Token:     token,
			Username:  username,
			ReadWrite: true,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		am.mu.Lock()
		am.sessions[token] = session
		am.mu.Unlock()
		return token, true, true
	}

	// Try read-only credentials
	if am.hasReadOnly && username == am.readOnlyUsername && hashedPassword == am.readOnlyPassword {
		token := generateToken()
		session := &Session{
			Token:     token,
			Username:  username,
			ReadWrite: false,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		am.mu.Lock()
		am.sessions[token] = session
		am.mu.Unlock()
		return token, false, true
	}

	return "", false, false
}

func (am *AuthManager) Logout(token string) {
	am.mu.Lock()
	delete(am.sessions, token)
	am.mu.Unlock()
}

func (am *AuthManager) ValidateSession(token string) bool {
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

// IsReadWrite checks if the token has read-write permissions
func (am *AuthManager) IsReadWrite(token string) bool {
	session := am.GetSession(token)
	if session == nil {
		return false
	}
	return session.ReadWrite
}

// Middleware handles authentication for routes
// requireAuth: if true, requires authenticated session
// requireReadWrite: if true, requires read-write session (only matters if requireAuth is true)
func (am *AuthManager) Middleware(next http.HandlerFunc, requireAuth bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Admin mode: allow everything without authentication
		if am.isAdmin {
			r.Header.Set("X-Authenticated", "true")
			r.Header.Set("X-ReadWrite", "true")
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
		isReadWrite := am.IsReadWrite(token)

		// Set headers for downstream handlers
		if isAuthenticated {
			r.Header.Set("X-Authenticated", "true")
			if isReadWrite {
				r.Header.Set("X-ReadWrite", "true")
			} else {
				r.Header.Set("X-ReadWrite", "false")
			}
		} else {
			r.Header.Set("X-Authenticated", "false")
			r.Header.Set("X-ReadWrite", "false")
		}

		// If requireAuth is true, check authentication
		if requireAuth {
			if !isAuthenticated {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		} else {
			// For non-auth-required routes, still need to check if login is required
			if am.RequiresLoginForReadOnly() && !isAuthenticated {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		next(w, r)
	}
}

// MiddlewareReadWrite is a convenience wrapper that requires read-write access
func (am *AuthManager) MiddlewareReadWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Admin mode: allow everything without authentication
		if am.isAdmin {
			r.Header.Set("X-Authenticated", "true")
			r.Header.Set("X-ReadWrite", "true")
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

		if !am.ValidateSession(token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if !am.IsReadWrite(token) {
			http.Error(w, "Forbidden: Read-write access required", http.StatusForbidden)
			return
		}

		r.Header.Set("X-Authenticated", "true")
		r.Header.Set("X-ReadWrite", "true")
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
