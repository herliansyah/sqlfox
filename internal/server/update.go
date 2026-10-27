package server

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitHubRelease represents the minimal schema needed from GitHub Releases API.
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a single binary release artifact.
type GitHubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// UpdateInfo holds the version diff, changelog, and target asset details.
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseName    string `json:"release_name"`
	ReleaseNotes   string `json:"release_notes"`
	PublishedAt    string `json:"published_at"`
	AssetURL       string `json:"asset_url"`
	AssetName      string `json:"asset_name"`
	DownloadSize   int64  `json:"download_size"`
}

// ApplyUpdateResult represents the outcome of an in-app binary replacement.
type ApplyUpdateResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	NewVersion string `json:"new_version"`
	Restarting bool   `json:"restarting"`
}

var (
	updateCacheMu sync.Mutex
	cachedInfo    *UpdateInfo
	cachedAt      time.Time
)

const updateCacheTTL = 24 * time.Hour
const githubReleasesURL = "https://api.github.com/repos/herliansyah/sqlfox/releases/latest"

// SetVersion sets the runtime version string of the server.
func (s *Server) SetVersion(v string) {
	if v != "" {
		s.version = v
	}
}

// GetVersion returns the runtime version string of the server.
func (s *Server) GetVersion() string {
	if s.version == "" {
		return "v0.1.0"
	}
	return s.version
}

// compareSemver returns:
//
//	 1 if v1 > v2
//	-1 if v1 < v2
//	 0 if v1 == v2
func compareSemver(v1, v2 string) int {
	clean := func(v string) []int {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		v = strings.TrimPrefix(v, "V")
		// Strip prerelease metadata like "-beta.1"
		if idx := strings.IndexAny(v, "-+"); idx != -1 {
			v = v[:idx]
		}
		parts := strings.Split(v, ".")
		res := make([]int, 3)
		for i := 0; i < len(parts) && i < 3; i++ {
			if n, err := strconv.Atoi(parts[i]); err == nil {
				res[i] = n
			}
		}
		return res
	}

	p1 := clean(v1)
	p2 := clean(v2)

	for i := 0; i < 3; i++ {
		if p1[i] > p2[i] {
			return 1
		}
		if p1[i] < p2[i] {
			return -1
		}
	}
	return 0
}

// findMatchingAsset finds the appropriate release archive for the current runtime OS and architecture.
func findMatchingAsset(assets []GitHubAsset, goos, goarch string) *GitHubAsset {
	expectedExt := ".tar.gz"
	if goos == "windows" {
		expectedExt = ".zip"
	}

	for _, a := range assets {
		lower := strings.ToLower(a.Name)
		if strings.Contains(lower, goos) && strings.Contains(lower, goarch) && strings.HasSuffix(lower, expectedExt) {
			return &a
		}
	}
	return nil
}

// FetchLatestRelease fetches release metadata from GitHub or from a custom URL if provided.
func FetchLatestRelease(ctx context.Context, apiURL string) (*GitHubRelease, error) {
	if apiURL == "" {
		apiURL = githubReleasesURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update request: %w", err)
	}
	req.Header.Set("User-Agent", "SqlFox-Update-Engine")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found on GitHub")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status: %s", resp.Status)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub release response: %w", err)
	}
	return &rel, nil
}

// handleCheckUpdate handles GET /api/system/update/check?force=true
func (s *Server) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true"
	currentVer := s.GetVersion()

	updateCacheMu.Lock()
	if !force && cachedInfo != nil && time.Since(cachedAt) < updateCacheTTL {
		info := *cachedInfo
		updateCacheMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
		return
	}
	updateCacheMu.Unlock()

	rel, err := FetchLatestRelease(r.Context(), s.updateAPIURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"error":           err.Error(),
			"current_version": currentVer,
			"has_update":      false,
		})
		return
	}

	hasUpdate := compareSemver(rel.TagName, currentVer) > 0
	targetAsset := findMatchingAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)

	info := UpdateInfo{
		CurrentVersion: currentVer,
		LatestVersion:  rel.TagName,
		HasUpdate:      hasUpdate,
		ReleaseName:    rel.Name,
		ReleaseNotes:   rel.Body,
		PublishedAt:    rel.PublishedAt.Format(time.RFC3339),
	}

	if targetAsset != nil {
		info.AssetURL = targetAsset.BrowserDownloadURL
		info.AssetName = targetAsset.Name
		info.DownloadSize = targetAsset.Size
	}

	updateCacheMu.Lock()
	cachedInfo = &info
	cachedAt = time.Now()
	updateCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleApplyUpdate handles POST /api/system/update/apply
func (s *Server) handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetURL string `json:"asset_url"`
		Version  string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AssetURL == "" {
		http.Error(w, `{"error":"asset_url is required"}`, http.StatusBadRequest)
		return
	}

	// Origin verification (Release Manifest Verification policy)
	if !s.isAllowedAssetURL(req.AssetURL) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "asset_url rejected: binary downloads must originate strictly from the official SqlFox GitHub release repository",
		})
		return
	}

	// Determine current running executable path
	var exePath string
	var err error
	if s.targetExePath != "" {
		exePath = s.targetExePath
	} else {
		exePath, err = os.Executable()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("cannot resolve executable path: %v", err)})
			return
		}
		exePath, err = filepath.EvalSymlinks(exePath)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("cannot resolve symlink: %v", err)})
			return
		}
	}

	// Verify write permission in the target directory
	dir := filepath.Dir(exePath)
	tempTestFile := filepath.Join(dir, fmt.Sprintf(".sqlfox-perm-test-%d", time.Now().UnixNano()))
	if err := os.WriteFile(tempTestFile, []byte("ok"), 0600); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("permission denied writing to %s. Please run with adequate privileges or update manually.", dir),
		})
		return
	}
	_ = os.Remove(tempTestFile)

	// Download and extract the executable to a temporary file in the same directory
	tempExePath := filepath.Join(dir, fmt.Sprintf(".sqlfox-update-%d", time.Now().UnixNano()))
	if runtime.GOOS == "windows" {
		tempExePath += ".exe"
	}

	if err := s.downloadAndExtractBinary(r.Context(), req.AssetURL, tempExePath); err != nil {
		_ = os.Remove(tempExePath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("failed to extract update: %v", err)})
		return
	}

	// Ensure executable permissions
	if err := os.Chmod(tempExePath, 0755); err != nil {
		_ = os.Remove(tempExePath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("failed to set executable permission: %v", err)})
		return
	}

	// Atomic replace
	if runtime.GOOS == "windows" {
		oldExe := exePath + ".old"
		_ = os.Remove(oldExe)
		if err := os.Rename(exePath, oldExe); err != nil {
			_ = os.Remove(tempExePath)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("failed to rename current binary on Windows: %v", err)})
			return
		}
		if err := os.Rename(tempExePath, exePath); err != nil {
			// Rollback
			_ = os.Rename(oldExe, exePath)
			_ = os.Remove(tempExePath)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("failed to move new binary into place: %v", err)})
			return
		}
	} else {
		if err := os.Rename(tempExePath, exePath); err != nil {
			_ = os.Remove(tempExePath)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("failed to replace binary: %v", err)})
			return
		}
	}

	// Clear cache so next check sees fresh state
	updateCacheMu.Lock()
	cachedInfo = nil
	updateCacheMu.Unlock()

	// Return success to the client before restarting
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ApplyUpdateResult{
		Success:    true,
		Message:    fmt.Sprintf("SqlFox successfully updated to %s", req.Version),
		NewVersion: req.Version,
		Restarting: true,
	})

	// Trigger restart
	if s.restartFn != nil {
		go func() {
			_ = s.restartFn()
		}()
	} else {
		go func() {
			time.Sleep(500 * time.Millisecond)
			cmd := exec.Command(exePath, os.Args[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			if err := cmd.Start(); err == nil {
				os.Exit(0)
			}
		}()
	}
}

func (s *Server) isAllowedAssetURL(rawURL string) bool {
	// Compare the parsed, dot-segment-free path: a raw prefix check would accept
	// ".../releases/download/../../../other/repo/..." which escapes the official repository.
	if u, err := url.Parse(rawURL); err == nil && u.Scheme == "https" && u.Host == "github.com" &&
		u.RawQuery == "" && u.Fragment == "" && u.User == nil &&
		!strings.Contains(u.Path, "..") && !strings.Contains(u.EscapedPath(), "%2") &&
		path.Clean(u.Path) == u.Path && strings.HasPrefix(u.Path, "/herliansyah/sqlfox/releases/download/") {
		return true
	}
	if s.updateAPIURL != "" {
		if u, err := url.Parse(s.updateAPIURL); err == nil {
			if au, err := url.Parse(rawURL); err == nil && au.Host == u.Host {
				return true
			}
		}
	}
	return false
}

func (s *Server) verifyAssetChecksum(ctx context.Context, assetURL string, archiveBytes []byte) error {
	idx := strings.LastIndex(assetURL, "/")
	if idx == -1 {
		return fmt.Errorf("invalid asset URL format")
	}
	baseDir := assetURL[:idx]
	assetName := assetURL[idx+1:]
	if assetName == "" {
		return fmt.Errorf("invalid asset URL: missing file name")
	}
	manifestURL := baseDir + "/checksums.txt"

	fetchManifest := func(targetURL string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "SqlFox-Update-Engine")
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("manifest HTTP status: %s", resp.Status)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	}

	manifestData, err := fetchManifest(manifestURL)
	if err != nil {
		manifestData, err = fetchManifest(baseDir + "/SHA256SUMS")
		if err != nil {
			return fmt.Errorf("release checksum manifest not found (fail-closed): %w", err)
		}
	}

	actualHash := sha256.Sum256(archiveBytes)
	actualHex := hex.EncodeToString(actualHash[:])

	lines := strings.Split(string(manifestData), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			expectedHash := strings.ToLower(fields[0])
			fileName := filepath.Base(strings.TrimPrefix(fields[1], "*"))
			if fileName == assetName {
				if subtle.ConstantTimeCompare([]byte(strings.ToLower(actualHex)), []byte(expectedHash)) != 1 {
					return fmt.Errorf("SHA-256 digest mismatch for %s: expected %s, got %s", assetName, expectedHash, actualHex)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("asset %s not found in release checksum manifest", assetName)
}

// downloadAndExtractBinary downloads the archive from assetURL, verifies the checksum, and extracts the sqlfox binary.
func (s *Server) downloadAndExtractBinary(ctx context.Context, assetURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "SqlFox-Update-Engine")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download release asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP status %s", resp.Status)
	}

	archiveBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read downloaded archive: %w", err)
	}

	// Release Manifest Verification (fail-closed)
	if err := s.verifyAssetChecksum(ctx, assetURL, archiveBytes); err != nil {
		return fmt.Errorf("supply chain verification failed: %w", err)
	}

	lower := strings.ToLower(assetURL)
	if strings.HasSuffix(lower, ".zip") {
		return extractFromZipStream(bytes.NewReader(archiveBytes), destPath)
	}
	return extractFromTarGzStream(bytes.NewReader(archiveBytes), destPath)
}

// extractFromTarGzStream extracts the sqlfox or sqlfox.exe binary from a .tar.gz stream.
func extractFromTarGzStream(r io.Reader, destPath string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader error: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		baseName := filepath.Base(header.Name)
		if baseName == "sqlfox" || baseName == "sqlfox.exe" {
			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("cannot create destination binary: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tr); err != nil {
				return fmt.Errorf("failed writing extracted binary: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("sqlfox binary not found in tar.gz archive")
}

// extractFromZipStream saves the zip stream to a temp file and extracts the executable.
func extractFromZipStream(r io.Reader, destPath string) error {
	tmpZip, err := os.CreateTemp("", "sqlfox-update-*.zip")
	if err != nil {
		return fmt.Errorf("cannot create temp zip file: %w", err)
	}
	defer func() {
		tmpZip.Close()
		_ = os.Remove(tmpZip.Name())
	}()

	if _, err := io.Copy(tmpZip, r); err != nil {
		return fmt.Errorf("failed writing zip temp file: %w", err)
	}
	_ = tmpZip.Close()

	zr, err := zip.OpenReader(tmpZip.Name())
	if err != nil {
		return fmt.Errorf("cannot open zip archive: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		baseName := filepath.Base(f.Name)
		if baseName == "sqlfox" || baseName == "sqlfox.exe" {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("cannot open zip entry: %w", err)
			}
			defer rc.Close()

			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("cannot create destination binary: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, rc); err != nil {
				return fmt.Errorf("failed writing extracted binary: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("sqlfox binary not found in zip archive")
}

// handleGetChangelog handles GET /api/system/changelog
func (s *Server) handleGetChangelog(w http.ResponseWriter, r *http.Request) {
	// ponytail: read local CHANGELOG.md during dev/tests, fallback to embedded asset
	var content []byte
	var err error

	for _, p := range []string{"CHANGELOG.md", "../../CHANGELOG.md"} {
		if data, readErr := os.ReadFile(p); readErr == nil && len(data) > 0 {
			content = data
			err = nil
			break
		}
	}

	if (err != nil || len(content) == 0) && s.assets != nil {
		if f, openErr := s.assets.Open("CHANGELOG.md"); openErr == nil {
			defer f.Close()
			content, err = io.ReadAll(f)
		}
	}

	if err != nil || len(content) == 0 {
		writeError(w, http.StatusNotFound, "Changelog not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"version":   s.GetVersion(),
		"changelog": string(content),
	})
}
