package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"sqlfox/internal/driver"
)

func TestPortHunting(t *testing.T) {
	// 1. Pre-occupy a port
	l1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()

	busyPort := l1.Addr().(*net.TCPAddr).Port

	// 2. Call listenWithPortHunting starting at busyPort
	l2, pickedPort, err := listenWithPortHunting("127.0.0.1", busyPort, 10)
	if err != nil {
		t.Fatalf("listenWithPortHunting failed: %v", err)
	}
	defer l2.Close()

	if pickedPort == busyPort {
		t.Fatalf("expected port hunting to skip busy port %d, but got %d", busyPort, pickedPort)
	}

	if pickedPort != busyPort+1 {
		t.Logf("picked subsequent free port: %d (busy: %d)", pickedPort, busyPort)
	}
}

func TestPortAutoScanPantauCollision(t *testing.T) {
	// Simulate Pantau or dual-stack service listening on IPv6 loopback [::1]
	// On Windows or dual-stack systems, net.Listen("tcp", "127.0.0.1:port") can bind silently
	// even when another service answers on [::1]:port or wildcard.
	l1, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("skipping IPv6 test: %v", err)
	}
	defer l1.Close()

	busyPort := l1.Addr().(*net.TCPAddr).Port

	// listenWithPortHunting on 127.0.0.1 should detect that busyPort is answering connections
	// and auto-scan past it.
	l2, pickedPort, err := listenWithPortHunting("127.0.0.1", busyPort, 10)
	if err != nil {
		t.Fatalf("listenWithPortHunting failed: %v", err)
	}
	defer l2.Close()

	if pickedPort == busyPort {
		t.Fatalf("BUG REPRODUCED: expected port hunting to detect busy port %d and scan past it, but stubbornly bound to %d without error", busyPort, pickedPort)
	}

	// In strict mode (maxAttempts = 1), listenWithPortHunting should fail-fast
	_, _, err = listenWithPortHunting("127.0.0.1", busyPort, 1)
	if err == nil {
		t.Fatalf("expected strict listenWithPortHunting to fail on busy port %d, but succeeded", busyPort)
	}
}

func TestGetLocalNetworkIP(t *testing.T) {
	ip := getLocalNetworkIP()
	fmt.Printf("Detected LAN IP: %s\n", ip)
}

func TestDriversRegisteredInMain(t *testing.T) {
	for _, driverName := range []string{"mysql", "mariadb", "sqlite", "postgres"} {
		t.Run(driverName, func(t *testing.T) {
			_, err := driver.GetDriver(driverName)
			if err != nil {
				t.Fatalf("❌ Connection failed: %v", err)
			}
		})
	}
}

func TestLoadEnvFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-env-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	envFile := filepath.Join(tmpDir, "sqlfox.env")
	err = os.WriteFile(envFile, []byte("TEST_SQLFOX_VAR=hello_windows\n# comment\nTEST_QUOTED=\"val123\"\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	loadEnvFilesFromPaths(envFile)

	if os.Getenv("TEST_SQLFOX_VAR") != "hello_windows" {
		t.Errorf("expected hello_windows, got %q", os.Getenv("TEST_SQLFOX_VAR"))
	}
	if os.Getenv("TEST_QUOTED") != "val123" {
		t.Errorf("expected val123, got %q", os.Getenv("TEST_QUOTED"))
	}
}

func TestGenerateFriendlyPINEntropy(t *testing.T) {
	pin1, err := generateFriendlyPIN()
	if err != nil {
		t.Fatalf("generateFriendlyPIN error: %v", err)
	}
	pin2, err := generateFriendlyPIN()
	if err != nil {
		t.Fatalf("generateFriendlyPIN error: %v", err)
	}
	if len(pin1) != 10 || len(pin2) != 10 {
		t.Fatalf("expected 10 characters (fox- + 6 chars), got %q and %q", pin1, pin2)
	}
	if pin1[:4] != "fox-" || pin2[:4] != "fox-" {
		t.Fatalf("expected 'fox-' prefix, got %q and %q", pin1, pin2)
	}
	if pin1 == pin2 {
		t.Fatalf("expected high-entropy PINs to differ, got %q == %q", pin1, pin2)
	}
}

func TestWindowsEmbeddedIconConsistency(t *testing.T) {
	// Root-relative path to assets/icon.ico and cmd/sqlfox syso files
	icoPath := filepath.Join("..", "..", "assets", "icon.ico")
	icoData, err := os.ReadFile(icoPath)
	if err != nil {
		t.Fatalf("failed to read assets/icon.ico: %v", err)
	}

	if len(icoData) < 6 {
		t.Fatalf("invalid icon.ico size: %d", len(icoData))
	}

	numImages := int(icoData[4]) | int(icoData[5])<<8
	if numImages == 0 {
		t.Fatalf("icon.ico contains 0 images")
	}

	type frameInfo struct {
		width  int
		height int
		data   []byte
	}
	frames := make([]frameInfo, 0, numImages)

	for i := 0; i < numImages; i++ {
		entryOffset := 6 + i*16
		if entryOffset+16 > len(icoData) {
			t.Fatalf("corrupted ico directory entry %d", i)
		}
		w := int(icoData[entryOffset])
		if w == 0 {
			w = 256
		}
		h := int(icoData[entryOffset+1])
		if h == 0 {
			h = 256
		}
		bytesLen := int(icoData[entryOffset+8]) | int(icoData[entryOffset+9])<<8 | int(icoData[entryOffset+10])<<16 | int(icoData[entryOffset+11])<<24
		imgOffset := int(icoData[entryOffset+12]) | int(icoData[entryOffset+13])<<8 | int(icoData[entryOffset+14])<<16 | int(icoData[entryOffset+15])<<24

		if imgOffset+bytesLen > len(icoData) {
			t.Fatalf("corrupted ico frame %d offsets", i)
		}
		frames = append(frames, frameInfo{
			width:  w,
			height: h,
			data:   icoData[imgOffset : imgOffset+bytesLen],
		})
	}

	sysoFiles := []string{
		"rsrc_windows_amd64.syso",
		"rsrc_windows_arm64.syso",
	}

	for _, sysoFile := range sysoFiles {
		sysoPath := sysoFile
		sysoData, err := os.ReadFile(sysoPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", sysoFile, err)
		}

		for _, frame := range frames {
			// Check if frame raw bytes exist in syso file
			checkLen := len(frame.data)
			if checkLen > 128 {
				checkLen = 128
			}
			if !bytesContains(sysoData, frame.data[:checkLen]) {
				t.Errorf("critical branding bug: %s does not contain the canonical %dx%d icon frame from assets/icon.ico (still embedding old icon)", sysoFile, frame.width, frame.height)
			}
		}
	}
}

func bytesContains(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestStartupBannerAndLoggingOrder(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-banner-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	binName := "sqlfox"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpDir, binName)
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v\nOutput: %s", err, string(out))
	}

	// Case 1: Default execution without logging flags.
	// Expected: Stderr should NOT contain "level=INFO msg=sqlfox_started"
	// Terminal should remain clean with only the banner on stdout.
	{
		cmd := exec.Command(binPath, "-port", "19888", "-multi", "-no-browser", "-db", filepath.Join(tmpDir, "config1.db"))
		var stdoutBuf, stderrBuf safeBuffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start cmd: %v", err)
		}

		// Wait for banner to appear on stdout
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(stdoutBuf.String(), "SqlFox") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		stopProcess(t, cmd)

		stderrStr := stderrBuf.String()
		stdoutStr := stdoutBuf.String()

		if strings.Contains(stderrStr, "level=INFO msg=sqlfox_started") {
			t.Errorf("default startup should NOT print log to console stderr unless logging is explicitly enabled, got stderr:\n%s", stderrStr)
		}
		if !strings.Contains(stdoutStr, "SqlFox") {
			t.Errorf("default startup should print banner to stdout, got stdout: %q, stderr: %q", stdoutStr, stderrStr)
		}
	}

	// Case 2: Explicit verbose flag (-v).
	// Expected: Both banner and log are produced, but banner MUST appear before any log in combined stream.
	{
		cmd := exec.Command(binPath, "-v", "-port", "19889", "-multi", "-no-browser", "-db", filepath.Join(tmpDir, "config2.db"))
		var combined safeBuffer
		cmd.Stdout = &combined
		cmd.Stderr = &combined

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start cmd: %v", err)
		}

		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(combined.String(), "SqlFox") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		stopProcess(t, cmd)

		combinedStr := combined.String()
		bannerIdx := strings.Index(combinedStr, "SqlFox")
		logIdx := strings.Index(combinedStr, "sqlfox_started")

		if bannerIdx == -1 {
			t.Fatalf("expected banner in output, got:\n%s", combinedStr)
		}
		if logIdx == -1 {
			t.Fatalf("expected sqlfox_started log in output when -v is enabled, got:\n%s", combinedStr)
		}
		if logIdx < bannerIdx {
			t.Errorf("BUG REPRODUCED: sqlfox_started log (idx %d) appeared BEFORE banner (idx %d) in output:\n%s", logIdx, bannerIdx, combinedStr)
		}
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func stopProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	_ = cmd.Process.Signal(os.Interrupt)

	select {
	case <-waitCh:
		return
	case <-time.After(2 * time.Second):
	}

	_ = cmd.Process.Kill()

	select {
	case <-waitCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for test process to stop")
	}
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
