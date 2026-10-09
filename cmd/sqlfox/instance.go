package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type instanceInfo struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// ponytail: Lightweight single-instance guard using lock file + loopback health check.
// Eliminates duplicate process port sprawl across platforms without extra dependencies.
func checkExistingInstance(configDir string, defaultPort int) (string, int, bool) {
	lockPath := filepath.Join(configDir, "sqlfox.lock")

	// 1. Check state file if present
	if data, err := os.ReadFile(lockPath); err == nil {
		var info instanceInfo
		if err := json.Unmarshal(data, &info); err == nil && info.Port > 0 {
			if pingSqlFox(info.Port) {
				return info.URL, info.PID, true
			}
			// Stale lock: previous process terminated abnormally
			_ = os.Remove(lockPath)
		}
	}

	// 2. Fallback: probe defaultPort directly in case lock file was removed or missing
	if defaultPort > 0 && pingSqlFox(defaultPort) {
		return fmt.Sprintf("http://localhost:%d", defaultPort), 0, true
	}

	return "", 0, false
}

func pingSqlFox(port int) bool {
	client := &http.Client{
		Timeout: 350 * time.Millisecond,
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/auth/status", port)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Host = "127.0.0.1"

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var status struct {
		IsRemote bool `json:"is_remote"`
		Unlocked bool `json:"unlocked"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err == nil {
		return status.Unlocked
	}
	return false
}

func writeInstanceLock(configDir string, port int, localURL string) (func(), error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return func() {}, err
	}
	lockPath := filepath.Join(configDir, "sqlfox.lock")
	info := instanceInfo{
		PID:       os.Getpid(),
		Port:      port,
		URL:       localURL,
		CreatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return func() {}, err
	}
	if err := os.WriteFile(lockPath, data, 0600); err != nil {
		return func() {}, err
	}
	cleanup := func() {
		_ = os.Remove(lockPath)
	}
	return cleanup, nil
}
