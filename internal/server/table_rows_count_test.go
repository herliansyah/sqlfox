package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herliansyah/sqlfox/internal/vault"
)

func TestHandleGetTableRows_Limit1000AndExactCount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-table-count-test-*")
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
		Name:         "Test SQLite Count",
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

	// Seed table with exactly 101 rows (reproducing user scenario)
	_, err = sess.Session.Exec(ctx, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	var sb strings.Builder
	sb.WriteString("INSERT INTO items VALUES ")
	for i := 1; i <= 101; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("(%d, 'item_%d')", i, i))
	}
	if _, err := sess.Session.Exec(ctx, sb.String()); err != nil {
		t.Fatalf("failed to insert rows: %v", err)
	}

	// 1. Request with limit=1000: must return all 101 rows and total_rows=101, is_exact=true
	resp1000, err := client.Get(fmt.Sprintf("%s/api/session/%s/table/items/rows?limit=1000", ts.URL, connRec.ID))
	if err != nil || resp1000.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows with limit 1000 failed: %v", err)
	}
	var res1000 struct {
		Rows      [][]any `json:"rows"`
		Limit     int     `json:"limit"`
		TotalRows int64   `json:"total_rows"`
		IsExact   bool    `json:"is_exact"`
	}
	if err := json.NewDecoder(resp1000.Body).Decode(&res1000); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp1000.Body.Close()

	if res1000.Limit != 1000 {
		t.Fatalf("expected limit 1000, got %d", res1000.Limit)
	}
	if len(res1000.Rows) != 101 {
		t.Fatalf("expected 101 rows returned when paging is 1000, got %d", len(res1000.Rows))
	}
	if res1000.TotalRows != 101 {
		t.Fatalf("expected total_rows=101, got %d", res1000.TotalRows)
	}
	if !res1000.IsExact {
		t.Fatalf("expected is_exact=true, got false")
	}

	// 2. Request with limit=25: must return 25 rows and total_rows=101, is_exact=true
	resp25, err := client.Get(fmt.Sprintf("%s/api/session/%s/table/items/rows?limit=25", ts.URL, connRec.ID))
	if err != nil || resp25.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows with limit 25 failed: %v", err)
	}
	var res25 struct {
		Rows      [][]any `json:"rows"`
		Limit     int     `json:"limit"`
		TotalRows int64   `json:"total_rows"`
		IsExact   bool    `json:"is_exact"`
	}
	if err := json.NewDecoder(resp25.Body).Decode(&res25); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp25.Body.Close()

	if len(res25.Rows) != 25 {
		t.Fatalf("expected 25 rows, got %d", len(res25.Rows))
	}
	if res25.TotalRows != 101 {
		t.Fatalf("expected total_rows=101, got %d", res25.TotalRows)
	}
	if !res25.IsExact {
		t.Fatalf("expected is_exact=true for limit=25 query, got false")
	}

	// 3. Request count endpoint: /api/session/{id}/table/items/count
	respCount, err := client.Get(fmt.Sprintf("%s/api/session/%s/table/items/count", ts.URL, connRec.ID))
	if err != nil || respCount.StatusCode != http.StatusOK {
		t.Fatalf("GET table count failed: %v", err)
	}
	var countRes struct {
		Table string `json:"table"`
		Count int64  `json:"count"`
	}
	if err := json.NewDecoder(respCount.Body).Decode(&countRes); err != nil {
		t.Fatalf("failed to decode count response: %v", err)
	}
	respCount.Body.Close()

	if countRes.Count != 101 {
		t.Fatalf("expected count=101, got %d", countRes.Count)
	}
}
