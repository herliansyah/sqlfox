package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/safety"
)

func assertNestedBackupDuringScript(t *testing.T, sess driver.Session, sqliteSess *SQLiteSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := sess.Exec(ctx, "CREATE TABLE victims (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatal(err)
	}

	backupDir := t.TempDir()
	err := sqliteSess.ExecScriptConn(ctx, func(exec func(context.Context, string) (*driver.ExecResult, error)) error {
		for _, stmt := range []string{
			"PRAGMA foreign_keys = OFF",
			"BEGIN",
			"INSERT INTO victims (id, v) VALUES (2, 'b')",
		} {
			if _, err := exec(ctx, stmt); err != nil {
				return err
			}
		}
		path, dataIncluded, err := safety.BackupTable(ctx, sess, backupDir, "victims", "sqlite")
		if err != nil {
			return err
		}
		if !dataIncluded {
			return errors.New("expected row data in backup")
		}
		snapshot, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(snapshot), "(2, 'b')") {
			return errors.New("backup missing uncommitted row from script")
		}
		_, err = exec(ctx, "COMMIT")
		return err
	})
	if err != nil {
		t.Fatalf("script run with nested backup failed (deadlock regression?): %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one backup snapshot, entries=%v err=%v", entries, err)
	}
	rows, err := sess.QueryRaw(ctx, "PRAGMA foreign_keys")
	if err != nil || len(rows) != 1 || rows[0][0] != int64(1) {
		t.Fatalf("foreign_keys not restored after script: rows=%v err=%v", rows, err)
	}
	rows, err = sess.QueryRaw(ctx, "SELECT COUNT(*) FROM victims")
	if err != nil || len(rows) != 1 || rows[0][0] != int64(1) {
		t.Fatalf("expected committed insert, rows=%v err=%v", rows, err)
	}
}

func TestExecScriptConnMemoryAllowsNestedBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sess, err := (&SQLiteDriver{}).Connect(ctx, driver.Config{DatabaseName: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	sqliteSess := sess.(*SQLiteSession)
	if !sqliteSess.singleConn {
		t.Fatal("expected :memory: session to use the single-connection pool path")
	}
	assertNestedBackupDuringScript(t, sess, sqliteSess)
}

func TestExecScriptConnSingleConnFileAllowsNestedBackup(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "fallback.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	sess := &SQLiteSession{
		db:         db,
		dsn:        dsn,
		singleConn: true,
		queries:    make(map[string]context.CancelFunc),
	}
	assertNestedBackupDuringScript(t, sess, sess)
}
