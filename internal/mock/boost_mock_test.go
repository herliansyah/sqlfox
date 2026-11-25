package mock

import (
	"context"
	"testing"

	"github.com/herliansyah/sqlfox/internal/driver"
	_ "github.com/herliansyah/sqlfox/internal/driver/sqlite"
)

func TestBoostP010_SelfRefFKPreservesExistingRootRowsAndAvoidsCycles(t *testing.T) {
	ctx := context.Background()
	sess, err := driver.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: ":memory:",
	})
	if err != nil {
		t.Fatalf("connect sqlite :memory: failed: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Exec(ctx, `
		CREATE TABLE categories (
			id INTEGER PRIMARY KEY,
			parent_id INTEGER NULL REFERENCES categories(id),
			name TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create table failed: %v", err)
	}

	// Pre-existing legitimate root node (parent_id IS NULL) MUST remain untouched!
	if _, err := sess.Exec(ctx, `INSERT INTO categories (id, parent_id, name) VALUES (1, NULL, 'Root')`); err != nil {
		t.Fatalf("insert root failed: %v", err)
	}

	res, err := Generate(ctx, sess, Config{
		DriverName: "sqlite",
		Database:   "main",
		Table:      "categories",
		Rows:       6,
	})
	if err != nil {
		t.Fatalf("mock.Generate failed: %v", err)
	}
	if res.Inserted != 6 {
		t.Fatalf("expected 6 inserted rows, got %d", res.Inserted)
	}

	rows, err := sess.QueryRaw(ctx, `SELECT id, parent_id FROM categories ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("select categories failed: %v", err)
	}
	if len(rows) != 7 {
		t.Fatalf("expected 7 total rows, got %d", len(rows))
	}

	// Row 1 MUST still have parent_id == nil!
	if rows[0][1] != nil {
		t.Fatalf("existing root row (id=1) had its parent_id clobbered to %v!", rows[0][1])
	}

	// No row may have self-cycle (id == parent_id), and newly inserted rows must have non-nil parent_id.
	for _, r := range rows[1:] {
		if r[1] == nil {
			t.Errorf("new row id=%v still has NULL parent_id", r[0])
		}
		if r[0] == r[1] {
			t.Errorf("self-cycle detected on row id=%v parent_id=%v", r[0], r[1])
		}
	}
}

type bytePKSession struct {
	driver.Session
}

func (b *bytePKSession) QueryRaw(ctx context.Context, querySQL string, args ...any) ([][]any, error) {
	rows, err := b.Session.QueryRaw(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		for j := range rows[i] {
			if s, ok := rows[i][j].(string); ok {
				rows[i][j] = []byte(s)
			}
		}
	}
	return rows, nil
}

func TestBoostP010_SelfRefFKPreservesExistingByteSlicePKRootRows(t *testing.T) {
	ctx := context.Background()
	baseSess, err := driver.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: ":memory:",
	})
	if err != nil {
		t.Fatalf("connect sqlite :memory: failed: %v", err)
	}
	defer baseSess.Close()

	if _, err := baseSess.Exec(ctx, `
		CREATE TABLE org_nodes (
			id TEXT PRIMARY KEY,
			parent_id TEXT NULL REFERENCES org_nodes(id),
			name TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	if _, err := baseSess.Exec(ctx, `INSERT INTO org_nodes (id, parent_id, name) VALUES ('root-uuid-1', NULL, 'HQ')`); err != nil {
		t.Fatalf("insert root failed: %v", err)
	}

	wrapped := &bytePKSession{Session: baseSess}
	if _, err := Generate(ctx, wrapped, Config{
		DriverName: "sqlite",
		Database:   "main",
		Table:      "org_nodes",
		Rows:       4,
	}); err != nil {
		t.Fatalf("mock.Generate failed: %v", err)
	}

	rows, err := baseSess.QueryRaw(ctx, `SELECT id, parent_id FROM org_nodes WHERE id = 'root-uuid-1'`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("select root failed: err=%v rows=%v", err, rows)
	}
	if rows[0][1] != nil {
		t.Fatalf("existing root row with []byte PK had parent_id overwritten to %v!", rows[0][1])
	}
}
