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

func TestUpdateAIProfilePatchPreservesOmittedFields(t *testing.T) {
	srv, v := setupTestServerWithVault(t)

	seed := vault.AIProfile{
		ID:              "p-patch",
		Name:            "Patch Me",
		Provider:        "custom",
		APIKey:          "sk-existing",
		BaseURL:         "https://1.1.1.1/v1",
		ModelName:       "example-model",
		Temperature:     0.7,
		Language:        "id",
		ResponseStyle:   "concise",
		CustomPrompt:    "keep me",
		IncludeSchema:   true,
		MaskIdentifiers: true,
		IsActive:        true,
	}
	if err := v.SaveAIProfile(&seed); err != nil {
		t.Fatal(err)
	}

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/ai/profiles/p-patch", bytes.NewBufferString(body))
		req.SetPathValue("id", "p-patch")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.handleUpdateAIProfile(rec, req)
		return rec
	}
	get := func() *vault.AIProfile {
		stored, err := v.GetAIProfile("p-patch")
		if err != nil {
			t.Fatal(err)
		}
		return stored
	}

	if rec := put(`{"name":"Renamed"}`); rec.Code != http.StatusOK {
		t.Fatalf("partial PUT failed: %d %s", rec.Code, rec.Body.String())
	}
	stored := get()
	if stored.Name != "Renamed" || stored.Provider != "custom" || stored.BaseURL != "https://1.1.1.1/v1" ||
		stored.ModelName != "example-model" || stored.Temperature != 0.7 || stored.Language != "id" ||
		stored.ResponseStyle != "concise" || stored.CustomPrompt != "keep me" ||
		!stored.IncludeSchema || !stored.MaskIdentifiers || !stored.IsActive {
		t.Fatalf("omitted fields were clobbered: %+v", stored)
	}
	if stored.APIKey != "sk-existing" || !stored.HasAPIKey {
		t.Fatalf("omitted API key was clobbered: key=%q has=%v", stored.APIKey, stored.HasAPIKey)
	}

	if rec := put(`{"mask_identifiers":false,"is_active":false}`); rec.Code != http.StatusOK {
		t.Fatalf("explicit false PUT failed: %d %s", rec.Code, rec.Body.String())
	}
	stored = get()
	if stored.MaskIdentifiers || stored.IsActive || !stored.IncludeSchema || stored.Name != "Renamed" {
		t.Fatalf("explicit flags not applied or other fields clobbered: %+v", stored)
	}

	if rec := put(`{"base_url":"https://8.8.8.8/v1"}`); rec.Code != http.StatusOK {
		t.Fatalf("endpoint-change PUT failed: %d %s", rec.Code, rec.Body.String())
	}
	stored = get()
	if stored.HasAPIKey || stored.APIKey != "" || stored.EncryptedAPIKey != "" {
		t.Fatalf("stored key was not cleared on endpoint change: %+v", stored)
	}
	if !stored.IncludeSchema || stored.MaskIdentifiers {
		t.Fatalf("privacy flags changed on unrelated PUT: %+v", stored)
	}

	if rec := put(`{"base_url":"https://9.9.9.9/v1","api_key":"sk-new"}`); rec.Code != http.StatusOK {
		t.Fatalf("new-key PUT failed: %d %s", rec.Code, rec.Body.String())
	}
	stored = get()
	if stored.APIKey != "sk-new" || !stored.HasAPIKey {
		t.Fatalf("new API key not stored: %+v", stored)
	}

	if rec := put(`{"name":"   "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank name, got %d", rec.Code)
	}
	stored = get()
	if stored.Name != "Renamed" {
		t.Fatalf("rejected PUT still mutated profile: name=%q", stored.Name)
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
