package vault

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CloudSyncConfig represents settings for GitHub-based notes & vault replication.
type CloudSyncConfig struct {
	Enabled            bool       `json:"enabled"`
	GitHubToken        string     `json:"github_token,omitempty"` // Sensitive: encrypted at rest in vault
	GitHubRepo         string     `json:"github_repo"`            // "owner/repo"
	GitHubBranch       string     `json:"github_branch"`          // default "main"
	PathPrefix         string     `json:"path_prefix"`            // optional subfolder, e.g. "sqlfox/"
	IncludeVaultBackup bool       `json:"include_vault_backup"`   // whether to push encrypted .sqlfox-vault archive
	APIBaseURL         string     `json:"api_base_url,omitempty"` // default "https://api.github.com", override for tests/enterprise
	LastSyncAt         *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus     string     `json:"last_sync_status,omitempty"` // "idle", "synced", "syncing", "pending", "error"
	LastSyncError      string     `json:"last_sync_error,omitempty"`
	LastCommitSHA      string     `json:"last_commit_sha,omitempty"`
}

// CloudSyncResult represents the outcome of a sync operation.
type CloudSyncResult struct {
	CommitSHA  string    `json:"commit_sha"`
	FilesCount int       `json:"files_count"`
	Message    string    `json:"message"`
	SyncedAt   time.Time `json:"synced_at"`
}

type cloudSyncStoredPayload struct {
	Enabled            bool       `json:"enabled"`
	EncryptedToken     string     `json:"encrypted_token,omitempty"`
	GitHubRepo         string     `json:"github_repo"`
	GitHubBranch       string     `json:"github_branch"`
	PathPrefix         string     `json:"path_prefix"`
	IncludeVaultBackup bool       `json:"include_vault_backup"`
	APIBaseURL         string     `json:"api_base_url,omitempty"`
	LastSyncAt         *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus     string     `json:"last_sync_status,omitempty"`
	LastSyncError      string     `json:"last_sync_error,omitempty"`
	LastCommitSHA      string     `json:"last_commit_sha,omitempty"`
}

// GetCloudSyncConfig loads the current cloud sync configuration from the vault.
func (v *Vault) GetCloudSyncConfig() (*CloudSyncConfig, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var val string
	err := v.db.QueryRow(`SELECT value FROM meta WHERE key = 'cloud_sync_config'`).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &CloudSyncConfig{
				Enabled:            false,
				GitHubBranch:       "main",
				IncludeVaultBackup: true,
				LastSyncStatus:     "idle",
			}, nil
		}
		return nil, fmt.Errorf("failed to load cloud sync config: %w", err)
	}

	var stored cloudSyncStoredPayload
	if err := json.Unmarshal([]byte(val), &stored); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cloud sync config: %w", err)
	}

	cfg := &CloudSyncConfig{
		Enabled:            stored.Enabled,
		GitHubRepo:         stored.GitHubRepo,
		GitHubBranch:       stored.GitHubBranch,
		PathPrefix:         stored.PathPrefix,
		IncludeVaultBackup: stored.IncludeVaultBackup,
		APIBaseURL:         stored.APIBaseURL,
		LastSyncAt:         stored.LastSyncAt,
		LastSyncStatus:     stored.LastSyncStatus,
		LastSyncError:      stored.LastSyncError,
		LastCommitSHA:      stored.LastCommitSHA,
	}
	if cfg.GitHubBranch == "" {
		cfg.GitHubBranch = "main"
	}
	if cfg.LastSyncStatus == "" {
		cfg.LastSyncStatus = "idle"
	}

	if stored.EncryptedToken != "" {
		dec, err := Decrypt(stored.EncryptedToken, v.key)
		if err == nil {
			cfg.GitHubToken = string(dec)
		}
	}

	return cfg, nil
}

// SaveCloudSyncConfig saves the cloud sync configuration to the vault, encrypting the GitHub token.
func (v *Vault) SaveCloudSyncConfig(cfg *CloudSyncConfig) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	var existingToken string
	var val string
	if err := v.db.QueryRow(`SELECT value FROM meta WHERE key = 'cloud_sync_config'`).Scan(&val); err == nil {
		var oldStored cloudSyncStoredPayload
		if err := json.Unmarshal([]byte(val), &oldStored); err == nil {
			existingToken = oldStored.EncryptedToken
		}
	}

	var encToken string
	if cfg.GitHubToken != "" {
		enc, err := Encrypt([]byte(cfg.GitHubToken), v.key)
		if err != nil {
			return fmt.Errorf("failed to encrypt github token: %w", err)
		}
		encToken = enc
	} else {
		encToken = existingToken
	}

	branch := cfg.GitHubBranch
	if branch == "" {
		branch = "main"
	}

	status := cfg.LastSyncStatus
	if status == "" {
		status = "idle"
	}

	stored := cloudSyncStoredPayload{
		Enabled:            cfg.Enabled,
		EncryptedToken:     encToken,
		GitHubRepo:         strings.TrimSpace(cfg.GitHubRepo),
		GitHubBranch:       branch,
		PathPrefix:         strings.Trim(cfg.PathPrefix, "/"),
		IncludeVaultBackup: cfg.IncludeVaultBackup,
		APIBaseURL:         cfg.APIBaseURL,
		LastSyncAt:         cfg.LastSyncAt,
		LastSyncStatus:     status,
		LastSyncError:      cfg.LastSyncError,
		LastCommitSHA:      cfg.LastCommitSHA,
	}

	raw, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to serialize cloud sync config: %w", err)
	}

	_, err = v.db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('cloud_sync_config', ?)`, string(raw))
	return err
}

// TestGitHubAccess verifies that the provided token has access to the target repository.
func TestGitHubAccess(ctx context.Context, apiBaseURL, token, repo, branch string) error {
	if token == "" {
		return errors.New("github token is required")
	}
	repo = strings.TrimSpace(repo)
	if repo == "" || !strings.Contains(repo, "/") {
		return errors.New("valid repository in 'owner/repo' format is required")
	}

	if apiBaseURL == "" {
		apiBaseURL = "https://api.github.com"
	}
	apiBaseURL = strings.TrimRight(apiBaseURL, "/")

	reqURL := fmt.Sprintf("%s/repos/%s", apiBaseURL, repo)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}
	setGitHubHeaders(req, token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to github: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("invalid or expired github token")
	}
	if resp.StatusCode == http.StatusNotFound {
		return errors.New("repository not found or token lacks access")
	}
	if resp.StatusCode == http.StatusForbidden {
		return errors.New("github api access forbidden or rate limit reached")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SyncToGitHub executes the full atomic synchronization of notes, snippets, and encrypted vault archive.
func SyncToGitHub(ctx context.Context, v *Vault, cfg *CloudSyncConfig, masterPassword string) (*CloudSyncResult, error) {
	if cfg.GitHubToken == "" {
		return nil, errors.New("github token is not configured")
	}
	repo := strings.TrimSpace(cfg.GitHubRepo)
	if repo == "" || !strings.Contains(repo, "/") {
		return nil, errors.New("invalid repository format, expected 'owner/repo'")
	}

	branch := cfg.GitHubBranch
	if branch == "" {
		branch = "main"
	}

	apiBaseURL := cfg.APIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = "https://api.github.com"
	}
	apiBaseURL = strings.TrimRight(apiBaseURL, "/")

	// 1. Export human-readable notes and snippets
	files, err := ExportNotesFiles(v)
	if err != nil {
		return nil, fmt.Errorf("failed to export notes files: %w", err)
	}

	// 2. Include encrypted Vault Archive if enabled
	if cfg.IncludeVaultBackup && masterPassword != "" {
		archiveBytes, err := ExportVault(v, masterPassword)
		if err == nil && len(archiveBytes) > 0 {
			files["vault-backup.sqlfox-vault"] = archiveBytes
		}
	}

	// 3. Prepend PathPrefix if set
	prefix := strings.Trim(cfg.PathPrefix, "/")
	if prefix != "" {
		prefixedFiles := make(map[string][]byte, len(files))
		for k, v := range files {
			prefixedFiles[prefix+"/"+k] = v
		}
		files = prefixedFiles
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// 4. Get the latest commit SHA of the target branch
	refURL := fmt.Sprintf("%s/repos/%s/git/ref/heads/%s", apiBaseURL, repo, branch)
	refReq, err := http.NewRequestWithContext(ctx, "GET", refURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHubHeaders(refReq, cfg.GitHubToken)

	refResp, err := client.Do(refReq)
	if err != nil {
		return nil, fmt.Errorf("failed to query branch ref: %w", err)
	}
	defer refResp.Body.Close()

	var latestCommitSHA string
	if refResp.StatusCode == http.StatusNotFound {
		// Empty repository: create initial commit with README.md
		initCommitSHA, initErr := createInitialCommit(ctx, client, apiBaseURL, repo, branch, cfg.GitHubToken)
		if initErr != nil {
			return nil, fmt.Errorf("repository branch %s not found and auto-init failed: %w", branch, initErr)
		}
		latestCommitSHA = initCommitSHA
	} else if refResp.StatusCode == http.StatusOK {
		var refData struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}
		if err := json.NewDecoder(refResp.Body).Decode(&refData); err != nil {
			return nil, fmt.Errorf("failed to parse branch ref response: %w", err)
		}
		latestCommitSHA = refData.Object.SHA
	} else {
		b, _ := io.ReadAll(refResp.Body)
		return nil, fmt.Errorf("querying branch ref failed (HTTP %d): %s", refResp.StatusCode, string(b))
	}

	// 5. Get base tree SHA from the latest commit
	commitURL := fmt.Sprintf("%s/repos/%s/git/commits/%s", apiBaseURL, repo, latestCommitSHA)
	commitReq, err := http.NewRequestWithContext(ctx, "GET", commitURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHubHeaders(commitReq, cfg.GitHubToken)

	commitResp, err := client.Do(commitReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", latestCommitSHA, err)
	}
	defer commitResp.Body.Close()

	if commitResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(commitResp.Body)
		return nil, fmt.Errorf("failed to fetch commit object (HTTP %d): %s", commitResp.StatusCode, string(b))
	}

	var commitData struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(commitResp.Body).Decode(&commitData); err != nil {
		return nil, fmt.Errorf("failed to parse commit details: %w", err)
	}
	baseTreeSHA := commitData.Tree.SHA

	// 6. Inspect existing tree for mirror deletions
	existingTreeURL := fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", apiBaseURL, repo, baseTreeSHA)
	treeReq, err := http.NewRequestWithContext(ctx, "GET", existingTreeURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHubHeaders(treeReq, cfg.GitHubToken)

	treeResp, err := client.Do(treeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect tree: %w", err)
	}
	defer treeResp.Body.Close()

	var existingFiles []string
	if treeResp.StatusCode == http.StatusOK {
		var treeData struct {
			Tree []struct {
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"tree"`
		}
		if err := json.NewDecoder(treeResp.Body).Decode(&treeData); err == nil {
			for _, item := range treeData.Tree {
				if item.Type == "blob" {
					if prefix == "" || strings.HasPrefix(item.Path, prefix+"/") {
						existingFiles = append(existingFiles, item.Path)
					}
				}
			}
		}
	}

	type treeEntry struct {
		Path string  `json:"path"`
		Mode string  `json:"mode"`
		Type string  `json:"type"`
		SHA  *string `json:"sha"`
	}
	var newTreeEntries []treeEntry

	// Mirror deletions: files that existed in previous tree under our sync scope but are now removed
	for _, oldPath := range existingFiles {
		if _, exists := files[oldPath]; !exists {
			// In GitHub API, passing sha: nil / omitting sha deletes the file from the tree
			newTreeEntries = append(newTreeEntries, treeEntry{
				Path: oldPath,
				Mode: "100644",
				Type: "blob",
				SHA:  nil,
			})
		}
	}

	// 7. Create blobs for active files
	for filePath, content := range files {
		blobSHA, err := createGitHubBlob(ctx, client, apiBaseURL, repo, cfg.GitHubToken, content)
		if err != nil {
			return nil, fmt.Errorf("failed to create blob for %s: %w", filePath, err)
		}
		newTreeEntries = append(newTreeEntries, treeEntry{
			Path: filePath,
			Mode: "100644",
			Type: "blob",
			SHA:  &blobSHA,
		})
	}

	// 8. Create new tree
	createTreeURL := fmt.Sprintf("%s/repos/%s/git/trees", apiBaseURL, repo)
	createTreeBody, _ := json.Marshal(map[string]any{
		"base_tree": baseTreeSHA,
		"tree":      newTreeEntries,
	})
	createTreeReq, err := http.NewRequestWithContext(ctx, "POST", createTreeURL, bytes.NewReader(createTreeBody))
	if err != nil {
		return nil, err
	}
	setGitHubHeaders(createTreeReq, cfg.GitHubToken)

	createTreeResp, err := client.Do(createTreeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create tree: %w", err)
	}
	defer createTreeResp.Body.Close()

	if createTreeResp.StatusCode != http.StatusCreated && createTreeResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(createTreeResp.Body)
		return nil, fmt.Errorf("failed to create git tree (HTTP %d): %s", createTreeResp.StatusCode, string(b))
	}

	var newTreeData struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(createTreeResp.Body).Decode(&newTreeData); err != nil {
		return nil, fmt.Errorf("failed to parse new tree response: %w", err)
	}

	now := time.Now()

	// If tree is identical, no changes to commit
	if newTreeData.SHA == baseTreeSHA {
		cfg.LastSyncAt = &now
		cfg.LastSyncStatus = "synced"
		cfg.LastCommitSHA = latestCommitSHA
		cfg.LastSyncError = ""
		_ = v.SaveCloudSyncConfig(cfg)
		return &CloudSyncResult{
			CommitSHA:  latestCommitSHA,
			FilesCount: len(files),
			Message:    "Already up to date (no changes detected)",
			SyncedAt:   now,
		}, nil
	}

	// 9. Create commit
	createCommitURL := fmt.Sprintf("%s/repos/%s/git/commits", apiBaseURL, repo)
	commitMsg := fmt.Sprintf("chore(sync): update %d notes and snippets from SqlFox", len(files))
	createCommitBody, _ := json.Marshal(map[string]any{
		"message": commitMsg,
		"tree":    newTreeData.SHA,
		"parents": []string{latestCommitSHA},
	})
	createCommitReq, err := http.NewRequestWithContext(ctx, "POST", createCommitURL, bytes.NewReader(createCommitBody))
	if err != nil {
		return nil, err
	}
	setGitHubHeaders(createCommitReq, cfg.GitHubToken)

	createCommitResp, err := client.Do(createCommitReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}
	defer createCommitResp.Body.Close()

	if createCommitResp.StatusCode != http.StatusCreated && createCommitResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(createCommitResp.Body)
		return nil, fmt.Errorf("failed to create git commit (HTTP %d): %s", createCommitResp.StatusCode, string(b))
	}

	var newCommitData struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(createCommitResp.Body).Decode(&newCommitData); err != nil {
		return nil, fmt.Errorf("failed to parse new commit response: %w", err)
	}

	// 10. Update branch ref
	updateRefURL := fmt.Sprintf("%s/repos/%s/git/refs/heads/%s", apiBaseURL, repo, branch)
	updateRefBody, _ := json.Marshal(map[string]any{
		"sha": newCommitData.SHA,
	})
	updateRefReq, err := http.NewRequestWithContext(ctx, "PATCH", updateRefURL, bytes.NewReader(updateRefBody))
	if err != nil {
		return nil, err
	}
	setGitHubHeaders(updateRefReq, cfg.GitHubToken)

	updateRefResp, err := client.Do(updateRefReq)
	if err != nil {
		return nil, fmt.Errorf("failed to update branch ref: %w", err)
	}
	defer updateRefResp.Body.Close()

	if updateRefResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(updateRefResp.Body)
		return nil, fmt.Errorf("failed to update branch ref to %s (HTTP %d): %s", newCommitData.SHA, updateRefResp.StatusCode, string(b))
	}

	// 11. Persist sync timestamp and status to vault
	cfg.LastSyncAt = &now
	cfg.LastSyncStatus = "synced"
	cfg.LastCommitSHA = newCommitData.SHA
	cfg.LastSyncError = ""
	_ = v.SaveCloudSyncConfig(cfg)

	return &CloudSyncResult{
		CommitSHA:  newCommitData.SHA,
		FilesCount: len(files),
		Message:    commitMsg,
		SyncedAt:   now,
	}, nil
}

func createGitHubBlob(ctx context.Context, client *http.Client, apiBaseURL, repo, token string, content []byte) (string, error) {
	blobURL := fmt.Sprintf("%s/repos/%s/git/blobs", apiBaseURL, repo)
	b64Content := base64.StdEncoding.EncodeToString(content)
	body, _ := json.Marshal(map[string]string{
		"content":  b64Content,
		"encoding": "base64",
	})

	req, err := http.NewRequestWithContext(ctx, "POST", blobURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	setGitHubHeaders(req, token)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github blob creation failed (HTTP %d): %s", resp.StatusCode, string(raw))
	}

	var res struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.SHA, nil
}

func createInitialCommit(ctx context.Context, client *http.Client, apiBaseURL, repo, branch, token string) (string, error) {
	readmeURL := fmt.Sprintf("%s/repos/%s/contents/README.md", apiBaseURL, repo)
	readmeContent := base64.StdEncoding.EncodeToString([]byte("# SqlFox Notes\nSynchronized with SqlFox.\n"))
	body, _ := json.Marshal(map[string]any{
		"message": "Initial commit from SqlFox",
		"content": readmeContent,
		"branch":  branch,
	})

	req, err := http.NewRequestWithContext(ctx, "PUT", readmeURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	setGitHubHeaders(req, token)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create initial README (HTTP %d): %s", resp.StatusCode, string(raw))
	}

	var res struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Commit.SHA, nil
}

func setGitHubHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "SqlFox-Client")
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
}
