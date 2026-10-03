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

	// MySQL 5.7 Safe DDL check
	expectedDDL := "ALTER TABLE `orders` ADD INDEX `idx_orders_user_id_created_at_amount` (`user_id`, `created_at`, `amount`) ALGORITHM=INPLACE, LOCK=NONE;"
	if rec.SafeDDL != expectedDDL {
		t.Errorf("expected Safe DDL %q, got %q", expectedDDL, rec.SafeDDL)
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
