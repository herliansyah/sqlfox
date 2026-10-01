package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sqlfox/internal/vault"
)

func TestValidateTableFilter(t *testing.T) {
	validCases := []string{
		"",
		"id > 10",
		"status = 'active' AND total >= 100",
		"name LIKE '%test%'",
		"id IN (1, 2, 3)",
		"is_deleted = 0",
		"updated_at > '2026-01-01'",
		"drop_count = 5", // column name containing keyword substring
		"alter_status IS NOT NULL",
	}

	for _, c := range validCases {
		if err := validateTableFilter(c); err != nil {
			t.Errorf("expected filter %q to be valid, got: %v", c, err)
		}
	}

	invalidCases := []string{
		"id = 1; DROP TABLE users",
		"1 = 1; SELECT 1",
		"id = 1 UNION SELECT 1, 2, 3",
		"id = 1 union select username, password from admin",
		"id = 1 INTO OUTFILE '/tmp/evil.txt'",
		"DROP TABLE users",
		"DELETE FROM users",
		"UPDATE users SET role = 'admin'",
		"INSERT INTO users VALUES (1)",
		"ALTER TABLE users ADD evil INT",
		"TRUNCATE TABLE users",
		"GRANT ALL PRIVILEGES ON *.* TO 'root'",
	}

	for _, c := range invalidCases {
		if err := validateTableFilter(c); err == nil {
			t.Errorf("expected filter %q to be rejected, but it was accepted", c)
		}
	}
}

func TestHandleGetTableRowsWithFilter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-filter-test-*")
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

	// Connect session
	connResp, err := client.Post(ts.URL+"/api/session/connect/"+connRec.ID, "application/json", nil)
	if err != nil || connResp.StatusCode != http.StatusOK {
		t.Fatalf("connect failed with code %d: %v", connResp.StatusCode, err)
	}
	connResp.Body.Close()

	// Create test table and insert sample rows
	sess, err := s.sessionMgr.GetSession(connRec.ID)
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = sess.Session.Exec(ctx, "CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price REAL, in_stock INTEGER)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	_, err = sess.Session.Exec(ctx, "INSERT INTO products VALUES (1, 'Apple', 1.5, 1), (2, 'Banana', 0.8, 1), (3, 'Cherry', 3.0, 0), (4, 'Durian', 15.0, 0)")
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// 1. Test unfiltered rows
	resp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/table/products/rows?limit=10")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("unfiltered rows request failed: %v", err)
	}
	var data struct {
		Rows [][]any `json:"rows"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	resp.Body.Close()
	if len(data.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(data.Rows))
	}

	// 2. Test valid filter: in_stock = 1
	fResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/table/products/rows?limit=10&filter=in_stock%20%3D%201")
	if err != nil || fResp.StatusCode != http.StatusOK {
		t.Fatalf("filtered rows request failed: %v", err)
	}
	var fData struct {
		Rows   [][]any `json:"rows"`
		Filter string  `json:"filter"`
	}
	json.NewDecoder(fResp.Body).Decode(&fData)
	fResp.Body.Close()
	if len(fData.Rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d", len(fData.Rows))
	}
	if fData.Filter != "in_stock = 1" {
		t.Fatalf("expected filter echo 'in_stock = 1', got %q", fData.Filter)
	}

	// 3. Test price filter with ordering: price > 1.0 AND in_stock = 0
	fResp2, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/table/products/rows?limit=10&filter=price%20%3E%201.0%20AND%20in_stock%20%3D%200&sort_col=price&sort_dir=DESC")
	if err != nil || fResp2.StatusCode != http.StatusOK {
		t.Fatalf("price filtered rows request failed: %v", err)
	}
	var fData2 struct {
		Rows [][]any `json:"rows"`
	}
	json.NewDecoder(fResp2.Body).Decode(&fData2)
	fResp2.Body.Close()
	if len(fData2.Rows) != 2 {
		t.Fatalf("expected 2 rows for price > 1.0 and in_stock = 0, got %d", len(fData2.Rows))
	}
	// First row should be Durian (price 15.0)
	if fData2.Rows[0][1] != "Durian" {
		t.Fatalf("expected Durian first due to DESC sort, got %v", fData2.Rows[0][1])
	}

	// 4. Test malicious filter rejection: injection attempt with semicolon
	badResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/table/products/rows?limit=10&filter=id%20%3D%201%3B%20DROP%20TABLE%20products")
	if err != nil {
		t.Fatalf("bad filter request error: %v", err)
	}
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request for malicious filter, got %d", badResp.StatusCode)
	}
}
