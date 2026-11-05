package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/vault"
)

func TestServer_DumpAndSmartSeedAndScriptRunner(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-dump-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "config.db")
	targetDB := filepath.Join(tempDir, "target.db")

	v, err := vault.OpenVault(vaultDB, "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "test-master")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// 1. Create SQLite connection
	createPayload := vault.ConnectionRecord{
		Name:         "Dump SQLite Test",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: targetDB,
	}
	body, _ := json.Marshal(createPayload)
	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create connection failed: %v", err)
	}
	var conn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&conn)
	resp.Body.Close()

	// 2. Connect session
	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, conn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("connect session failed: %v", err)
	}
	resp.Body.Close()

	// 3. Create parent table and child table with FK
	setupSQL := `
CREATE TABLE authors (id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE books (id INTEGER PRIMARY KEY, author_id INTEGER, title TEXT, FOREIGN KEY(author_id) REFERENCES authors(id));
INSERT INTO authors (id, name) VALUES (1, 'Tolkien'), (2, 'Asimov');
INSERT INTO books (id, author_id, title) VALUES (10, 1, 'The Hobbit'), (20, 2, 'Foundation');
CREATE VIEW a_titles AS SELECT title FROM books;
`
	for _, stmt := range strings.Split(setupSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		sqlBody, _ := json.Marshal(map[string]any{"sql": stmt})
		resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(sqlBody))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("execute setup failed: %s: %v", stmt, err)
		}
		resp.Body.Close()
	}

	// 4. Test GET /api/session/{id}/export-dump
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/export-dump?schema=true&data=true", ts.URL, conn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("export-dump failed: status=%d, err=%v", resp.StatusCode, err)
	}
	dumpBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read dump body failed: %v", err)
	}
	dumpContent := string(dumpBytes)
	if !strings.Contains(dumpContent, "CREATE TABLE") || !strings.Contains(dumpContent, "INSERT INTO") {
		t.Fatalf("expected CREATE TABLE and INSERT INTO in dump, got:\n%s", dumpContent)
	}
	if !strings.Contains(dumpContent, "Tolkien") || !strings.Contains(dumpContent, "The Hobbit") {
		t.Fatalf("expected data in dump, got:\n%s", dumpContent)
	}
	// Views must be dropped with DROP VIEW and created after their base tables.
	if strings.Contains(dumpContent, `DROP TABLE IF EXISTS "a_titles"`) || !strings.Contains(dumpContent, `DROP VIEW IF EXISTS "a_titles"`) {
		t.Fatalf("expected DROP VIEW for view in dump, got:\n%s", dumpContent)
	}
	if strings.Index(dumpContent, `CREATE VIEW a_titles`) < strings.Index(dumpContent, `CREATE TABLE books`) {
		t.Fatalf("expected view after base tables in dump, got:\n%s", dumpContent)
	}
	// Drops come first, children before parents, so re-imports over an existing DB succeed.
	if dropBooks, dropAuthors := strings.Index(dumpContent, `DROP TABLE IF EXISTS "books"`), strings.Index(dumpContent, `DROP TABLE IF EXISTS "authors"`); dropBooks < 0 || dropBooks > dropAuthors || dropAuthors > strings.Index(dumpContent, `CREATE TABLE authors`) {
		t.Fatalf("expected reverse-dependency DROPs before CREATEs, got:\n%s", dumpContent)
	}

	// 5. Test GET /api/session/{id}/export-smart-seed?limit=2&ddl=true
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/export-smart-seed?limit=2&ddl=true", ts.URL, conn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("export-smart-seed failed: status=%d, err=%v", resp.StatusCode, err)
	}
	seedBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read smart seed body failed: %v", err)
	}
	seedContent := string(seedBytes)
	// Authors (parent) must appear before Books (child) in DDL and INSERT
	authorsIdx := strings.Index(seedContent, "CREATE TABLE authors")
	booksIdx := strings.Index(seedContent, "CREATE TABLE books")
	if authorsIdx == -1 || booksIdx == -1 || authorsIdx > booksIdx {
		t.Fatalf("expected parent 'authors' to precede child 'books' in smart seed, got:\n%s", seedContent)
	}
	if !strings.Contains(seedContent, "Tolkien") {
		t.Fatalf("expected authors data in smart seed, got:\n%s", seedContent)
	}

	// 6. Test POST /api/session/{id}/run-sql-script
	scriptContent := `
CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO tags (id, name) VALUES (1, 'Fantasy');
INSERT INTO tags (id, name) VALUES (2, 'Sci-Fi');
`
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, err := w.CreateFormFile("file", "script.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(scriptContent))
	_ = w.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/session/%s/run-sql-script?stop_on_error=true", ts.URL, conn.ID), &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("run-sql-script failed: status=%d, err=%v", resp.StatusCode, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(respBody), `"status":"ok"`) && !strings.Contains(string(respBody), `"status":"completed"`) {
		t.Fatalf("expected completed status in run-sql-script output, got: %s", string(respBody))
	}

	// Verify tags table created and populated
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/table/tags/rows?limit=10", ts.URL, conn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get tags rows failed: %v", err)
	}
	var tagRows struct {
		Rows [][]any `json:"rows"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tagRows)
	resp.Body.Close()
	if len(tagRows.Rows) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tagRows.Rows))
	}
}

// TestRunSQLScript_SafetyGuardGate verifies the Large SQL Runner no longer bypasses the Safety
// Guard: a script containing a destructive DROP must be blocked (HTTP 409) unless confirm=true, and
// a confirmed destructive run must leave a pre-DDL backup snapshot behind.
func TestRunSQLScript_SafetyGuardGate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-script-guard-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	backupDir := filepath.Join(tempDir, "backups")
	v, err := vault.OpenVault(filepath.Join(tempDir, "config.db"), "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, backupDir, "test-master")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	// Production environment so DROP is a hard CRITICAL challenge regardless of row estimates.
	createPayload := vault.ConnectionRecord{
		Name:         "Script Guard Test",
		Environment:  "Production",
		Driver:       "sqlite",
		DatabaseName: filepath.Join(tempDir, "target.db"),
	}
	body, _ := json.Marshal(createPayload)
	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create connection failed: %v", err)
	}
	var conn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&conn)
	resp.Body.Close()

	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, conn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("connect session failed: %v", err)
	}
	resp.Body.Close()

	// Production sessions auto-lock to read-only; unlock so we can exercise mutations/DDL.
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/toggle-readonly", ts.URL, conn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle read-only failed: %v", err)
	}
	resp.Body.Close()
	// Seed a table with rows. In Production even CREATE is challenge-gated, so the seed must pass
	// confirm=true (this also exercises the confirmed path for a non-DROP statement).
	seedScript := "CREATE TABLE victims (id INTEGER PRIMARY KEY, v TEXT);\nINSERT INTO victims (id, v) VALUES (1, 'a'), (2, 'b');\n"
	runScript := func(script string, query string) (*http.Response, []byte) {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		part, _ := mw.CreateFormFile("file", "script.sql")
		_, _ = part.Write([]byte(script))
		_ = mw.Close()
		url := fmt.Sprintf("%s/api/session/%s/run-sql-script?stop_on_error=true", ts.URL, conn.ID)
		if query != "" {
			url += "&" + query
		}
		req, _ := http.NewRequest("POST", url, &b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		r, e := client.Do(req)
		if e != nil {
			t.Fatalf("run-sql-script request failed: %v", e)
		}
		data, _ := io.ReadAll(r.Body)
		r.Body.Close()
		return r, data
	}

	if r, data := runScript(seedScript, "confirm=true"); r.StatusCode != http.StatusOK {
		t.Fatalf("seed script failed: status=%d body=%s", r.StatusCode, data)
	}

	// 1. Destructive script WITHOUT confirm must be blocked with 409 and must not drop the table.
	dropScript := "INSERT INTO victims (id, v) VALUES (3, 'c');\nDROP TABLE victims;\n"
	r, data := runScript(dropScript, "")
	if r.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for unconfirmed destructive script, got %d: %s", r.StatusCode, data)
	}
	var blocked struct {
		RequiresChallenge bool `json:"requires_challenge"`
		Blockers          []struct {
			Operation   string `json:"operation"`
			TargetTable string `json:"target_table"`
		} `json:"blockers"`
	}
	if err := json.Unmarshal(data, &blocked); err != nil {
		t.Fatalf("decode block response: %v (%s)", err, data)
	}
	if !blocked.RequiresChallenge || len(blocked.Blockers) != 1 || blocked.Blockers[0].Operation != "DROP" {
		t.Fatalf("expected one DROP blocker, got: %s", data)
	}

	// The table must still exist and have its original 2 rows (the INSERT before DROP must not have run).
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/table/victims/rows?limit=10", ts.URL, conn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("victims should still exist after blocked script: status=%d err=%v", resp.StatusCode, err)
	}
	var rowsResp struct {
		Rows [][]any `json:"rows"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rowsResp)
	resp.Body.Close()
	if len(rowsResp.Rows) != 2 {
		t.Fatalf("expected table untouched (2 rows) after blocked script, got %d", len(rowsResp.Rows))
	}

	// 2. Same script WITH confirm=true must run and drop the table, leaving a pre-DDL backup.
	r, data = runScript("DROP TABLE victims;\n", "confirm=true")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("confirmed destructive script should succeed: status=%d body=%s", r.StatusCode, data)
	}
	if !strings.Contains(string(data), `"status":"completed"`) {
		t.Fatalf("expected completed status, got: %s", data)
	}

	// A pre-DDL Smart Tiered Backup of victims must exist.
	entries, _ := os.ReadDir(backupDir)
	foundBackup := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "victims") && strings.HasSuffix(e.Name(), ".sql") {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Fatalf("expected a pre-DDL backup snapshot for 'victims' in %s, entries=%v", backupDir, entries)
	}
}

func TestRunSQLScript_PayloadSizeLimit(t *testing.T) {
	tempDir := t.TempDir()
	v, err := vault.OpenVault(filepath.Join(tempDir, "config.db"), "testpass")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "testpass")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	conn := vault.ConnectionRecord{
		Name:         "Limit Test",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: filepath.Join(tempDir, "limit.db"),
	}
	body, _ := json.Marshal(conn)
	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create conn: %v", err)
	}
	var created vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, created.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("connect: %v", err)
	}
	resp.Body.Close()

	// An oversized raw body (>100MB ContentLength) must be rejected with 413 Payload Too Large.
	url := fmt.Sprintf("%s/api/session/%s/run-sql-script", ts.URL, created.ID)
	oversized := io.LimitReader(&repeatReader{b: ' '}, maxScriptPayloadBytes+1)
	req, _ := http.NewRequest("POST", url, oversized)
	req.Header.Set("Content-Type", "text/plain")
	r, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized script, got: %d", r.StatusCode)
	}
}

type repeatReader struct {
	b byte
}

func (r *repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

func TestSplitSQLStatements_DoubledQuotesAndDollarQuotes(t *testing.T) {
	script := `
INSERT INTO logs (msg) VALUES ('Failed; couldn''t connect');
CREATE FUNCTION hello() RETURNS text AS $$
BEGIN
    SELECT 'internal; semicolon';
    RETURN 'ok';
END;
$$ LANGUAGE plpgsql;
SELECT * FROM users;
`
	stmts := splitSQLStatements(strings.NewReader(script))
	if len(stmts) != 3 {
		t.Fatalf("expected exactly 3 statements, got %d: %+v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "couldn''t connect") {
		t.Errorf("stmt 0 should preserve doubled quotes: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "$$") || !strings.Contains(stmts[1], "internal; semicolon") {
		t.Errorf("stmt 1 should keep entire dollar block intact: %s", stmts[1])
	}
	if !strings.Contains(stmts[2], "SELECT * FROM users") {
		t.Errorf("stmt 2 should be select: %s", stmts[2])
	}
}

func TestSplitSQLStatements_InlineCommentsAndEscapedBackslashes(t *testing.T) {
	// Case 1: Inline comment with semicolon should NOT terminate the statement prematurely
	script1 := "SELECT 1 -- ; this is a comment with semicolon\n;"
	stmts1 := splitSQLStatements(strings.NewReader(script1))
	if len(stmts1) != 1 {
		t.Fatalf("expected exactly 1 statement, got %d: %+v", len(stmts1), stmts1)
	}

	// Case 2: Windows path with trailing escaped backslash
	script2 := "INSERT INTO paths VALUES ('c:\\\\');\nSELECT 2;"
	stmts2 := splitSQLStatements(strings.NewReader(script2))
	if len(stmts2) != 2 {
		t.Fatalf("expected exactly 2 statements, got %d: %+v", len(stmts2), stmts2)
	}
	if !strings.Contains(stmts2[0], "c:\\") {
		t.Errorf("stmt 0 should contain path: %s", stmts2[0])
	}
	if !strings.Contains(stmts2[1], "SELECT 2") {
		t.Errorf("stmt 1 should be SELECT 2: %s", stmts2[1])
	}
}

func TestSplitSQLStatements_PostgresHashOperatorAndTrailingComments(t *testing.T) {
	// Case 1: Postgres #>> operator should NOT be treated as a comment!
	pgScript := "SELECT data #>> '{a}' FROM t;\nSELECT 2;"
	stmts1 := splitSQLStatements(strings.NewReader(pgScript), "postgres")
	if len(stmts1) != 2 {
		t.Fatalf("expected 2 statements for postgres #>> operator, got %d: %+v", len(stmts1), stmts1)
	}
	if !strings.Contains(stmts1[0], "#>>") {
		t.Errorf("stmt 0 should contain #>>: %s", stmts1[0])
	}
	if !strings.Contains(stmts1[1], "SELECT 2") {
		t.Errorf("stmt 1 should be SELECT 2: %s", stmts1[1])
	}

	// Case 2: Trailing comment only should NOT produce an empty statement
	script2 := "COMMIT;\n-- done comment"
	stmts2 := splitSQLStatements(strings.NewReader(script2))
	if len(stmts2) != 1 {
		t.Fatalf("expected 1 statement (excluding comment-only trailing statement), got %d: %+v", len(stmts2), stmts2)
	}
}

func TestSplitSQLStatements_MySQLVersionComments(t *testing.T) {
	script := `
/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
CREATE TABLE users (id int) ENGINE=InnoDB /*!40101 DEFAULT CHARSET=utf8mb4 */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
`
	stmts := splitSQLStatements(strings.NewReader(script), "mysql")
	if len(stmts) != 4 {
		t.Fatalf("expected 4 statements including version comments, got %d: %+v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "FOREIGN_KEY_CHECKS") && !strings.Contains(stmts[1], "FOREIGN_KEY_CHECKS=0") {
		t.Errorf("expected FOREIGN_KEY_CHECKS statement, got %s", stmts[1])
	}
	if !strings.Contains(stmts[2], "/*!40101 DEFAULT CHARSET=utf8mb4 */") {
		t.Errorf("expected table DDL to preserve version comment, got %s", stmts[2])
	}
}

func TestSplitSQLStatements_PostgresTrailingBackslashInString(t *testing.T) {
	// In Postgres, backslash is not an escape character in standard strings
	pgScript := "INSERT INTO paths VALUES ('C:\\');\nSELECT 2;"
	stmts := splitSQLStatements(strings.NewReader(pgScript), "postgres")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements in postgres, got %d: %+v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "'C:\\'") {
		t.Errorf("stmt 0 should contain 'C:\\', got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "SELECT 2") {
		t.Errorf("stmt 1 should be SELECT 2, got: %s", stmts[1])
	}
}

func TestSplitSQLStatements_TaggedDollarQuotesAndLongLines(t *testing.T) {
	pgScript := "CREATE FUNCTION f() RETURNS int AS $body$ BEGIN PERFORM 1; RETURN 2; END $body$ LANGUAGE plpgsql;\n" +
		"SELECT $1::int, price$usd FROM t;\n" +
		"SELECT $_$ a;b $_$;"
	stmts := splitSQLStatements(strings.NewReader(pgScript), "postgres")
	if len(stmts) != 3 || !strings.Contains(stmts[0], "RETURN 2; END $body$ LANGUAGE plpgsql") {
		t.Fatalf("tagged dollar quotes split incorrectly: %q", stmts)
	}

	// A single line longer than any scanner buffer must not truncate the script.
	long := "INSERT INTO t VALUES ('" + strings.Repeat("x", 11*1024*1024) + "');\nSELECT 2;"
	stmts = splitSQLStatements(strings.NewReader(long), "mysql")
	if len(stmts) != 2 || stmts[1] != "SELECT 2" {
		t.Fatalf("long line truncated the script: got %d statements", len(stmts))
	}
}
