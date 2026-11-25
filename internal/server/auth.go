package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// allowDefaultTestHost permits Go's httptest default Host ("example.com") through the DNS Rebinding
// guard. It is false in production builds and is flipped on only by the test harness (auth_test.go's
// init), so the production ingress guard never links the testing package or accepts example.com.
var allowDefaultTestHost = false

type ipLockoutRecord struct {
	failedCount int
	lastAttempt time.Time
	lockedUntil time.Time
}

// AuthManager manages authentication tokens, ephemeral LAN pairing codes, and remote access verification.
type AuthManager struct {
	tokens             map[string]time.Time
	lockouts           map[string]*ipLockoutRecord
	pairingCode        string
	pairingExpiresAt   time.Time
	pairingFailedCount int
	argon2Sem          chan struct{}
	mu                 sync.RWMutex
}

func NewAuthManager() *AuthManager {
	return &AuthManager{
		tokens:    make(map[string]time.Time),
		lockouts:  make(map[string]*ipLockoutRecord),
		argon2Sem: make(chan struct{}, 4),
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
			continue
		}
		// Evict non-locked entries after 15 minutes of inactivity to prevent unbounded RAM growth.
		if rec.lockedUntil.IsZero() && !rec.lastAttempt.IsZero() && now.Sub(rec.lastAttempt) > 15*time.Minute {
			delete(a.lockouts, ip)
		}
	}
}

const (
	maxActiveTokens  = 1000
	maxLockoutRecord = 10000
)

func (a *AuthManager) GenerateToken() string {
	bytes := make([]byte, 24)
	_, _ = rand.Read(bytes)
	token := hex.EncodeToString(bytes)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLocked()
	if len(a.tokens) >= maxActiveTokens {
		// Evict oldest token to bound memory growth under high login churn.
		var oldestKey string
		var oldestExp time.Time
		for k, exp := range a.tokens {
			if oldestKey == "" || exp.Before(oldestExp) {
				oldestKey = k
				oldestExp = exp
			}
		}
		if oldestKey != "" {
			delete(a.tokens, oldestKey)
		}
	}
	a.tokens[token] = time.Now().Add(24 * time.Hour)
	return token
}

func (a *AuthManager) ValidateToken(token string) bool {
	if token == "" {
		return false
	}
	a.mu.RLock()
	exp, ok := a.tokens[token]
	a.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		a.mu.Lock()
		if curExp, stillThere := a.tokens[token]; stillThere && time.Now().After(curExp) {
			delete(a.tokens, token)
		}
		a.mu.Unlock()
		return false
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
	now := time.Now()
	rec, ok := a.lockouts[ip]
	if !ok || (!rec.lockedUntil.IsZero() && now.After(rec.lockedUntil)) {
		if !ok && len(a.lockouts) >= maxLockoutRecord {
			// Bound lockout table size under distributed spoofed-IP brute force.
			for k, v := range a.lockouts {
				if v.lockedUntil.IsZero() {
					delete(a.lockouts, k)
					if len(a.lockouts) < maxLockoutRecord {
						break
					}
				}
			}
		}
		rec = &ipLockoutRecord{}
		a.lockouts[ip] = rec
	}
	rec.lastAttempt = now
	rec.failedCount++
	if rec.failedCount >= 5 {
		rec.lockedUntil = now.Add(10 * time.Minute)
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

const pairingCodeTTL = 5 * time.Minute

// GeneratePairingCode issues a short-lived, single-use 8-digit LAN pairing code so the
// vault's master password is never exposed over HTTP.
func (a *AuthManager) GeneratePairingCode() (string, time.Time) {
	var b strings.Builder
	for i := 0; i < 8; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			panic("crypto/rand unavailable for pairing code generation: " + err.Error())
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	code := b.String()
	exp := time.Now().Add(pairingCodeTTL)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.pairingCode = code
	a.pairingExpiresAt = exp
	a.pairingFailedCount = 0
	return code, exp
}

// GetOrCreatePairingCode returns the active non-expired pairing code without mutating it,
// or generates an initial code if none exists or the previous code has expired.
func (a *AuthManager) GetOrCreatePairingCode() (string, time.Time) {
	a.mu.RLock()
	if a.pairingCode != "" && time.Now().Before(a.pairingExpiresAt) {
		code, exp := a.pairingCode, a.pairingExpiresAt
		a.mu.RUnlock()
		return code, exp
	}
	a.mu.RUnlock()
	return a.GeneratePairingCode()
}

// ConsumePairingCode validates and invalidates the active ephemeral LAN pairing code.
// Burns the code globally after 5 failed attempts to defeat distributed/spoofed brute force.
func (a *AuthManager) ConsumePairingCode(candidate string) bool {
	if candidate == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pairingCode == "" || time.Now().After(a.pairingExpiresAt) {
		a.pairingCode = ""
		a.pairingExpiresAt = time.Time{}
		a.pairingFailedCount = 0
		return false
	}
	if len(candidate) != len(a.pairingCode) || subtle.ConstantTimeCompare([]byte(candidate), []byte(a.pairingCode)) != 1 {
		a.pairingFailedCount++
		if a.pairingFailedCount >= 5 {
			a.pairingCode = ""
			a.pairingExpiresAt = time.Time{}
			a.pairingFailedCount = 0
		}
		return false
	}
	// Single-use: burn immediately on success.
	a.pairingCode = ""
	a.pairingExpiresAt = time.Time{}
	a.pairingFailedCount = 0
	return true
}

func (s *Server) isHostAllowed(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSpace(host))
	// Strip IPv6 literal brackets if present without a port (e.g. "[fe80::1]" or "[::1]")
	// so net.ParseIP and loopback checks accept the bare IP literal.
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	// ponytail: Go's httptest.NewRequest sets default Host to example.com; accept it strictly under unit tests
	if allowDefaultTestHost && host == "example.com" {
		return true
	}
	s.mu.Lock()
	allowedHosts := append([]string(nil), s.allowedHosts...)
	lanEnabled := s.lanEnabled
	lanIP := s.lanIP
	s.mu.Unlock()

	for _, ah := range allowedHosts {
		if strings.EqualFold(host, ah) {
			return true
		}
	}
	if lanEnabled {
		if ip := net.ParseIP(host); ip != nil {
			return isAllowedLANHostIPWith(ip, lanIP)
		}
	}
	return false
}

// isAllowedLANHostIPWith decides whether an IP-literal Host header is acceptable in LAN mode. It accepts
// the detected LAN IP, loopback, and private/link-local ranges (RFC 1918, RFC 4193, link-local).
// A public IP in the Host header is a DNS-rebinding signal and is rejected: SqlFox is never meant
// to be reached over a routable public address.
func isAllowedLANHostIPWith(ip net.IP, lanIP string) bool {
	if lanIP != "" {
		if want := net.ParseIP(lanIP); want != nil && want.Equal(ip) {
			return true
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
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

		// 2. CSRF Guard: State-mutating API requests and sensitive/mutating GET endpoints
		isMutatingMethod := false
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			isMutatingMethod = true
		}
		isSensitiveGET := r.Method == http.MethodGet && (r.URL.Path == "/api/auth/master-key" ||
			r.URL.Path == "/api/ai/ollama-check" ||
			strings.HasPrefix(r.URL.Path, "/debug/pprof"))
		if (strings.HasPrefix(r.URL.Path, "/api") && isMutatingMethod) || isSensitiveGET {
			if sfs := r.Header.Get("Sec-Fetch-Site"); sfs == "cross-site" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": "Cross-site request blocked (CSRF protection)",
				})
				return
			}

			if origin := r.Header.Get("Origin"); origin != "" {
				// Same-origin only: an allowed-but-different origin (another localhost port,
				// any LAN IP, or scheme mismatch) must not be able to ride the session cookie.
				// Only trust X-Forwarded-Proto when behind a configured trusted reverse proxy.
				expectedScheme := "http"
				if r.TLS != nil {
					expectedScheme = "https"
				} else {
					s.mu.Lock()
					bp := s.behindProxy
					s.mu.Unlock()
					if !bp {
						envVal := strings.ToLower(strings.TrimSpace(os.Getenv("SQLFOX_BEHIND_PROXY")))
						bp = envVal == "1" || envVal == "true"
					}
					if bp && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
						remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
						if err != nil {
							remoteHost = r.RemoteAddr
						}
						if s.isTrustedProxyIP(net.ParseIP(remoteHost)) {
							expectedScheme = "https"
						}
					}
				}
				if u, err := url.Parse(origin); err != nil || !strings.EqualFold(u.Host, r.Host) || !strings.EqualFold(u.Scheme, expectedScheme) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"error": "Unauthorized Origin (CSRF protection)",
					})
					return
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

// isTrustedProxyIP reports whether ip is a trusted reverse proxy allowed to supply forwarding headers.
// Only loopback and explicitly configured trustedProxies IPs/CIDRs are trusted (never blanket RFC 1918),
// preventing sibling LAN/VPC/container peers from spoofing X-Forwarded-For to bypass lockout.
func (s *Server) isTrustedProxyIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	s.mu.Lock()
	proxies := s.trustedProxies
	s.mu.Unlock()
	for _, entry := range proxies {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, cidr, err := net.ParseCIDR(entry); err == nil {
			if cidr.Contains(ip) {
				return true
			}
			continue
		}
		if tip := net.ParseIP(entry); tip != nil && tip.Equal(ip) {
			return true
		}
	}
	return false
}

// GetClientIP extracts the effective client IP, respecting forwarding headers only when
// behindProxy is enabled AND the direct TCP peer (r.RemoteAddr) is a trusted proxy.
// X-Forwarded-For is parsed from right to left to prevent client-supplied left-edge spoofing.
func (s *Server) GetClientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	remoteIP := net.ParseIP(remoteHost)

	s.mu.Lock()
	proxyEnabled := s.behindProxy
	s.mu.Unlock()

	if !proxyEnabled {
		envVal := strings.ToLower(strings.TrimSpace(os.Getenv("SQLFOX_BEHIND_PROXY")))
		proxyEnabled = envVal == "1" || envVal == "true"
	}

	if proxyEnabled && s.isTrustedProxyIP(remoteIP) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			var fallbackTrusted string
			for i := len(parts) - 1; i >= 0; i-- {
				candidate := strings.TrimSpace(parts[i])
				ip := net.ParseIP(candidate)
				if ip == nil {
					continue
				}
				if !s.isTrustedProxyIP(ip) {
					return candidate
				}
				fallbackTrusted = candidate
			}
			if fallbackTrusted != "" {
				return fallbackTrusted
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			clientIP := strings.TrimSpace(xri)
			if ip := net.ParseIP(clientIP); ip != nil {
				return clientIP
			}
		}
	}

	return remoteHost
}

// IsRemoteRequest reports whether a request originates from a non-loopback host,
// accurately inspecting reverse proxy headers when behind-proxy mode is active.
// Requests carrying forwarding headers from a trusted proxy are never granted unauthenticated
// loopback bypass even if X-Forwarded-For / X-Real-IP claims 127.0.0.1.
func (s *Server) IsRemoteRequest(r *http.Request) bool {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	remoteIP := net.ParseIP(remoteHost)

	s.mu.Lock()
	proxyEnabled := s.behindProxy
	s.mu.Unlock()
	if !proxyEnabled {
		envVal := strings.ToLower(strings.TrimSpace(os.Getenv("SQLFOX_BEHIND_PROXY")))
		proxyEnabled = envVal == "1" || envVal == "true"
	}

	if proxyEnabled && s.isTrustedProxyIP(remoteIP) {
		if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" || r.Header.Get("Forwarded") != "" {
			return true
		}
	}

	clientIP := s.GetClientIP(r)
	ip := net.ParseIP(clientIP)
	if ip == nil {
		if clientIP == "localhost" {
			return false
		}
		return true
	}
	return !ip.IsLoopback()
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

		// Loopback (localhost) is granted immediate access if not a remote request behind proxy
		if !s.IsRemoteRequest(r) {
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
	return ""
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	isRemote := s.IsRemoteRequest(r)
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
		"version":   s.GetVersion(),
		"is_remote": isRemote,
		"unlocked":  unlocked,
	})
}

func (s *Server) handleAuthUnlock(w http.ResponseWriter, r *http.Request) {
	clientIP := s.GetClientIP(r)

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

	// Bound concurrent Argon2id derivations (64 MB each) to prevent OOM DoS under parallel unlock floods.
	select {
	case s.authMgr.argon2Sem <- struct{}{}:
		defer func() { <-s.authMgr.argon2Sem }()
	case <-r.Context().Done():
		return
	case <-time.After(2 * time.Second):
		writeError(w, http.StatusServiceUnavailable, "Authentication service busy, please retry.")
		return
	}

	// Re-check lockout after acquiring argon2Sem to close parallel unlock race at failedCount == 4.
	if locked, remaining := s.authMgr.IsLocked(clientIP); locked {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("Too many failed attempts. Locked out for %d minutes.", int(remaining.Minutes())+1),
		})
		return
	}

	s.mu.Lock()
	lanEnabled := s.lanEnabled
	behindProxy := s.behindProxy
	s.mu.Unlock()
	if !behindProxy {
		envVal := strings.ToLower(strings.TrimSpace(os.Getenv("SQLFOX_BEHIND_PROXY")))
		behindProxy = envVal == "1" || envVal == "true"
	}

	valid := s.vault.ValidateMasterPassword(req.Password)
	if !valid && lanEnabled {
		valid = s.authMgr.ConsumePairingCode(req.Password)
	}
	if !valid {
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

	isSecureConn := r.TLS != nil
	if !isSecureConn && behindProxy {
		remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteHost = r.RemoteAddr
		}
		if s.isTrustedProxyIP(net.ParseIP(remoteHost)) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			isSecureConn = true
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "sqlfox_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   isSecureConn,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   token,
		"message": "Vault unlocked successfully.",
	})
}

func (s *Server) handleGetMasterPassword(w http.ResponseWriter, r *http.Request) {
	if s.IsRemoteRequest(r) {
		writeError(w, http.StatusForbidden, "Master password inspection is only allowed from localhost.")
		return
	}

	s.mu.Lock()
	lanEnabled := s.lanEnabled
	lanIP := s.lanIP
	s.mu.Unlock()

	if !lanEnabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"lan_enabled": false,
		})
		return
	}

	// GET is idempotent (returns active pairing code without rotating/invalidating an in-flight pairing);
	// POST explicitly rotates and issues a fresh pairing code.
	var code string
	var exp time.Time
	if r.Method == http.MethodPost {
		code, exp = s.authMgr.GeneratePairingCode()
	} else {
		code, exp = s.authMgr.GetOrCreatePairingCode()
	}
	resp := map[string]any{
		"lan_enabled":            true,
		"pairing_code":           code,
		"pairing_expires_in_sec": int(time.Until(exp).Seconds()),
	}
	if lanIP != "" {
		resp["lan_ip"] = lanIP
	}
	writeJSON(w, http.StatusOK, resp)
}
