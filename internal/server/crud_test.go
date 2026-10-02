package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sqlfox/internal/vault"
)

func TestServer_InsertRowAndBatchDelete(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-crud-test-*")
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
		Name:         "CRUD SQLite Test",
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

	// 3. Create table
	createSQL := map[string]any{
		"sql": "CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, stock INTEGER);",
	}
	sqlBody, _ := json.Marshal(createSQL)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, conn.ID), "application/json", bytes.NewReader(sqlBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create table failed: %v", err)
	}
	resp.Body.Close()

	// 4. Test insert-row
	insertPayload1 := map[string]any{
		"table": "items",
		"data": map[string]any{
			"name":  "Widget A",
			"stock": 10,
		},
	}
	body, _ = json.Marshal(insertPayload1)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/insert-row", ts.URL, conn.ID), "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("insert-row 1 failed: status=%d, err=%v", resp.StatusCode, err)
	}
	var res1 struct {
		RowsAffected int64 `json:"rows_affected"`
		LastInsertID int64 `json:"last_insert_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res1)
	resp.Body.Close()
	if res1.RowsAffected != 1 || res1.LastInsertID != 1 {
		t.Fatalf("unexpected insert-row result: %+v", res1)
	}

	// Insert row 2 and row 3
	for i, name := range []string{"Widget B", "Widget C"} {
		insertPayload := map[string]any{
			"table": "items",
			"data": map[string]any{
				"name":  name,
				"stock": (i + 2) * 10,
			},
		}
		body, _ = json.Marshal(insertPayload)
		resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/insert-row", ts.URL, conn.ID), "application/json", bytes.NewReader(body))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("insert-row %s failed: status=%d", name, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// 5. Verify 3 rows exist
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/table/items/rows?limit=10", ts.URL, conn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get table rows failed: %v", err)
	}
	var rowsResult struct {
		Rows [][]any `json:"rows"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rowsResult)
	resp.Body.Close()
	if len(rowsResult.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rowsResult.Rows))
	}

	// 6. Test delete-rows (delete id 1 and 3)
	delPayload := map[string]any{
		"table":     "items",
		"pk_column": "id",
		"pk_values": []any{1, 3},
	}
	body, _ = json.Marshal(delPayload)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/delete-rows", ts.URL, conn.ID), "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("delete-rows failed: status=%d, err=%v", resp.StatusCode, err)
	}
	var delRes struct {
		RowsAffected int64 `json:"rows_affected"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&delRes)
	resp.Body.Close()
	if delRes.RowsAffected != 2 {
		t.Fatalf("expected 2 rows affected on delete, got %d", delRes.RowsAffected)
	}

	// 7. Verify remaining row is Widget B (id 2)
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/table/items/rows?limit=10", ts.URL, conn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get table rows after delete failed: %v", err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&rowsResult)
	resp.Body.Close()
	if len(rowsResult.Rows) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(rowsResult.Rows))
	}
	if fmt.Sprintf("%v", rowsResult.Rows[0][1]) != "Widget B" {
		t.Fatalf("expected remaining row to be Widget B, got %v", rowsResult.Rows[0][1])
	}
}
