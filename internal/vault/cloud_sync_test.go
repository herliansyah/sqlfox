package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCloudSync_ConfigStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-cloudsync-test-*")
	if err != nil {
		t.Fatalf("temp dir creation failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "vault.db")
	v, err := OpenVault(dbPath, "master-pin-123")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	// 1. Initial should return default disabled config
	cfg, err := v.GetCloudSyncConfig()
	if err != nil {
		t.Fatalf("GetCloudSyncConfig failed: %v", err)
	}
	if cfg.Enabled {
		t.Errorf("expected initial enabled to be false")
	}
	if cfg.GitHubBranch != "main" {
		t.Errorf("expected default branch to be 'main', got %q", cfg.GitHubBranch)
	}

	// 2. Save config with sensitive token
	cfg.Enabled = true
	cfg.GitHubToken = "ghp_secret_token_12345"
	cfg.GitHubRepo = "myuser/sqlfox-notes"
	cfg.PathPrefix = "notes-vault"
	cfg.IncludeVaultBackup = true

	if err := v.SaveCloudSyncConfig(cfg); err != nil {
		t.Fatalf("SaveCloudSyncConfig failed: %v", err)
	}

	// 3. Re-read config: token should be decrypted and match
	readCfg, err := v.GetCloudSyncConfig()
	if err != nil {
		t.Fatalf("GetCloudSyncConfig after save failed: %v", err)
	}
	if !readCfg.Enabled {
		t.Errorf("expected enabled true")
	}
	if readCfg.GitHubToken != "ghp_secret_token_12345" {
		t.Errorf("expected token match, got %q", readCfg.GitHubToken)
	}
	if readCfg.GitHubRepo != "myuser/sqlfox-notes" {
		t.Errorf("expected repo match, got %q", readCfg.GitHubRepo)
	}
	if readCfg.PathPrefix != "notes-vault" {
		t.Errorf("expected prefix match, got %q", readCfg.PathPrefix)
	}
}

func TestCloudSync_MockGitHubReplication(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-cloudsync-engine-*")
	if err != nil {
		t.Fatalf("temp dir creation failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "vault.db")
	v, err := OpenVault(dbPath, "master-pin-123")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	// Add sample note
	if err := v.CreateOperationalNote(&OperationalNote{
		ID:         "note-abc",
		Title:      "Investigasi Lock",
		Content:    "SELECT * FROM pg_locks;",
		Status:     "inbox",
		CapturedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateOperationalNote failed: %v", err)
	}

	var mu sync.Mutex
	blobsCreated := 0
	treeCreated := false
	commitCreated := false
	refUpdated := false

	mockGH := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		auth := r.Header.Get("Authorization")
		if auth != "Bearer ghp_valid_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		path := r.URL.Path
		switch {
		// 1. Check repo access
		case path == "/repos/octocat/my-notes":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"full_name": "octocat/my-notes"})

		// 2. Query branch ref
		case path == "/repos/octocat/my-notes/git/ref/heads/main" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": map[string]any{"sha": "commit-sha-001"},
			})

		// 3. Query commit details
		case path == "/repos/octocat/my-notes/git/commits/commit-sha-001" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tree": map[string]any{"sha": "tree-sha-001"},
			})

		// 4. Inspect tree for mirror deletions
		case path == "/repos/octocat/my-notes/git/trees/tree-sha-001" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tree": []map[string]any{
					{"path": "notes/old-deleted-note.md", "type": "blob", "sha": "blob-old"},
				},
			})

		// 5. Create blob
		case path == "/repos/octocat/my-notes/git/blobs" && r.Method == "POST":
			blobsCreated++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "new-blob-sha"})

		// 6. Create tree
		case path == "/repos/octocat/my-notes/git/trees" && r.Method == "POST":
			treeCreated = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "new-tree-sha-002"})

		// 7. Create commit
		case path == "/repos/octocat/my-notes/git/commits" && r.Method == "POST":
			commitCreated = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "commit-sha-002"})

		// 8. Update ref
		case path == "/repos/octocat/my-notes/git/refs/heads/main" && r.Method == "PATCH":
			refUpdated = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]any{"sha": "commit-sha-002"}})

		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGH.Close()

	// 1. Test access test function
	ctx := context.Background()
	if err := TestGitHubAccess(ctx, mockGH.URL, "ghp_valid_token", "octocat/my-notes", "main"); err != nil {
		t.Fatalf("TestGitHubAccess failed: %v", err)
	}

	// Test invalid token
	if err := TestGitHubAccess(ctx, mockGH.URL, "ghp_bad_token", "octocat/my-notes", "main"); err == nil {
		t.Fatalf("expected error for bad token, got nil")
	}

	// 2. Test SyncToGitHub
	cfg := &CloudSyncConfig{
		Enabled:            true,
		GitHubToken:        "ghp_valid_token",
		GitHubRepo:         "octocat/my-notes",
		GitHubBranch:       "main",
		APIBaseURL:         mockGH.URL,
		IncludeVaultBackup: false,
	}

	res, err := SyncToGitHub(ctx, v, cfg, "master-pin-123")
	if err != nil {
		t.Fatalf("SyncToGitHub failed: %v", err)
	}

	if res.CommitSHA != "commit-sha-002" {
		t.Errorf("expected commit SHA commit-sha-002, got %s", res.CommitSHA)
	}
	if !treeCreated {
		t.Errorf("expected tree to be created")
	}
	if !commitCreated {
		t.Errorf("expected commit to be created")
	}
	if !refUpdated {
		t.Errorf("expected branch ref to be updated")
	}
	if blobsCreated == 0 {
		t.Errorf("expected at least 1 blob to be created")
	}

	// 3. Verify status persisted in vault
	updatedCfg, err := v.GetCloudSyncConfig()
	if err != nil {
		t.Fatalf("GetCloudSyncConfig failed: %v", err)
	}
	if updatedCfg.LastSyncStatus != "synced" {
		t.Errorf("expected status 'synced', got %q", updatedCfg.LastSyncStatus)
	}
	if updatedCfg.LastCommitSHA != "commit-sha-002" {
		t.Errorf("expected commit SHA 'commit-sha-002', got %q", updatedCfg.LastCommitSHA)
	}
	if updatedCfg.LastSyncAt == nil {
		t.Errorf("expected non-nil LastSyncAt")
	}
}
