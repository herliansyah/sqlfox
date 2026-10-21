package advisor

import (
	"slices"
	"strings"
	"testing"

	"sqlfox/internal/driver"
)

func TestAdvise_ESRRuleOrdering(t *testing.T) {
	query := "SELECT * FROM orders WHERE user_id = 42 AND amount > 100.00 ORDER BY created_at DESC"
	tables := map[string]driver.TableMetadata{
		"orders": {
			Name: "orders",
			Columns: []driver.ColumnMetadata{
				{Name: "id", Type: "INT", IsPrimaryKey: true},
				{Name: "user_id", Type: "INT"},
				{Name: "amount", Type: "DECIMAL(10,2)"},
				{Name: "created_at", Type: "DATETIME"},
			},
			Indexes: []driver.IndexMetadata{
				{Name: "PRIMARY", Columns: []string{"id"}, IsUnique: true},
			},
		},
	}

	res := Advise(query, "mysql", tables)
	if len(res.Recommendations) == 0 {
		t.Fatalf("expected 1 recommendation, got 0")
	}

	rec := res.Recommendations[0]
	if rec.Table != "orders" {
		t.Errorf("expected table 'orders', got %q", rec.Table)
	}

	// ESR Rule check: Equality (user_id) -> Sort (created_at) -> Range (amount)
	expectedCols := []string{"user_id", "created_at", "amount"}
	if !slices.Equal(rec.Columns, expectedCols) {
		t.Errorf("expected columns %v under ESR rule, got %v", expectedCols, rec.Columns)
	}

	// MySQL 5.7 Safe DDL check (with valid comma separator before ALGORITHM)
	expectedDDL := "ALTER TABLE `orders` ADD INDEX `idx_orders_user_id_created_at_amount` (`user_id`, `created_at`, `amount`), ALGORITHM=INPLACE, LOCK=NONE;"
	if rec.SafeDDL != expectedDDL {
		t.Errorf("expected Safe DDL %q, got %q", expectedDDL, rec.SafeDDL)
	}
}

func TestAdvise_DialectAndVersionVariations(t *testing.T) {
	query := "SELECT * FROM orders WHERE user_id = 1 AND amount > 100 ORDER BY created_at DESC"
	tables := map[string]driver.TableMetadata{
		"orders": {
			Name: "orders",
			Columns: []driver.ColumnMetadata{
				{Name: "user_id", Type: "int"},
				{Name: "created_at", Type: "datetime"},
				{Name: "amount", Type: "decimal(10,2)"},
				{Name: "notes", Type: "text"},
			},
		},
	}

	tests := []struct {
		name        string
		dialect     string
		query       string
		expectedDDL string
	}{
		{
			name:        "MySQL 8.0 default non-blocking online DDL",
			dialect:     "mysql 8.0.35",
			query:       query,
			expectedDDL: "ALTER TABLE `orders` ADD INDEX `idx_orders_user_id_created_at_amount` (`user_id`, `created_at`, `amount`), ALGORITHM=INPLACE, LOCK=NONE;",
		},
		{
			name:        "MySQL 5.7 non-blocking online DDL",
			dialect:     "mysql 5.7.42-log",
			query:       query,
			expectedDDL: "ALTER TABLE `orders` ADD INDEX `idx_orders_user_id_created_at_amount` (`user_id`, `created_at`, `amount`), ALGORITHM=INPLACE, LOCK=NONE;",
		},
		{
			name:        "MySQL 5.5 legacy DDL without unsupported ALGORITHM",
			dialect:     "mysql 5.5.62",
			query:       query,
			expectedDDL: "ALTER TABLE `orders` ADD INDEX `idx_orders_user_id_created_at_amount` (`user_id`, `created_at`, `amount`);",
		},
		{
			name:        "MariaDB 10.5 non-blocking online DDL",
			dialect:     "mariadb 10.5.18-MariaDB",
			query:       query,
			expectedDDL: "ALTER TABLE `orders` ADD INDEX `idx_orders_user_id_created_at_amount` (`user_id`, `created_at`, `amount`), ALGORITHM=INPLACE, LOCK=NONE;",
		},
		{
			name:        "MariaDB 5.5 legacy DDL without unsupported ALGORITHM",
			dialect:     "mariadb 5.5.68-MariaDB",
			query:       query,
			expectedDDL: "ALTER TABLE `orders` ADD INDEX `idx_orders_user_id_created_at_amount` (`user_id`, `created_at`, `amount`);",
		},
		{
			name:        "PostgreSQL 14 modern DDL with IF NOT EXISTS",
			dialect:     "postgres 14.9",
			query:       query,
			expectedDDL: "CREATE INDEX CONCURRENTLY IF NOT EXISTS \"idx_orders_user_id_created_at_amount\" ON \"orders\" (\"user_id\", \"created_at\", \"amount\");",
		},
		{
			name:        "PostgreSQL 9.4 legacy DDL without IF NOT EXISTS",
			dialect:     "postgres 9.4.26",
			query:       query,
			expectedDDL: "CREATE INDEX CONCURRENTLY \"idx_orders_user_id_created_at_amount\" ON \"orders\" (\"user_id\", \"created_at\", \"amount\");",
		},
		{
			name:        "MySQL TEXT column with prefix length limit (191)",
			dialect:     "mysql 8.0.35",
			query:       "SELECT * FROM orders WHERE notes = 'rush'",
			expectedDDL: "ALTER TABLE `orders` ADD INDEX `idx_orders_notes` (`notes`(191)), ALGORITHM=INPLACE, LOCK=NONE;",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Advise(tc.query, tc.dialect, tables)
			if len(res.Recommendations) == 0 {
				t.Fatalf("expected 1 recommendation, got 0")
			}
			if res.Recommendations[0].SafeDDL != tc.expectedDDL {
				t.Errorf("\nexpected: %s\ngot:      %s", tc.expectedDDL, res.Recommendations[0].SafeDDL)
			}
		})
	}
}

func TestAdvise_PostgresAndSQLiteDDL(t *testing.T) {
	query := "SELECT * FROM users WHERE status = 'active' AND age >= 18"
	tables := map[string]driver.TableMetadata{
		"users": {Name: "users"},
	}

	// Test PostgreSQL
	pgRes := Advise(query, "postgres", tables)
	if len(pgRes.Recommendations) == 0 {
		t.Fatalf("expected recommendation for postgres")
	}
	if !strings.HasPrefix(pgRes.Recommendations[0].SafeDDL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS") {
		t.Errorf("expected PG safe DDL with CONCURRENTLY, got: %s", pgRes.Recommendations[0].SafeDDL)
	}

	// Test SQLite
	sqliteRes := Advise(query, "sqlite", tables)
	if len(sqliteRes.Recommendations) == 0 {
		t.Fatalf("expected recommendation for sqlite")
	}
	if !strings.HasPrefix(sqliteRes.Recommendations[0].SafeDDL, "CREATE INDEX IF NOT EXISTS") {
		t.Errorf("expected SQLite safe DDL, got: %s", sqliteRes.Recommendations[0].SafeDDL)
	}
}

func TestAdvise_RedundancyAndPrefixConsolidation(t *testing.T) {
	query := "SELECT * FROM orders WHERE tenant_id = 1 AND status = 'COMPLETED'"
	tables := map[string]driver.TableMetadata{
		"orders": {
			Name: "orders",
			Indexes: []driver.IndexMetadata{
				// Existing index already has (tenant_id, status, created_at)
				{Name: "idx_orders_tenant_status_date", Columns: []string{"tenant_id", "status", "created_at"}},
			},
		},
	}

	res := Advise(query, "mysql", tables)
	if len(res.Recommendations) == 0 {
		t.Fatalf("expected recommendation")
	}
	rec := res.Recommendations[0]
	if !rec.IsRedundant {
		t.Errorf("expected recommendation to be flagged as redundant because existing index covers it")
	}

	// Now test prefix consolidation: existing index has only (tenant_id)
	tables2 := map[string]driver.TableMetadata{
		"orders": {
			Name: "orders",
			Indexes: []driver.IndexMetadata{
				{Name: "idx_orders_tenant", Columns: []string{"tenant_id"}},
			},
		},
	}
	res2 := Advise(query, "mysql", tables2)
	rec2 := res2.Recommendations[0]
	if rec2.IsRedundant {
		t.Errorf("expected recommendation NOT to be marked redundant")
	}
	if rec2.ExistingPrefixIdx != "idx_orders_tenant" {
		t.Errorf("expected ExistingPrefixIdx 'idx_orders_tenant', got %q", rec2.ExistingPrefixIdx)
	}
	if !strings.Contains(rec2.ReplaceSuggestion, "Consolidate") && !strings.Contains(rec2.ReplaceSuggestion, "consolidating") {
		t.Errorf("expected consolidation advice, got %q", rec2.ReplaceSuggestion)
	}
}

func TestAdvise_IndexBloatWarningWithCandidateKept(t *testing.T) {
	// Table with 6 existing indexes (Bloat threshold is > 5)
	query := "SELECT * FROM payments WHERE merchant_id = 99 AND status = 'SETTLED'"
	tables := map[string]driver.TableMetadata{
		"payments": {
			Name: "payments",
			Indexes: []driver.IndexMetadata{
				{Name: "idx_1", Columns: []string{"col1"}},
				{Name: "idx_2", Columns: []string{"col2"}},
				{Name: "idx_3", Columns: []string{"col3"}},
				{Name: "idx_4", Columns: []string{"col4"}},
				{Name: "idx_5", Columns: []string{"col5"}},
				{Name: "idx_6", Columns: []string{"col6"}},
			},
		},
	}

	res := Advise(query, "mysql", tables)

	// User rule: "jika index sudah kebanyakan di infokan juga secara jelas..tapi tetep menampilkan saran index"
	if len(res.BloatWarnings) == 0 {
		t.Fatalf("expected Index Bloat warning for table with 6 indexes")
	}
	bloat := res.BloatWarnings[0]
	if !bloat.IsBloated || !strings.Contains(bloat.Warning, "Index Bloat") {
		t.Errorf("expected explicit bloat warning, got: %s", bloat.Warning)
	}

	// Must STILL return the recommended index!
	if len(res.Recommendations) == 0 {
		t.Fatalf("expected candidate index recommendation to STILL be returned even with bloat warning")
	}
	if res.Recommendations[0].Table != "payments" {
		t.Errorf("expected recommendation for payments, got: %s", res.Recommendations[0].Table)
	}
}

func TestAdvise_MultiTableJoin(t *testing.T) {
	query := `
		SELECT o.id, c.name, o.total 
		FROM orders o 
		JOIN customers c ON o.customer_id = c.id 
		WHERE o.status = 'PENDING' AND c.tier = 'VIP' 
		ORDER BY o.created_at DESC
	`
	tables := map[string]driver.TableMetadata{
		"orders":    {Name: "orders"},
		"customers": {Name: "customers"},
	}

	res := Advise(query, "mysql", tables)
	if len(res.AnalyzedTables) < 2 {
		t.Errorf("expected at least 2 analyzed tables, got: %v", res.AnalyzedTables)
	}

	// Find recommendations for orders
	var ordersRec *Recommendation
	for i := range res.Recommendations {
		if res.Recommendations[i].Table == "orders" {
			ordersRec = &res.Recommendations[i]
		}
	}

	if ordersRec == nil {
		t.Fatalf("expected recommendation for orders")
	}
	// On orders: Equality (status) -> Sort (created_at)
	if !containsStr(ordersRec.Columns, "status") {
		t.Errorf("expected status column in orders index, got %v", ordersRec.Columns)
	}
}

func TestExecutionPlanNormalizer_PostgresAndSQLite(t *testing.T) {
	// 1. PostgreSQL JSON Execution Plan with Seq Scan and Sort
	pgPlanJSON := `[
		{
			"Plan": {
				"Node Type": "Sort",
				"Sort Key": ["created_at DESC"],
				"Plans": [
					{
						"Node Type": "Seq Scan",
						"Relation Name": "audit_logs",
						"Plan Rows": 45000
					}
				]
			}
		}
	]`

	pgPlan := &driver.ExplainPlan{
		RawJSON: pgPlanJSON,
	}

	pgHints := ParseExplainHints(pgPlan)
	auditHint, ok := pgHints["audit_logs"]
	if !ok {
		t.Fatalf("expected hint for audit_logs in postgres JSON plan")
	}
	if auditHint.AccessType != "ALL" {
		t.Errorf("expected AccessType ALL for Seq Scan, got %s", auditHint.AccessType)
	}
	if auditHint.RowsExamined != 45000 {
		t.Errorf("expected RowsExamined 45000, got %d", auditHint.RowsExamined)
	}
	if !auditHint.UsingFilesort {
		t.Errorf("expected UsingFilesort true due to Sort node")
	}

	// Test AdviseWithExplain with PG plan
	query := "SELECT * FROM audit_logs WHERE action = 'LOGIN' ORDER BY created_at DESC"
	tables := map[string]driver.TableMetadata{
		"audit_logs": {Name: "audit_logs"},
	}
	pgRes := AdviseWithExplain(query, "postgres", tables, pgPlan)
	if len(pgRes.Recommendations) == 0 {
		t.Fatalf("expected recommendation for audit_logs")
	}
	rec := pgRes.Recommendations[0]
	if !strings.Contains(rec.EstimatedBenefit, "Full Table Scan (ALL on ~45000 rows)") {
		t.Errorf("expected EstimatedBenefit to mention ~45000 rows, got: %s", rec.EstimatedBenefit)
	}
	if !strings.Contains(rec.EstimatedBenefit, "Eliminates memory filesort") {
		t.Errorf("expected EstimatedBenefit to mention Eliminates memory filesort, got: %s", rec.EstimatedBenefit)
	}

	// 2. SQLite EXPLAIN QUERY PLAN detail normalization
	sqlitePlan := &driver.ExplainPlan{
		Rows: []driver.ExplainRow{
			{
				ID:    1,
				Extra: "SCAN TABLE transactions",
			},
			{
				ID:    2,
				Extra: "USE TEMP B-TREE FOR ORDER BY",
			},
		},
	}

	sqliteHints := ParseExplainHints(sqlitePlan)
	txHint, ok := sqliteHints["transactions"]
	if !ok {
		t.Fatalf("expected hint for transactions in sqlite plan")
	}
	if txHint.AccessType != "ALL" {
		t.Errorf("expected AccessType ALL for SCAN TABLE, got %s", txHint.AccessType)
	}
	if !txHint.UsingFilesort {
		t.Errorf("expected UsingFilesort true for SQLite TEMP B-TREE")
	}
}
