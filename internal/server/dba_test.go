package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sqlfox/internal/vault"
)

func TestDBAOperationsAndAuditLog(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-dba-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "vault.db")
	v, err := vault.OpenVault(dbPath, "test-master-pw")
	if err != nil {
		t.Fatalf("failed to open vault: %v", err)
	}
	defer v.Close()

	// Create test sqlite connection
	sqliteDBPath := filepath.Join(tmpDir, "target.db")
	connRec := &vault.ConnectionRecord{
		Name:         "Test SQLite",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: sqliteDBPath,
	}
	if err := v.SaveConnection(connRec); err != nil {
		t.Fatalf("failed to save connection: %v", err)
	}

	backupDir := filepath.Join(tmpDir, "backups")
	srv := NewServer(v, nil, backupDir, "test-master-pw")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// Connect to session
	connectResp, err := client.Post(ts.URL+"/api/session/connect/"+connRec.ID, "application/json", nil)
	if err != nil || connectResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(connectResp.Body)
		t.Fatalf("connect failed with code %d: %s", connectResp.StatusCode, string(body))
	}
	connectResp.Body.Close()

	// 1. Test GetProcesslist
	plResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/processlist")
	if err != nil || plResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(plResp.Body)
		t.Fatalf("processlist failed with code %d: %s", plResp.StatusCode, string(body))
	}
	var plData map[string]any
	_ = json.NewDecoder(plResp.Body).Decode(&plData)
	plResp.Body.Close()

	if _, ok := plData["threads"]; !ok {
		t.Fatalf("expected 'threads' in processlist output")
	}

	// 2. Test KillProcess
	killPayload := bytes.NewBufferString(`{"thread_id": 1, "mode": "query"}`)
	killResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/kill", "application/json", killPayload)
	if err != nil || killResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(killResp.Body)
		t.Fatalf("kill failed with code %d: %s", killResp.StatusCode, string(body))
	}
	killResp.Body.Close()

	// 3. Test Execute Query records into Session Audit Log
	execPayload := bytes.NewBufferString(`{"sql": "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO items (name) VALUES ('widget'); SELECT * FROM items;"}`)
	execResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/execute", "application/json", execPayload)
	if err != nil || execResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(execResp.Body)
		t.Fatalf("execute failed with code %d: %s", execResp.StatusCode, string(body))
	}
	execResp.Body.Close()

	// Sleep briefly for async audit log write
	time.Sleep(150 * time.Millisecond)

	// 4. Test GetAuditLogs
	auditResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/audit-logs")
	if err != nil || auditResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(auditResp.Body)
		t.Fatalf("get audit logs failed with code %d: %s", auditResp.StatusCode, string(body))
	}

	var auditData struct {
		Logs   []map[string]any `json:"logs"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	_ = json.NewDecoder(auditResp.Body).Decode(&auditData)
	auditResp.Body.Close()

	if auditData.Total < 1 || len(auditData.Logs) < 1 {
		t.Fatalf("expected at least 1 audit log, got total=%d len=%d", auditData.Total, len(auditData.Logs))
	}

	// 5. Test ClearAuditLogs
	delReq, _ := http.NewRequest("DELETE", ts.URL+"/api/session/"+connRec.ID+"/audit-logs", nil)
	clearResp, err := client.Do(delReq)
	if err != nil || clearResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(clearResp.Body)
		t.Fatalf("clear audit logs failed with code %d: %s", clearResp.StatusCode, string(body))
	}
	clearResp.Body.Close()

	// Verify empty after clear
	auditResp2, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/audit-logs")
	if err != nil || auditResp2.StatusCode != http.StatusOK {
		t.Fatalf("get audit logs after clear failed: %v", err)
	}
	var auditData2 struct {
		Logs  []map[string]any `json:"logs"`
		Total int              `json:"total"`
	}
	_ = json.NewDecoder(auditResp2.Body).Decode(&auditData2)
	auditResp2.Body.Close()

	if auditData2.Total != 0 || len(auditData2.Logs) != 0 {
		t.Fatalf("expected 0 audit logs after clear, got total=%d", auditData2.Total)
	}
}

func TestAdvancedDiagnosticsAndProfiling(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-diag-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "vault.db")
	v, err := vault.OpenVault(dbPath, "test-master-pw")
	if err != nil {
		t.Fatalf("failed to open vault: %v", err)
	}
	defer v.Close()

	sqliteDBPath := filepath.Join(tmpDir, "target.db")
	connRec := &vault.ConnectionRecord{
		Name:         "Diag SQLite",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: sqliteDBPath,
	}
	if err := v.SaveConnection(connRec); err != nil {
		t.Fatalf("failed to save connection: %v", err)
	}

	srv := NewServer(v, nil, filepath.Join(tmpDir, "backups"), "test-master-pw")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// Connect
	cResp, err := client.Post(ts.URL+"/api/session/connect/"+connRec.ID, "application/json", nil)
	if err != nil || cResp.StatusCode != http.StatusOK {
		t.Fatalf("connect failed: %v", err)
	}
	cResp.Body.Close()

	// Seed tables
	seedSQL := `
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
		CREATE INDEX idx_users_name ON users(name);
		CREATE INDEX idx_users_name_dup ON users(name);
		INSERT INTO users (name) VALUES ('Alice'), ('Bob');
	`
	execPayload := bytes.NewBufferString(`{"sql": "` + strings.ReplaceAll(strings.ReplaceAll(seedSQL, "\n", " "), "\t", " ") + `"}`)
	eResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/execute", "application/json", execPayload)
	if err != nil || eResp.StatusCode != http.StatusOK {
		t.Fatalf("seed failed: %v", err)
	}
	eResp.Body.Close()

	// 1. Table Diagnostics
	tdResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/diagnostics/tables")
	if err != nil || tdResp.StatusCode != http.StatusOK {
		t.Fatalf("get table diagnostics failed: %v", err)
	}
	var diagRows []map[string]any
	_ = json.NewDecoder(tdResp.Body).Decode(&diagRows)
	tdResp.Body.Close()

	if len(diagRows) == 0 {
		t.Fatalf("expected at least 1 table in diagnostics, got 0")
	}

	// 2. Execute Maintenance
	maintPayload := bytes.NewBufferString(`{"tables": ["users"], "action": "VACUUM"}`)
	mResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/maintenance", "application/json", maintPayload)
	if err != nil || mResp.StatusCode != http.StatusOK {
		t.Fatalf("maintenance failed: %v", err)
	}
	var maintRes []map[string]any
	_ = json.NewDecoder(mResp.Body).Decode(&maintRes)
	mResp.Body.Close()

	if len(maintRes) == 0 || maintRes[0]["status"] != "OK" {
		t.Fatalf("unexpected maintenance result: %+v", maintRes)
	}

	// 3. Redundant Indexes
	riResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/diagnostics/redundant-indexes?table=users")
	if err != nil || riResp.StatusCode != http.StatusOK {
		t.Fatalf("get redundant indexes failed: %v", err)
	}
	var redIndexes []map[string]any
	_ = json.NewDecoder(riResp.Body).Decode(&redIndexes)
	riResp.Body.Close()

	if len(redIndexes) == 0 {
		t.Fatalf("expected redundant index for idx_users_name_dup, got none")
	}

	// 4. FK Peeking
	peekResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/peek?table=users&column=id&value=1")
	if err != nil || peekResp.StatusCode != http.StatusOK {
		t.Fatalf("peek failed: %v", err)
	}
	var peekData map[string]any
	_ = json.NewDecoder(peekResp.Body).Decode(&peekData)
	peekResp.Body.Close()

	if row, ok := peekData["row"].(map[string]any); !ok || row["name"] != "Alice" {
		t.Fatalf("expected row with name=Alice, got %+v", peekData)
	}

	// 5. Query Profiling
	profPayload := bytes.NewBufferString(`{"sql": "SELECT * FROM users WHERE id = 1", "profile": true}`)
	profResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/execute", "application/json", profPayload)
	if err != nil || profResp.StatusCode != http.StatusOK {
		t.Fatalf("execute with profile failed: %v", err)
	}
	var profExecRes map[string]any
	_ = json.NewDecoder(profResp.Body).Decode(&profExecRes)
	profResp.Body.Close()

	if prof, ok := profExecRes["profile"].(map[string]any); !ok || len(prof["stages"].([]any)) == 0 {
		t.Fatalf("expected profile with stages, got %+v", profExecRes)
	}

	// 6. User Management on SQLite (gracefully returns error)
	userResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/users")
	if err != nil {
		t.Fatalf("user request failed: %v", err)
	}
	if userResp.StatusCode == http.StatusOK {
		t.Fatalf("expected SQLite to fail user management, but got 200 OK")
	}
	userResp.Body.Close()
}

