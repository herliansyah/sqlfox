package driver_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/sqlite"
)

func TestInspectTableRelationships(t *testing.T) {
	schema := &driver.SchemaMetadata{
		DatabaseName: "testdb",
		Tables: []driver.TableMetadata{
			{
				Name: "order_status",
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
					{Name: "name", Type: "TEXT"},
				},
				ForeignKeys: []driver.ForeignKeyMetadata{},
			},
			{
				Name: "orders",
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
					{Name: "status_id", Type: "INTEGER"},
					{Name: "amount", Type: "REAL"},
				},
				ForeignKeys: []driver.ForeignKeyMetadata{
					{
						Name:             "fk_orders_status",
						Column:           "status_id",
						ReferencedTable:  "order_status",
						ReferencedColumn: "id",
					},
				},
			},
		},
	}

	// 1. Inspect master table order_status (should detect inbound dependency from orders)
	masterRels := driver.InspectTableRelationships(schema, "order_status")
	if len(masterRels) != 1 {
		t.Fatalf("expected 1 relationship for order_status, got %d", len(masterRels))
	}
	if masterRels[0].Direction != "PARENT" {
		t.Errorf("expected direction PARENT, got %s", masterRels[0].Direction)
	}
	if masterRels[0].RelatedTable != "orders" {
		t.Errorf("expected related table orders, got %s", masterRels[0].RelatedTable)
	}

	// 2. Inspect child table orders (should detect outbound dependency to order_status)
	childRels := driver.InspectTableRelationships(schema, "orders")
	if len(childRels) != 1 {
		t.Fatalf("expected 1 relationship for orders, got %d", len(childRels))
	}
	if childRels[0].Direction != "CHILD" {
		t.Errorf("expected direction CHILD, got %s", childRels[0].Direction)
	}
	if childRels[0].RelatedTable != "order_status" {
		t.Errorf("expected related table order_status, got %s", childRels[0].RelatedTable)
	}
}

func TestGenerateOSCCommand(t *testing.T) {
	// 1. gh-ost generator
	ghostCmd := driver.GenerateOSCCommand("gh-ost", "10.0.0.1", 3306, "admin", "prod_db", "orders", "ADD COLUMN tracking_num VARCHAR(64)")
	if !strings.Contains(ghostCmd, "gh-ost") {
		t.Fatalf("expected gh-ost in output: %s", ghostCmd)
	}
	if !strings.Contains(ghostCmd, "--table=\"orders\"") {
		t.Fatalf("expected table orders in gh-ost: %s", ghostCmd)
	}
	if !strings.Contains(ghostCmd, "--alter=\"ADD COLUMN tracking_num VARCHAR(64)\"") {
		t.Fatalf("expected alter clause in gh-ost: %s", ghostCmd)
	}

	// 2. pt-online-schema-change generator
	ptCmd := driver.GenerateOSCCommand("pt-osc", "10.0.0.1", 3306, "admin", "prod_db", "orders", "ALTER TABLE orders ADD COLUMN tracking_num VARCHAR(64)")
	if !strings.Contains(ptCmd, "pt-online-schema-change") {
		t.Fatalf("expected pt-online-schema-change in output: %s", ptCmd)
	}
	if !strings.Contains(ptCmd, "D=\"prod_db\",t=\"orders\"") {
		t.Fatalf("expected db/table target in pt-osc: %s", ptCmd)
	}
}

func TestSQLiteSession_ExecSafeDDL(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sess, err := driver.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("failed to connect to in-memory sqlite: %v", err)
	}
	defer sess.Close()

	// Create initial table
	_, err = sess.Exec(ctx, "CREATE TABLE items (id INTEGER PRIMARY KEY, title TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	// Execute safe DDL with fail-fast timeout
	res, err := sess.ExecSafeDDL(ctx, "ALTER TABLE items ADD COLUMN price REAL", 2)
	if err != nil {
		t.Fatalf("ExecSafeDDL failed: %v", err)
	}
	if res == nil || res.ExecutionMs < 0 {
		t.Fatalf("unexpected ExecResult: %+v", res)
	}

	// Verify schema alteration took effect
	schema, err := sess.InspectSchema(ctx, "")
	if err != nil {
		t.Fatalf("InspectSchema failed: %v", err)
	}
	var foundCol bool
	for _, tbl := range schema.Tables {
		if tbl.Name == "items" {
			for _, col := range tbl.Columns {
				if col.Name == "price" {
					foundCol = true
				}
			}
		}
	}
	if !foundCol {
		t.Fatalf("expected column 'price' to exist after safe DDL alteration")
	}
}
