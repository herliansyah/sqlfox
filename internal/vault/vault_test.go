package vault

import (
	"os"
	"path/filepath"
	"strings"
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
	if !retrieved.UseSSL {
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

func TestVault_Snippets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-vault-snip-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "config.db")
	v, err := OpenVault(dbPath, "test-pass")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	// 1. Initial seeds should be present
	snippets, err := v.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(snippets) < 3 {
		t.Fatalf("expected default seeded snippets, got %d", len(snippets))
	}

	// 2. Add custom snippet
	newSnip := &Snippet{
		Title:      "Monthly Revenue Summary",
		SQLContent: "SELECT DATE_FORMAT(created_at, '%Y-%m') AS month, SUM(amount) AS revenue FROM orders WHERE tenant_id = {{tenant_id:1001}} GROUP BY 1 ORDER BY 1 DESC LIMIT {{limit:12}};",
		Tags:       "mysql,reporting",
	}
	if err := v.SaveSnippet(newSnip); err != nil {
		t.Fatalf("SaveSnippet failed: %v", err)
	}
	if newSnip.ID == "" {
		t.Fatal("expected assigned snippet ID")
	}

	// 3. Retrieve via ListSnippets
	allSnips, err := v.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	var fetched *Snippet
	for _, s := range allSnips {
		if s.ID == newSnip.ID {
			fetched = s
			break
		}
	}
	if fetched == nil || fetched.Title != "Monthly Revenue Summary" || fetched.Tags != "mysql,reporting" {
		t.Fatalf("snippet fields mismatch: %+v", fetched)
	}

	// 4. Update
	fetched.Title = "Updated Revenue Summary"
	if err := v.SaveSnippet(fetched); err != nil {
		t.Fatalf("SaveSnippet update failed: %v", err)
	}
	allSnips, err = v.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	var updated *Snippet
	for _, s := range allSnips {
		if s.ID == newSnip.ID {
			updated = s
			break
		}
	}
	if updated == nil || updated.Title != "Updated Revenue Summary" {
		t.Fatalf("expected updated title, got %v", updated)
	}

	// 5. Delete
	if err := v.DeleteSnippet(newSnip.ID); err != nil {
		t.Fatalf("DeleteSnippet failed: %v", err)
	}
	err = v.DeleteSnippet(newSnip.ID)
	if err != ErrSnippetNotFound {
		t.Fatalf("expected ErrSnippetNotFound on second delete, got: %v", err)
	}
}

func TestVault_TableNotes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-vault-notes-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "config.db")
	v, err := OpenVault(dbPath, "test-pass")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	// 1. Initial should return ErrTableNoteNotFound
	_, err = v.GetTableNote("conn-1", "sales_db", "orders")
	if err != ErrTableNoteNotFound {
		t.Fatalf("expected ErrTableNoteNotFound, got: %v", err)
	}

	// 2. Save note
	note := &TableNote{
		ConnectionID: "conn-1",
		DatabaseName: "sales_db",
		TableName:    "orders",
		Content:      "## Orders Table Runbook\n- Filter `is_deleted = 0` for all active metrics.\n- Amount is stored in cents.",
	}
	if err := v.SaveTableNote(note); err != nil {
		t.Fatalf("SaveTableNote failed: %v", err)
	}

	// 3. Retrieve note
	retrieved, err := v.GetTableNote("conn-1", "sales_db", "orders")
	if err != nil {
		t.Fatalf("GetTableNote failed: %v", err)
	}
	if retrieved.Content != note.Content {
		t.Fatalf("content mismatch: got %s", retrieved.Content)
	}

	// 4. Update note
	retrieved.Content = "## Updated Runbook"
	if err := v.SaveTableNote(retrieved); err != nil {
		t.Fatalf("SaveTableNote update failed: %v", err)
	}
	updated, err := v.GetTableNote("conn-1", "sales_db", "orders")
	if err != nil || updated.Content != "## Updated Runbook" {
		t.Fatalf("expected updated content, got %v", updated)
	}

	// 5. List notes for connection
	notes, err := v.ListTableNotesForConnection("conn-1")
	if err != nil || len(notes) != 1 {
		t.Fatalf("expected 1 note for conn-1, got %d", len(notes))
	}
}

// Regression: the SSL client private key must be encrypted at rest and round-trip intact.
func TestVault_SSLKeyEncryptedAtRest(t *testing.T) {
	v, err := OpenVault(filepath.Join(t.TempDir(), "config.db"), "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	const key = "-----BEGIN PRIVATE KEY-----\nSECRET\n-----END PRIVATE KEY-----"
	conn := &ConnectionRecord{Name: "c", Driver: "mysql", Environment: "Development", SSLKey: key}
	if err := v.SaveConnection(conn); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := v.db.QueryRow(`SELECT ssl_key FROM connections WHERE id = ?`, conn.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "SECRET") || !strings.HasPrefix(raw, sealedPrefix) {
		t.Fatalf("ssl_key stored in plaintext: %q", raw)
	}

	// Re-saving a listed (still sealed) record must not double-encrypt.
	list, _ := v.ListConnections()
	if err := v.SaveConnection(list[0]); err != nil {
		t.Fatal(err)
	}
	got, err := v.GetConnection(conn.ID)
	if err != nil || got.SSLKey != key {
		t.Fatalf("round-trip failed: %q %v", got.SSLKey, err)
	}
}

// Regression: an actor with raw vault write access must not be able to swap one connection's
// encrypted password ciphertext into another connection and have it decrypt (AAD field binding).
func TestVault_PasswordCiphertextSwapRejected(t *testing.T) {
	v, err := OpenVault(filepath.Join(t.TempDir(), "config.db"), "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	a := &ConnectionRecord{Name: "A", Driver: "mysql", Environment: "Development", Password: "alpha-secret"}
	b := &ConnectionRecord{Name: "B", Driver: "mysql", Environment: "Development", Password: "bravo-secret"}
	if err := v.SaveConnection(a); err != nil {
		t.Fatal(err)
	}
	if err := v.SaveConnection(b); err != nil {
		t.Fatal(err)
	}

	// Baseline: both decrypt to their own secret.
	if ga, _ := v.GetConnection(a.ID); ga == nil || ga.Password != "alpha-secret" {
		t.Fatalf("A password did not round-trip")
	}

	// Read B's ciphertext and force it into A's row (the swap an attacker with DB access would do).
	var bCipher string
	if err := v.db.QueryRow(`SELECT encrypted_password FROM connections WHERE id = ?`, b.ID).Scan(&bCipher); err != nil {
		t.Fatal(err)
	}
	if _, err := v.db.Exec(`UPDATE connections SET encrypted_password = ? WHERE id = ?`, bCipher, a.ID); err != nil {
		t.Fatal(err)
	}

	// A's AAD is connection:A:password; B's ciphertext was bound to connection:B:password, so
	// decryption must fail rather than leak B's secret under A's identity.
	if _, err := v.GetConnection(a.ID); err == nil {
		t.Fatal("expected swapped ciphertext to fail decryption, but GetConnection succeeded")
	}
}

// Regression: updating a connection from the web modal (which omits SSL fields) must not wipe
// existing SSL certificates, keys, or use_ssl flag in the database.
func TestVault_SaveConnectionPreservesSSLOnPartialUpdate(t *testing.T) {
	v, err := OpenVault(filepath.Join(t.TempDir(), "config.db"), "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	initial := &ConnectionRecord{
		Name:        "mTLS Conn",
		Driver:      "mysql",
		Environment: "Production",
		UseSSL:      true,
		SSLCA:       "ca-cert-data",
		SSLCert:     "client-cert-data",
		SSLKey:      "client-key-data",
	}
	if err := v.SaveConnection(initial); err != nil {
		t.Fatal(err)
	}

	// Web UI modal edit: only sends name/port/host, omitting SSL fields
	partialUpdate := &ConnectionRecord{
		ID:          initial.ID,
		Name:        "mTLS Conn Renamed",
		Driver:      "mysql",
		Environment: "Production",
		Host:        "db.internal",
		Port:        3307,
	}
	if err := v.SaveConnection(partialUpdate); err != nil {
		t.Fatal(err)
	}

	got, err := v.GetConnection(initial.ID)
	if err != nil {
		t.Fatalf("GetConnection failed: %v", err)
	}
	if !got.UseSSL || got.SSLCA != "ca-cert-data" || got.SSLCert != "client-cert-data" || got.SSLKey != "client-key-data" {
		t.Fatalf("SSL configuration was wiped by partial update: %+v", got)
	}
}

// Regression: legacy secrets sealed with nil AAD (pre-ADR-0060) must be automatically migrated to
// per-field AAD on OpenVault so they are no longer swap-vulnerable.
func TestVault_OpenVaultMigratesLegacyAAD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "config.db")
	v, err := OpenVault(dbPath, "pass")
	if err != nil {
		t.Fatal(err)
	}

	conn := &ConnectionRecord{Name: "Old", Driver: "mysql", Environment: "Development", Password: "old-secret"}
	if err := v.SaveConnection(conn); err != nil {
		t.Fatal(err)
	}

	// Manually overwrite with a legacy nil-AAD ciphertext to simulate a pre-AAD vault.
	legacyCipher, err := Encrypt([]byte("old-secret"), v.key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.db.Exec(`UPDATE connections SET encrypted_password = ? WHERE id = ?`, legacyCipher, conn.ID); err != nil {
		t.Fatal(err)
	}
	v.Close()

	// Reopen vault: migrateLegacyAAD must re-seal with bound AAD.
	v2, err := OpenVault(dbPath, "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()

	var migratedCipher string
	if err := v2.db.QueryRow(`SELECT encrypted_password FROM connections WHERE id = ?`, conn.ID).Scan(&migratedCipher); err != nil {
		t.Fatal(err)
	}

	// Verifying: decrypting with bound AAD directly (without fallback) must succeed.
	plain, err := decrypt(migratedCipher, v2.key, connFieldAAD(conn.ID, "password"))
	if err != nil || string(plain) != "old-secret" {
		t.Fatalf("expected cipher to be re-sealed with bound AAD, got plain=%q err=%v", plain, err)
	}
}
