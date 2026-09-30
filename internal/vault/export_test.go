package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVault_ExportImport(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-export-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db1Path := filepath.Join(tempDir, "vault1.db")
	db2Path := filepath.Join(tempDir, "vault2.db")

	v1, err := OpenVault(db1Path, "master1")
	if err != nil {
		t.Fatal(err)
	}
	defer v1.Close()

	// Add test connection
	err = v1.SaveConnection(&ConnectionRecord{
		Name:         "Analytics Cluster",
		Environment:  "Staging",
		Driver:       "mysql",
		Host:         "staging-db.internal",
		Port:         3306,
		DatabaseName: "analytics",
		User:         "analyst",
		Password:     "staging-pass-secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	exportPass := "export-archive-passphrase-999"
	archiveJSON, err := ExportVault(v1, exportPass)
	if err != nil {
		t.Fatalf("ExportVault failed: %v", err)
	}

	// Open second fresh vault with different master password
	v2, err := OpenVault(db2Path, "master2")
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()

	// Test import with wrong passphrase
	_, err = ImportVault(v2, archiveJSON, "wrong-export-pass")
	if err == nil {
		t.Fatal("expected import error with wrong passphrase, got nil")
	}

	// Test import with correct passphrase
	count, err := ImportVault(v2, archiveJSON, exportPass)
	if err != nil {
		t.Fatalf("ImportVault failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported connection, got %d", count)
	}

	// Verify imported record and decrypted password in vault 2
	list, err := v2.ListConnections()
	if err != nil || len(list) != 1 {
		t.Fatalf("failed to list connections in vault2: %v", err)
	}

	importedConn, err := v2.GetConnection(list[0].ID)
	if err != nil {
		t.Fatalf("failed to get imported connection: %v", err)
	}

	if importedConn.Name != "Analytics Cluster" {
		t.Fatalf("unexpected name: %s", importedConn.Name)
	}
	if importedConn.Password != "staging-pass-secret" {
		t.Fatalf("unexpected decrypted password: %s", importedConn.Password)
	}

	// Verify imported snippets in vault 2
	importedSnippets, err := v2.ListSnippets()
	if err != nil {
		t.Fatalf("failed to list snippets in vault2: %v", err)
	}
	if len(importedSnippets) == 0 {
		t.Fatal("expected imported snippets in vault2, got 0")
	}
}

