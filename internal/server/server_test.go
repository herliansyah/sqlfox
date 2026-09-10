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
	_ "sqlfox/internal/driver/sqlite"
)

func TestServer_API_Lifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-server-test-*")
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

	// 1. Create a Connection
	createPayload := vault.ConnectionRecord{
		Name:         "Local Test SQLite",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: targetDB,
	}
	body, _ := json.Marshal(createPayload)

	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/connections failed: %v, status: %d", err, resp.StatusCode)
	}

	var savedConn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&savedConn)
	resp.Body.Close()

	if savedConn.ID == "" {
		t.Fatal("expected saved connection ID")
	}

	// 2. List Connections
	resp, err = client.Get(ts.URL + "/api/connections")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/connections failed: %v", err)
	}
	var list []vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()

	if len(list) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(list))
	}

	// 3. Connect Session
	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, savedConn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/session/connect failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Execute CREATE TABLE
	createTableSQL := map[string]any{
		"sql": "CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price REAL);",
	}
	sqlBody, _ := json.Marshal(createTableSQL)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, savedConn.ID), "application/json", bytes.NewReader(sqlBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Execute CREATE TABLE failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Test Safety Guard Inspection on dangerous query
	dangerousSQL := map[string]any{
		"sql": "DELETE FROM products;",
	}
	dangBody, _ := json.Marshal(dangerousSQL)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/inspect", ts.URL, savedConn.ID), "application/json", bytes.NewReader(dangBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Inspect failed: %v", err)
	}
	var inspRes struct {
		RiskLevel         string `json:"risk_level"`
		RequiresChallenge bool   `json:"requires_challenge"`
		TargetTable       string `json:"target_table"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&inspRes)
	resp.Body.Close()

	if inspRes.RiskLevel != "CRITICAL" || !inspRes.RequiresChallenge || inspRes.TargetTable != "products" {
		t.Fatalf("unexpected inspection outcome: %+v", inspRes)
	}

	// 6. Attempt executing dangerous query without confirmation -> Expect 422 Unprocessable
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, savedConn.ID), "application/json", bytes.NewReader(dangBody))
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422 for blocked query, got status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 7. Insert and Query Rows
	insertSQL := map[string]any{
		"sql": "INSERT INTO products (name, price) VALUES ('Widget A', 19.99), ('Widget B', 29.99);",
	}
	insBody, _ := json.Marshal(insertSQL)
	resp, _ = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, savedConn.ID), "application/json", bytes.NewReader(insBody))
	resp.Body.Close()

	// Fetch table rows via keyset/viewer endpoint
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/table/products/rows", ts.URL, savedConn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows failed: %v", err)
	}
	var rowsRes struct {
		Rows [][]any `json:"rows"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rowsRes)
	resp.Body.Close()

	if len(rowsRes.Rows) != 2 {
		t.Fatalf("expected 2 rows in products table, got %d", len(rowsRes.Rows))
	}

	// 8. Toggle ReadOnly
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/toggle-readonly", ts.URL, savedConn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Toggle read-only failed: %v", err)
	}
	resp.Body.Close()
}
