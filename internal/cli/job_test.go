package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJobSpec_LoadAndInterpolation(t *testing.T) {
	tempDir := t.TempDir()
	jobFile := filepath.Join(tempDir, "test-job.json")

	os.Setenv("TEST_DB_NAME", "my_special_db")
	os.Setenv("TEST_BACKUP_DIR", tempDir)
	defer os.Unsetenv("TEST_DB_NAME")
	defer os.Unsetenv("TEST_BACKUP_DIR")

	content := `{
		"name": "Nightly Backup Job",
		"tasks": [
			{
				"name": "Check Health",
				"type": "health-check",
				"target": "sqlite://${TEST_BACKUP_DIR}/${TEST_DB_NAME}.sqlite"
			},
			{
				"name": "Backup SQLite",
				"type": "dump",
				"target": "sqlite://${TEST_BACKUP_DIR}/${TEST_DB_NAME}.sqlite",
				"output": "${TEST_BACKUP_DIR}/backup-{DATE}.sql",
				"rotation": {
					"keep_last": 3,
					"max_age": "7d"
				}
			}
		]
	}`

	if err := os.WriteFile(jobFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write job file: %v", err)
	}

	spec, err := LoadJobSpec(jobFile)
	if err != nil {
		t.Fatalf("LoadJobSpec failed: %v", err)
	}

	if spec.Name != "Nightly Backup Job" {
		t.Errorf("expected job name 'Nightly Backup Job', got '%s'", spec.Name)
	}
	if len(spec.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(spec.Tasks))
	}

	expectedTarget := fmt.Sprintf("sqlite://%s/my_special_db.sqlite", tempDir)
	if spec.Tasks[0].Target != expectedTarget {
		t.Errorf("expected target '%s', got '%s'", expectedTarget, spec.Tasks[0].Target)
	}

	expectedOutput := fmt.Sprintf("%s/backup-{DATE}.sql", tempDir)
	if spec.Tasks[1].Output != expectedOutput {
		t.Errorf("expected output '%s', got '%s'", expectedOutput, spec.Tasks[1].Output)
	}
	if spec.Tasks[1].Rotation == nil || spec.Tasks[1].Rotation.KeepLast != 3 {
		t.Errorf("expected keep_last=3, got %+v", spec.Tasks[1].Rotation)
	}
}

func TestJob_RunAndCheckEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "source.db")
	destDbPath := filepath.Join(tempDir, "dest.db")
	jobFile := filepath.Join(tempDir, "pipeline.json")

	// 1. Initialize SQLite database with seed data
	initSql := `
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
		INSERT INTO users (id, name, email) VALUES (1, 'Alice', 'alice@test.com'), (2, 'Bob', 'bob@test.com');
		CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, amount REAL, FOREIGN KEY(user_id) REFERENCES users(id));
		INSERT INTO orders (id, user_id, amount) VALUES (101, 1, 99.5), (102, 2, 45.0);
	`
	initJob := JobTask{
		Type:   "query",
		Target: "sqlite://" + dbPath,
		SQL:    initSql,
		Tx:     true,
	}
	var initStderr bytes.Buffer
	if code := executeQueryTask(context.Background(), initJob, "", "", ioDiscard{}, &initStderr, nil); code != 0 {
		t.Fatalf("failed to init db: %s", initStderr.String())
	}

	// 2. Formulate pipeline job with health-check, dump, smart-seed, query, transfer
	dumpOut := filepath.Join(tempDir, "dumps", "source-backup-{DATETIME}.sql")
	queryOut := filepath.Join(tempDir, "query_report.csv")

	jobSpec := JobSpec{
		Name: "E2E Pipeline Test",
		Tasks: []JobTask{
			{
				Name:   "Health Ping",
				Type:   "health-check",
				Target: "sqlite://" + dbPath,
			},
			{
				Name:   "Standard Dump",
				Type:   "dump",
				Target: "sqlite://" + dbPath,
				Output: dumpOut,
				Rotation: &JobRotation{
					KeepLast: 2,
				},
			},
			{
				Name:   "Export Report Query",
				Type:   "query",
				Target: "sqlite://" + dbPath,
				SQL:    "SELECT id, name, email FROM users ORDER BY id ASC;",
				Format: "csv",
				Output: queryOut,
			},
			{
				Name:          "Transfer Tables",
				Type:          "transfer",
				From:          "sqlite://" + dbPath,
				To:            "sqlite://" + destDbPath,
				AllTables:     true,
				CreateMissing: true,
			},
		},
	}
	jobBytes, err := json.Marshal(jobSpec)
	if err != nil {
		t.Fatalf("failed to build job JSON: %v", err)
	}

	if err := os.WriteFile(jobFile, jobBytes, 0644); err != nil {
		t.Fatalf("failed to write job JSON: %v", err)
	}

	// 3. Test `sqlfox job check`
	var checkStdout, checkStderr bytes.Buffer
	checkCode := RunJob([]string{"check", "--file", jobFile}, &checkStdout, &checkStderr, nil)
	if checkCode != 0 {
		t.Fatalf("expected check exit code 0, got %d. Stderr: %s", checkCode, checkStderr.String())
	}
	if !strings.Contains(checkStdout.String(), "All pre-flight checks passed successfully") {
		t.Errorf("unexpected check stdout: %s", checkStdout.String())
	}

	// 4. Test `sqlfox job run`
	var runStdout, runStderr bytes.Buffer
	runCode := RunJob([]string{"run", "--file", jobFile}, &runStdout, &runStderr, nil)
	if runCode != 0 {
		t.Fatalf("expected run exit code 0, got %d. Stderr: %s", runCode, runStderr.String())
	}

	// 5. Verify outputs
	// Check CSV report
	reportBytes, err := os.ReadFile(queryOut)
	if err != nil {
		t.Fatalf("query output file missing: %v", err)
	}
	if !strings.Contains(string(reportBytes), "Alice") || !strings.Contains(string(reportBytes), "bob@test.com") {
		t.Errorf("query report content invalid: %s", string(reportBytes))
	}

	// Check dump file
	dumpEntries, err := os.ReadDir(filepath.Join(tempDir, "dumps"))
	if err != nil || len(dumpEntries) == 0 {
		t.Fatalf("dump file was not generated: %v", err)
	}
	dumpContent, _ := os.ReadFile(filepath.Join(tempDir, "dumps", dumpEntries[0].Name()))
	if !strings.Contains(string(dumpContent), "users") || !strings.Contains(string(dumpContent), "orders") {
		t.Errorf("dump content missing tables: %s", string(dumpContent))
	}

	// Check transfer target
	if _, err := os.Stat(destDbPath); os.IsNotExist(err) {
		t.Errorf("destination db file was not created by transfer")
	}
}

func TestApplyDumpRotation(t *testing.T) {
	tempDir := t.TempDir()
	template := filepath.Join(tempDir, "app-backup-{DATETIME}.sql")

	now := time.Now()
	// Create 5 fake dump files with varying timestamps
	for i := 1; i <= 5; i++ {
		fileName := fmt.Sprintf("app-backup-2026100%d-120000.sql", i)
		filePath := filepath.Join(tempDir, fileName)
		if err := os.WriteFile(filePath, []byte("fake dump"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		// Set modTime so file 5 is newest, file 1 is oldest
		mTime := now.Add(time.Duration(i-5) * 24 * time.Hour)
		_ = os.Chtimes(filePath, mTime, mTime)
	}

	// Also create an unrelated file that should NEVER be pruned
	otherFile := filepath.Join(tempDir, "other-data.sql")
	_ = os.WriteFile(otherFile, []byte("preserve me"), 0644)
	oldTime := now.Add(-100 * 24 * time.Hour)
	_ = os.Chtimes(otherFile, oldTime, oldTime)

	var stderr bytes.Buffer
	// Keep last 2 files
	ApplyDumpRotation(tempDir, template, &JobRotation{KeepLast: 2}, &stderr)

	entries, _ := os.ReadDir(tempDir)
	remaining := make(map[string]bool)
	for _, e := range entries {
		remaining[e.Name()] = true
	}

	// Expected retained: app-backup-20261005-120000.sql, app-backup-20261004-120000.sql, and other-data.sql
	if !remaining["app-backup-20261005-120000.sql"] {
		t.Errorf("expected newest file to be retained")
	}
	if !remaining["app-backup-20261004-120000.sql"] {
		t.Errorf("expected 2nd newest file to be retained")
	}
	if remaining["app-backup-20261001-120000.sql"] {
		t.Errorf("expected oldest file to be pruned")
	}
	if !remaining["other-data.sql"] {
		t.Errorf("unrelated file 'other-data.sql' was erroneously deleted")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func TestApplyDumpRotation_OnlyExactTemplateMatches(t *testing.T) {
	tempDir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	files := []string{
		"2026-10-01.sql", "2026-10-02.sql", // this job ({DATE}.sql)
		"app-users-2026-10-01.sql", // another job's dump
		"notes.sql", "schema.sql",  // hand-placed files
	}
	for _, f := range files {
		p := filepath.Join(tempDir, f)
		_ = os.WriteFile(p, []byte("x"), 0644)
		_ = os.Chtimes(p, old, old)
	}
	var stderr bytes.Buffer
	ApplyDumpRotation(tempDir, filepath.Join(tempDir, "{DATE}.sql"), &JobRotation{MaxAge: "1d"}, &stderr)
	for _, keep := range []string{"app-users-2026-10-01.sql", "notes.sql", "schema.sql"} {
		if _, err := os.Stat(filepath.Join(tempDir, keep)); err != nil {
			t.Errorf("%s must not be rotated: %v", keep, err)
		}
	}
	for _, gone := range []string{"2026-10-01.sql", "2026-10-02.sql"} {
		if _, err := os.Stat(filepath.Join(tempDir, gone)); err == nil {
			t.Errorf("%s should have been rotated", gone)
		}
	}

	// Token-less template: nothing in the directory may be deleted.
	ApplyDumpRotation(tempDir, filepath.Join(tempDir, "fixed.sql"), &JobRotation{KeepLast: 1}, &stderr)
	if _, err := os.Stat(filepath.Join(tempDir, "notes.sql")); err != nil {
		t.Errorf("token-less template rotated unrelated files: %v", err)
	}
}

func TestExpandEnv_PreservesLiteralDollars(t *testing.T) {
	t.Setenv("SQLFOX_TEST_HOST", "db.local")
	cases := map[string]string{
		"${SQLFOX_TEST_HOST}":            "db.local",
		"$SQLFOX_TEST_HOST:3306":         "db.local:3306",
		"${SQLFOX_TEST_UNSET_VAR}x":      "x",
		"DO $$ BEGIN PERFORM 1; END $$":  "DO $$ BEGIN PERFORM 1; END $$",
		"WHERE price = '$5' AND id = $1": "WHERE price = '$5' AND id = $1",
		"pa$word_unset_SQLFOX":           "pa$word_unset_SQLFOX",
		"AS $body$ SELECT 1 $body$":      "AS $body$ SELECT 1 $body$",
	}
	for in, want := range cases {
		if got := expandEnv(in); got != want {
			t.Errorf("expandEnv(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJob_SmartSeedHonorsSampleRowsAndFailedDumpKeepsOldFile(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "src.db")
	var sb strings.Builder
	sb.WriteString("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT);")
	for i := 1; i <= 250; i++ {
		sb.WriteString(fmt.Sprintf("INSERT INTO t (id, v) VALUES (%d, 'v');", i))
	}
	var stderr bytes.Buffer
	if code := executeQueryTask(context.Background(), JobTask{Type: "query", Target: "sqlite://" + dbPath, SQL: sb.String(), Tx: true}, "", "", ioDiscard{}, &stderr, nil); code != 0 {
		t.Fatalf("seed failed: %s", stderr.String())
	}

	out := filepath.Join(tempDir, "seed.sql")
	task := JobTask{Type: "dump", DumpType: "smart-seed", SampleRows: 150, Target: "sqlite://" + dbPath, Output: out}
	if code := executeDumpTask(context.Background(), task, "", "", ioDiscard{}, &stderr); code != 0 {
		t.Fatalf("dump failed: %s", stderr.String())
	}
	content, _ := os.ReadFile(out)
	if n := strings.Count(string(content), ", 'v')"); n != 150 {
		t.Errorf("smart-seed wrote %d rows, want 150", n)
	}

	// A failing dump (unreachable target file inside a missing dir is created by sqlite, so use a
	// cancelled context) must leave the previous output untouched.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := executeDumpTask(ctx, JobTask{Type: "dump", Target: "sqlite://" + dbPath, Output: out}, "", "", ioDiscard{}, &stderr); code == 0 {
		t.Fatalf("expected dump with cancelled context to fail")
	}
	after, _ := os.ReadFile(out)
	if string(after) != string(content) {
		t.Errorf("failed dump modified the existing output file")
	}
	entries, _ := os.ReadDir(tempDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
