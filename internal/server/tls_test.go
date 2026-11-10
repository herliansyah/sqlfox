package server

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTLS_EnsureSelfSignedCertificate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-tls-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	certFile, keyFile, err := EnsureSelfSignedCertificate(tempDir, []string{"192.168.1.100", "sqlfox.local"})
	if err != nil {
		t.Fatalf("EnsureSelfSignedCertificate failed: %v", err)
	}

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Fatalf("cert file %s was not created", certFile)
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Fatalf("key file %s was not created", keyFile)
	}

	// Verify key pair is valid and loadable
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("failed to load generated key pair: %v", err)
	}
	if len(pair.Certificate) == 0 {
		t.Fatalf("no certificate found in key pair")
	}

	// Re-running with existing valid cert should return same paths without regenerating
	certFile2, keyFile2, err := EnsureSelfSignedCertificate(tempDir, []string{"192.168.1.100"})
	if err != nil {
		t.Fatalf("re-running EnsureSelfSignedCertificate failed: %v", err)
	}
	if certFile != certFile2 || keyFile != keyFile2 {
		t.Fatalf("expected identical file paths on re-run, got %s and %s", certFile, certFile2)
	}

	// Re-running with a new LAN IP should automatically regenerate certificate to include new IP in SAN
	certFile3, keyFile3, err := EnsureSelfSignedCertificate(tempDir, []string{"192.168.1.100", "10.0.0.99"})
	if err != nil {
		t.Fatalf("EnsureSelfSignedCertificate with new LAN IP failed: %v", err)
	}
	pair3, err := tls.LoadX509KeyPair(certFile3, keyFile3)
	if err != nil {
		t.Fatalf("failed to load regenerated key pair: %v", err)
	}
	parsed3, err := x509.ParseCertificate(pair3.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse regenerated cert: %v", err)
	}
	foundNewIP := false
	for _, ip := range parsed3.IPAddresses {
		if ip.String() == "10.0.0.99" {
			foundNewIP = true
			break
		}
	}
	if !foundNewIP {
		t.Fatalf("expected 10.0.0.99 to be included in regenerated SAN IPAddresses, got %v", parsed3.IPAddresses)
	}
}

func TestTLS_LoadOrCreateTLSConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-tls-cfg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, certPath, keyPath, err := LoadOrCreateTLSConfig("", "", filepath.Join(tempDir, "certs"), []string{"10.0.0.5"})
	if err != nil {
		t.Fatalf("LoadOrCreateTLSConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil *tls.Config")
	}
	if len(cfg.Certificates) == 0 {
		t.Fatalf("expected at least 1 certificate loaded in tls.Config")
	}
	if certPath == "" || keyPath == "" {
		t.Fatalf("expected non-empty cert and key paths")
	}

	// Verify providing only certFile or only keyFile produces an error
	if _, _, _, err := LoadOrCreateTLSConfig("only-cert.pem", "", filepath.Join(tempDir, "certs"), nil); err == nil {
		t.Errorf("expected error when certFile is provided without keyFile")
	}
	if _, _, _, err := LoadOrCreateTLSConfig("", "only-key.pem", filepath.Join(tempDir, "certs"), nil); err == nil {
		t.Errorf("expected error when keyFile is provided without certFile")
	}
}

func TestTLS_LiveHandshake(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-tls-live-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, _, _, err := LoadOrCreateTLSConfig("", "", filepath.Join(tempDir, "certs"), []string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("failed to load TLS config: %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("failed to listen on TLS: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		if n > 0 {
			conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\npong"))
		}
	}()

	client := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", ln.Addr().String(), client)
	if err != nil {
		t.Fatalf("failed to establish TLS dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("GET /ping HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")); err != nil {
		t.Fatalf("failed to write over TLS: %v", err)
	}

	resBuf := make([]byte, 1024)
	n, err := conn.Read(resBuf)
	if err != nil {
		t.Fatalf("failed to read TLS response: %v", err)
	}
	if !strings.Contains(string(resBuf[:n]), "pong") {
		t.Fatalf("expected pong in response, got %s", string(resBuf[:n]))
	}
	<-done
}

func TestTLS_RegenerateOnCorruptedKey(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-tls-corrupt-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	certFile, keyFile, err := EnsureSelfSignedCertificate(tempDir, []string{"localhost"})
	if err != nil {
		t.Fatalf("initial cert creation failed: %v", err)
	}

	// Corrupt key file
	if err := os.WriteFile(keyFile, []byte("INVALID_PEM_DATA"), 0600); err != nil {
		t.Fatalf("failed to write corrupted key: %v", err)
	}

	// Should automatically regenerate key without returning error
	certFile2, keyFile2, err := EnsureSelfSignedCertificate(tempDir, []string{"localhost"})
	if err != nil {
		t.Fatalf("failed to regenerate corrupted key: %v", err)
	}

	// Key pair must now be valid
	if _, err := tls.LoadX509KeyPair(certFile2, keyFile2); err != nil {
		t.Fatalf("regenerated key pair failed to load: %v", err)
	}
	if certFile != certFile2 || keyFile != keyFile2 {
		t.Errorf("expected paths %s and %s", certFile, certFile2)
	}
}
