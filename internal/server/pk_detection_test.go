package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sqlfox/internal/vault"
)

func TestGetTableRows_PrimaryKeyDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-pk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "config.db")
	v, err := vault.OpenVault(dbPath, "testpass123")
	if err != nil {
		t.Fatalf("failed to open vault: %v", err)
	}
	defer v.Close()

	testSQLitePath := filepath.Join(tmpDir, "test.sqlite")
	connRec := &vault.ConnectionRecord{
		Name:         "Test SQLite",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: testSQLitePath,
	}
	if err := v.SaveConnection(connRec); err != nil {
		t.Fatalf("failed to save connection: %v", err)
	}

	s := NewServer(v, nil, filepath.Join(tmpDir, "backups"), "testpass123")
	ts := httptest.NewServer(s.Router())
	defer ts.Close()

	client := ts.Client()

	// 1. Connect session before tables exist (CachedSchema is empty)
	connResp, err := client.Post(ts.URL+"/api/session/connect/"+connRec.ID, "application/json", nil)
	if err != nil || connResp.StatusCode != http.StatusOK {
		t.Fatalf("connect failed: %v", err)
	}
	connResp.Body.Close()

	sess, err := s.sessionMgr.GetSession(connRec.ID)
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 2. Create table with primary key AFTER connection (stale CachedSchema scenario)
	_, err = sess.Session.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	_, err = sess.Session.Exec(ctx, "INSERT INTO users VALUES (1, 'alice'), (2, 'bob')")
	if err != nil {
		t.Fatalf("failed to insert data: %v", err)
	}

	// 3. Request rows for 'users'
	resp, err := client.Get(fmt.Sprintf("%s/api/session/%s/table/users/rows", ts.URL, connRec.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows failed: %v", err)
	}
	var res struct {
		PKCol string `json:"pk_col"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()

	if res.PKCol != "id" {
		t.Fatalf("expected pk_col to be 'id', got %q (primary key was not detected for table with primary key!)", res.PKCol)
	}

	// 4. Request rows with different casing (e.g. 'Users' vs 'users')
	respCase, err := client.Get(fmt.Sprintf("%s/api/session/%s/table/Users/rows", ts.URL, connRec.ID))
	if err != nil || respCase.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows for 'Users' failed: %v", err)
	}
	var resCase struct {
		PKCol string `json:"pk_col"`
	}
	_ = json.NewDecoder(respCase.Body).Decode(&resCase)
	respCase.Body.Close()

	if resCase.PKCol != "id" {
		t.Fatalf("expected pk_col to be 'id' for 'Users', got %q", resCase.PKCol)
	}

	// 5. Test another table created dynamically without prior schema fetch
	_, err = sess.Session.Exec(ctx, "CREATE TABLE products (product_id INTEGER PRIMARY KEY, title TEXT)")
	if err != nil {
		t.Fatalf("failed to create products table: %v", err)
	}
	respProd, err := client.Get(fmt.Sprintf("%s/api/session/%s/table/products/rows", ts.URL, connRec.ID))
	if err != nil || respProd.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows for 'products' failed: %v", err)
	}
	var resProd struct {
		PKCol string `json:"pk_col"`
	}
	_ = json.NewDecoder(respProd.Body).Decode(&resProd)
	respProd.Body.Close()

	if resProd.PKCol != "product_id" {
		t.Fatalf("expected pk_col to be 'product_id', got %q", resProd.PKCol)
	}

	// 6. Test table with UNIQUE KEY but NO primary key (uses unique key as pk_col)
	_, err = sess.Session.Exec(ctx, "CREATE TABLE unique_items (uuid TEXT UNIQUE, label TEXT)")
	if err != nil {
		t.Fatalf("failed to create unique_items table: %v", err)
	}
	respUnique, err := client.Get(fmt.Sprintf("%s/api/session/%s/table/unique_items/rows", ts.URL, connRec.ID))
	if err != nil || respUnique.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows for 'unique_items' failed: %v", err)
	}
	var resUnique struct {
		PKCol string `json:"pk_col"`
	}
	_ = json.NewDecoder(respUnique.Body).Decode(&resUnique)
	respUnique.Body.Close()

	if resUnique.PKCol != "uuid" {
		t.Fatalf("expected unique key 'uuid' to be detected as pk_col, got %q", resUnique.PKCol)
	}
}

func TestInlineEditWithoutPrimaryKey_RowCriteria(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-nopk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	v, err := vault.OpenVault(filepath.Join(tmpDir, "config.db"), "testpass123")
	if err != nil {
		t.Fatalf("failed to open vault: %v", err)
	}
	defer v.Close()

	connRec := &vault.ConnectionRecord{
		Name:         "Test SQLite NoPK",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: filepath.Join(tmpDir, "test.sqlite"),
	}
	if err := v.SaveConnection(connRec); err != nil {
		t.Fatalf("failed to save connection: %v", err)
	}

	s := NewServer(v, nil, filepath.Join(tmpDir, "backups"), "testpass123")
	ts := httptest.NewServer(s.Router())
	defer ts.Close()
	client := ts.Client()

	connResp, _ := client.Post(ts.URL+"/api/session/connect/"+connRec.ID, "application/json", nil)
	connResp.Body.Close()

	sess, _ := s.sessionMgr.GetSession(connRec.ID)
	ctx := context.Background()

	// Table with NO primary key and NO unique key
	_, err = sess.Session.Exec(ctx, "CREATE TABLE audit_records (event TEXT, severity TEXT, count INT)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	_, err = sess.Session.Exec(ctx, "INSERT INTO audit_records VALUES ('login_failed', 'WARN', 3), ('disk_full', 'CRIT', 1)")
	if err != nil {
		t.Fatalf("failed to insert data: %v", err)
	}

	// 1. Test cell-edit using row_criteria (when table has no PK)
	editPayload := map[string]any{
		"table":  "audit_records",
		"column": "count",
		"value":  5,
		"row_criteria": map[string]any{
			"event":    "login_failed",
			"severity": "WARN",
			"count":    3,
		},
	}
	body, _ := json.Marshal(editPayload)
	editResp, err := client.Post(fmt.Sprintf("%s/api/session/%s/cell-edit", ts.URL, connRec.ID), "application/json", bytes.NewReader(body))
	if err != nil || editResp.StatusCode != http.StatusOK {
		var errData map[string]any
		_ = json.NewDecoder(editResp.Body).Decode(&errData)
		t.Fatalf("cell-edit with row_criteria failed: status=%d, err=%v, body=%v", editResp.StatusCode, err, errData)
	}
	editResp.Body.Close()

	// Verify update succeeded
	qRows, err := sess.Session.QueryRaw(ctx, "SELECT count FROM audit_records WHERE event = 'login_failed'")
	if err != nil || len(qRows) == 0 {
		t.Fatalf("failed to query updated record: %v", err)
	}
	if fmt.Sprint(qRows[0][0]) != "5" {
		t.Fatalf("expected updated count to be 5, got %v", qRows[0][0])
	}

	// 2. Test delete-rows using row_criterias (when table has no PK)
	delPayload := map[string]any{
		"table": "audit_records",
		"row_criterias": []map[string]any{
			{"event": "disk_full", "severity": "CRIT", "count": 1},
		},
	}
	delBody, _ := json.Marshal(delPayload)
	delResp, err := client.Post(fmt.Sprintf("%s/api/session/%s/delete-rows", ts.URL, connRec.ID), "application/json", bytes.NewReader(delBody))
	if err != nil || delResp.StatusCode != http.StatusOK {
		t.Fatalf("delete-rows with row_criterias failed: status=%d, err=%v", delResp.StatusCode, err)
	}
	delResp.Body.Close()

	remaining, _ := sess.Session.QueryRaw(ctx, "SELECT count(*) FROM audit_records")
	if len(remaining) == 0 || fmt.Sprint(remaining[0][0]) != "1" {
		t.Fatalf("expected 1 record remaining after delete, got %v", remaining)
	}
}
