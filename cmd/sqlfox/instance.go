package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type instanceInfo struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"` // 0 while the owning process is still starting up
	Host      string    `json:"host,omitempty"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

const instanceStartupWait = 15 * time.Second // a claimed-but-not-yet-listening instance (vault open, bind)

// ponytail: Lightweight single-instance guard using lock file + health check.
// Eliminates duplicate process port sprawl across platforms without extra dependencies.
func checkExistingInstance(configDir string, defaultPort int) (string, int, bool) {
	lockPath := filepath.Join(configDir, "sqlfox.lock")
	deadline := time.Now().Add(instanceStartupWait)

	// 1. Check state file if present
	for {
		data, err := os.ReadFile(lockPath)
		if err != nil {
			break
		}
		var info instanceInfo
		if err := json.Unmarshal(data, &info); err != nil {
			_ = os.Remove(lockPath) // corrupt lock
			break
		}
		if info.PID > 0 && !isPIDAlive(info.PID) {
			// Process is dead: remove stale lock immediately
			_ = os.Remove(lockPath)
			break
		}
		if info.Port <= 0 {
			// Another instance claimed the lock and is still binding its port: wait for it.
			if time.Now().After(deadline) {
				_ = os.Remove(lockPath)
				break
			}
			time.Sleep(150 * time.Millisecond)
			continue
		}
		// Retry once: a busy (but alive) instance must not lose its lock to a single slow probe.
		if pingSqlFoxHost(info.Host, info.Port) || pingSqlFoxHost(info.Host, info.Port) {
			return info.URL, info.PID, true
		}
		// Stale lock: previous process terminated abnormally or port occupied by another service
		_ = os.Remove(lockPath)
		break
	}

	// 2. Fallback: probe defaultPort directly in case lock file was removed or missing
	if defaultPort > 0 && pingSqlFox(defaultPort) {
		scheme := "http"
		if probeSqlFoxScheme("https", "127.0.0.1", defaultPort) {
			scheme = "https"
		}
		return fmt.Sprintf("%s://localhost:%d", scheme, defaultPort), 0, true
	}

	return "", 0, false
}

// claimInstanceLock atomically creates the lock file (O_EXCL) before startup, so two launches at
// the same moment (e.g. a double-click) cannot both pass the check. It returns os.ErrExist if
// another process holds the lock. The release func only removes a lock this process still owns.
func claimInstanceLock(configDir string) (func(), error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return func() {}, err
	}
	lockPath := filepath.Join(configDir, "sqlfox.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return func() {}, err
	}
	data, _ := json.Marshal(instanceInfo{PID: os.Getpid(), CreatedAt: time.Now()})
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(lockPath)
		return func() {}, fmt.Errorf("write instance lock: %v %v", werr, cerr)
	}
	return func() { removeOwnLock(lockPath) }, nil
}

// removeOwnLock deletes the lock only if it still records this process, so an exiting instance
// never deletes a lock that a newer instance has taken over.
func removeOwnLock(lockPath string) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return
	}
	var info instanceInfo
	if json.Unmarshal(data, &info) == nil && info.PID != os.Getpid() {
		return
	}
	_ = os.Remove(lockPath)
}

// isPIDAlive checks if a process with the specified PID is currently alive.
// ponytail: Uses signal 0 error-checking on POSIX; zero runtime overhead without external CLI calls.
func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return true // Defer to loopback ping on Windows
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func pingSqlFox(port int) bool { return pingSqlFoxHost("", port) }

// pingSqlFoxHost probes the instance on the address it is bound to; wildcard/empty binds are
// reachable on loopback, but an instance bound to a specific LAN IP is not.
// Supports both plaintext HTTP and TLS listeners.
func pingSqlFoxHost(host string, port int) bool {
	if probeSqlFoxScheme("http", host, port) {
		return true
	}
	return probeSqlFoxScheme("https", host, port)
}

func probeSqlFoxScheme(scheme, host string, port int) bool {
	if host == "" || host == "0.0.0.0" || host == "::" || host == "localhost" {
		host = "127.0.0.1"
	}
	client := &http.Client{
		Timeout: 750 * time.Millisecond,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	url := fmt.Sprintf("%s://%s/api/auth/status", scheme, net.JoinHostPort(host, strconv.Itoa(port)))
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Host = host
	if strings.Contains(host, ":") {
		req.Host = "[" + host + "]" // IPv6 literal
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var status struct {
		App      string `json:"app"`
		IsRemote bool   `json:"is_remote"`
		Unlocked bool   `json:"unlocked"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err == nil {
		// Strictly verify this is SqlFox and not a third-party application on the same port
		isSqlFox := status.App == "sqlfox" || resp.Header.Get("X-SqlFox-App") == "sqlfox"
		// Probing a LAN-bound instance on its LAN IP is seen as a remote client (locked), which
		// still proves the instance is alive.
		return isSqlFox && (status.Unlocked || status.IsRemote)
	}
	return false
}

// writeInstanceLock records the bound port/host in the lock (atomically via rename) once listening.
func writeInstanceLock(configDir string, port int, localURL string, bindHost ...string) (func(), error) {
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
	if len(bindHost) > 0 {
		info.Host = bindHost[0]
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return func() {}, err
	}
	tmp := lockPath + fmt.Sprintf(".%d.tmp", os.Getpid())
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return func() {}, err
	}
	if err := os.Rename(tmp, lockPath); err != nil {
		_ = os.Remove(tmp)
		return func() {}, err
	}
	return func() { removeOwnLock(lockPath) }, nil
}

// stopExistingInstance terminates a running background SqlFox instance recorded in sqlfox.lock.
// If the lock file is missing but an instance responds on defaultPort, it attempts graceful shutdown via loopback HTTP API.
func stopExistingInstance(configDir string, defaultPort int) error {
	lockPath := filepath.Join(configDir, "sqlfox.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if defaultPort > 0 && pingSqlFox(defaultPort) {
			scheme := "http"
			if probeSqlFoxScheme("https", "127.0.0.1", defaultPort) {
				scheme = "https"
			}
			client := &http.Client{
				Timeout: 2 * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			}
			resp, postErr := client.Post(fmt.Sprintf("%s://127.0.0.1:%d/api/system/shutdown", scheme, defaultPort), "application/json", nil)
			if postErr == nil {
				_ = resp.Body.Close()
				return nil
			}
		}
		return fmt.Errorf("no running SqlFox instance found (lock file missing)")
	}

	var info instanceInfo
	if err := json.Unmarshal(data, &info); err != nil {
		_ = os.Remove(lockPath)
		return fmt.Errorf("invalid lock file format; cleared stale lock")
	}

	if info.PID <= 0 {
		_ = os.Remove(lockPath)
		return fmt.Errorf("no valid PID recorded in lock file")
	}

	// If process is already terminated and port is quiet, clean up stale lock immediately
	if !isPIDAlive(info.PID) && (info.Port <= 0 || !pingSqlFoxHost(info.Host, info.Port)) {
		_ = os.Remove(lockPath)
		return fmt.Errorf("SqlFox (PID %d) was not running; cleared stale lock", info.PID)
	}

	proc, err := os.FindProcess(info.PID)
	if err != nil {
		_ = os.Remove(lockPath)
		return fmt.Errorf("process with PID %d not found: %w", info.PID, err)
	}

	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(info.PID)).Run()
	} else {
		_ = proc.Signal(syscall.SIGTERM)
	}

	// Wait up to 2 seconds for clean shutdown
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !isPIDAlive(info.PID) || (info.Port > 0 && !pingSqlFoxHost(info.Host, info.Port)) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if isPIDAlive(info.PID) {
		_ = proc.Kill()
	}

	_ = os.Remove(lockPath)
	return nil
}
