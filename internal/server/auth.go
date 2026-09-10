package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthManager manages authentication tokens and remote access verification.
type AuthManager struct {
	tokens map[string]time.Time
	mu     sync.RWMutex
}

func NewAuthManager() *AuthManager {
	return &AuthManager{
		tokens: make(map[string]time.Time),
	}
}

func (a *AuthManager) GenerateToken() string {
	bytes := make([]byte, 24)
	_, _ = rand.Read(bytes)
	token := hex.EncodeToString(bytes)

	a.mu.Lock()
	defer a.mu.Unlock()
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
	return true
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

	writeJSON(w, http.StatusOK, map[string]any{
		"is_remote": isRemote,
		"unlocked":  unlocked,
	})
}

func (s *Server) handleAuthUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if !s.vault.ValidateMasterPassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "Incorrect master password.")
		return
	}

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

	writeJSON(w, http.StatusOK, map[string]string{
		"master_password": s.masterPassword,
	})
}
