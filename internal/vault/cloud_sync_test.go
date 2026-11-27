package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestCloudSync_MirrorDeletionScope(t *testing.T) {
	cases := []struct {
		path, prefix string
		managed      bool
	}{
		{"README.md", "", false},
		{"src/main.go", "", false},
		{"notes/1-a.md", "", true},
		{"table-notes/c/d/t.md", "", true},
		{"snippets/1-x.sql", "", true},
		{"vault-backup.sqlfox-vault", "", true},
		{"backup/notes/1-a.md", "backup", true},
		{"notes/1-a.md", "backup", false},
		{"backup/README.md", "backup", false},
	}
	for _, c := range cases {
		if got := isSyncManagedPath(c.path, c.prefix); got != c.managed {
			t.Errorf("isSyncManagedPath(%q, %q) = %v, want %v", c.path, c.prefix, got, c.managed)
		}
	}
}

func TestResolveGitHubAPIBase(t *testing.T) {
	// Default and explicit https hosts are accepted.
	okCases := []string{
		"",
		"https://api.github.com",
		"https://ghe.corp.example.com/api/v3",
		"http://127.0.0.1:12345", // loopback http tolerated for local mocks/tests
		"http://localhost:8080",
	}
	for _, c := range okCases {
		if _, err := resolveGitHubAPIBase(c); err != nil {
			t.Errorf("expected api_base_url %q to be accepted, got: %v", c, err)
		}
	}

	// Cleartext to a network host, non-http(s) schemes, and malformed URLs are rejected so the
	// bearer token can never be exfiltrated to an attacker-chosen endpoint (SSRF guard).
	badCases := []string{
		"http://attacker.example.com",
		"http://10.0.0.5",
		"ftp://api.github.com",
		"https://api.github.com?x=1",
		"https://user:pass@api.github.com",
		"://nonsense",
	}
	for _, c := range badCases {
		if _, err := resolveGitHubAPIBase(c); err == nil {
			t.Errorf("expected api_base_url %q to be rejected, but it was accepted", c)
		}
	}
}

func TestCloudSyncDialGuardBlocksInternalEndpoints(t *testing.T) {
	for _, base := range []string{
		"https://10.0.0.5",        // RFC1918
		"https://169.254.169.254", // link-local metadata
		"https://100.64.0.1",      // CGNAT
	} {
		err := TestGitHubAccess(context.Background(), base, "test-token", "owner/repo", "main")
		if err == nil || !strings.Contains(err.Error(), "blocked dial to restricted IP") {
			t.Errorf("TestGitHubAccess(%s) error = %v; want blocked dial", base, err)
		}
	}
}

func TestCloudSyncRemoteRequestBlocksLoopback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if err := TestGitHubAccess(context.Background(), ts.URL, "test-token", "owner/repo", "main"); err != nil {
		t.Fatalf("local loopback request should pass: %v", err)
	}
	err := TestGitHubAccess(context.Background(), ts.URL, "test-token", "owner/repo", "main", true)
	if err == nil || !strings.Contains(err.Error(), "blocked dial to restricted IP") {
		t.Fatalf("remote loopback request error = %v; want blocked dial", err)
	}
}

func TestValidateGitHubTarget(t *testing.T) {
	if err := validateGitHubTarget("octocat/my-notes", "main"); err != nil {
		t.Errorf("expected valid repo/branch, got: %v", err)
	}
	if err := validateGitHubTarget("octocat/my-notes", ""); err != nil {
		t.Errorf("empty branch should be allowed (defaults applied by caller), got: %v", err)
	}
	badTargets := []struct{ repo, branch string }{
		{"no-slash", "main"},
		{"owner/repo/extra", "main"},
		{"owner/../etc", "main"},
		{"owner/repo", "main?query"},
		{"owner/repo", "feature branch"},
		{"", "main"},
	}
	for _, c := range badTargets {
		if err := validateGitHubTarget(c.repo, c.branch); err == nil {
			t.Errorf("expected repo=%q branch=%q to be rejected", c.repo, c.branch)
		}
	}
}

func TestCloudSync_EmptyRepositoryAutoInit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-empty-repo-test-*")
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

	var branchOmittedInAutoInit bool
	mockGH := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/repos/octocat/empty-repo/git/ref/heads/main" && r.Method == "GET":
			// 409 Conflict ("Git Repository is empty.") indicates empty repo without any commits
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Git Repository is empty.","status":"409"}`))
		case path == "/repos/octocat/empty-repo/contents/README.md" && r.Method == "PUT":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, hasBranch := body["branch"]; !hasBranch {
				branchOmittedInAutoInit = true
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"commit": map[string]any{"sha": "init-sha-001"},
			})
		case path == "/repos/octocat/empty-repo/git/refs" && r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"ref": "refs/heads/main"})
		case path == "/repos/octocat/empty-repo/git/commits/init-sha-001":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": map[string]any{"sha": "tree-001"}})
		case path == "/repos/octocat/empty-repo/git/trees/tree-001":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": []any{}})
		case path == "/repos/octocat/empty-repo/git/blobs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "blob-1"})
		case path == "/repos/octocat/empty-repo/git/trees":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "tree-002"})
		case path == "/repos/octocat/empty-repo/git/commits":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "commit-002"})
		case path == "/repos/octocat/empty-repo/git/refs/heads/main" && r.Method == "PATCH":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]any{"sha": "commit-002"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGH.Close()

	cfg := &CloudSyncConfig{
		Enabled:            true,
		GitHubToken:        "ghp_test_token",
		GitHubRepo:         "octocat/empty-repo",
		GitHubBranch:       "main",
		APIBaseURL:         mockGH.URL,
		IncludeVaultBackup: false,
	}

	res, err := SyncToGitHub(context.Background(), v, cfg, "master-pin-123")
	if err != nil {
		t.Fatalf("SyncToGitHub on empty repo failed: %v", err)
	}

	if !branchOmittedInAutoInit {
		t.Errorf("expected branch parameter to be omitted in initial commit payload to prevent 409 Conflict")
	}
	if res.CommitSHA != "commit-002" {
		t.Errorf("expected commit-002, got %s", res.CommitSHA)
	}
}

func TestSyncToGitHub_TwoWayPullMerge(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-twoway-sync-*")
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

	remoteNoteMD := "---\nid: remote-note-99\ntitle: Remote Runbook\nstatus: inbox\n---\n\nCheck replication lag.\n"
	remoteBlobB64 := "LS0tCmlkOiByZW1vdGUtbm90ZS05OQp0aXRsZTogUmVtb3RlIFJ1bmJvb2sKc3RhdHVzOiBpbmJveAotLS0KCkNoZWNrIHJlcGxpY2F0aW9uIGxhZy4K"
	_ = remoteNoteMD

	mockGH := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/repos/octocat/notes/git/ref/heads/main" && r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]any{"sha": "remote-commit-2"}})
		case p == "/repos/octocat/notes/git/commits/remote-commit-2" && r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": map[string]any{"sha": "remote-tree-2"}})
		case p == "/repos/octocat/notes/git/trees/remote-tree-2" && r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tree": []map[string]any{
					{"path": "notes/remote-note-99-remote-runbook.md", "type": "blob", "sha": "blob-remote-99"},
				},
			})
		case p == "/repos/octocat/notes/git/blobs/blob-remote-99" && r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"encoding": "base64",
				"content":  remoteBlobB64,
			})
		case p == "/repos/octocat/notes/git/blobs" && r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "blob-uploaded"})
		case p == "/repos/octocat/notes/git/trees" && r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "remote-tree-2"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGH.Close()

	cfg := &CloudSyncConfig{
		Enabled:            true,
		GitHubToken:        "ghp_test_token",
		GitHubRepo:         "octocat/notes",
		GitHubBranch:       "main",
		APIBaseURL:         mockGH.URL,
		IncludeVaultBackup: false,
	}

	res, err := SyncToGitHub(context.Background(), v, cfg, "master-pin-123")
	if err != nil {
		t.Fatalf("SyncToGitHub two-way pull/merge failed: %v", err)
	}
	if res.PulledCount != 1 {
		t.Fatalf("expected PulledCount=1, got %d", res.PulledCount)
	}
	notes, err := v.ListAllOperationalNotes()
	if err != nil {
		t.Fatalf("ListAllOperationalNotes failed: %v", err)
	}
	found := false
	for _, n := range notes {
		if n.ID == "remote-note-99" && n.Title == "Remote Runbook" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected remote-note-99 to be merged into local vault, got %+v", notes)
	}
}

// Regression: on a fresh machine (no local notes) with a PathPrefix set, a remote note
// must survive mirror-deletion. Before the notesPrefix fix, prefix+"notes/" produced
// "sqlfoxnotes/" and never matched "sqlfox/notes/...", so the guard was bypassed.
func TestSyncToGitHub_PrefixedFreshMachineKeepsRemoteNotes(t *testing.T) {
	tmpDir := t.TempDir()
	v, err := OpenVault(filepath.Join(tmpDir, "vault.db"), "master-pin-123")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	const remotePath = "sqlfox/notes/remote-keep.md"
	var deletedPaths []string
	mockGH := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/git/ref/heads/main") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]any{"sha": "c1"}})
		case strings.HasSuffix(p, "/git/commits/c1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": map[string]any{"sha": "t1"}})
		case strings.HasSuffix(p, "/git/trees/t1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": []map[string]any{{"path": remotePath, "type": "blob", "sha": "b1"}}})
		case strings.HasSuffix(p, "/git/blobs/b1") && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound) // pull cannot import it; note still must survive
		case strings.HasSuffix(p, "/git/blobs") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "b2"})
		case strings.HasSuffix(p, "/git/trees") && r.Method == http.MethodPost:
			var body struct {
				Tree []struct {
					Path string  `json:"path"`
					SHA  *string `json:"sha"`
				} `json:"tree"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, e := range body.Tree {
				if e.SHA == nil {
					deletedPaths = append(deletedPaths, e.Path)
				}
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "t2"})
		case strings.HasSuffix(p, "/git/commits") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "c2"})
		case strings.HasSuffix(p, "/git/refs/heads/main") && r.Method == http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]any{"sha": "c2"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGH.Close()

	cfg := &CloudSyncConfig{
		GitHubToken:        "ghp_test",
		GitHubRepo:         "octocat/notes",
		GitHubBranch:       "main",
		PathPrefix:         "sqlfox",
		APIBaseURL:         mockGH.URL,
		IncludeVaultBackup: false,
	}
	if _, err := SyncToGitHub(context.Background(), v, cfg, "master-pin-123"); err != nil {
		t.Fatalf("SyncToGitHub failed: %v", err)
	}
	for _, path := range deletedPaths {
		if path == remotePath {
			t.Fatalf("fresh machine deleted remote note %q; expected it preserved", remotePath)
		}
	}
}
