package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sqlfox/internal/vault"
)

func TestRemoteAccessGuard(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-auth-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "config.db")
	masterPass := "strong-master-pass"

	v, err := vault.OpenVault(vaultDB, masterPass)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), masterPass)
	handler := srv.Router()

	// 1. Loopback request from 127.0.0.1 -> Allowed immediately
	reqLocal := httptest.NewRequest("GET", "/api/connections", nil)
	reqLocal.RemoteAddr = "127.0.0.1:54321"
	recLocal := httptest.NewRecorder()
	handler.ServeHTTP(recLocal, reqLocal)

	if recLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for localhost, got: %d", recLocal.Code)
	}

	// 2. Remote request from LAN IP (192.168.1.100) -> Blocked with 401
	reqRemote := httptest.NewRequest("GET", "/api/connections", nil)
	reqRemote.RemoteAddr = "192.168.1.100:54321"
	recRemote := httptest.NewRecorder()
	handler.ServeHTTP(recRemote, reqRemote)

	if recRemote.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for remote IP, got: %d", recRemote.Code)
	}

	// 3. Remote request to /api/auth/unlock with wrong password -> 401
	wrongUnlockReq := httptest.NewRequest("POST", "/api/auth/unlock", bytes.NewReader([]byte(`{"password":"wrong"}`)))
	wrongUnlockReq.RemoteAddr = "192.168.1.100:54321"
	recWrong := httptest.NewRecorder()
	handler.ServeHTTP(recWrong, wrongUnlockReq)

	if recWrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong master password, got: %d", recWrong.Code)
	}

	// 4. Remote request to /api/auth/unlock with correct password -> 200 with Token
	unlockReq := httptest.NewRequest("POST", "/api/auth/unlock", bytes.NewReader([]byte(`{"password":"strong-master-pass"}`)))
	unlockReq.RemoteAddr = "192.168.1.100:54321"
	recUnlock := httptest.NewRecorder()
	handler.ServeHTTP(recUnlock, unlockReq)

	if recUnlock.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for unlock, got: %d", recUnlock.Code)
	}

	var authResp struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(recUnlock.Body).Decode(&authResp)
	if authResp.Token == "" {
		t.Fatal("expected token in unlock response")
	}

	// 5. Remote request with Bearer Token -> 200 OK!
	reqAuthed := httptest.NewRequest("GET", "/api/connections", nil)
	reqAuthed.RemoteAddr = "192.168.1.100:54321"
	reqAuthed.Header.Set("Authorization", "Bearer "+authResp.Token)
	recAuthed := httptest.NewRecorder()
	handler.ServeHTTP(recAuthed, reqAuthed)

	if recAuthed.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with valid bearer token, got: %d", recAuthed.Code)
	}

	// 6. Localhost requests master password -> 200 OK
	reqKeyLocal := httptest.NewRequest("GET", "/api/auth/master-key", nil)
	reqKeyLocal.RemoteAddr = "127.0.0.1:54321"
	recKeyLocal := httptest.NewRecorder()
	handler.ServeHTTP(recKeyLocal, reqKeyLocal)

	if recKeyLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for localhost master-key, got: %d", recKeyLocal.Code)
	}

	// 7. Remote IP requests master password -> 403 Forbidden
	reqKeyRemote := httptest.NewRequest("GET", "/api/auth/master-key", nil)
	reqKeyRemote.RemoteAddr = "192.168.1.100:54321"
	reqKeyRemote.Header.Set("Authorization", "Bearer "+authResp.Token)
	recKeyRemote := httptest.NewRecorder()
	handler.ServeHTTP(recKeyRemote, reqKeyRemote)

	if recKeyRemote.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for remote master-key, got: %d", recKeyRemote.Code)
	}
}

func TestRemoteLockoutPolicy(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-lockout-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	v, err := vault.OpenVault(filepath.Join(tempDir, "config.db"), "pin-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "pin-secret")
	handler := srv.Router()

	remoteIP := "10.0.0.55:12345"

	// 1-4 failed attempts should yield 401 Unauthorized
	for i := 1; i <= 4; i++ {
		req := httptest.NewRequest("POST", "/api/auth/unlock", bytes.NewReader([]byte(`{"password":"bad"}`)))
		req.RemoteAddr = remoteIP
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 Unauthorized, got: %d", i, rec.Code)
		}
	}

	// 5th failed attempt triggers lockout -> 429 Too Many Requests
	req5 := httptest.NewRequest("POST", "/api/auth/unlock", bytes.NewReader([]byte(`{"password":"bad"}`)))
	req5.RemoteAddr = remoteIP
	rec5 := httptest.NewRecorder()
	handler.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusTooManyRequests {
		t.Fatalf("5th attempt: expected 429 Too Many Requests, got: %d (%s)", rec5.Code, rec5.Body.String())
	}

	// Subsequent attempt (even with correct password) is blocked by quarantine
	reqLocked := httptest.NewRequest("POST", "/api/auth/unlock", bytes.NewReader([]byte(`{"password":"pin-secret"}`)))
	reqLocked.RemoteAddr = remoteIP
	recLocked := httptest.NewRecorder()
	handler.ServeHTTP(recLocked, reqLocked)
	if recLocked.Code != http.StatusTooManyRequests {
		t.Fatalf("locked attempt: expected 429 Too Many Requests, got: %d", recLocked.Code)
	}
}

func TestDNSRebindingGuard(t *testing.T) {
	tempDir := t.TempDir()
	v, _ := vault.OpenVault(filepath.Join(tempDir, "config.db"), "pin")
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "pin")
	handler := srv.Router()

	// Malicious Host header (DNS Rebinding attack)
	reqEvil := httptest.NewRequest("GET", "/api/connections", nil)
	reqEvil.Host = "evil.attacker.com:8080"
	recEvil := httptest.NewRecorder()
	handler.ServeHTTP(recEvil, reqEvil)

	if recEvil.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for evil Host header, got: %d", recEvil.Code)
	}

	// Legitimate localhost Host header
	reqLocal := httptest.NewRequest("GET", "/api/connections", nil)
	reqLocal.Host = "localhost:8080"
	reqLocal.RemoteAddr = "127.0.0.1:12345"
	recLocal := httptest.NewRecorder()
	handler.ServeHTTP(recLocal, reqLocal)

	if recLocal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for localhost Host header, got: %d", recLocal.Code)
	}
}

func TestCrossSiteCSRFGuard(t *testing.T) {
	tempDir := t.TempDir()
	v, _ := vault.OpenVault(filepath.Join(tempDir, "config.db"), "pin")
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "pin")
	handler := srv.Router()

	// Cross-site mutating request with Sec-Fetch-Site: cross-site
	reqCSRF := httptest.NewRequest("POST", "/api/connections", bytes.NewReader([]byte(`{}`)))
	reqCSRF.Host = "localhost:8080"
	reqCSRF.Header.Set("Sec-Fetch-Site", "cross-site")
	recCSRF := httptest.NewRecorder()
	handler.ServeHTTP(recCSRF, reqCSRF)

	if recCSRF.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-site mutation, got: %d", recCSRF.Code)
	}

	// Unauthorized Origin on mutating request
	reqOrigin := httptest.NewRequest("POST", "/api/connections", bytes.NewReader([]byte(`{}`)))
	reqOrigin.Host = "localhost:8080"
	reqOrigin.Header.Set("Origin", "http://malicious.org")
	recOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recOrigin, reqOrigin)

	if recOrigin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for malicious origin, got: %d", recOrigin.Code)
	}
}

func TestMasterKeyLANExclusivity(t *testing.T) {
	tempDir := t.TempDir()
	v, _ := vault.OpenVault(filepath.Join(tempDir, "config.db"), "pin-123456")
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "pin-123456")

	// 1. In standard localhost mode: lan_enabled is false, PIN is omitted
	reqDefault := httptest.NewRequest("GET", "/api/auth/master-key", nil)
	reqDefault.Host = "127.0.0.1:8080"
	reqDefault.RemoteAddr = "127.0.0.1:12345"
	recDefault := httptest.NewRecorder()
	srv.Router().ServeHTTP(recDefault, reqDefault)

	if recDefault.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", recDefault.Code)
	}
	var resDefault map[string]any
	_ = json.Unmarshal(recDefault.Body.Bytes(), &resDefault)
	if resDefault["lan_enabled"] != false {
		t.Fatalf("expected lan_enabled: false, got: %+v", resDefault)
	}
	if _, hasPin := resDefault["master_password"]; hasPin {
		t.Fatalf("master_password should be omitted when lan_enabled is false")
	}

	// 2. In LAN mode: lan_enabled is true, master_password is provided
	srv.SetLANEnabled(true)
	recLAN := httptest.NewRecorder()
	srv.Router().ServeHTTP(recLAN, reqDefault)

	if recLAN.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", recLAN.Code)
	}
	var resLAN map[string]any
	_ = json.Unmarshal(recLAN.Body.Bytes(), &resLAN)
	if resLAN["lan_enabled"] != true {
		t.Fatalf("expected lan_enabled: true, got: %+v", resLAN)
	}
	if resLAN["master_password"] != "pin-123456" {
		t.Fatalf("expected pin-123456, got: %v", resLAN["master_password"])
	}
}
