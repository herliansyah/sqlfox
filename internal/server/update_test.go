package server

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.1.0", 1},
		{"v1.0.0", "v1.0.0", 0},
		{"0.1.0", "v0.1.0", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v2.0.0", "v1.99.99", 1},
		{"v0.2.0-beta.1", "v0.1.9", 1},
	}

	for _, tt := range tests {
		got := compareSemver(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("compareSemver(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestFindMatchingAsset(t *testing.T) {
	assets := []GitHubAsset{
		{Name: "sqlfox-v0.2.0-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux-amd64.tar.gz"},
		{Name: "sqlfox-v0.2.0-darwin-arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin-arm64.tar.gz"},
		{Name: "sqlfox-v0.2.0-windows-amd64.zip", BrowserDownloadURL: "https://example.com/windows-amd64.zip"},
	}

	matchLinux := findMatchingAsset(assets, "linux", "amd64")
	if matchLinux == nil || matchLinux.Name != "sqlfox-v0.2.0-linux-amd64.tar.gz" {
		t.Fatalf("expected linux-amd64 asset, got %+v", matchLinux)
	}

	matchMac := findMatchingAsset(assets, "darwin", "arm64")
	if matchMac == nil || matchMac.Name != "sqlfox-v0.2.0-darwin-arm64.tar.gz" {
		t.Fatalf("expected darwin-arm64 asset, got %+v", matchMac)
	}

	matchWin := findMatchingAsset(assets, "windows", "amd64")
	if matchWin == nil || matchWin.Name != "sqlfox-v0.2.0-windows-amd64.zip" {
		t.Fatalf("expected windows-amd64 asset, got %+v", matchWin)
	}

	matchNone := findMatchingAsset(assets, "freebsd", "riscv64")
	if matchNone != nil {
		t.Fatalf("expected nil for unsupported os/arch, got %+v", matchNone)
	}
}

func TestUpdateCheckEndpoint(t *testing.T) {
	// Reset cache
	updateCacheMu.Lock()
	cachedInfo = nil
	cachedAt = time.Time{}
	updateCacheMu.Unlock()

	mockRelease := GitHubRelease{
		TagName:     "v0.2.0",
		Name:        "SqlFox v0.2.0 - Auto-Update Release",
		Body:        "## What's Changed\n* Added in-app self-update\n* Added changelog viewer",
		PublishedAt: time.Now(),
		Assets: []GitHubAsset{
			{
				Name:               "sqlfox-v0.2.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz",
				BrowserDownloadURL: "https://example.com/download.tar.gz",
				Size:               12345678,
			},
			{
				Name:               "sqlfox-v0.2.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip",
				BrowserDownloadURL: "https://example.com/download.zip",
				Size:               12345678,
			},
		},
	}

	mockGH := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockRelease)
	}))
	defer mockGH.Close()

	srv := &Server{
		version:      "v0.1.0",
		updateAPIURL: mockGH.URL,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/system/update/check?force=true", nil)
	rr := httptest.NewRecorder()

	srv.handleCheckUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var info UpdateInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !info.HasUpdate {
		t.Errorf("expected HasUpdate = true (current: %s, latest: %s)", info.CurrentVersion, info.LatestVersion)
	}
	if info.LatestVersion != "v0.2.0" {
		t.Errorf("expected LatestVersion 'v0.2.0', got %q", info.LatestVersion)
	}
	if info.ReleaseNotes == "" {
		t.Errorf("expected ReleaseNotes to be populated")
	}
	if info.AssetURL == "" {
		t.Errorf("expected AssetURL to be resolved for current runtime %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func TestExtractFromTarGzStream(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "sqlfox")

	// Create mock .tar.gz with dummy "sqlfox" binary content
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	payload := []byte("#!/bin/sh\necho updated\n")
	hdr := &tar.Header{
		Name: "sqlfox",
		Mode: 0755,
		Size: int64(len(payload)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatalf("failed to write tar body: %v", err)
	}
	tw.Close()
	gzw.Close()

	if err := extractFromTarGzStream(&buf, destPath); err != nil {
		t.Fatalf("extractFromTarGzStream failed: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("extracted content mismatch. got %q, want %q", string(content), string(payload))
	}
}

func TestExtractFromZipStream(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "sqlfox.exe")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	payload := []byte("MZ-dummy-binary")
	f, err := zw.Create("sqlfox.exe")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := f.Write(payload); err != nil {
		t.Fatalf("failed to write payload: %v", err)
	}
	zw.Close()

	if err := extractFromZipStream(&buf, destPath); err != nil {
		t.Fatalf("extractFromZipStream failed: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("extracted content mismatch. got %q, want %q", string(content), string(payload))
	}
}

func TestHandleApplyUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	fakeExe := filepath.Join(tmpDir, "sqlfox")
	if err := os.WriteFile(fakeExe, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("failed to write fake executable: %v", err)
	}

	// Create mock .tar.gz archive
	var archiveBuf bytes.Buffer
	gzw := gzip.NewWriter(&archiveBuf)
	tw := tar.NewWriter(gzw)
	newPayload := []byte("new-binary-v0.2.0")
	hdr := &tar.Header{
		Name: "sqlfox",
		Mode: 0755,
		Size: int64(len(newPayload)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar write header error: %v", err)
	}
	if _, err := tw.Write(newPayload); err != nil {
		t.Fatalf("tar write body error: %v", err)
	}
	tw.Close()
	gzw.Close()

	actualHash := sha256.Sum256(archiveBuf.Bytes())
	hexHash := hex.EncodeToString(actualHash[:])

	mockDownload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(fmt.Sprintf("%s  sqlfox-v0.2.0-linux-amd64.tar.gz\n", hexHash)))
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(archiveBuf.Bytes())
	}))
	defer mockDownload.Close()

	restartChan := make(chan struct{}, 1)
	srv := &Server{
		updateAPIURL:  mockDownload.URL,
		targetExePath: fakeExe,
		restartFn: func() error {
			restartChan <- struct{}{}
			return nil
		},
	}

	body := map[string]string{
		"asset_url": mockDownload.URL + "/sqlfox-v0.2.0-linux-amd64.tar.gz",
		"version":   "v0.2.0",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/system/update/apply", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()

	srv.handleApplyUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handleApplyUpdate returned code %d: %s", rr.Code, rr.Body.String())
	}

	var res ApplyUpdateResult
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response error: %v", err)
	}
	if !res.Success {
		t.Errorf("expected Success = true")
	}

	// Verify binary was replaced
	updatedContent, err := os.ReadFile(fakeExe)
	if err != nil {
		t.Fatalf("failed to read updated binary: %v", err)
	}
	if string(updatedContent) != string(newPayload) {
		t.Errorf("expected binary content %q, got %q", string(newPayload), string(updatedContent))
	}

	// Verify restartFn was triggered without flaking on slow CI runners
	select {
	case <-restartChan:
	case <-time.After(3 * time.Second):
		t.Errorf("expected restartFn to have been called")
	}
}

func TestHandleGetChangelog(t *testing.T) {
	srv := &Server{}
	srv.SetVersion("v0.1.0")

	req := httptest.NewRequest(http.MethodGet, "/api/system/changelog", nil)
	rr := httptest.NewRecorder()

	srv.handleGetChangelog(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if resp["version"] != "v0.1.0" {
		t.Errorf("expected version 'v0.1.0', got %v", resp["version"])
	}

	changelog, ok := resp["changelog"].(string)
	if !ok || len(changelog) == 0 {
		t.Fatalf("expected non-empty changelog string, got %v", resp["changelog"])
	}

	if !strings.Contains(changelog, "Keep a Changelog") {
		t.Errorf("expected changelog to contain 'Keep a Changelog'")
	}
}

func TestRejectUntrustedUpdateOrigin(t *testing.T) {
	srv := &Server{}
	body := map[string]string{
		"asset_url": "http://evil.com/malicious_sqlfox.tar.gz",
		"version":   "v0.9.9",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/system/update/apply", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()

	srv.handleApplyUpdate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for untrusted origin, got: %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestRejectMismatchedChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	fakeExe := filepath.Join(tmpDir, "sqlfox")
	_ = os.WriteFile(fakeExe, []byte("old"), 0755)

	var archiveBuf bytes.Buffer
	gzw := gzip.NewWriter(&archiveBuf)
	tw := tar.NewWriter(gzw)
	payload := []byte("compromised-content")
	_ = tw.WriteHeader(&tar.Header{Name: "sqlfox", Mode: 0755, Size: int64(len(payload))})
	_, _ = tw.Write(payload)
	_ = tw.Close()
	_ = gzw.Close()

	mockDownload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			w.Header().Set("Content-Type", "text/plain")
			// Provide completely wrong hash
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  sqlfox-compromised.tar.gz\n"))
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(archiveBuf.Bytes())
	}))
	defer mockDownload.Close()

	srv := &Server{
		updateAPIURL:  mockDownload.URL,
		targetExePath: fakeExe,
	}

	body := map[string]string{
		"asset_url": mockDownload.URL + "/sqlfox-compromised.tar.gz",
		"version":   "v0.2.0",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/system/update/apply", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()

	srv.handleApplyUpdate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 InternalServerError for checksum mismatch, got: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "digest mismatch") && !strings.Contains(rr.Body.String(), "supply chain verification failed") {
		t.Fatalf("expected digest mismatch error, got: %s", rr.Body.String())
	}
}
