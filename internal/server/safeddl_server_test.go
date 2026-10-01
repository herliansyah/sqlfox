package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/sqlite"
	"sqlfox/internal/safety"
)

func TestPreflightDDLAndSafeExecution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-safeddl-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "app.db")
	backupDir := filepath.Join(tempDir, "backups")

	ctx := context.Background()
	dbSession, err := driver.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer dbSession.Close()

	// Setup relational schema: master_status and orders
	_, err = dbSession.Exec(ctx, "CREATE TABLE master_status (id INTEGER PRIMARY KEY, status_name TEXT)")
	if err != nil {
		t.Fatalf("CREATE master_status failed: %v", err)
	}
	_, err = dbSession.Exec(ctx, "CREATE TABLE orders (id INTEGER PRIMARY KEY, status_id INTEGER REFERENCES master_status(id), total REAL)")
	if err != nil {
		t.Fatalf("CREATE orders failed: %v", err)
	}

	backupMgr := safety.NewBackupManager(backupDir)

	activeSess := &ActiveSession{
		ID:          "conn-prod-1",
		Name:        "Prod Database",
		Environment: "Production",
		DriverName:  "sqlite",
		ReadOnly:    false,
		Session:     dbSession,
		backupMgr:   backupMgr,
	}

	sm := &SessionManager{
		backupMgr: backupMgr,
		sessions:  make(map[string]*ActiveSession),
	}
	sm.sessions[activeSess.ID] = activeSess

	srv := &Server{
		sessionMgr: sm,
		backupMgr:  backupMgr,
	}

	// 1. Test handlePreflightDDL endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/session/"+activeSess.ID+"/preflight-ddl/master_status?alter=ADD+COLUMN+code+TEXT", nil)
	req.SetPathValue("id", activeSess.ID)
	req.SetPathValue("table", "master_status")

	rr := httptest.NewRecorder()
	srv.handlePreflightDDL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handlePreflightDDL returned status %d: %s", rr.Code, rr.Body.String())
	}

	var preflightRes struct {
		Table         string                         `json:"table"`
		Relationships []driver.TableRelationshipInfo `json:"relationships"`
		OSCGhost      string                         `json:"osc_ghost"`
		OSCPt         string                         `json:"osc_pt"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &preflightRes); err != nil {
		t.Fatalf("failed to decode preflight response: %v", err)
	}

	if preflightRes.Table != "master_status" {
		t.Errorf("expected table master_status, got %s", preflightRes.Table)
	}
	if len(preflightRes.Relationships) != 1 {
		t.Fatalf("expected 1 inbound relationship from orders, got %d", len(preflightRes.Relationships))
	}
	if preflightRes.Relationships[0].RelatedTable != "orders" {
		t.Errorf("expected related table orders, got %s", preflightRes.Relationships[0].RelatedTable)
	}

	// 2. Test Safe Execution Pipeline: Attempt ALTER TABLE on Production
	alterSQL := "ALTER TABLE master_status ADD COLUMN code TEXT"
	execResp, err := activeSess.Execute(ctx, ExecuteRequest{
		SQL: alterSQL,
	})
	if err != nil {
		t.Fatalf("Execute ALTER failed unexpectedly: %v", err)
	}
	if !execResp.Blocked {
		t.Fatalf("expected ALTER on Production to be blocked by Challenge Modal")
	}
	if execResp.Inspection == nil || len(execResp.Inspection.Relationships) != 1 {
		t.Fatalf("expected inspection to contain relationships warning, got %+v", execResp.Inspection)
	}

	// 3. Confirm challenge and execute safe DDL
	execResp, err = activeSess.Execute(ctx, ExecuteRequest{
		SQL:            alterSQL,
		Confirmed:      true,
		ChallengeInput: "master_status",
		LockTimeoutSec: 2,
	})
	if err != nil {
		t.Fatalf("Confirmed ALTER execution failed: %v", err)
	}
	if execResp.Blocked {
		t.Fatalf("expected confirmed ALTER not to be blocked")
	}

	// Verify column exists
	schema, err := dbSession.InspectSchema(ctx, "")
	if err != nil {
		t.Fatalf("InspectSchema failed: %v", err)
	}
	var found bool
	for _, tbl := range schema.Tables {
		if tbl.Name == "master_status" {
			for _, col := range tbl.Columns {
				if col.Name == "code" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected column 'code' to be present after ALTER TABLE")
	}
}
