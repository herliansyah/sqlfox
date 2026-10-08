package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sqlfox/internal/vault"
)

type cloudSyncConfigResponse struct {
	Enabled            bool       `json:"enabled"`
	HasToken           bool       `json:"has_token"`
	MaskedToken        string     `json:"masked_token"`
	GitHubRepo         string     `json:"github_repo"`
	GitHubBranch       string     `json:"github_branch"`
	PathPrefix         string     `json:"path_prefix"`
	IncludeVaultBackup bool       `json:"include_vault_backup"`
	LastSyncAt         *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus     string     `json:"last_sync_status"`
	LastSyncError      string     `json:"last_sync_error,omitempty"`
	LastCommitSHA      string     `json:"last_commit_sha,omitempty"`
}

type saveCloudSyncConfigRequest struct {
	Enabled            bool   `json:"enabled"`
	GitHubToken        string `json:"github_token"`
	GitHubRepo         string `json:"github_repo"`
	GitHubBranch       string `json:"github_branch"`
	PathPrefix         string `json:"path_prefix"`
	IncludeVaultBackup bool   `json:"include_vault_backup"`
}

type testCloudSyncRequest struct {
	GitHubToken  string `json:"github_token"`
	GitHubRepo   string `json:"github_repo"`
	GitHubBranch string `json:"github_branch"`
}

func maskToken(t string) string {
	if t == "" {
		return ""
	}
	if len(t) <= 8 {
		return "••••••••"
	}
	return t[:4] + "••••••••" + t[len(t)-4:]
}

func (s *Server) handleGetCloudSyncConfig(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	cfg, err := s.vault.GetCloudSyncConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get cloud sync config: "+err.Error())
		return
	}

	resp := cloudSyncConfigResponse{
		Enabled:            cfg.Enabled,
		HasToken:           cfg.GitHubToken != "",
		MaskedToken:        maskToken(cfg.GitHubToken),
		GitHubRepo:         cfg.GitHubRepo,
		GitHubBranch:       cfg.GitHubBranch,
		PathPrefix:         cfg.PathPrefix,
		IncludeVaultBackup: cfg.IncludeVaultBackup,
		LastSyncAt:         cfg.LastSyncAt,
		LastSyncStatus:     cfg.LastSyncStatus,
		LastSyncError:      cfg.LastSyncError,
		LastCommitSHA:      cfg.LastCommitSHA,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSaveCloudSyncConfig(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	var req saveCloudSyncConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	currentCfg, err := s.vault.GetCloudSyncConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read existing config: "+err.Error())
		return
	}

	currentCfg.Enabled = req.Enabled
	currentCfg.GitHubRepo = strings.TrimSpace(req.GitHubRepo)
	if req.GitHubBranch != "" {
		currentCfg.GitHubBranch = strings.TrimSpace(req.GitHubBranch)
	}
	currentCfg.PathPrefix = strings.Trim(req.PathPrefix, "/")
	currentCfg.IncludeVaultBackup = req.IncludeVaultBackup

	// Only update token if a new non-masked token is sent
	if req.GitHubToken != "" && !strings.Contains(req.GitHubToken, "••••") {
		currentCfg.GitHubToken = strings.TrimSpace(req.GitHubToken)
	}

	if err := s.vault.SaveCloudSyncConfig(currentCfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save cloud sync config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Cloud sync configuration saved successfully",
	})
}

func (s *Server) handleTestCloudSync(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	var req testCloudSyncRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	token := strings.TrimSpace(req.GitHubToken)
	repo := strings.TrimSpace(req.GitHubRepo)
	branch := strings.TrimSpace(req.GitHubBranch)

	// If token omitted or masked, load saved token
	if token == "" || strings.Contains(token, "••••") {
		savedCfg, err := s.vault.GetCloudSyncConfig()
		if err == nil {
			token = savedCfg.GitHubToken
			if repo == "" {
				repo = savedCfg.GitHubRepo
			}
			if branch == "" {
				branch = savedCfg.GitHubBranch
			}
		}
	}

	if token == "" {
		writeError(w, http.StatusBadRequest, "GitHub token is required for test")
		return
	}
	if repo == "" {
		writeError(w, http.StatusBadRequest, "GitHub repository (owner/repo) is required for test")
		return
	}

	savedCfg, _ := s.vault.GetCloudSyncConfig()
	apiBaseURL := ""
	if savedCfg != nil {
		apiBaseURL = savedCfg.APIBaseURL
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := vault.TestGitHubAccess(ctx, apiBaseURL, token, repo, branch); err != nil {
		writeError(w, http.StatusBadRequest, "Connection test failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Repository access verified successfully",
	})
}

func (s *Server) handleRunCloudSync(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	cfg, err := s.vault.GetCloudSyncConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read cloud sync config: "+err.Error())
		return
	}

	if cfg.GitHubToken == "" {
		writeError(w, http.StatusBadRequest, "GitHub token not configured")
		return
	}
	if cfg.GitHubRepo == "" {
		writeError(w, http.StatusBadRequest, "GitHub repository not configured")
		return
	}

	cfg.LastSyncStatus = "syncing"
	_ = s.vault.SaveCloudSyncConfig(cfg)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	res, err := vault.SyncToGitHub(ctx, s.vault, cfg, s.masterPassword)
	if err != nil {
		cfg.LastSyncStatus = "error"
		cfg.LastSyncError = err.Error()
		_ = s.vault.SaveCloudSyncConfig(cfg)
		writeError(w, http.StatusInternalServerError, "Sync failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetCloudSyncStatus(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	cfg, err := s.vault.GetCloudSyncConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read cloud sync config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":         cfg.Enabled,
		"status":          cfg.LastSyncStatus,
		"last_sync_at":    cfg.LastSyncAt,
		"last_commit_sha": cfg.LastCommitSHA,
		"last_error":      cfg.LastSyncError,
		"repo":            cfg.GitHubRepo,
	})
}

func (s *Server) handleExportNotesBundle(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	zipBytes, err := vault.ExportNotesBundle(s.vault)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export notes bundle: "+err.Error())
		return
	}

	filename := fmt.Sprintf("sqlfox-notes-bundle-%s.zip", time.Now().Format("2006-01-02-150405"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBytes)
}

func (s *Server) handleImportNotesBundle(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	var zipData []byte
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(50 << 20); err != nil { // 50MB max
			writeError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
			return
		}
		file, _, err := r.FormFile("bundle")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing 'bundle' form file: "+err.Error())
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read upload file: "+err.Error())
			return
		}
		zipData = data
	} else {
		// Raw binary body
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read request body: "+err.Error())
			return
		}
		zipData = data
	}

	if len(zipData) == 0 {
		writeError(w, http.StatusBadRequest, "empty zip bundle data")
		return
	}

	importedCount, err := vault.ImportNotesBundle(s.vault, zipData)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to import bundle: "+err.Error())
		return
	}

	// Trigger cloud sync notification if enabled
	s.notifyCloudSyncChange()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"imported_count": importedCount,
		"message":        fmt.Sprintf("Successfully imported %d notes and snippets", importedCount),
	})
}

// notifyCloudSyncChange debounces background synchronization after changes to notes or snippets.
func (s *Server) notifyCloudSyncChange() {
	if s.vault == nil {
		return
	}

	cfg, err := s.vault.GetCloudSyncConfig()
	if err != nil || !cfg.Enabled || cfg.GitHubToken == "" || cfg.GitHubRepo == "" {
		return
	}

	s.cloudSyncMu.Lock()
	defer s.cloudSyncMu.Unlock()

	cfg.LastSyncStatus = "pending"
	_ = s.vault.SaveCloudSyncConfig(cfg)

	if s.cloudSyncTimer != nil {
		s.cloudSyncTimer.Stop()
	}

	s.cloudSyncTimer = time.AfterFunc(30*time.Second, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		currentCfg, err := s.vault.GetCloudSyncConfig()
		if err != nil || !currentCfg.Enabled || currentCfg.GitHubToken == "" || currentCfg.GitHubRepo == "" {
			return
		}

		currentCfg.LastSyncStatus = "syncing"
		_ = s.vault.SaveCloudSyncConfig(currentCfg)

		_, syncErr := vault.SyncToGitHub(ctx, s.vault, currentCfg, s.masterPassword)
		if syncErr != nil {
			currentCfg.LastSyncStatus = "error"
			currentCfg.LastSyncError = syncErr.Error()
			_ = s.vault.SaveCloudSyncConfig(currentCfg)
		}
	})
}
