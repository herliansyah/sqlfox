package mock

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

// Config specifies what to generate.
type Config struct {
	DriverName string // "mysql", "sqlite", "postgres", "pg", "postgresql"
	Database   string
	Table      string
	Rows       int
}

// Result is returned after successful generation.
type Result struct {
	Inserted   int   `json:"inserted"`
	DurationMs int64 `json:"duration_ms"`
}

// Generate seeds Config.Table with Config.Rows rows of synthetic data,
// respecting foreign key referential integrity.
func Generate(ctx context.Context, sess driver.Session, cfg Config) (*Result, error) {
	if cfg.Rows <= 0 || cfg.Rows > 5000 {
		return nil, fmt.Errorf("rows must be between 1 and 5000, got %d", cfg.Rows)
	}

	start := time.Now()
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	schema, err := sess.InspectSchema(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("inspect schema: %w", err)
	}

	var targetTable *driver.TableMetadata
	for i := range schema.Tables {
		if strings.EqualFold(schema.Tables[i].Name, cfg.Table) {
			targetTable = &schema.Tables[i]
			break
		}
	}
	if targetTable == nil {
		return nil, fmt.Errorf("table %q not found in database %q", cfg.Table, cfg.Database)
	}

	isPostgres := strings.HasPrefix(cfg.DriverName, "pg") || cfg.DriverName == "postgres" || cfg.DriverName == "postgresql"
	isMySQL := cfg.DriverName == "mysql" || cfg.DriverName == "mariadb"
	isSQLite := cfg.DriverName == "sqlite"

	// FK checks are relaxed on the pinned transaction connection below (MySQL FOREIGN_KEY_CHECKS,
	// SQLite defer_foreign_keys); session-level PRAGMAs on the pool would leak to arbitrary connections.

	// Detect self-referential FKs and build FK pool for cross-table references.
	isSelfRef := map[string]bool{} // col.Name → true if self-ref FK
	fkPool := map[string][]any{}   // col.Name → sampled parent PKs

	for _, fk := range targetTable.ForeignKeys {
		if strings.EqualFold(fk.ReferencedTable, cfg.Table) {
			isSelfRef[fk.Column] = true
			continue
		}
		// Build pool from parent table.
		qRef := driver.QuoteIdentifier(fk.ReferencedColumn, cfg.DriverName)
		qTable := driver.QuoteIdentifier(fk.ReferencedTable, cfg.DriverName)
		poolSQL := fmt.Sprintf("SELECT %s FROM %s LIMIT 1000", qRef, qTable)
		rows, err := sess.QueryRaw(ctx, poolSQL)
		if err == nil {
			pool := make([]any, 0, len(rows))
			for _, row := range rows {
				if len(row) > 0 && row[0] != nil {
					pool = append(pool, row[0])
				}
			}
			if len(pool) > 0 {
				fkPool[fk.Column] = pool
			}
		}
	}

	// Build insert column list — skip auto-increment PKs.
	var insertCols []driver.ColumnMetadata
	for _, col := range targetTable.Columns {
		extra := strings.ToLower(col.Extra)
		if col.IsPrimaryKey && (strings.Contains(extra, "auto_increment") || strings.Contains(extra, "autoincrement")) {
			continue // DB generates this
		}
		// For postgres serial/sequence columns (no explicit auto_increment flag but type is serial)
		tp := strings.ToLower(col.Type)
		if col.IsPrimaryKey && (tp == "serial" || tp == "bigserial" || tp == "smallserial" ||
			strings.HasPrefix(strings.ToLower(col.DefaultValue), "nextval(")) {
			continue // explicit ids would leave the sequence behind and break later inserts
		}
		insertCols = append(insertCols, col)
	}

	if len(insertCols) == 0 {
		return nil, fmt.Errorf("no insertable columns found in table %q", cfg.Table)
	}

	// Sequence counter for integer PKs, continuing after the current maximum so repeated runs
	// (or a table that already has rows) never collide on the primary key.
	seq := 0
	for _, col := range insertCols {
		if !col.IsPrimaryKey || !isIntegerType(col.Type) {
			continue
		}
		maxSQL := fmt.Sprintf("SELECT MAX(%s) FROM %s", driver.QuoteIdentifier(col.Name, cfg.DriverName), driver.QuoteIdentifier(cfg.Table, cfg.DriverName))
		if rows, err := sess.QueryRaw(ctx, maxSQL); err == nil && len(rows) > 0 && len(rows[0]) > 0 && rows[0][0] != nil {
			raw := rows[0][0]
			if b, ok := raw.([]byte); ok {
				raw = string(b)
			}
			if n, err := strconv.ParseInt(fmt.Sprint(raw), 10, 64); err == nil && int(n) > seq {
				seq = int(n)
			}
		}
	}
	qTable := driver.QuoteIdentifier(cfg.Table, cfg.DriverName)

	// Build quoted column list once.
	colNames := make([]string, len(insertCols))
	for i, c := range insertCols {
		colNames[i] = driver.QuoteIdentifier(c.Name, cfg.DriverName)
	}
	colList := strings.Join(colNames, ", ")

	const batchSize = 200

	// One transaction wrapping all batches for this table pinned to a dedicated connection.
	tx, err := sess.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if isSQLite {
		// Scoped to this transaction; SQLite resets it automatically at COMMIT/ROLLBACK.
		if _, err := tx.ExecRaw(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
			return nil, fmt.Errorf("defer FK checks: %w", err)
		}
	}
	if isMySQL {
		if _, err := tx.ExecRaw(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
			return nil, fmt.Errorf("disable FK checks: %w", err)
		}
		// Restore before the connection returns to the pool (runs before Rollback on error paths).
		defer tx.ExecRaw(ctx, "SET FOREIGN_KEY_CHECKS=1") //nolint:errcheck
	}

	totalInserted := 0
	for batchStart := 0; batchStart < cfg.Rows; batchStart += batchSize {
		count := batchSize
		if batchStart+count > cfg.Rows {
			count = cfg.Rows - batchStart
		}

		// Build parameterized VALUES.
		var args []any
		var placeholderRows []string
		argIdx := 1

		for row := 0; row < count; row++ {
			ph := make([]string, len(insertCols))
			for ci, col := range insertCols {
				v := generateValue(col, fkPool, isSelfRef, &seq, r)
				args = append(args, v)
				if isPostgres {
					ph[ci] = fmt.Sprintf("$%d", argIdx)
				} else {
					ph[ci] = "?"
				}
				argIdx++
			}
			placeholderRows = append(placeholderRows, "("+strings.Join(ph, ", ")+")")
		}

		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
			qTable, colList, strings.Join(placeholderRows, ", "))

		n, err := tx.ExecRaw(ctx, insertSQL, args...)
		if err != nil {
			return nil, fmt.Errorf("insert batch at offset %d: %w", batchStart, err)
		}
		totalInserted += int(n)
	}

	if isMySQL {
		if _, err := tx.ExecRaw(ctx, "SET FOREIGN_KEY_CHECKS=1"); err != nil {
			return nil, fmt.Errorf("restore FK checks: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Second pass: resolve self-referential FK columns.
	// Find the PK column name.
	pkCol := ""
	for _, col := range targetTable.Columns {
		if col.IsPrimaryKey {
			pkCol = col.Name
			break
		}
	}

	if pkCol != "" {
		qPK := driver.QuoteIdentifier(pkCol, cfg.DriverName)
		for col := range isSelfRef {
			qCol := driver.QuoteIdentifier(col, cfg.DriverName)
			var updateSQL string
			if isMySQL {
				// MySQL doesn't allow UPDATE...SELECT from same table without subquery alias.
				updateSQL = fmt.Sprintf(
					"UPDATE %s SET %s = (SELECT pk FROM (SELECT %s AS pk FROM %s ORDER BY RAND() LIMIT 1) AS _t) WHERE %s IS NULL",
					qTable, qCol, qPK, qTable, qCol)
			} else {
				// SQLite and PostgreSQL support direct subquery.
				updateSQL = fmt.Sprintf(
					"UPDATE %s SET %s = (SELECT %s FROM %s ORDER BY RANDOM() LIMIT 1) WHERE %s IS NULL",
					qTable, qCol, qPK, qTable, qCol)
			}
			sess.ExecRaw(ctx, updateSQL) //nolint:errcheck — best-effort, data still valid with NULLs
		}
	}

	return &Result{
		Inserted:   cfg.Rows,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

func isIntegerType(t string) bool {
	tp := strings.ToLower(strings.TrimSpace(t))
	if tp == "tinyint(1)" || tp == "bool" || tp == "boolean" {
		return false
	}
	return strings.Contains(tp, "int")
}

var enumRegex = regexp.MustCompile(`'([^']*)'`)

// generateValue synthesizes a single column value.
// ponytail: Type matching is a linear string-prefix scan — fine for the dozens of types we handle.
func generateValue(col driver.ColumnMetadata, fkPool map[string][]any, isSelfRef map[string]bool, seq *int, r *rand.Rand) any {
	if isSelfRef[col.Name] {
		return nil // NULL placeholder; resolved in second pass
	}
	if pool, ok := fkPool[col.Name]; ok && len(pool) > 0 {
		return pool[r.Intn(len(pool))]
	}
	if col.IsNullable && !col.IsPrimaryKey && r.Intn(5) == 0 {
		return nil // 20% NULL for nullable non-PK columns
	}

	*seq++
	tp := strings.ToLower(strings.TrimSpace(col.Type))
	if col.IsPrimaryKey && isIntegerType(tp) {
		return *seq // unique and increasing; random values would collide (birthday bound)
	}

	switch {
	case tp == "tinyint(1)" || tp == "bool" || tp == "boolean":
		return r.Intn(2)
	case strings.HasPrefix(tp, "tinyint"):
		return r.Intn(128)
	case strings.HasPrefix(tp, "smallint"):
		return r.Intn(32000) + 1
	case strings.HasPrefix(tp, "bigint"):
		return r.Int63n(999999) + 1
	case strings.HasPrefix(tp, "int") || tp == "integer":
		if col.IsPrimaryKey {
			return *seq
		}
		return r.Intn(10000) + 1
	case strings.HasPrefix(tp, "float") || strings.HasPrefix(tp, "double") ||
		tp == "real" || strings.HasPrefix(tp, "decimal") || strings.HasPrefix(tp, "numeric"):
		return fmt.Sprintf("%.2f", float64(r.Intn(100000)+1)/100.0)
	case tp == "uuid" || tp == "char(36)":
		return randUUID(r)
	case strings.HasPrefix(tp, "varchar") || strings.HasPrefix(tp, "char") ||
		tp == "tinytext" || tp == "text" || tp == "mediumtext" || tp == "longtext" || tp == "clob":
		words := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"}
		return words[r.Intn(len(words))] + "_" + randStr(r, 6)
	case tp == "date":
		return fmt.Sprintf("20%02d-%02d-%02d", r.Intn(6)+20, r.Intn(12)+1, r.Intn(28)+1)
	case tp == "timestamptz":
		return fmt.Sprintf("2024-%02d-%02dT%02d:%02d:00Z", r.Intn(12)+1, r.Intn(28)+1, r.Intn(24), r.Intn(60))
	case strings.HasPrefix(tp, "timestamp") || tp == "datetime":
		return fmt.Sprintf("2024-%02d-%02d %02d:%02d:00", r.Intn(12)+1, r.Intn(28)+1, r.Intn(24), r.Intn(60))
	case strings.HasPrefix(tp, "json"):
		return fmt.Sprintf(`{"mock":true,"id":%d}`, r.Intn(9999)+1)
	case tp == "blob" || tp == "binary" || tp == "bytea" || strings.HasPrefix(tp, "varbinary"):
		return "0xDEADBEEF"
	case strings.HasPrefix(tp, "enum"):
		matches := enumRegex.FindAllStringSubmatch(tp, -1)
		if len(matches) > 0 {
			return matches[r.Intn(len(matches))][1]
		}
		return "unknown"
	default:
		return fmt.Sprintf("mock_value_%d", *seq)
	}
}

func randUUID(r *rand.Rand) string {
	return fmt.Sprintf("%08x-%04x-4%03x-%04x-%012x",
		r.Uint32(),
		r.Intn(0xffff),
		r.Intn(0xfff),
		r.Intn(0xffff)|0x8000,
		r.Int63n(0xffffffffffff),
	)
}

func randStr(r *rand.Rand, n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[r.Intn(len(chars))]
	}
	return string(b)
}
