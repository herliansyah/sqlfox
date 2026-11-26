package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/herliansyah/sqlfox/internal/ai"
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
	if fetched.APIKey != "" {
		t.Fatalf("expected API key to be redacted in single get, got %q", fetched.APIKey)
	}
	if !fetched.HasAPIKey {
		t.Fatalf("expected HasAPIKey=true in single get")
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

func TestValidateAIEndpoint(t *testing.T) {
	// Valid public & local endpoints
	validCases := []struct {
		url      string
		provider ai.Provider
		isRemote bool
	}{
		{"", ai.ProviderOpenAI, true},
		{"https://api.openai.com/v1", ai.ProviderOpenAI, true},
		{"http://localhost:11434", ai.ProviderOllama, false},
		{"http://127.0.0.1:4000/v1", ai.ProviderOpenAI, false},
	}
	for _, tc := range validCases {
		if err := validateAIEndpoint(tc.url, tc.provider, tc.isRemote); err != nil {
			t.Errorf("expected endpoint %q (provider=%s, remote=%v) to be valid, got: %v", tc.url, tc.provider, tc.isRemote, err)
		}
	}

	// Blocked SSRF targets
	blockedCases := []struct {
		url      string
		provider ai.Provider
		isRemote bool
	}{
		{"http://169.254.169.254/latest/meta-data/", ai.ProviderOpenAI, false},
		{"http://metadata.google.internal/computeMetadata/v1/", ai.ProviderOllama, false},
		{"file:///etc/passwd", ai.ProviderOpenAI, false},
		{"http://127.0.0.1:8080/api/connections", ai.ProviderOpenAI, true},
		{"http://localhost:11434", ai.ProviderOllama, true},
		{"http://192.168.1.10:8080", ai.ProviderOpenAI, false},
	}
	for _, tc := range blockedCases {
		if err := validateAIEndpoint(tc.url, tc.provider, tc.isRemote); err == nil {
			t.Errorf("expected endpoint %q (provider=%s, remote=%v) to be blocked", tc.url, tc.provider, tc.isRemote)
		}
	}
}
