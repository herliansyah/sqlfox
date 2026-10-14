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

	"sqlfox/internal/driver"
	"sqlfox/internal/vault"
)

func setupTwoTestDatabases(t *testing.T) (*Server, *httptest.Server, string, string) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "sqlfox-sync-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	vaultPath := filepath.Join(tempDir, "config.db")
	v, err := vault.OpenVault(vaultPath, "test-master")
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}
	t.Cleanup(func() { v.Close() })

	srcDBPath := filepath.Join(tempDir, "source.db")
	tgtDBPath := filepath.Join(tempDir, "target.db")

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "test-master")
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(func() { ts.Close() })

	client := ts.Client()

	// 1. Create Source connection
	srcRec := vault.ConnectionRecord{
		Name:         "SourceDB",
		Driver:       "sqlite",
		DatabaseName: srcDBPath,
		Environment:  "Development",
	}
	body, _ := json.Marshal(srcRec)
	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create source connection failed")
	}
	var srcSaved vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&srcSaved)
	resp.Body.Close()

	// 2. Create Target connection
	tgtRec := vault.ConnectionRecord{
		Name:         "TargetDB",
		Driver:       "sqlite",
		DatabaseName: tgtDBPath,
		Environment:  "Development",
	}
	body, _ = json.Marshal(tgtRec)
	resp, err = client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create target connection failed")
	}
	var tgtSaved vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&tgtSaved)
	resp.Body.Close()

	// 3. Connect both sessions
	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, srcSaved.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("connect src session failed")
	}
	resp.Body.Close()

	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, tgtSaved.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("connect tgt session failed")
	}
	resp.Body.Close()

	// Populate Source DB:
	srcSess, _ := srv.sessionMgr.GetSession(srcSaved.ID)
	_, err = srcSess.Session.Exec(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT,
			age INTEGER
		);
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY,
			user_id INTEGER,
			amount REAL
		);
	`)
	if err != nil {
		t.Fatalf("failed to populate source db: %v", err)
	}

	// Populate Target DB:
	tgtSess, _ := srv.sessionMgr.GetSession(tgtSaved.ID)
	_, err = tgtSess.Session.Exec(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to populate target db: %v", err)
	}

	return srv, ts, srcSaved.ID, tgtSaved.ID
}

func TestSyncStructureCompareAndApply(t *testing.T) {
	_, ts, srcID, tgtID := setupTwoTestDatabases(t)
	client := ts.Client()

	// 1. Compare structure
	compareReq := SyncStructureCompareRequest{
		SourceSessionID: srcID,
		TargetSessionID: tgtID,
		IncludeDrops:    true,
	}
	body, _ := json.Marshal(compareReq)
	resp, err := client.Post(ts.URL+"/api/sync/structure/compare", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("compare expected status 200, got %v", resp)
	}

	var cmpResp SyncStructureCompareResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmpResp); err != nil {
		t.Fatalf("failed to decode compare response: %v", err)
	}
	resp.Body.Close()

	if len(cmpResp.Diffs) == 0 {
		t.Fatalf("expected diff items, got 0")
	}

	var foundMissingTable, foundMissingCol bool
	var statements []string
	for _, diff := range cmpResp.Diffs {
		if diff.Type == "table_missing" && diff.Table == "orders" {
			foundMissingTable = true
		}
		if diff.Type == "column_missing" && diff.Table == "users" {
			foundMissingCol = true
		}
		if diff.SQL != "" && diff.Selected {
			statements = append(statements, diff.SQL)
		}
	}

	if !foundMissingTable {
		t.Errorf("expected missing table 'orders' in diff")
	}
	if !foundMissingCol {
		t.Errorf("expected missing columns for 'users' in diff")
	}

	// 2. Apply structure migration
	applyReq := SyncStructureApplyRequest{
		TargetSessionID: tgtID,
		Statements:      statements,
		Confirmed:       true,
	}
	applyBody, _ := json.Marshal(applyReq)
	resp, err = client.Post(ts.URL+"/api/sync/structure/apply", "application/json", bytes.NewReader(applyBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("apply expected status 200, got %v", resp)
	}

	var applyResult map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&applyResult); err != nil {
		t.Fatalf("failed to decode apply response: %v", err)
	}
	resp.Body.Close()

	if applyResult["success"] != true {
		t.Fatalf("apply expected success: true, got %v", applyResult)
	}
}

func TestSyncDataTransfer(t *testing.T) {
	srv, ts, srcID, tgtID := setupTwoTestDatabases(t)
	client := ts.Client()

	srcSess, _ := srv.sessionMgr.GetSession(srcID)

	// Insert 25 rows in source table 'orders'
	for i := 1; i <= 25; i++ {
		_, err := srcSess.Session.Exec(context.Background(),
			fmt.Sprintf("INSERT INTO orders (id, user_id, amount) VALUES (%d, %d, %f)", i, 100+i, float64(i)*10.5))
		if err != nil {
			t.Fatalf("failed to insert order: %v", err)
		}
	}

	// Transfer data from Source orders -> Target orders with create_tables: true
	transferReq := SyncDataTransferRequest{
		SourceSessionID: srcID,
		TargetSessionID: tgtID,
		Tables:          []string{"orders"},
		CreateTables:    true,
		TruncateTarget:  true,
		BatchSize:       10,
	}

	reqBody, _ := json.Marshal(transferReq)
	resp, err := client.Post(ts.URL+"/api/sync/data/transfer", "application/json", bytes.NewReader(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("transfer expected status 200, got %v", resp)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	resp.Body.Close()

	output := buf.String()
	if !strings.Contains(output, `"event":"done"`) {
		t.Fatalf("transfer expected 'done' event, got: %s", output)
	}

	// Query target to verify row count
	tgtSess, _ := srv.sessionMgr.GetSession(tgtID)
	stream, err := tgtSess.Session.QueryStream(context.Background(), "SELECT COUNT(*) FROM orders", 1)
	if err != nil {
		t.Fatalf("failed to count target orders: %v", err)
	}
	defer stream.Close()

	chunk, _ := stream.NextChunk(1)
	if len(chunk) == 0 {
		t.Fatalf("expected count row from target orders")
	}

	countVal := fmt.Sprintf("%v", chunk[0][0])
	if countVal != "25" {
		t.Errorf("expected 25 transferred rows, got %s", countVal)
	}
}

func TestSyncDataTransfer_ErrorRollbackAndStatus(t *testing.T) {
	srv, ts, srcID, tgtID := setupTwoTestDatabases(t)
	client := ts.Client()

	srcSess, _ := srv.sessionMgr.GetSession(srcID)
	tgtSess, _ := srv.sessionMgr.GetSession(tgtID)

	// Create source table with rows
	_, _ = srcSess.Session.Exec(context.Background(), "CREATE TABLE items (id INTEGER, val TEXT);")
	_, _ = srcSess.Session.Exec(context.Background(), "INSERT INTO items VALUES (1, 'a'), (1, 'b');")

	// Create target table with UNIQUE constraint so inserting duplicate id fails midway
	_, _ = tgtSess.Session.Exec(context.Background(), "CREATE TABLE items (id INTEGER UNIQUE, val TEXT);")

	reqBody := SyncDataTransferRequest{
		SourceSessionID: srcID,
		TargetSessionID: tgtID,
		Tables:          []string{"items"},
		BatchSize:       1,
	}
	b, _ := json.Marshal(reqBody)
	resp, err := client.Post(ts.URL+"/api/sync/data/transfer", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("transfer request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	if !strings.Contains(bodyStr, `"event":"table_error"`) {
		t.Errorf("expected table_error event in stream, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"status":"Transfer completed with errors"`) {
		t.Errorf("expected status 'Transfer completed with errors', got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"has_errors":true`) {
		t.Errorf("expected has_errors:true, got:\n%s", bodyStr)
	}

	// Verify target table was rolled back and contains 0 rows
	stream, err := tgtSess.Session.QueryStream(context.Background(), "SELECT COUNT(*) FROM items", 1)
	if err != nil {
		t.Fatalf("failed to query target items: %v", err)
	}
	defer stream.Close()
	chunk, _ := stream.NextChunk(1)
	if len(chunk) == 0 || fmt.Sprintf("%v", chunk[0][0]) != "0" {
		t.Fatalf("expected 0 rows in target due to rollback, got %v", chunk)
	}
}

func TestSyncDataCompareAndApply(t *testing.T) {
	srv, ts, srcID, tgtID := setupTwoTestDatabases(t)
	client := ts.Client()

	srcSess, _ := srv.sessionMgr.GetSession(srcID)
	tgtSess, _ := srv.sessionMgr.GetSession(tgtID)

	// Ensure users table in target has identical columns
	_, _ = tgtSess.Session.Exec(context.Background(), "ALTER TABLE users ADD COLUMN email TEXT")
	_, _ = tgtSess.Session.Exec(context.Background(), "ALTER TABLE users ADD COLUMN age INTEGER")

	// Source:
	// User 1: Alice, alice@example.com, 25
	// User 2: Bob, bob@example.com, 30
	// User 3: Charlie, charlie@example.com, 35
	_, _ = srcSess.Session.Exec(context.Background(), "INSERT INTO users (id, name, email, age) VALUES (1, 'Alice', 'alice@example.com', 25)")
	_, _ = srcSess.Session.Exec(context.Background(), "INSERT INTO users (id, name, email, age) VALUES (2, 'Bob', 'bob@example.com', 30)")
	_, _ = srcSess.Session.Exec(context.Background(), "INSERT INTO users (id, name, email, age) VALUES (3, 'Charlie', 'charlie@example.com', 35)")

	// Target:
	// User 1: Alice, alice@example.com, 25 (Same)
	// User 2: Bob, bob@old.com, 29 (Different -> UPDATE)
	// User 4: Dave, dave@example.com, 40 (Missing in source -> DELETE if delete_missing)
	// Missing User 3 in target -> INSERT
	_, _ = tgtSess.Session.Exec(context.Background(), "INSERT INTO users (id, name, email, age) VALUES (1, 'Alice', 'alice@example.com', 25)")
	_, _ = tgtSess.Session.Exec(context.Background(), "INSERT INTO users (id, name, email, age) VALUES (2, 'Bob', 'bob@old.com', 29)")
	_, _ = tgtSess.Session.Exec(context.Background(), "INSERT INTO users (id, name, email, age) VALUES (4, 'Dave', 'dave@example.com', 40)")

	// 1. Compare
	compareReq := SyncDataCompareRequest{
		SourceSessionID: srcID,
		TargetSessionID: tgtID,
		Table:           "users",
		DeleteMissing:   true,
	}

	body, _ := json.Marshal(compareReq)
	resp, err := client.Post(ts.URL+"/api/sync/data/compare", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("data compare expected 200, got %v", resp)
	}

	var cmpResp SyncDataCompareResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmpResp); err != nil {
		t.Fatalf("failed to decode data compare response: %v", err)
	}
	resp.Body.Close()

	if cmpResp.Summary["insert_count"] != 1 {
		t.Errorf("expected 1 insert (Charlie), got %d", cmpResp.Summary["insert_count"])
	}
	if cmpResp.Summary["update_count"] != 1 {
		t.Errorf("expected 1 update (Bob), got %d", cmpResp.Summary["update_count"])
	}
	if cmpResp.Summary["delete_count"] != 1 {
		t.Errorf("expected 1 delete (Dave), got %d", cmpResp.Summary["delete_count"])
	}

	// 2. Apply operations
	applyReq := SyncDataApplyRequest{
		TargetSessionID: tgtID,
		Table:           "users",
		Operations:      cmpResp.Operations,
		Confirmed:       true,
	}

	applyBody, _ := json.Marshal(applyReq)
	resp, err = client.Post(ts.URL+"/api/sync/data/apply", "application/json", bytes.NewReader(applyBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("data apply expected 200, got %v", resp)
	}
	resp.Body.Close()

	// 3. Verify Target now has exact rows of Source: Alice(1), Bob(2), Charlie(3) and Dave(4) deleted
	stream, err := tgtSess.Session.QueryStream(context.Background(), "SELECT id, name, email, age FROM users ORDER BY id", 10)
	if err != nil {
		t.Fatalf("failed to query target users: %v", err)
	}
	defer stream.Close()

	rows, _ := stream.NextChunk(10)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows in target after sync, got %d: %v", len(rows), rows)
	}

	// Check Bob has updated email
	bobRow := rows[1]
	if fmt.Sprintf("%v", bobRow[2]) != "bob@example.com" {
		t.Errorf("expected Bob email to be updated to 'bob@example.com', got '%v'", bobRow[2])
	}
}

func TestPostgresStructureSyncColumnAlter(t *testing.T) {
	srcCol := driver.ColumnMetadata{Name: "score", Type: "bigint", IsNullable: false, DefaultValue: "0"}
	tgtCol := driver.ColumnMetadata{Name: "score", Type: "integer", IsNullable: true, DefaultValue: ""}

	if columnEquals(srcCol, tgtCol) {
		t.Fatal("expected columns to not equal")
	}

	// Verify DDL generation logic for postgres target
	tgtDriver := "postgres"
	var pgStmts []string
	tblQ := quoteIdent("leaderboard", tgtDriver)
	colQ := quoteIdent(srcCol.Name, tgtDriver)
	if !strings.EqualFold(srcCol.Type, tgtCol.Type) {
		pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tblQ, colQ, srcCol.Type))
	}
	if srcCol.IsNullable != tgtCol.IsNullable {
		if srcCol.IsNullable {
			pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", tblQ, colQ))
		} else {
			pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tblQ, colQ))
		}
	}
	if srcCol.DefaultValue != tgtCol.DefaultValue {
		if srcCol.DefaultValue == "" {
			pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", tblQ, colQ))
		} else {
			pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", tblQ, colQ, srcCol.DefaultValue))
		}
	}
	sql := strings.Join(pgStmts, "\n")
	expected := "ALTER TABLE \"leaderboard\" ALTER COLUMN \"score\" TYPE bigint;\nALTER TABLE \"leaderboard\" ALTER COLUMN \"score\" SET NOT NULL;\nALTER TABLE \"leaderboard\" ALTER COLUMN \"score\" SET DEFAULT 0;"
	if sql != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, sql)
	}
}

func TestSyncData_CompositePKCompareAndApply(t *testing.T) {
	srv, ts, srcID, tgtID := setupTwoTestDatabases(t)
	client := ts.Client()

	srcSess, _ := srv.sessionMgr.GetSession(srcID)
	tgtSess, _ := srv.sessionMgr.GetSession(tgtID)

	schemaSQL := `CREATE TABLE order_items (
		order_id INTEGER,
		product_id INTEGER,
		quantity INTEGER,
		PRIMARY KEY (order_id, product_id)
	);`
	_, _ = srcSess.Session.Exec(context.Background(), schemaSQL)
	_, _ = tgtSess.Session.Exec(context.Background(), schemaSQL)

	// Source: (1, 10, 2), (1, 20, 5)
	_, _ = srcSess.Session.Exec(context.Background(), "INSERT INTO order_items VALUES (1, 10, 2), (1, 20, 5);")

	// Target: (1, 10, 1), (1, 20, 5), (1, 30, 9)
	_, _ = tgtSess.Session.Exec(context.Background(), "INSERT INTO order_items VALUES (1, 10, 1), (1, 20, 5), (1, 30, 9);")

	// 1. Compare
	compReq := SyncDataCompareRequest{
		SourceSessionID: srcID,
		TargetSessionID: tgtID,
		Table:           "order_items",
		DeleteMissing:   true,
	}
	b, _ := json.Marshal(compReq)
	resp, err := client.Post(ts.URL+"/api/sync/data/compare", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("compare failed: %v", err)
	}
	var compResp SyncDataCompareResponse
	_ = json.NewDecoder(resp.Body).Decode(&compResp)
	resp.Body.Close()

	if compResp.Summary["update_count"] != 1 || compResp.Summary["delete_count"] != 1 {
		t.Fatalf("expected 1 update and 1 delete in summary, got: %+v", compResp.Summary)
	}

	// 2. Apply
	applyReq := SyncDataApplyRequest{
		TargetSessionID: tgtID,
		Table:           "order_items",
		Operations:      compResp.Operations,
	}
	b, _ = json.Marshal(applyReq)
	resp, err = client.Post(ts.URL+"/api/sync/data/apply", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("apply failed: %v", err)
	}
	var applyResp struct {
		Success  bool     `json:"success"`
		Inserted int      `json:"inserted"`
		Updated  int      `json:"updated"`
		Deleted  int      `json:"deleted"`
		Errors   []string `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&applyResp)
	resp.Body.Close()

	if len(applyResp.Errors) > 0 {
		t.Fatalf("apply had errors: %v", applyResp.Errors)
	}

	// 3. Verify target has exactly 2 rows: (1, 10, 2) and (1, 20, 5)
	stream, err := tgtSess.Session.QueryStream(context.Background(), "SELECT order_id, product_id, quantity FROM order_items ORDER BY product_id ASC", 10)
	if err != nil {
		t.Fatalf("query target failed: %v", err)
	}
	defer stream.Close()
	rows, _ := stream.NextChunk(10)
	if len(rows) != 2 {
		t.Fatalf("expected exactly 2 rows in target after sync, got %d: %v", len(rows), rows)
	}

	// Row 1: product 10, quantity 2
	if fmt.Sprintf("%v", rows[0][1]) != "10" || fmt.Sprintf("%v", rows[0][2]) != "2" {
		t.Errorf("expected (1, 10, 2), got %v", rows[0])
	}
	// Row 2: product 20, quantity 5
	if fmt.Sprintf("%v", rows[1][1]) != "20" || fmt.Sprintf("%v", rows[1][2]) != "5" {
		t.Errorf("expected (1, 20, 5), got %v", rows[1])
	}
}

// Regression: target columns ordered differently from source must not receive swapped values.
func TestSyncData_ColumnOrderMismatch(t *testing.T) {
	srv, ts, srcID, tgtID := setupTwoTestDatabases(t)
	client := ts.Client()
	ctx := context.Background()
	srcSess, _ := srv.sessionMgr.GetSession(srcID)
	tgtSess, _ := srv.sessionMgr.GetSession(tgtID)

	_, _ = srcSess.Session.Exec(ctx, "CREATE TABLE p (id INTEGER PRIMARY KEY, name TEXT, city TEXT)")
	_, _ = tgtSess.Session.Exec(ctx, "CREATE TABLE p (city TEXT, id INTEGER PRIMARY KEY, name TEXT)")
	_, _ = srcSess.Session.Exec(ctx, "INSERT INTO p VALUES (1, 'ann', 'oslo'), (2, 'bob', 'rome')")
	_, _ = tgtSess.Session.Exec(ctx, "INSERT INTO p VALUES ('oslo', 1, 'ann'), ('paris', 2, 'bob')")

	b, _ := json.Marshal(SyncDataCompareRequest{SourceSessionID: srcID, TargetSessionID: tgtID, Table: "p"})
	resp, err := client.Post(ts.URL+"/api/sync/data/compare", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("compare failed: %v", err)
	}
	var comp SyncDataCompareResponse
	_ = json.NewDecoder(resp.Body).Decode(&comp)
	resp.Body.Close()
	if comp.Summary["update_count"] != 1 || comp.Summary["insert_count"] != 0 {
		t.Fatalf("expected exactly 1 update (row 2 city), got %+v", comp.Summary)
	}

	b, _ = json.Marshal(SyncDataApplyRequest{TargetSessionID: tgtID, Table: "p", Columns: comp.Columns, Operations: comp.Operations})
	resp, err = client.Post(ts.URL+"/api/sync/data/apply", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("apply failed: %v", err)
	}
	resp.Body.Close()

	stream, err := tgtSess.Session.QueryStream(ctx, "SELECT id, name, city FROM p ORDER BY id", 10)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	rows, _ := stream.NextChunk(10)
	got := fmt.Sprint(rows)
	if got != "[[1 ann oslo] [2 bob rome]]" {
		t.Fatalf("target rows wrong after sync: %s", got)
	}
}
