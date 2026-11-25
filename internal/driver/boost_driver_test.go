package driver_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/herliansyah/sqlfox/internal/driver"
)

func TestBoostP1_DetectRedundantIndexesSkipsPGAndSQLitePK(t *testing.T) {
	tables := []driver.TableMetadata{
		{
			Name: "users",
			Columns: []driver.ColumnMetadata{
				{Name: "id", IsPrimaryKey: true},
				{Name: "email"},
			},
			Indexes: []driver.IndexMetadata{
				{Name: "users_pkey", Columns: []string{"id"}, IsUnique: true, Type: "BTREE"},
				{Name: "sqlite_autoindex_users_1", Columns: []string{"id"}, IsUnique: true, Type: "BTREE"},
			},
		},
	}

	got := driver.DetectRedundantIndexes("postgres", "public", tables)
	if len(got) != 0 {
		t.Fatalf("expected 0 redundant indexes for PG/SQLite PK indexes, got %+v", got)
	}
}

func TestBoostP1_SpecialFloatsJSONAndSQL(t *testing.T) {
	row := []any{math.NaN(), math.Inf(1), math.Inf(-1), int64(9007199254740993)}
	formatted := driver.FormatRowForJSON(row)
	b, err := json.Marshal(formatted)
	if err != nil {
		t.Fatalf("json.Marshal failed on formatted NaN/Inf row: %v", err)
	}
	want := `["NaN","Infinity","-Infinity","9007199254740993"]`
	if string(b) != want {
		t.Fatalf("formatted JSON = %s; want %s", string(b), want)
	}

	if got := driver.FormatSQLValueForDriver(math.NaN(), "postgres"); got != "'NaN'::float8" {
		t.Errorf("PG NaN SQL literal = %q; want 'NaN'::float8", got)
	}
	if got := driver.FormatSQLValueForDriver(math.Inf(1), "postgres"); got != "'Infinity'::float8" {
		t.Errorf("PG +Inf SQL literal = %q; want 'Infinity'::float8", got)
	}
	if got := driver.FormatSQLValueForDriver(math.Inf(-1), "postgres"); got != "'-Infinity'::float8" {
		t.Errorf("PG -Inf SQL literal = %q; want '-Infinity'::float8", got)
	}
}

func TestBoost_SplitAndExplainTarget_BackslashFollowedByClosingQuote(t *testing.T) {
	// PostgreSQL E'\'' closes the string after the escaped quote; the trailing DROP TABLE is a separate statement.
	pgSQL := `SELECT E'\''; DROP TABLE users; SELECT ''`
	pgStmts := driver.SplitSQLStatements(pgSQL, "postgres")
	if len(pgStmts) != 3 {
		t.Fatalf("SplitSQLStatements(postgres, %q) = %d statements (%#v); want 3", pgSQL, len(pgStmts), pgStmts)
	}
	if _, err := driver.ExplainTarget(pgStmts); err == nil {
		t.Fatalf("ExplainTarget must reject stacked PG E'\\'' payload")
	}

	// MySQL '\'' likewise closes the string after the backslash-escaped quote.
	mySQL := `SELECT '\''; DROP TABLE users; SELECT ''`
	myStmts := driver.SplitSQLStatements(mySQL, "mysql")
	if len(myStmts) != 3 {
		t.Fatalf("SplitSQLStatements(mysql, %q) = %d statements (%#v); want 3", mySQL, len(myStmts), myStmts)
	}
	if _, err := driver.ExplainTarget(myStmts); err == nil {
		t.Fatalf("ExplainTarget must reject stacked MySQL '\\'' payload")
	}

	if got := driver.Placeholder("pgx", 2); got != "$2" {
		t.Fatalf("Placeholder(pgx, 2) = %q; want $2", got)
	}
}
