package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/advisor"
	"sqlfox/internal/driver"
)

func TestCLI_AdviseStaticQuery(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"advise",
		"--driver", "mysql",
		"--sql", "SELECT * FROM orders WHERE user_id = 10 AND status = 'PAID' ORDER BY created_at DESC",
	}

	code := Run(args, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "SqlFox Index Advisor") {
		t.Errorf("expected header, got: %s", out)
	}
	if !strings.Contains(out, "idx_orders_user_id_status_created_at") {
		t.Errorf("expected suggested index name, got: %s", out)
	}
	if !strings.Contains(out, "ALGORITHM=INPLACE, LOCK=NONE") {
		t.Errorf("expected MySQL non-blocking DDL, got: %s", out)
	}
}

func TestCLI_AdviseJSONFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"advise",
		"--driver", "postgres",
		"--format", "json",
		"--sql", "SELECT * FROM users WHERE email = 'test@example.com'",
	}

	code := Run(args, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}

	var res advisor.AdviceResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON output: %v, raw: %s", err, stdout.String())
	}

	if len(res.Recommendations) == 0 {
		t.Fatalf("expected at least 1 recommendation")
	}
	if !strings.Contains(res.Recommendations[0].SafeDDL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS") {
		t.Errorf("expected PG safe DDL, got: %s", res.Recommendations[0].SafeDDL)
	}
}

func TestCLI_AdviseWithSQLiteDSN(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "advise_test.db")

	ctx := context.Background()
	sess, err := driver.Connect(ctx, driver.Config{Driver: "sqlite", DatabaseName: dbPath})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, _ = sess.Exec(ctx, "CREATE TABLE customers (id INTEGER PRIMARY KEY, region TEXT, score REAL);")
	sess.Close()

	var stdout, stderr bytes.Buffer
	args := []string{
		"advise",
		"--dsn", "sqlite://" + dbPath,
		"--sql", "SELECT * FROM customers WHERE region = 'APAC' AND score > 80.0",
	}

	code := Run(args, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "CREATE INDEX IF NOT EXISTS") {
		t.Errorf("expected SQLite index DDL, got: %s", out)
	}
}
