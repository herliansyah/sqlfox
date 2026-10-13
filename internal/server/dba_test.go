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
	if auditData.Logs[0]["source"] != "editor" {
		t.Fatalf("expected source 'editor', got %v", auditData.Logs[0]["source"])
	}

	// 4b. Test Cell-Edit records with source 'grid_cell_edit'
	cellEditPayload := bytes.NewBufferString(`{
		"table": "items",
		"column": "name",
		"value": "gadget",
		"pk_column": "id",
		"pk_value": 16906691
	}`)
	ceResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/cell-edit", "application/json", cellEditPayload)
	if err != nil || ceResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(ceResp.Body)
		t.Fatalf("cell-edit failed with code %d: %s", ceResp.StatusCode, string(body))
	}
	ceResp.Body.Close()

	time.Sleep(150 * time.Millisecond)

	// Verify cell-edit source filtering in audit logs
	ceAuditResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/audit-logs?source=grid_cell_edit")
	if err != nil || ceAuditResp.StatusCode != http.StatusOK {
		t.Fatalf("get audit logs with source failed: %v", err)
	}
	var ceAuditData struct {
		Logs  []map[string]any `json:"logs"`
		Total int              `json:"total"`
	}
	_ = json.NewDecoder(ceAuditResp.Body).Decode(&ceAuditData)
	ceAuditResp.Body.Close()

	if ceAuditData.Total != 1 || len(ceAuditData.Logs) != 1 {
		t.Fatalf("expected 1 grid_cell_edit log, got total=%d len=%d", ceAuditData.Total, len(ceAuditData.Logs))
	}
	if ceAuditData.Logs[0]["source"] != "grid_cell_edit" {
		t.Fatalf("expected source grid_cell_edit, got %v", ceAuditData.Logs[0]["source"])
	}
	auditSQL, _ := ceAuditData.Logs[0]["query_sql"].(string)
	if strings.Contains(auditSQL, "1.6906691e+07") || !strings.Contains(auditSQL, "16906691") {
		t.Fatalf("expected query_sql to contain integer ID 16906691 without scientific notation, got: %s", auditSQL)
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

	// 0. Database Diagnostics (Level 1)
	ddResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/diagnostics/databases")
	if err != nil || ddResp.StatusCode != http.StatusOK {
		t.Fatalf("get database diagnostics failed: %v", err)
	}
	var dbDiagRows []map[string]any
	_ = json.NewDecoder(ddResp.Body).Decode(&dbDiagRows)
	ddResp.Body.Close()

	if len(dbDiagRows) == 0 {
		t.Fatalf("expected at least 1 database in diagnostics, got 0")
	}
	if dbDiagRows[0]["table_count"].(float64) < 1 {
		t.Fatalf("expected table_count >= 1, got %v", dbDiagRows[0]["table_count"])
	}

	// 1. Table Diagnostics (Level 2)
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
	if hasPK, ok := diagRows[0]["has_primary_key"].(bool); !ok || !hasPK {
		t.Fatalf("expected has_primary_key true for users table, got %v", diagRows[0]["has_primary_key"])
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

	// 7. Cluster Health Inspection
	clusterResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/diagnostics/cluster")
	if err != nil || clusterResp.StatusCode != http.StatusOK {
		t.Fatalf("cluster health request failed: %v", err)
	}
	var clusterData map[string]any
	_ = json.NewDecoder(clusterResp.Body).Decode(&clusterData)
	clusterResp.Body.Close()

	if clusterData["topology_type"] != "standalone" {
		t.Fatalf("expected topology_type standalone, got %v", clusterData["topology_type"])
	}
	if clusterData["bottleneck_severity"] != "ok" {
		t.Fatalf("expected bottleneck_severity ok, got %v", clusterData["bottleneck_severity"])
	}

	// 8. Server Variables & Advisor Inspection
	varsResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/diagnostics/variables")
	if err != nil || varsResp.StatusCode != http.StatusOK {
		t.Fatalf("variables request failed: %v", err)
	}
	var varsData map[string]any
	_ = json.NewDecoder(varsResp.Body).Decode(&varsData)
	varsResp.Body.Close()

	if varsData["engine"] != "sqlite" {
		t.Fatalf("expected engine sqlite, got %v", varsData["engine"])
	}
	varsList, ok := varsData["variables"].([]any)
	if !ok || len(varsList) == 0 {
		t.Fatalf("expected non-empty variables list, got %v", varsData["variables"])
	}

	// 9. Deadlock Inspection API
	deadlockResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/diagnostics/deadlock")
	if err != nil || deadlockResp.StatusCode != http.StatusOK {
		t.Fatalf("deadlock request failed: %v", err)
	}
	var deadlockData map[string]any
	_ = json.NewDecoder(deadlockResp.Body).Decode(&deadlockData)
	deadlockResp.Body.Close()
	if deadlockData["has_deadlock"] != false {
		t.Fatalf("expected has_deadlock false for sqlite, got %v", deadlockData["has_deadlock"])
	}

	// 10. Statement Digest API
	digestResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/diagnostics/statement-digest?limit=10")
	if err != nil || digestResp.StatusCode != http.StatusOK {
		t.Fatalf("statement digest request failed: %v", err)
	}
	var digestList []any
	_ = json.NewDecoder(digestResp.Body).Decode(&digestList)
	digestResp.Body.Close()

	// 11. Reset Statement Digest API
	resetResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/diagnostics/statement-digest/reset", "application/json", nil)
	if err != nil || resetResp.StatusCode != http.StatusOK {
		t.Fatalf("reset statement digest request failed: %v", err)
	}
	resetResp.Body.Close()

	// 12. Routine & Event DDL and Partitions APIs
	rDdlResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/ddl/routine/test_proc?type=PROCEDURE")
	if err != nil || rDdlResp.StatusCode != http.StatusOK {
		t.Fatalf("routine DDL request failed: %v", err)
	}
	rDdlResp.Body.Close()

	eDdlResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/ddl/event/test_ev")
	if err != nil || eDdlResp.StatusCode != http.StatusOK {
		t.Fatalf("event DDL request failed: %v", err)
	}
	eDdlResp.Body.Close()

	togglePayload, _ := json.Marshal(map[string]bool{"enable": true})
	toggleResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/event/test_ev/toggle", "application/json", bytes.NewReader(togglePayload))
	if err != nil || toggleResp.StatusCode != http.StatusOK {
		t.Fatalf("toggle event request failed: %v", err)
	}
	toggleResp.Body.Close()

	partResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/partitions/items")
	if err != nil || partResp.StatusCode != http.StatusOK {
		t.Fatalf("partitions request failed: %v", err)
	}
	partResp.Body.Close()
}



