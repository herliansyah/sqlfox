package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/herliansyah/sqlfox/internal/vault"
)

func setupTestServerWithVault(t *testing.T) (*Server, *vault.Vault) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "server_test_vault.db")
	v, err := vault.OpenVault(dbPath, "test-master-pass")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })

	sessionMgr := NewSessionManager(v, tmpDir)
	srv := &Server{
		vault:      v,
		sessionMgr: sessionMgr,
	}
	return srv, v
}

func TestAIProfilesServerEndpoints(t *testing.T) {
	srv, _ := setupTestServerWithVault(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ai/profiles", srv.handleListAIProfiles)
	mux.HandleFunc("POST /api/ai/profiles", srv.handleCreateAIProfile)
	mux.HandleFunc("GET /api/ai/profiles/{id}", srv.handleGetAIProfile)
	mux.HandleFunc("PUT /api/ai/profiles/{id}", srv.handleUpdateAIProfile)
	mux.HandleFunc("DELETE /api/ai/profiles/{id}", srv.handleDeleteAIProfile)
	mux.HandleFunc("POST /api/ai/profiles/{id}/active", srv.handleSetActiveAIProfile)

	// 1. GET /api/ai/profiles -> seeded default
	req := httptest.NewRequest("GET", "/api/ai/profiles", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var list []*vault.AIProfile
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 seeded profile, got %d", len(list))
	}
	defaultID := list[0].ID

	// 2. POST /api/ai/profiles -> create new profile
	newProf := vault.AIProfile{
		ID:        "p-deepseek",
		Name:      "DeepSeek Coder",
		Provider:  "custom",
		APIKey:    "sk-test-deepseek-key",
		BaseURL:   "https://api.deepseek.com/v1",
		ModelName: "deepseek-coder",
	}
	body, _ := json.Marshal(newProf)
	req = httptest.NewRequest("POST", "/api/ai/profiles", bytes.NewReader(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 3. GET /api/ai/profiles/{id}
	req = httptest.NewRequest("GET", "/api/ai/profiles/p-deepseek", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fetched vault.AIProfile
	if err := json.NewDecoder(w.Body).Decode(&fetched); err != nil {
		t.Fatalf("failed to decode profile: %v", err)
	}
	if fetched.APIKey != "sk-test-deepseek-key" {
		t.Fatalf("expected decrypted key in single get, got %s", fetched.APIKey)
	}

	// 4. POST /api/ai/profiles/{id}/active
	req = httptest.NewRequest("POST", "/api/ai/profiles/p-deepseek/active", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on set active, got %d", w.Code)
	}

	// 5. DELETE default profile -> should succeed because 2 exist
	req = httptest.NewRequest("DELETE", "/api/ai/profiles/"+defaultID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete default, got %d", w.Code)
	}

	// 6. DELETE p-deepseek -> must fail because it's the only remaining profile
	req = httptest.NewRequest("DELETE", "/api/ai/profiles/p-deepseek", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when deleting only profile, got %d", w.Code)
	}
}
