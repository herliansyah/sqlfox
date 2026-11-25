package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
)

func TestBoostP08_InspectSchemaMemoryNoDeadlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	drv := &SQLiteDriver{}
	sess, err := drv.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: ":memory:",
	})
	if err != nil {
		t.Fatalf("connect :memory: failed: %v", err)
	}
	defer sess.Close()

	ddls := []string{
		`CREATE TABLE parents (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE children (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents(id), code TEXT)`,
		`CREATE INDEX idx_children_parent ON children(parent_id)`,
		`CREATE UNIQUE INDEX idx_children_code ON children(code)`,
		`CREATE VIEW v_children AS SELECT id, code FROM children`,
	}
	for _, ddl := range ddls {
		if _, err := sess.Exec(ctx, ddl); err != nil {
			t.Fatalf("exec %q failed: %v", ddl, err)
		}
	}

	schema, err := sess.InspectSchema(ctx, "main")
	if err != nil {
		t.Fatalf("InspectSchema on :memory: failed (deadlock regression?): %v", err)
	}
	if len(schema.Tables) != 3 {
		t.Fatalf("expected 3 tables/views, got %d", len(schema.Tables))
	}
	foundIdx := false
	for _, tbl := range schema.Tables {
		if tbl.Name == "children" {
			if len(tbl.ForeignKeys) == 0 {
				t.Errorf("expected FK metadata on children")
			}
			for _, idx := range tbl.Indexes {
				if idx.Name == "idx_children_parent" && len(idx.Columns) == 1 && idx.Columns[0] == "parent_id" {
					foundIdx = true
				}
			}
		}
	}
	if !foundIdx {
		t.Errorf("expected idx_children_parent with column parent_id in inspected schema")
	}
}

func TestBoostP02_ExplainRejectsStackedDrop(t *testing.T) {
	ctx := context.Background()
	drv := &SQLiteDriver{}
	sess, err := drv.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: ":memory:",
	})
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table failed: %v", err)
	}

	if _, err := sess.Explain(ctx, `SELECT 1; DROP TABLE users`); err == nil {
		t.Fatalf("expected stacked DROP inside Explain to be rejected")
	}

	// Table users MUST still exist!
	if _, err := sess.QueryRaw(ctx, `SELECT COUNT(*) FROM users`); err != nil {
		t.Fatalf("users table was dropped by Explain! err=%v", err)
	}
}
