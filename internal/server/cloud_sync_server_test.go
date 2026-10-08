package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sqlfox/internal/vault"
)

func TestCloudSync_ServerIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-server-sync-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "vault.db")
	v, err := vault.OpenVault(dbPath, "test-master-pass")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tmpDir, "backups"), "test-master-pass")

	newLocalRequest := func(method, target string, body io.Reader) *http.Request {
		req := httptest.NewRequest(method, target, body)
		req.RemoteAddr = "127.0.0.1:54321"
		return req
	}

	// 1. Test GET /api/cloud-sync/config initially disabled
	req := newLocalRequest("GET", "/api/cloud-sync/config", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var getCfg cloudSyncConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&getCfg); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if getCfg.Enabled {
		t.Errorf("expected initial enabled false")
	}

	// 2. Test Mock GitHub Server
	mockGH := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/repos/myorg/notes-repo":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"full_name": "myorg/notes-repo"})
		case path == "/repos/myorg/notes-repo/git/ref/heads/main":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]any{"sha": "c1"}})
		case path == "/repos/myorg/notes-repo/git/commits/c1":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": map[string]any{"sha": "t1"}})
		case path == "/repos/myorg/notes-repo/git/trees/t1":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": []any{}})
		case path == "/repos/myorg/notes-repo/git/blobs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "b1"})
		case path == "/repos/myorg/notes-repo/git/trees":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "t2"})
		case path == "/repos/myorg/notes-repo/git/commits":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "c2"})
		case path == "/repos/myorg/notes-repo/git/refs/heads/main":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]any{"sha": "c2"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGH.Close()

	// 3. Test POST /api/cloud-sync/config
	saveReqBody, _ := json.Marshal(saveCloudSyncConfigRequest{
		Enabled:            true,
		GitHubToken:        "ghp_my_secret_token_123",
		GitHubRepo:         "myorg/notes-repo",
		GitHubBranch:       "main",
		PathPrefix:         "my-sqlfox",
		IncludeVaultBackup: false,
	})
	req = newLocalRequest("POST", "/api/cloud-sync/config", bytes.NewReader(saveReqBody))
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Override APIBaseURL to test server
	cfgInVault, _ := v.GetCloudSyncConfig()
	cfgInVault.APIBaseURL = mockGH.URL
	_ = v.SaveCloudSyncConfig(cfgInVault)

	// 4. Test POST /api/cloud-sync/test
	testReqBody, _ := json.Marshal(testCloudSyncRequest{
		GitHubToken:  "ghp_my_secret_token_123",
		GitHubRepo:   "myorg/notes-repo",
		GitHubBranch: "main",
	})
	req = newLocalRequest("POST", "/api/cloud-sync/test", bytes.NewReader(testReqBody))
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for test, got %d: %s", w.Code, w.Body.String())
	}

	// 5. Test POST /api/cloud-sync/run
	req = newLocalRequest("POST", "/api/cloud-sync/run", nil)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for run, got %d: %s", w.Code, w.Body.String())
	}

	// 6. Test GET /api/cloud-sync/status
	req = newLocalRequest("GET", "/api/cloud-sync/status", nil)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for status, got %d: %s", w.Code, w.Body.String())
	}
	var statusResp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&statusResp)
	if statusResp["status"] != "synced" {
		t.Errorf("expected status 'synced', got %v", statusResp["status"])
	}

	// 7. Seed sample operational note
	_ = v.CreateOperationalNote(&vault.OperationalNote{
		ID:         "srv-note-1",
		Title:      "Server Note 1",
		Content:    "Hello server note",
		Status:     "inbox",
		CapturedAt: time.Now(),
	})

	// 8. Test GET /api/notes/bundle/export
	req = newLocalRequest("GET", "/api/notes/bundle/export", nil)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for bundle export, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("expected application/zip content type, got %s", w.Header().Get("Content-Type"))
	}
	exportedZip := w.Body.Bytes()
	if len(exportedZip) == 0 {
		t.Fatalf("exported zip is empty")
	}

	// 9. Test POST /api/notes/bundle/import with multipart
	bodyBuf := new(bytes.Buffer)
	mpw := multipart.NewWriter(bodyBuf)
	part, err := mpw.CreateFormFile("bundle", "notes.zip")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	_, _ = part.Write(exportedZip)
	_ = mpw.Close()

	req = newLocalRequest("POST", "/api/notes/bundle/import", bodyBuf)
	req.Header.Set("Content-Type", mpw.FormDataContentType())
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for bundle import, got %d: %s", w.Code, w.Body.String())
	}

	var importResp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&importResp)
	if importResp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", importResp["status"])
	}
}
