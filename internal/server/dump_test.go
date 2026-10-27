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
	if strings.Index(dumpContent, `DROP VIEW IF EXISTS "a_titles"`) < strings.Index(dumpContent, `CREATE TABLE books`) {
		t.Fatalf("expected view after base tables in dump, got:\n%s", dumpContent)
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
	authorsIdx := strings.Index(seedContent, "authors")
	booksIdx := strings.Index(seedContent, "books")
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
