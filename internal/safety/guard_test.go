package safety

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/sqlite"
)

func TestSafetyGuard_Inspect(t *testing.T) {

	tests := []struct {
		name              string
		query             string
		env               string
		expectedRisk      RiskLevel
		expectedChallenge bool
		expectedTable     string
	}{
		{
			name:              "Unqualified DELETE in Dev",
			query:             "DELETE FROM users;",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "users",
		},
		{
			name:              "Qualified DELETE in Dev",
			query:             "DELETE FROM users WHERE id = 10;",
			env:               "Development",
			expectedRisk:      RiskNone,
			expectedChallenge: false,
		},
		{
			name:              "Qualified DELETE in Prod",
			query:             "DELETE FROM users WHERE id = 10;",
			env:               "Production",
			expectedRisk:      RiskWarning,
			expectedChallenge: false,
			expectedTable:     "users",
		},
		{
			name:              "Unqualified UPDATE",
			query:             "UPDATE accounts SET balance = 0;",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "accounts",
		},
		{
			name:              "CTE-wrapped unqualified DELETE",
			query:             "WITH x AS (SELECT 1 WHERE 1=1) DELETE FROM users",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "users",
		},
		{
			name:              "CTE-wrapped unqualified DELETE with single-line comment containing parenthesis",
			query:             "WITH cte AS (SELECT 1 -- (ignore this\n) DELETE FROM users",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "users",
		},
		{
			name:              "CTE-wrapped unqualified DELETE with block comment containing parenthesis",
			query:             "WITH cte AS (SELECT 1 /* (nested note) */ ) DELETE FROM users",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "users",
		},
		{
			name:              "CTE-wrapped unqualified DELETE with escaped quote and parenthesis",
			query:             "WITH cte AS (SELECT 'don\\'t (break' AS msg) DELETE FROM users",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "users",
		},
		{
			name:              "CTE-wrapped unqualified UPDATE",
			query:             "with recursive x(n) as (select 1) update accounts set balance = 0",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "accounts",
		},
		{
			name:              "DROP TABLE",
			query:             "DROP TABLE customers;",
			env:               "Production",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "customers",
		},
		{
			name:              "TRUNCATE TABLE",
			query:             "TRUNCATE TABLE audit_logs;",
			env:               "Staging",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "audit_logs",
		},
		{
			name:              "Safe SELECT",
			query:             "SELECT * FROM users WHERE active = 1;",
			env:               "Production",
			expectedRisk:      RiskNone,
			expectedChallenge: false,
		},
		{
			name:              "PostgreSQL Unqualified DELETE with RETURNING",
			query:             `DELETE FROM "public"."orders" RETURNING *;`,
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "orders",
		},
		{
			name:              "PostgreSQL Qualified DELETE with RETURNING in Dev",
			query:             `DELETE FROM "public"."orders" WHERE id = $1 RETURNING *;`,
			env:               "Development",
			expectedRisk:      RiskNone,
			expectedChallenge: false,
		},
		{
			name:              "PostgreSQL DROP TABLE with quotes",
			query:             `DROP TABLE IF EXISTS "analytics"."page_views";`,
			env:               "Production",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "page_views",
		},
		{
			name:              "PostgreSQL DROP TABLE with leading comments",
			query:             "-- Important migration\n/* block comment */\nDROP TABLE IF EXISTS \"analytics\".\"page_views\";",
			env:               "Production",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "page_views",
		},
		{
			name:              "Unqualified DELETE with leading comments",
			query:             "-- purge data\nDELETE FROM customers;",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "customers",
		},
		{
			name:              "CREATE TABLE in Dev requires affirmation",
			query:             "CREATE TABLE orders (id INT, total DECIMAL);",
			env:               "Development",
			expectedRisk:      RiskWarning,
			expectedChallenge: false,
			expectedTable:     "orders",
		},
		{
			name:              "CREATE TABLE in Prod requires challenge",
			query:             "CREATE TABLE orders (id INT, total DECIMAL);",
			env:               "Production",
			expectedRisk:      RiskWarning,
			expectedChallenge: true,
			expectedTable:     "orders",
		},
		{
			name:              "ALTER TABLE in Dev requires affirmation",
			query:             "ALTER TABLE orders ADD COLUMN status VARCHAR(20);",
			env:               "Development",
			expectedRisk:      RiskDangerous,
			expectedChallenge: false,
			expectedTable:     "orders",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Inspect(tc.query, tc.env)
			if res.RiskLevel != tc.expectedRisk {
				t.Fatalf("expected risk %s, got %s (reason: %s)", tc.expectedRisk, res.RiskLevel, res.Reason)
			}
			if res.RequiresChallenge != tc.expectedChallenge {
				t.Fatalf("expected challenge %v, got %v", tc.expectedChallenge, res.RequiresChallenge)
			}
			if tc.expectedTable != "" && res.TargetTable != tc.expectedTable {
				t.Fatalf("expected target table %s, got %s", tc.expectedTable, res.TargetTable)
			}
		})
	}
}

func TestSafetyGuard_InspectWithScope(t *testing.T) {
	// 1. Qualified DDL with matching tab scope
	res := InspectWithScope("ALTER TABLE `db_a`.`orders` ADD COLUMN status INT", "Development", "db_a")
	if res.SchemaScopeMismatch {
		t.Fatalf("expected no mismatch for matching db_a, got mismatch: %s", res.Reason)
	}
	if res.TargetDatabase != "db_a" {
		t.Fatalf("expected target database 'db_a', got '%s'", res.TargetDatabase)
	}

	// 2. Qualified DDL with mismatched tab scope (targets db_b while tab is db_a)
	mismatchRes := InspectWithScope("ALTER TABLE `db_b`.`orders` ADD COLUMN status INT", "Development", "db_a")
	if !mismatchRes.SchemaScopeMismatch {
		t.Fatalf("expected SchemaScopeMismatch=true when query targets db_b but scope is db_a")
	}
	if !mismatchRes.RequiresChallenge {
		t.Fatalf("expected RequiresChallenge=true on SchemaScopeMismatch")
	}

	// 3. Unqualified DDL adopts tab scope
	unqualifiedRes := InspectWithScope("CREATE TABLE items (id INT);", "Development", "db_production")
	if unqualifiedRes.TargetDatabase != "db_production" {
		t.Fatalf("expected unqualified DDL to adopt scoped database 'db_production', got '%s'", unqualifiedRes.TargetDatabase)
	}
	if !unqualifiedRes.RequiresAffirmation {
		t.Fatalf("expected RequiresAffirmation=true for CREATE TABLE")
	}
}

func TestBackupManager_BackupTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-backup-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	ctx := context.Background()

	sess, err := driver.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	// Create table with data
	_, err = sess.Exec(ctx, `
		CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);
		INSERT INTO items (name) VALUES ('Keyboard'), ('Mouse');
	`)
	if err != nil {
		t.Fatal(err)
	}

	backupPath, dataIncluded, err := BackupTable(ctx, sess, filepath.Join(tempDir, "backups"), "items")
	if err != nil {
		t.Fatalf("BackupTable failed: %v", err)
	}

	if !dataIncluded {
		t.Fatal("expected data to be included in small table backup")
	}

	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(content) == 0 {
		t.Fatal("expected non-empty backup content")
	}
}

// Regression: lexical fallback must not trust WHERE inside comments or string literals.
func TestSafetyGuard_LexicalFallbackIgnoresWhereInCommentsAndStrings(t *testing.T) {
	for _, q := range []string{
		"DELETE FROM t -- keep rows WHERE active=1\nRETURNING *",
		"UPDATE t SET note='only WHERE active' RETURNING *",
		"DELETE FROM t /* WHERE x */ RETURNING *",
	} {
		if r := Inspect(q, "Development"); !r.RequiresChallenge {
			t.Errorf("expected challenge for %q", q)
		}
	}
	if r := Inspect("DELETE FROM t WHERE id = 1 RETURNING *", "Development"); r.RequiresChallenge {
		t.Errorf("qualified DELETE ... RETURNING must not require challenge")
	}
}

// Regression: MySQL executable comments (/*! ... */) run their body, so the guard must inspect it.
func TestSafetyGuard_MySQLExecutableCommentNotBypassed(t *testing.T) {
	for _, q := range []string{
		"/*! DROP TABLE users */",
		"/*!40000 DROP TABLE users */",
		"/*! DELETE FROM users */",
		"/* note */ /*!40001 UPDATE accounts SET balance=0 */",
		"/*!TRUNCATE TABLE users*/",
	} {
		if r := Inspect(q, "Development"); !r.RequiresChallenge {
			t.Errorf("expected challenge for %q, got risk=%v", q, r.RiskLevel)
		}
	}
	if r := Inspect("/* plain comment */ SELECT 1", "Development"); r.RequiresChallenge {
		t.Errorf("plain comment before SELECT must not require challenge")
	}
}

func TestSafetyGuard_PreExecutionCountQuery(t *testing.T) {
	tests := []struct {
		query            string
		env              string
		expectedOp       string
		expectedHasWhere bool
		expectedCount    string
	}{
		{
			query:            "DELETE FROM users WHERE id = 10",
			env:              "Development",
			expectedOp:       "DELETE",
			expectedHasWhere: true,
			expectedCount:    "SELECT COUNT(*) FROM users where id = 10",
		},
		{
			query:            "UPDATE accounts SET balance = 100 WHERE status = 'active'",
			env:              "Production",
			expectedOp:       "UPDATE",
			expectedHasWhere: true,
			expectedCount:    "SELECT COUNT(*) FROM accounts where `status` = 'active'",
		},
		{
			query:            "DELETE FROM orders RETURNING *",
			env:              "Development",
			expectedOp:       "DELETE",
			expectedHasWhere: false,
			expectedCount:    "SELECT COUNT(*) FROM orders",
		},
		{
			query:            "DELETE FROM orders WHERE status = 'cancelled' RETURNING *",
			env:              "Development",
			expectedOp:       "DELETE",
			expectedHasWhere: true,
			expectedCount:    "SELECT COUNT(*) FROM orders WHERE status = 'cancelled'",
		},
	}

	for _, tc := range tests {
		res := Inspect(tc.query, tc.env)
		if res.Operation != tc.expectedOp {
			t.Errorf("[%s] expected op %q, got %q", tc.query, tc.expectedOp, res.Operation)
		}
		if res.HasWhere != tc.expectedHasWhere {
			t.Errorf("[%s] expected hasWhere %v, got %v", tc.query, tc.expectedHasWhere, res.HasWhere)
		}
		if res.CountQuery != tc.expectedCount {
			t.Errorf("[%s] expected count query %q, got %q", tc.query, tc.expectedCount, res.CountQuery)
		}
	}
}

func TestSafetyGuard_DropDatabaseAndDialectLiteralBypasses(t *testing.T) {
	critical := []string{
		"DROP DATABASE prod_db",
		"DROP SCHEMA prod_db",
		"DROP PROCEDURE sp_cleanup",
		// PostgreSQL/SQLite: '\' is a complete literal, so these have no real WHERE.
		`UPDATE accounts SET note = '\', flag = ' WHERE '`,
		`UPDATE accounts SET a = '\', b = ' WHERE id=1 OR 1=1 -- '`,
		`DELETE FROM accounts /* /* */ WHERE id = 1 */`,
		`UPDATE accounts SET a = $$ WHERE id = 1 $$`,
		`UPDATE accounts SET a = $x$ WHERE id = 1 $x$`,
	}
	for _, q := range critical {
		r := Inspect(q, "Production")
		if r.RiskLevel != RiskCritical || !r.RequiresChallenge {
			t.Errorf("%q: expected CRITICAL + challenge, got risk=%v challenge=%v", q, r.RiskLevel, r.RequiresChallenge)
		}
	}
	if r := Inspect("DROP DATABASE prod_db", "Development"); r.TargetType != "DATABASE" || r.TargetTable != "prod_db" {
		t.Errorf("DROP DATABASE target: got type=%q table=%q", r.TargetType, r.TargetTable)
	}

	qualified := []string{
		"UPDATE accounts SET a = 'x' WHERE id = 1",
		`UPDATE accounts SET a = 'it''s' WHERE id = 1`,
		"DELETE FROM accounts WHERE id = $1",
	}
	for _, q := range qualified {
		if r := Inspect(q, "Development"); !r.HasWhere || r.RequiresChallenge {
			t.Errorf("%q: expected qualified mutation, got where=%v challenge=%v", q, r.HasWhere, r.RequiresChallenge)
		}
	}
}

func TestBackupFileName_StaysInsideBackupDir(t *testing.T) {
	for _, name := range []string{"/../../../../home/user/.profile", "..", "a/b\\c", ""} {
		fn := backupFileName("20260101-000000", name)
		if filepath.Base(fn) != fn || filepath.Clean(filepath.Join("/backups", fn)) != filepath.Join("/backups", fn) {
			t.Errorf("backupFileName(%q) = %q escapes the backup dir", name, fn)
		}
	}
}
