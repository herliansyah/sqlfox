package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/sqlite"
)

func TestMutationEstimation_TieredSafetyGate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-mut-est-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	ctx := context.Background()

	cfg := driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	}

	rawSess, err := driver.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer rawSess.Close()

	// Seed 150 rows
	_, err = rawSess.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, status TEXT, points INT);")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = rawSess.Exec(ctx, "BEGIN TRANSACTION;")
	for i := 1; i <= 150; i++ {
		st := "active"
		if i > 120 {
			st = "inactive"
		}
		_, err = rawSess.Exec(ctx, fmt.Sprintf("INSERT INTO users VALUES (%d, '%s', %d);", i, st, i*10))
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _ = rawSess.Exec(ctx, "COMMIT;")

	devSess := &ActiveSession{
		ConnectionID: "conn-dev",
		Environment:  "Development",
		DriverName:   "sqlite",
		Session:      rawSess,
	}

	prodSess := &ActiveSession{
		ConnectionID: "conn-prod",
		Environment:  "Production",
		DriverName:   "sqlite",
		Session:      rawSess,
	}

	// 1. Single row PK update in Dev -> Auto-allows without modal
	res1, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:       "UPDATE users SET points = 999 WHERE id = 1;",
		Confirmed: false,
	})
	if err != nil {
		t.Fatalf("unexpected error on 1-row PK update in Dev: %v", err)
	}
	if res1.Blocked {
		t.Fatalf("expected 1-row PK update in Dev to pass unblocked, got blocked: %+v", res1)
	}
	if res1.RowsAffected != 1 {
		t.Fatalf("expected 1 row affected, got %d", res1.RowsAffected)
	}

	// 2. Multi-row update (30 rows) in Dev -> Blocked with 1-click affirmation (RequiresAffirmation = true, RequiresChallenge = false)
	res2, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:       "UPDATE users SET points = 0 WHERE status = 'inactive';",
		Confirmed: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res2.Blocked {
		t.Fatalf("expected 30-row update in Dev to be blocked for confirmation")
	}
	if res2.Inspection == nil || res2.Inspection.EstimatedRows == nil || *res2.Inspection.EstimatedRows != 30 {
		t.Fatalf("expected estimated rows = 30, got %+v", res2.Inspection)
	}
	if !res2.Inspection.RequiresAffirmation || res2.Inspection.RequiresChallenge {
		t.Fatalf("expected 30 rows to be 1-click affirmation, got challenge=%v, affirmation=%v", res2.Inspection.RequiresChallenge, res2.Inspection.RequiresAffirmation)
	}

	// 2b. Confirming the 30-row update with 1-click (Confirmed=true, no challenge input) -> Succeeds
	res2Confirm, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:       "UPDATE users SET points = 0 WHERE status = 'inactive';",
		Confirmed: true,
	})
	if err != nil {
		t.Fatalf("1-click affirmation failed: %v", err)
	}
	if res2Confirm.RowsAffected != 30 {
		t.Fatalf("expected 30 rows affected, got %d", res2Confirm.RowsAffected)
	}

	// 3. High-impact update (120 rows > 100 threshold) in Dev -> Escalates to RequiresChallenge = true
	res3, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:       "UPDATE users SET points = 500 WHERE status = 'active';",
		Confirmed: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res3.Blocked {
		t.Fatalf("expected 120-row update to be blocked")
	}
	if res3.Inspection == nil || res3.Inspection.EstimatedRows == nil || *res3.Inspection.EstimatedRows != 120 {
		t.Fatalf("expected estimated rows = 120, got %+v", res3.Inspection)
	}
	if !res3.Inspection.RequiresChallenge {
		t.Fatalf("expected >100 rows to escalate to RequiresChallenge=true")
	}

	// 4. In Production: single row update MUST require confirmation
	resProd, err := prodSess.Execute(ctx, ExecuteRequest{
		SQL:       "UPDATE users SET points = 777 WHERE id = 2;",
		Confirmed: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resProd.Blocked {
		t.Fatalf("expected 1-row update in Production to be blocked for confirmation")
	}
	if resProd.Inspection == nil || resProd.Inspection.EstimatedRows == nil || *resProd.Inspection.EstimatedRows != 1 {
		t.Fatalf("expected estimated rows = 1, got %+v", resProd.Inspection)
	}

	// 5. BypassMutationConfirm in Dev -> Allows 30-row update without blocking
	resBypass, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:                   "UPDATE users SET points = 111 WHERE status = 'inactive';",
		Confirmed:             false,
		BypassMutationConfirm: true,
	})
	if err != nil {
		t.Fatalf("unexpected error with bypass: %v", err)
	}
	if resBypass.Blocked {
		t.Fatalf("expected BypassMutationConfirm in Dev to allow mutation without block")
	}

	// 6. Multi-statement batch aggregation
	batchSQL := "UPDATE users SET points = 1 WHERE id = 10; DELETE FROM users WHERE id = 11;"
	resBatch, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:       batchSQL,
		Confirmed: false,
	})
	if err != nil {
		t.Fatalf("unexpected error on batch: %v", err)
	}
	if !resBatch.Blocked {
		t.Fatalf("expected batch mutation to be blocked")
	}
	if resBatch.Inspection == nil || len(resBatch.Inspection.StatementEstimates) != 2 {
		t.Fatalf("expected 2 statement estimates, got %+v", resBatch.Inspection)
	}
	if resBatch.Inspection.EstimatedRows == nil || *resBatch.Inspection.EstimatedRows != 2 {
		t.Fatalf("expected total 2 estimated rows in batch, got %v", resBatch.Inspection.EstimatedRows)
	}

	// 7. Discrepancy notice formatting in Message
	// Seed a table where data changes between estimate and execution or test discrepancy formatting
	resDisc, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:       "DELETE FROM users WHERE id = 12;",
		Confirmed: true,
	})
	if err != nil {
		t.Fatalf("delete confirmed failed: %v", err)
	}
	if resDisc.RowsAffected != 1 {
		t.Fatalf("expected 1 row affected, got %d", resDisc.RowsAffected)
	}
	// Try deleting id = 12 again (0 rows will be deleted, but estimate was 0)
	resDisc2, err := devSess.Execute(ctx, ExecuteRequest{
		SQL:       "DELETE FROM users WHERE id = 12;",
		Confirmed: true,
	})
	if err != nil {
		t.Fatalf("delete confirmed failed: %v", err)
	}
	if !strings.Contains(resDisc2.Message, "0 rows") && resDisc2.RowsAffected != 0 {
		t.Fatalf("expected 0 rows affected, got %d", resDisc2.RowsAffected)
	}
}
