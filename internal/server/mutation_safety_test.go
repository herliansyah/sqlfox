package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// Regression checks for mutation paths that could touch the wrong rows or lose data.
func TestMutationSafetyRegressions(t *testing.T) {
	srv, ts, srcID, tgtID := setupTwoTestDatabases(t)
	client := ts.Client()
	ctx := context.Background()
	src, _ := srv.sessionMgr.GetSession(srcID)
	tgt, _ := srv.sessionMgr.GetSession(tgtID)

	post := func(path string, payload any) *http.Response {
		b, _ := json.Marshal(payload)
		resp, err := client.Post(ts.URL+path, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	scalar := func(sql string) string {
		rows, err := tgt.Session.QueryRaw(ctx, sql)
		if err != nil || len(rows) == 0 {
			t.Fatalf("%s: %v", sql, err)
		}
		return fmt.Sprint(rows[0][0])
	}

	t.Run("PK-less edit/delete touches exactly one of identical rows", func(t *testing.T) {
		if _, err := tgt.Session.Exec(ctx, "CREATE TABLE dup (a TEXT, b INT); INSERT INTO dup VALUES ('x',1),('x',1),('x',1)"); err != nil {
			t.Fatal(err)
		}
		crit := map[string]any{"a": "x", "b": 1}
		if r := post("/api/session/"+tgtID+"/cell-edit", map[string]any{"table": "dup", "column": "b", "value": 2, "row_criteria": crit}); r.StatusCode != 200 {
			t.Fatalf("cell-edit status %d", r.StatusCode)
		}
		if got := scalar("SELECT COUNT(*) FROM dup WHERE b = 2"); got != "1" {
			t.Fatalf("cell-edit updated %s rows, want 1", got)
		}
		if r := post("/api/session/"+tgtID+"/delete-rows", map[string]any{"table": "dup", "row_criterias": []map[string]any{crit}}); r.StatusCode != 200 {
			t.Fatalf("delete status %d", r.StatusCode)
		}
		if got := scalar("SELECT COUNT(*) FROM dup"); got != "2" {
			t.Fatalf("delete left %s rows, want 2", got)
		}
	})

	t.Run("multi-statement cannot smuggle an uninspected DROP", func(t *testing.T) {
		_, _ = tgt.Session.Exec(ctx, "CREATE TABLE victim (id INT); INSERT INTO victim VALUES (1)")
		if _, err := tgt.Execute(ctx, ExecuteRequest{SQL: "DELETE FROM dup; DROP TABLE victim", Confirmed: true, ChallengeInput: "dup"}); err == nil {
			t.Fatal("expected multi-destructive input to be rejected")
		}
		resp, err := tgt.Execute(ctx, ExecuteRequest{SQL: "SELECT 1; DROP TABLE victim"})
		if err != nil || !resp.Blocked || resp.Inspection.TargetTable != "victim" {
			t.Fatalf("expected trailing DROP to require challenge, got %+v, %v", resp, err)
		}
		if got := scalar("SELECT COUNT(*) FROM victim"); got != "1" {
			t.Fatalf("victim table altered: %s", got)
		}
	})

	t.Run("truncate+transfer failure keeps original target rows", func(t *testing.T) {
		_, _ = src.Session.Exec(ctx, "CREATE TABLE items (id INTEGER, val TEXT); INSERT INTO items VALUES (1,'a'),(1,'b')")
		_, _ = tgt.Session.Exec(ctx, "CREATE TABLE items (id INTEGER UNIQUE, val TEXT); INSERT INTO items VALUES (99,'keep')")
		post("/api/sync/data/transfer", SyncDataTransferRequest{SourceSessionID: srcID, TargetSessionID: tgtID, Tables: []string{"items"}, TruncateTarget: true, BatchSize: 1})
		if got := scalar("SELECT COUNT(*) FROM items WHERE id = 99"); got != "1" {
			t.Fatalf("original target row lost after failed transfer")
		}
	})

	t.Run("row-level sync refuses tables beyond the comparison cap", func(t *testing.T) {
		if _, err := src.Session.Exec(ctx, "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 10001) INSERT INTO users (id, name) SELECT i, 'u' FROM n"); err != nil {
			t.Fatal(err)
		}
		r := post("/api/sync/data/compare", SyncDataCompareRequest{SourceSessionID: srcID, TargetSessionID: tgtID, Table: "users", DeleteMissing: true})
		if r.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for oversized table, got %d", r.StatusCode)
		}
	})
}
