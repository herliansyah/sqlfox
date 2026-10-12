package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

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

