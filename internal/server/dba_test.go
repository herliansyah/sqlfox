package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
