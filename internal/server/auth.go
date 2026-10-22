package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ipLockoutRecord struct {
	failedCount int
	lockedUntil time.Time
}

// AuthManager manages authentication tokens and remote access verification.
type AuthManager struct {
	tokens   map[string]time.Time
	lockouts map[string]*ipLockoutRecord
	mu       sync.RWMutex
}

func NewAuthManager() *AuthManager {
	return &AuthManager{
		tokens:   make(map[string]time.Time),
		lockouts: make(map[string]*ipLockoutRecord),
	}
}

// purgeExpiredLocked cleans up stale tokens and expired lockouts to prevent memory leaks.
// Caller must hold a.mu.Lock().
func (a *AuthManager) purgeExpiredLocked() {
	now := time.Now()
	for t, exp := range a.tokens {
		if now.After(exp) {
			delete(a.tokens, t)
		}
	}
	for ip, rec := range a.lockouts {
		if !rec.lockedUntil.IsZero() && now.After(rec.lockedUntil) {
			delete(a.lockouts, ip)
		}
	}
}

func (a *AuthManager) GenerateToken() string {
	bytes := make([]byte, 24)
	_, _ = rand.Read(bytes)
	token := hex.EncodeToString(bytes)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLocked()
	a.tokens[token] = time.Now().Add(24 * time.Hour)
	return token
}

func (a *AuthManager) ValidateToken(token string) bool {
	if token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	exp, ok := a.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(a.tokens, token)
		return false
	}
	if len(a.tokens) > 50 {
		a.purgeExpiredLocked()
	}
	return true
}

// IsLocked checks if a client IP is currently quarantined by the Remote Lockout Policy.
func (a *AuthManager) IsLocked(ip string) (bool, time.Duration) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	rec, ok := a.lockouts[ip]
	if !ok || rec.lockedUntil.IsZero() {
		return false, 0
	}
	remaining := time.Until(rec.lockedUntil)
	if remaining > 0 {
		return true, remaining
	}
	return false, 0
}

// RecordFailedAttempt registers a failed authentication attempt and triggers a 10-minute lockout on the 5th failure.
func (a *AuthManager) RecordFailedAttempt(ip string) (locked bool, remaining time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLocked()
	rec, ok := a.lockouts[ip]
	if !ok || (!rec.lockedUntil.IsZero() && time.Now().After(rec.lockedUntil)) {
		rec = &ipLockoutRecord{}
		a.lockouts[ip] = rec
	}
	rec.failedCount++
	if rec.failedCount >= 5 {
		rec.lockedUntil = time.Now().Add(10 * time.Minute)
		return true, 10 * time.Minute
	}
	return false, 0
}

// RecordSuccessfulAttempt clears any recorded failures for a successfully authenticated IP.
func (a *AuthManager) RecordSuccessfulAttempt(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.lockouts, ip)
}

func (s *Server) isHostAllowed(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]" || host == "example.com" {
		return true
	}
	for _, ah := range s.allowedHosts {
		if strings.EqualFold(host, ah) {
			return true
		}
	}
	if s.lanEnabled {
		if ip := net.ParseIP(host); ip != nil {
			return true
		}
	}
	return false
}

// IngressGuardMiddleware enforces DNS Rebinding and Cross-Origin CSRF protections.
func (s *Server) IngressGuardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. DNS Rebinding Guard: Verify Host header
		if !s.isHostAllowed(r.Host) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "Invalid Host header (DNS Rebinding protection)",
			})
			return
		}

		// 2. CSRF Guard: State-mutating API requests
		if strings.HasPrefix(r.URL.Path, "/api") {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
				if sfs := r.Header.Get("Sec-Fetch-Site"); sfs == "cross-site" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"error": "Cross-site request blocked (CSRF protection)",
					})
					return
				}

				if origin := r.Header.Get("Origin"); origin != "" {
					if u, err := url.Parse(origin); err == nil {
						if !s.isHostAllowed(u.Host) {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusForbidden)
							_ = json.NewEncoder(w).Encode(map[string]any{
								"error": "Unauthorized Origin (CSRF protection)",
							})
							return
						}
					}
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost"
	}
	return ip.IsLoopback()
}

// RemoteAccessGuardMiddleware intercepts requests from non-localhost IPs.
func (s *Server) RemoteAccessGuardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets and auth endpoints are always accessible
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api") || path == "/api/auth/status" || path == "/api/auth/unlock" {
			next.ServeHTTP(w, r)
			return
		}

		// Loopback (localhost) is granted immediate access
		if isLoopback(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}

		// Remote IP check: requires valid token
		token := extractToken(r)
		if !s.authMgr.ValidateToken(token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"locked":  true,
				"message": "Remote access requires Master Password verification.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	if cookie, err := r.Cookie("sqlfox_token"); err == nil {
		return cookie.Value
	}
	return r.URL.Query().Get("token")
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	isRemote := !isLoopback(r.RemoteAddr)
	unlocked := false

	if !isRemote {
		unlocked = true
	} else {
		token := extractToken(r)
		unlocked = s.authMgr.ValidateToken(token)
	}

	w.Header().Set("X-SqlFox-App", "sqlfox")
	writeJSON(w, http.StatusOK, map[string]any{
		"app":       "sqlfox",
		"version":   s.version,
		"is_remote": isRemote,
		"unlocked":  unlocked,
	})
}

func (s *Server) handleAuthUnlock(w http.ResponseWriter, r *http.Request) {
	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}

	if locked, remaining := s.authMgr.IsLocked(clientIP); locked {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("Too many failed attempts. Locked out for %d minutes.", int(remaining.Minutes())+1),
		})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if !s.vault.ValidateMasterPassword(req.Password) {
		locked, remaining := s.authMgr.RecordFailedAttempt(clientIP)
		if locked {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": fmt.Sprintf("Too many failed attempts. Locked out for %d minutes.", int(remaining.Minutes())+1),
			})
			return
		}
		writeError(w, http.StatusUnauthorized, "Incorrect master password.")
		return
	}

	s.authMgr.RecordSuccessfulAttempt(clientIP)

	token := s.authMgr.GenerateToken()

	http.SetCookie(w, &http.Cookie{
		Name:     "sqlfox_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   token,
		"message": "Vault unlocked successfully.",
	})
}

func (s *Server) handleGetMasterPassword(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		writeError(w, http.StatusForbidden, "Master password inspection is only allowed from localhost.")
		return
	}

	if !s.lanEnabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"lan_enabled": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"lan_enabled":     true,
		"master_password": s.masterPassword,
	})
}
