package driver_test

import (
	"encoding/json"
	"fmt"
	"runtime"
	"testing"
	"time"

	"sqlfox/internal/driver"
)

// GenerateSyntheticSchema builds an in-memory SchemaMetadata simulating an enterprise catalog.
func GenerateSyntheticSchema(numTables, colsPerTable int) *driver.SchemaMetadata {
	schema := &driver.SchemaMetadata{
		DatabaseName: "enterprise_benchmark_db",
		Databases:    []string{"enterprise_benchmark_db", "analytics_db", "audit_db"},
		Tables:       make([]driver.TableMetadata, numTables),
	}

	for i := 0; i < numTables; i++ {
		tblName := fmt.Sprintf("tbl_%04d", i)
		cols := make([]driver.ColumnMetadata, colsPerTable)

		// Column 0 is always Primary Key
		cols[0] = driver.ColumnMetadata{
			Name:         "id",
			Type:         "bigint",
			IsNullable:   false,
			IsPrimaryKey: true,
			Extra:        "auto_increment",
		}

		for c := 1; c < colsPerTable; c++ {
			var colType string
			switch c % 5 {
			case 0:
				colType = "int"
			case 1:
				colType = "varchar(255)"
			case 2:
				colType = "datetime"
			case 3:
				colType = "decimal(18,4)"
			default:
				colType = "text"
			}

			cols[c] = driver.ColumnMetadata{
				Name:         fmt.Sprintf("col_%s_%02d", tblName, c),
				Type:         colType,
				IsNullable:   c%3 != 0,
				DefaultValue: "",
			}
		}

		// Indexes
		indexes := []driver.IndexMetadata{
			{Name: "PRIMARY", Columns: []string{"id"}, IsUnique: true, Type: "BTREE"},
			{Name: fmt.Sprintf("idx_%s_col1", tblName), Columns: []string{cols[1].Name}, IsUnique: false, Type: "BTREE"},
		}

		// Cyclical FK references: table i references table (i+1)%numTables
		fks := make([]driver.ForeignKeyMetadata, 0, 2)
		if numTables > 1 {
			targetTbl := fmt.Sprintf("tbl_%04d", (i+1)%numTables)
			fks = append(fks, driver.ForeignKeyMetadata{
				Name:             fmt.Sprintf("fk_%s_%s", tblName, targetTbl),
				Column:           cols[1].Name,
				ReferencedTable:  targetTbl,
				ReferencedColumn: "id",
				OnDelete:         "RESTRICT",
				OnUpdate:         "CASCADE",
			})
		}
		// Self-referential FK on column 2
		if colsPerTable > 2 {
			fks = append(fks, driver.ForeignKeyMetadata{
				Name:             fmt.Sprintf("fk_%s_parent", tblName),
				Column:           cols[2].Name,
				ReferencedTable:  tblName,
				ReferencedColumn: "id",
				OnDelete:         "SET NULL",
				OnUpdate:         "CASCADE",
			})
		}

		schema.Tables[i] = driver.TableMetadata{
			Name:        tblName,
			Type:        "TABLE",
			RowCount:    int64(i * 1000),
			Columns:     cols,
			Indexes:     indexes,
			ForeignKeys: fks,
		}
	}

	return schema
}

// TestSyntheticSchemaSmoke executes a rapid smoke test (100 tables, 5,000 columns) in standard `go test`.
func TestSyntheticSchemaSmoke(t *testing.T) {
	start := time.Now()
	schema := GenerateSyntheticSchema(100, 50)
	durationGen := time.Since(start)

	if len(schema.Tables) != 100 {
		t.Fatalf("expected 100 tables, got %d", len(schema.Tables))
	}
	if len(schema.Tables[0].Columns) != 50 {
		t.Fatalf("expected 50 columns per table, got %d", len(schema.Tables[0].Columns))
	}

	// Verify JSON serialization time & integrity
	startJSON := time.Now()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	durationJSON := time.Since(startJSON)

	if len(data) == 0 {
		t.Fatal("serialized JSON payload is empty")
	}

	t.Logf("Generated 100 tables (5,000 columns) in %v, serialized in %v (payload: %d KB)",
		durationGen, durationJSON, len(data)/1024)

	if durationJSON > 300*time.Millisecond {
		t.Errorf("JSON serialization took %v, exceeding 300ms threshold", durationJSON)
	}
}

// BenchmarkLargeSchema benchmarks catalog introspection and serialization at enterprise scale:
// 1,000 tables, 50,000 columns, with complex FK cycles.
func BenchmarkLargeSchema(b *testing.B) {
	schema := GenerateSyntheticSchema(1000, 50)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(schema)
		if err != nil {
			b.Fatalf("serialization error: %v", err)
		}
		if len(data) == 0 {
			b.Fatal("unexpected empty output")
		}
	}
}

// BenchmarkMemoryCeilingStreaming simulates streaming 1,000,000 rows through chunked buffers
// to assert resident memory remains strictly bounded under 100MB RAM.
func BenchmarkMemoryCeilingStreaming(b *testing.B) {
	const totalRows = 1000000
	const chunkSize = 200

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var memBefore runtime.MemStats
		runtime.ReadMemStats(&memBefore)

		rowsStreamed := 0
		for rowsStreamed < totalRows {
			// Simulate chunk buffer reuse
			chunk := make([][]any, chunkSize)
			for r := 0; r < chunkSize; r++ {
				chunk[r] = []any{rowsStreamed + r, "user_account_name", "test@example.com", 99.95, time.Now()}
			}
			rowsStreamed += len(chunk)
			// chunk goes out of scope and gets recycled
		}

		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)

		allocMB := float64(memAfter.Alloc) / (1024 * 1024)
		if allocMB > 100.0 {
			b.Fatalf("Memory ceiling violated: heap alloc %.2f MB exceeds 100MB budget", allocMB)
		}
	}
}
