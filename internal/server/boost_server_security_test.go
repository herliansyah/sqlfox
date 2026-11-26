package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herliansyah/sqlfox/internal/cli"
	"github.com/herliansyah/sqlfox/internal/driver"
	_ "github.com/herliansyah/sqlfox/internal/driver/sqlite"
	"github.com/herliansyah/sqlfox/internal/vault"
)

func TestBoostP06_AIProfileKeyExfilAndSecureDefaults(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.OpenVault(filepath.Join(dir, "config.db"), "master-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "master-secret")
	defer srv.Close()
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	// 1. Creating a new AI profile without mask_identifiers must default to MaskIdentifiers=true (P1).
	createBody := `{"name":"Prod Copilot","provider":"openai","model_name":"gpt-4o-mini","base_url":"https://api.openai.com/v1","api_key":"sk-victim-secret"}`
	resp, err := client.Post(ts.URL+"/api/ai/profiles", "application/json", bytes.NewBufferString(createBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create AI profile failed: status=%d err=%v", resp.StatusCode, err)
	}
	var created vault.AIProfile
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	if !created.MaskIdentifiers {
		t.Fatalf("expected new AI profile to default MaskIdentifiers=true")
	}
	if !created.HasAPIKey {
		t.Fatalf("expected HasAPIKey=true after saving API key")
	}

	// 2. Updating BaseURL with empty api_key MUST permanently clear the stored key in SQLite (P0.6).
	updateBody := `{"name":"Prod Copilot","provider":"openai","model_name":"gpt-4o-mini","base_url":"https://1.1.1.1/v1","api_key":"","encrypted_api_key":"injected-ciphertext"}`
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/ai/profiles/%s", ts.URL, created.ID), bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("update AI profile failed: status=%d err=%v", resp.StatusCode, err)
	}
	var updated vault.AIProfile
	_ = json.NewDecoder(resp.Body).Decode(&updated)
	resp.Body.Close()

	if updated.HasAPIKey {
		t.Fatalf("expected HasAPIKey=false after changing BaseURL without new APIKey")
	}
	stored, err := v.GetAIProfile(created.ID)
	if err != nil {
		t.Fatalf("GetAIProfile failed: %v", err)
	}
	if stored.APIKey != "" || stored.EncryptedAPIKey != "" {
		t.Fatalf("stored API key was NOT cleared on BaseURL change (exfil vulnerability!): key=%q enc=%q", stored.APIKey, stored.EncryptedAPIKey)
	}
}

func TestBoostP012_ConnectionPathAndVaultExportAuth(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "config.db")
	v, err := vault.OpenVault(vaultPath, "master-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "master-secret")
	defer srv.Close()
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	// 1. Opening SqlFox's own vault config.db as a SQLite connection must be rejected.
	badVaultConn, _ := json.Marshal(vault.ConnectionRecord{
		Name:         "Self Vault",
		Driver:       "sqlite",
		DatabaseName: vaultPath,
	})
	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(badVaultConn))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when opening own vault DB via SQLite connection, got %d", resp.StatusCode)
	}

	// 2. Passing a host filesystem path in ssl_ca must be rejected (inline PEM only).
	badSSLConn, _ := json.Marshal(vault.ConnectionRecord{
		Name:   "SSL Path Probe",
		Driver: "postgres",
		Host:   "db.example.com",
		UseSSL: true,
		SSLCA:  "/etc/passwd",
	})
	resp, err = client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(badSSLConn))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when ssl_ca is a filesystem path, got %d", resp.StatusCode)
	}

	// 3. Vault export without current_master_password must fail with 401; with wrong password 401; with right password 200.
	resp, err = client.Post(ts.URL+"/api/vault/export", "application/json", bytes.NewBufferString(`{"export_passphrase":"archive-pass"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for vault export without current_master_password, got %d", resp.StatusCode)
	}

	resp, err = client.Post(ts.URL+"/api/vault/export", "application/json", bytes.NewBufferString(`{"export_passphrase":"archive-pass","current_master_password":"wrong"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for vault export with wrong master password, got %d", resp.StatusCode)
	}

	resp, err = client.Post(ts.URL+"/api/vault/export", "application/json", bytes.NewBufferString(`{"export_passphrase":"archive-pass","current_master_password":"master-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for vault export with valid master password, got %d", resp.StatusCode)
	}
}

func TestBoostP1_FilterLiteralsAndCompositePKGuardAndRedactDSN(t *testing.T) {
	// Quoted # and -- literals must be allowed in validateTableFilter, while unquoted comments/semicolons remain blocked.
	if err := validateTableFilter(`tag = '#bug' AND note = 'a--b'`); err != nil {
		t.Fatalf("expected valid quoted literals with # and -- to pass validateTableFilter, got: %v", err)
	}
	if err := validateTableFilter(`id = 1 -- comment`); err == nil {
		t.Fatalf("expected unquoted -- comment to be blocked")
	}
	if err := validateTableFilter(`id = 1; DROP TABLE users`); err == nil {
		t.Fatalf("expected semicolon to be blocked")
	}

	// RedactDSN must strip credentials in all supported DSN formats.
	cases := map[string]string{
		"postgres://admin:supersecret@db.internal:5432/prod": "postgres://admin:xxxxx@db.internal:5432/prod",
		"root:s3cr3t@tcp(127.0.0.1:3306)/app":                "root:xxxxx@tcp(127.0.0.1:3306)/app",
		"mysql://u:p@ss!@host/db":                            "mysql://u:xxxxx@host/db",
	}
	for raw, want := range cases {
		if got := cli.RedactDSN(raw); got != want {
			t.Errorf("RedactDSN(%q) = %q; want %q", raw, got, want)
		}
	}

	// Composite PK partial-key guard in handleCellEdit
	dir := t.TempDir()
	v, err := vault.OpenVault(filepath.Join(dir, "config.db"), "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "pw")
	defer srv.Close()
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	body, _ := json.Marshal(vault.ConnectionRecord{
		Name:         "CompositePK Test",
		Driver:       "sqlite",
		DatabaseName: filepath.Join(dir, "cpk.db"),
	})
	resp, _ := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	var conn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&conn)
	resp.Body.Close()

	resp, _ = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, conn.ID), "application/json", nil)
	resp.Body.Close()

	for _, sqlStmt := range []string{
		`CREATE TABLE memberships (org_id INT, user_id INT, role TEXT, PRIMARY KEY (org_id, user_id))`,
		`INSERT INTO memberships VALUES (1, 10, 'member'), (1, 20, 'member')`,
	} {
		eb, _ := json.Marshal(map[string]any{"sql": sqlStmt})
		r, _ := client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(eb))
		r.Body.Close()
	}

	// Sending only 1 of 2 composite PK columns in pk_values MUST be rejected with 400.
	editPayload := `{"table":"memberships","column":"role","value":"admin","pk_values":{"org_id":1}}`
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/cell-edit", ts.URL, conn.ID), "application/json", bytes.NewBufferString(editPayload))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when partial composite PK is supplied in pk_values, got %d", resp.StatusCode)
	}
}

func TestBoostReview_MultiStatementTxStaysAliveInExecuteAndStream(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.OpenVault(filepath.Join(dir, "config.db"), "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "pw")
	defer srv.Close()
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	body, _ := json.Marshal(vault.ConnectionRecord{
		Name:         "MultiStmt Tx Test",
		Driver:       "sqlite",
		DatabaseName: filepath.Join(dir, "tx.db"),
	})
	resp, _ := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	var conn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&conn)
	resp.Body.Close()

	resp, _ = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, conn.ID), "application/json", nil)
	resp.Body.Close()

	initPayload, _ := json.Marshal(map[string]any{"sql": "CREATE TABLE items (id INTEGER PRIMARY KEY, v TEXT)"})
	resp, _ = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(initPayload))
	resp.Body.Close()

	// 1. Multi-statement BEGIN; INSERT; COMMIT via /execute MUST succeed and persist the row.
	txPayload, _ := json.Marshal(map[string]any{
		"sql":    "BEGIN; INSERT INTO items (id, v) VALUES (1, 'from-execute'); COMMIT;",
		"tab_id": "tab-exec",
	})
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(txPayload))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("multi-statement BEGIN; INSERT; COMMIT failed in /execute: status=%d body=%s", resp.StatusCode, string(raw))
	}

	// 2. Multi-statement BEGIN; INSERT; COMMIT via /execute-stream MUST succeed and persist the row.
	streamPayload, _ := json.Marshal(map[string]any{
		"sql":    "BEGIN; INSERT INTO items (id, v) VALUES (2, 'from-stream'); COMMIT;",
		"tab_id": "tab-stream",
	})
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute-stream", ts.URL, conn.ID), "application/json", bytes.NewReader(streamPayload))
	if err != nil {
		t.Fatal(err)
	}
	streamRaw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.Contains(string(streamRaw), `"type":"error"`) {
		t.Fatalf("multi-statement BEGIN; INSERT; COMMIT failed in /execute-stream: status=%d body=%s", resp.StatusCode, string(streamRaw))
	}

	// Verify both rows were actually committed!
	verifyPayload, _ := json.Marshal(map[string]any{"sql": "SELECT COUNT(*) FROM items"})
	resp, _ = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(verifyPayload))
	var qRes ExecuteResponse
	_ = json.NewDecoder(resp.Body).Decode(&qRes)
	resp.Body.Close()
	if len(qRes.Rows) != 1 || fmt.Sprint(qRes.Rows[0][0]) != "2" {
		t.Fatalf("expected 2 committed rows from multi-statement transactions, got %+v", qRes.Rows)
	}
}

func TestBoostReview_UnsafeCountPredicateAndSQLiteVaultURIsAndDDLInjection(t *testing.T) {
	windowsURIs := map[string]string{
		`file://C:\Users\test\config%2edb`:        `C:/Users/test/config.db`,
		`file://localhostC:\Users\test\config.db`: `C:/Users/test/config.db`,
		`file:///C:/Users/test/config.db`:         `C:/Users/test/config.db`,
	}
	for uri, want := range windowsURIs {
		if got := normalizeSQLiteTargetPath(uri); got != want {
			t.Errorf("normalizeSQLiteTargetPath(%q) = %q, want %q", uri, got, want)
		}
	}

	// 1. hasUnsafeCountPredicate must catch newline/tab/comment/parenthesis bypasses
	unsafeCases := []string{
		"SELECT COUNT(*) FROM users\nWHERE pg_terminate_backend(123) = true",
		"SELECT COUNT(*) FROM users WHERE pg_terminate_backend\n(123) = true",
		"SELECT COUNT(*) FROM users WHERE EXISTS(SELECT(1))",
		"SELECT COUNT(*) FROM users\tWHERE\tnextval('seq') > 0",
	}
	for _, q := range unsafeCases {
		if !hasUnsafeCountPredicate(q) {
			t.Errorf("hasUnsafeCountPredicate(%q) = false; want true", q)
		}
	}
	safeCases := []string{
		"SELECT COUNT(*) FROM users WHERE id IN (1, 2, 3) AND note = 'fn(x)'",
		"SELECT COUNT(*) FROM users WHERE status = 'SELECT 1' AND active = 1",
	}
	for _, q := range safeCases {
		if hasUnsafeCountPredicate(q) {
			t.Errorf("hasUnsafeCountPredicate(%q) = true; want false", q)
		}
	}

	// 2. validateConnectionPayload must block URL-encoded, authority, symlink, and master.key SQLite targets
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "config.db")
	v, err := vault.OpenVault(vaultPath, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "pw")
	defer srv.Close()

	symPath := filepath.Join(dir, "alias.db")
	if err := os.Symlink(vaultPath, symPath); err != nil {
		t.Fatalf("symlink failed: %v", err)
	}

	blockedPaths := []string{
		"file://" + filepath.Join(dir, "config%2edb"),
		"file://localhost" + vaultPath,
		symPath,
		filepath.Join(dir, "master.key"),
		vaultPath + "-wal",
	}
	for _, p := range blockedPaths {
		err := srv.validateConnectionPayload(&vault.ConnectionRecord{
			Driver:       "sqlite",
			DatabaseName: p,
		}, false)
		if err == nil {
			t.Errorf("validateConnectionPayload must reject %q", p)
		}
	}

	// 3. Structure sync metadata sanitization blocks DDL injection
	if got := normalizeColumnType("INT); DROP TABLE users; --", "mysql"); got != "TEXT" {
		t.Errorf("normalizeColumnType did not sanitize injection: got %q", got)
	}
	if got := renderColumnDefault(driver.ColumnMetadata{
		DefaultValue: "uuid(); DROP TABLE users",
		Extra:        "DEFAULT_GENERATED",
	}, "mysql", "mysql"); got != "" {
		t.Errorf("renderColumnDefault did not strip injection: got %q", got)
	}
	if got := extraClause("ON UPDATE CURRENT_TIMESTAMP; DROP TABLE users"); got != "" {
		t.Errorf("extraClause did not reject injected ON UPDATE: got %q", got)
	}
	if got := extraClause("ON UPDATE CURRENT_TIMESTAMP"); got != "ON UPDATE CURRENT_TIMESTAMP" {
		t.Errorf("extraClause broke legitimate ON UPDATE CURRENT_TIMESTAMP: got %q", got)
	}
}

func TestBoostReview_PKLessBatchEditDoesNotCorruptDistinctRowsAndDetectsConflicts(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.OpenVault(filepath.Join(dir, "config.db"), "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "pw")
	defer srv.Close()
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	body, _ := json.Marshal(vault.ConnectionRecord{
		Name:         "PKLess Batch Test",
		Driver:       "sqlite",
		DatabaseName: filepath.Join(dir, "pkless.db"),
	})
	resp, _ := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	var conn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&conn)
	resp.Body.Close()

	resp, _ = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, conn.ID), "application/json", nil)
	resp.Body.Close()

	for _, sqlStmt := range []string{
		`CREATE TABLE tasks (owner TEXT, status TEXT, note TEXT)`,
		`INSERT INTO tasks VALUES ('Alice', 'open', 'x'), ('Bob', 'open', 'y')`,
	} {
		eb, _ := json.Marshal(map[string]any{"sql": sqlStmt})
		r, _ := client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(eb))
		r.Body.Close()
	}

	// Batch edit: Row 1 (Alice) changes status 'open' -> 'closed', Row 2 (Bob) changes note 'y' -> 'z'.
	// Both rows start with status='open'; Row 1's edit MUST NOT mutate Row 2's RowCriteria!
	batchPayload := `{
		"table": "tasks",
		"edits": [
			{"table":"tasks","column":"status","value":"closed","row_criteria":{"owner":"Alice","status":"open","note":"x"}},
			{"table":"tasks","column":"note","value":"z","row_criteria":{"owner":"Bob","status":"open","note":"y"}}
		]
	}`
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/cell-edit", ts.URL, conn.ID), "application/json", bytes.NewBufferString(batchPayload))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for distinct-row PK-less batch edit, got %d: %s", resp.StatusCode, string(raw))
	}

	// Verify both Alice and Bob were updated accurately
	qBody, _ := json.Marshal(map[string]any{"sql": "SELECT owner, status, note FROM tasks ORDER BY owner ASC"})
	resp, _ = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(qBody))
	var qRes ExecuteResponse
	_ = json.NewDecoder(resp.Body).Decode(&qRes)
	resp.Body.Close()
	if len(qRes.Rows) != 2 || qRes.Rows[0][1] != "closed" || qRes.Rows[1][1] != "open" || qRes.Rows[1][2] != "z" {
		t.Fatalf("unexpected rows after PK-less batch edit: %+v", qRes.Rows)
	}

	// Stale RowCriteria edit (row no longer matches) MUST return 409 Conflict instead of silent success.
	stalePayload := `{"table":"tasks","column":"note","value":"stale","row_criteria":{"owner":"Alice","status":"open","note":"x"}}`
	resp, _ = client.Post(fmt.Sprintf("%s/api/session/%s/cell-edit", ts.URL, conn.ID), "application/json", bytes.NewBufferString(stalePayload))
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on stale PK-less cell edit, got %d", resp.StatusCode)
	}

	// Same-session identical source/target DB DataTransfer MUST be rejected with 400.
	transferPayload := fmt.Sprintf(`{"source_session_id":%q,"source_db":"main","target_session_id":%q,"target_db":"main","tables":["tasks"],"truncate_target":true}`, conn.ID, conn.ID)
	resp, _ = client.Post(ts.URL+"/api/sync/data/transfer", "application/json", bytes.NewBufferString(transferPayload))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for same-session identical DB DataTransfer, got %d", resp.StatusCode)
	}
}

func TestHandleImportVault_EnforcesConnectionAndAIValidationAtomically(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "config.db")
	v, err := vault.OpenVault(vaultPath, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "pw")
	defer srv.Close()

	makeArchive := func(payload vault.ExportPayload) json.RawMessage {
		t.Helper()
		plain, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		salt, err := vault.GenerateSalt()
		if err != nil {
			t.Fatal(err)
		}
		key := vault.DeriveKey("export-pass", salt)
		enc, err := vault.Encrypt(plain, key)
		if err != nil {
			t.Fatal(err)
		}
		archiveBytes, err := json.Marshal(vault.VaultArchive{
			Version: 1,
			Salt:    "not-used",
			Data:    enc,
		})
		if err != nil {
			t.Fatal(err)
		}
		var archive map[string]any
		_ = json.Unmarshal(archiveBytes, &archive)
		// Encode salt properly via ExportVault helper? Let's use base64 via encoding/base64 or build via ExportVault:
		_ = archive
		return archiveBytes
	}
	_ = makeArchive

	// Helper using a temporary source vault so we use the exact production ExportVault format
	buildArchiveFromVault := func(mutate func(src *vault.Vault)) json.RawMessage {
		t.Helper()
		srcDir := t.TempDir()
		src, err := vault.OpenVault(filepath.Join(srcDir, "src.db"), "src-pw")
		if err != nil {
			t.Fatal(err)
		}
		defer src.Close()
		mutate(src)
		raw, err := vault.ExportVault(src, "export-pass")
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	postImport := func(archive json.RawMessage, remoteAddr string) *httptest.ResponseRecorder {
		reqBody, _ := json.Marshal(map[string]any{
			"export_passphrase": "export-pass",
			"archive":           archive,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/vault/import", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		srv.handleImportVault(rec, req)
		return rec
	}

	// 1. Archive with a valid connection + an SSLKey filesystem path must be rejected and leave 0 connections.
	sslPathArchive := buildArchiveFromVault(func(src *vault.Vault) {
		_ = src.SaveConnection(&vault.ConnectionRecord{
			Name:     "Valid MySQL",
			Driver:   "mysql",
			Host:     "db.internal",
			Port:     3306,
			User:     "app",
			Password: "secret",
		})
		_ = src.SaveConnection(&vault.ConnectionRecord{
			Name:   "Malicious TLS Path",
			Driver: "mysql",
			Host:   "attacker.example",
			Port:   3306,
			UseSSL: true,
			SSLKey: "/etc/passwd",
		})
	})
	if rec := postImport(sslPathArchive, "127.0.0.1:12345"); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for filesystem SSLKey in vault import, got %d: %s", rec.Code, rec.Body.String())
	}
	if conns, _ := v.ListConnections(); len(conns) != 0 {
		t.Fatalf("expected atomic rollback (0 connections) on invalid SSLKey import, got %d", len(conns))
	}

	// 2. Archive targeting SqlFox's own config.db via SQLite must be rejected.
	selfDBArchive := buildArchiveFromVault(func(src *vault.Vault) {
		_ = src.SaveConnection(&vault.ConnectionRecord{
			Name:         "Self Vault",
			Driver:       "sqlite",
			DatabaseName: vaultPath,
		})
	})
	if rec := postImport(selfDBArchive, "127.0.0.1:12345"); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for config.db SQLite target in vault import, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Remote request importing an arbitrary SQLite filesystem path must be rejected.
	remoteSQLiteArchive := buildArchiveFromVault(func(src *vault.Vault) {
		_ = src.SaveConnection(&vault.ConnectionRecord{
			Name:         "Remote SQLite",
			Driver:       "sqlite",
			DatabaseName: filepath.Join(dir, "other.db"),
		})
	})
	if rec := postImport(remoteSQLiteArchive, "10.20.30.40:5555"); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for remote SQLite path in vault import, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Archive with a valid connection + an SSRF AIProfile BaseURL must be rejected atomically.
	ssrfAIArchive := buildArchiveFromVault(func(src *vault.Vault) {
		_ = src.SaveConnection(&vault.ConnectionRecord{
			Name:   "Valid Postgres",
			Driver: "postgres",
			Host:   "pg.internal",
			Port:   5432,
		})
		_ = src.SaveAIProfile(&vault.AIProfile{
			Name:      "SSRF Profile",
			Provider:  "openai",
			BaseURL:   "http://169.254.169.254/latest/meta-data/",
			ModelName: "gpt-4o",
		})
	})
	if rec := postImport(ssrfAIArchive, "127.0.0.1:12345"); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for SSRF AIProfile BaseURL in vault import, got %d: %s", rec.Code, rec.Body.String())
	}
	if conns, _ := v.ListConnections(); len(conns) != 0 {
		t.Fatalf("expected 0 connections after SSRF AIProfile rejection, got %d", len(conns))
	}
}

func TestRemoteHostFileSQLHardBlock(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.OpenVault(filepath.Join(dir, "config.db"), "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(dir, "backups"), "pw")
	defer srv.Close()

	conn := &vault.ConnectionRecord{
		Name:         "MemSQLite",
		Driver:       "sqlite",
		DatabaseName: ":memory:",
		Environment:  "Development",
	}
	if err := v.SaveConnection(conn); err != nil {
		t.Fatal(err)
	}

	drv, err := driver.GetDriver("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	drvSess, err := drv.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: ":memory:",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer drvSess.Close()

	active := &ActiveSession{
		ID:           conn.ID,
		ConnectionID: conn.ID,
		Name:         conn.Name,
		DatabaseName: ":memory:",
		Environment:  "Development",
		DriverName:   "sqlite",
		Session:      drvSess,
		vault:        v,
		sessionMgr:   srv.sessionMgr,
	}
	srv.sessionMgr.mu.Lock()
	srv.sessionMgr.sessions[conn.ID] = active
	srv.sessionMgr.mu.Unlock()

	callHandler := func(handler func(http.ResponseWriter, *http.Request), remoteAddr, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+conn.ID+"/execute", strings.NewReader(body))
		req.SetPathValue("id", conn.ID)
		req.RemoteAddr = remoteAddr
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}

	blockedPayloads := []string{
		fmt.Sprintf(`{"sql":"ATTACH DATABASE '%s' AS x","confirmed":true,"challenge_input":":memory:","remote_request":false}`, filepath.Join(dir, "config.db")),
		`{"sql":"DETACH DATABASE x","confirmed":true,"challenge_input":":memory:"}`,
		fmt.Sprintf(`{"sql":"VACUUM INTO '%s'","confirmed":true,"challenge_input":":memory:"}`, filepath.Join(dir, "leak.db")),
		fmt.Sprintf(`{"sql":"SELECT 1; ATTACH DATABASE '%s' AS x","confirmed":true,"challenge_input":":memory:"}`, filepath.Join(dir, "config.db")),
		fmt.Sprintf(`{"sql":"/* c */ VACUUM INTO '%s'","confirmed":true,"challenge_input":":memory:"}`, filepath.Join(dir, "leak2.db")),
	}

	for _, payload := range blockedPayloads {
		if rec := callHandler(srv.handleExecuteQuery, "10.20.30.40:5555", payload); rec.Code != http.StatusForbidden {
			t.Fatalf("handleExecuteQuery remote payload %s: expected 403 Forbidden, got %d (%s)", payload, rec.Code, rec.Body.String())
		}
		if rec := callHandler(srv.handleExecuteStream, "10.20.30.40:5555", payload); rec.Code != http.StatusForbidden {
			t.Fatalf("handleExecuteStream remote payload %s: expected 403 Forbidden, got %d (%s)", payload, rec.Code, rec.Body.String())
		}
	}

	// Plain VACUUM (without INTO) is still allowed remotely.
	if rec := callHandler(srv.handleExecuteQuery, "10.20.30.40:5555", `{"sql":"VACUUM;"}`); rec.Code != http.StatusOK {
		t.Fatalf("expected plain VACUUM to succeed remotely, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Loopback requests can still execute ATTACH with valid challenge confirmation.
	if rec := callHandler(srv.handleExecuteQuery, "127.0.0.1:12345", `{"sql":"ATTACH DATABASE ':memory:' AS extra","confirmed":true,"challenge_input":":memory:"}`); rec.Code != http.StatusOK {
		t.Fatalf("expected loopback confirmed ATTACH to succeed, got %d (%s)", rec.Code, rec.Body.String())
	}
}
