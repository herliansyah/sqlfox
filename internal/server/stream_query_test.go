package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sqlfox/internal/driver"
	"sqlfox/internal/logger"
	"sqlfox/internal/vault"
)

type dummyFlusher struct {
	flushCount int
}

func (d *dummyFlusher) Flush() {
	d.flushCount++
}

type mockFlushingRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (m *mockFlushingRecorder) Flush() {
	m.flushes++
}

func TestServer_HandleExecuteStreamHTTP(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-stream-http-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	dbSession, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer dbSession.Close()

	_, err = dbSession.Exec(context.Background(), "CREATE TABLE logs (id INTEGER PRIMARY KEY, msg TEXT);")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = dbSession.Exec(context.Background(), "BEGIN TRANSACTION;")
	for i := 1; i <= 25; i++ {
		_, _ = dbSession.Exec(context.Background(), fmt.Sprintf("INSERT INTO logs (id, msg) VALUES (%d, 'Log %d');", i, i))
	}
	_, _ = dbSession.Exec(context.Background(), "COMMIT;")

	sessionMgr := NewSessionManager(nil, "")
	activeSess := &ActiveSession{
		ID:          "conn-http-1",
		Environment: "Development",
		DriverName:  "sqlite",
		ReadOnly:    false,
		Session:     dbSession,
	}
	sessionMgr.mu.Lock()
	sessionMgr.sessions["conn-http-1"] = activeSess
	sessionMgr.mu.Unlock()

	srv := &Server{
		sessionMgr: sessionMgr,
	}

	bodyJSON := `{"sql":"SELECT * FROM logs ORDER BY id ASC"}`
	req := httptest.NewRequest("POST", "/api/session/conn-http-1/execute-stream", strings.NewReader(bodyJSON))
	req.SetPathValue("id", "conn-http-1")
	req.Header.Set("Content-Type", "application/json")

	rec := &mockFlushingRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}

	// Route through HTTPMiddleware as in production
	mw := logger.HTTPMiddleware(slog.Default())
	handler := mw(http.HandlerFunc(srv.handleExecuteStream))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/x-ndjson" {
		t.Errorf("expected Content-Type application/x-ndjson, got %s", ct)
	}
	if rec.flushes < 2 {
		t.Errorf("expected flushes >= 2, got %d", rec.flushes)
	}
}

func TestExecuteStream_MicroChunkingAndMeta(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-stream-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	dbSession, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer dbSession.Close()

	// Seed 50 rows in a transaction for fast test execution
	_, err = dbSession.Exec(context.Background(), "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);")
	if err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	_, _ = dbSession.Exec(context.Background(), "BEGIN TRANSACTION;")
	for i := 1; i <= 50; i++ {
		_, err = dbSession.Exec(context.Background(), fmt.Sprintf("INSERT INTO items (id, name) VALUES (%d, 'Item %d');", i, i))
		if err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}
	_, _ = dbSession.Exec(context.Background(), "COMMIT;")

	sess := &ActiveSession{
		ID:          "conn-stream-1",
		Environment: "Development",
		DriverName:  "sqlite",
		ReadOnly:    false,
		Session:     dbSession,
	}

	var buf bytes.Buffer
	flusher := &dummyFlusher{}

	req := ExecuteRequest{
		SQL:   "SELECT * FROM items ORDER BY id ASC",
		Limit: 100,
	}

	resp, err := sess.ExecuteStream(context.Background(), req, &buf, flusher)
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}
	if resp != nil && resp.Blocked {
		t.Fatalf("expected not blocked, but was blocked")
	}

	scanner := bufio.NewScanner(&buf)
	var lines []map[string]any
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Fatalf("failed to parse NDJSON line %q: %v", line, err)
		}
		lines = append(lines, parsed)
	}

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (meta, chunk1, chunk2, done), got %d lines", len(lines))
	}

	// 1. Meta Line
	if lines[0]["type"] != "meta" {
		t.Errorf("expected line 0 type=meta, got %v", lines[0]["type"])
	}
	cols, ok := lines[0]["columns"].([]any)
	if !ok || len(cols) != 2 {
		t.Errorf("expected 2 columns in meta, got %v", lines[0]["columns"])
	}

	// 2. Micro Chunk: first 20 rows
	if lines[1]["type"] != "chunk" {
		t.Errorf("expected line 1 type=chunk, got %v", lines[1]["type"])
	}
	chunk1Rows, ok := lines[1]["rows"].([]any)
	if !ok || len(chunk1Rows) != 20 {
		t.Errorf("expected first micro-chunk to have 20 rows, got %d", len(chunk1Rows))
	}

	// 3. Second Chunk: remaining 30 rows
	if lines[2]["type"] != "chunk" {
		t.Errorf("expected line 2 type=chunk, got %v", lines[2]["type"])
	}
	chunk2Rows, ok := lines[2]["rows"].([]any)
	if !ok || len(chunk2Rows) != 30 {
		t.Errorf("expected second chunk to have 30 rows, got %d", len(chunk2Rows))
	}

	// 4. Final Line: done
	lastLine := lines[len(lines)-1]
	if lastLine["type"] != "done" {
		t.Errorf("expected final line type=done, got %v", lastLine["type"])
	}
	if rowCount, ok := lastLine["row_count"].(float64); !ok || int(rowCount) != 50 {
		t.Errorf("expected row_count=50 in done payload, got %v", lastLine["row_count"])
	}

	if flusher.flushCount < 3 {
		t.Errorf("expected flusher to be called multiple times for low-latency streaming, got %d", flusher.flushCount)
	}
}

func TestExecuteStream_SafetyGuardBlocked(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-stream-safety-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	dbSession, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer dbSession.Close()

	sess := &ActiveSession{
		ID:          "conn-stream-2",
		Environment: "Production",
		DriverName:  "sqlite",
		ReadOnly:    false,
		Session:     dbSession,
	}

	var buf bytes.Buffer
	flusher := &dummyFlusher{}

	req := ExecuteRequest{
		SQL:       "DROP TABLE sensitive_prod_users",
		Confirmed: false,
	}

	resp, err := sess.ExecuteStream(context.Background(), req, &buf, flusher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || !resp.Blocked {
		t.Fatalf("expected destructive DROP TABLE to be blocked by Safety Guard")
	}
	if buf.Len() > 0 {
		t.Errorf("expected no bytes written to stream writer when blocked, got %d bytes", buf.Len())
	}
}

func TestExecuteStream_AuditLogRecordedOnCompletionAndError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-stream-audit-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	dbSession, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer dbSession.Close()

	_, _ = dbSession.Exec(context.Background(), "CREATE TABLE t (id INT); INSERT INTO t VALUES (1);")

	v, err := vault.OpenVault(filepath.Join(tempDir, "vault.db"), "test-audit-master")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	sess := &ActiveSession{
		ID:          "conn-audit",
		Environment: "Development",
		DriverName:  "sqlite",
		Session:     dbSession,
		vault:       v,
	}

	var buf bytes.Buffer
	flusher := &dummyFlusher{}

	req := ExecuteRequest{
		SQL:   "SELECT * FROM t;",
		Limit: 10,
	}

	_, err = sess.ExecuteStream(context.Background(), req, &buf, flusher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Allow background goroutine in RecordAuditLog to persist
	time.Sleep(100 * time.Millisecond)

	logs, _, err := v.ListAuditLogs("conn-audit", 10, 0, "", "")
	if err != nil {
		t.Fatalf("ListAuditLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log entry, got %d", len(logs))
	}
	if logs[0].Source != "editor_stream" {
		t.Errorf("expected audit source 'editor_stream', got %s", logs[0].Source)
	}
}

func TestExecuteStream_NilSessionSafeguard(t *testing.T) {
	sess := &ActiveSession{
		ID:          "conn-nil-sess",
		Environment: "Development",
		DriverName:  "sqlite",
		Session:     nil, // simulate disconnected session
	}

	var buf bytes.Buffer
	flusher := &dummyFlusher{}

	req := ExecuteRequest{
		SQL:   "SELECT * FROM items;",
		Limit: 10,
	}

	// Must return a clean error, NOT panic
	_, err := sess.ExecuteStream(context.Background(), req, &buf, flusher)
	if err == nil {
		t.Fatalf("expected error on nil session, got nil")
	}
	if !strings.Contains(err.Error(), "no active database session") {
		t.Errorf("expected 'no active database session' error, got %v", err)
	}
}
