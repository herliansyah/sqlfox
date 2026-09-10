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
