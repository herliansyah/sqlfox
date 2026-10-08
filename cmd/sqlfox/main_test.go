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

