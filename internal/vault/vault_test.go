package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVault_Lifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-vault-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "config.db")
	masterPass := "my-secret-master-pass"

	// 1. Open new vault
	v, err := OpenVault(dbPath, masterPass)
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}

	// 2. Save a connection
	conn := &ConnectionRecord{
		Name:         "Production MySQL",
		Environment:  "Production",
		Driver:       "mysql",
		Host:         "10.0.0.1",
		Port:         3306,
		DatabaseName: "app_prod",
		User:         "admin",
		Password:     "super_secret_db_password",
		UseSSL:       true,
		UseSSH:       true,
		SSHHost:      "bastion.example.com",
		SSHPort:      22,
		SSHUser:      "ubuntu",
		SSHPassword:  "ssh_secret",
	}

	if err := v.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection failed: %v", err)
	}

	if conn.ID == "" {
		t.Fatal("expected assigned ID")
	}

	// 3. Retrieve and verify decryption
	retrieved, err := v.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("GetConnection failed: %v", err)
	}

	if retrieved.Password != "super_secret_db_password" {
		t.Fatalf("expected decrypted password, got: %s", retrieved.Password)
	}
	if retrieved.SSHPassword != "ssh_secret" {
		t.Fatalf("expected decrypted ssh password, got: %s", retrieved.SSHPassword)
	}
	if !retrieved.UseSSL || !retrieved.UseSSH {
		t.Fatal("expected boolean flags preserved")
	}

	// 4. Close vault
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}

	// 5. Test opening with wrong master password
	_, err = OpenVault(dbPath, "wrong-pass")
	if err == nil {
		t.Fatal("expected error on wrong master password, got nil")
	}

	// 6. Test opening with correct master password
	v2, err := OpenVault(dbPath, masterPass)
	if err != nil {
		t.Fatalf("re-opening vault failed: %v", err)
	}
	defer v2.Close()

	list, err := v2.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(list))
	}
	// Password should not be exposed in List
	if list[0].Password != "" {
		t.Fatal("expected empty password in ListConnections")
	}

	// 7. Delete connection
	if err := v2.DeleteConnection(conn.ID); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	_, err = v2.GetConnection(conn.ID)
	if err != ErrConnectionNotFound {
		t.Fatalf("expected ErrConnectionNotFound, got: %v", err)
	}
}
