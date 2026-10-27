package mock

import (
	"context"
	"math/rand"
	"path/filepath"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/sqlite"
)

// Self-check: covers generateValue type dispatch and basic topology helpers.

func TestGenerateValueTypes(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	seq := 0

	cases := []struct {
		col  driver.ColumnMetadata
		desc string
		want func(any) bool
	}{
		{driver.ColumnMetadata{Name: "active", Type: "tinyint(1)"}, "bool", func(v any) bool { n, ok := v.(int); return ok && (n == 0 || n == 1) }},
		{driver.ColumnMetadata{Name: "age", Type: "smallint"}, "smallint", func(v any) bool { n, ok := v.(int); return ok && n >= 1 }},
		{driver.ColumnMetadata{Name: "big", Type: "bigint"}, "bigint", func(v any) bool { _, ok := v.(int64); return ok }},
		{driver.ColumnMetadata{Name: "id", Type: "int", IsPrimaryKey: true}, "int pk", func(v any) bool { n, ok := v.(int); return ok && n > 0 }},
		{driver.ColumnMetadata{Name: "price", Type: "decimal(10,2)"}, "decimal", func(v any) bool { s, ok := v.(string); return ok && len(s) > 0 }},
		{driver.ColumnMetadata{Name: "uid", Type: "uuid"}, "uuid", func(v any) bool { s, ok := v.(string); return ok && len(s) == 36 }},
		{driver.ColumnMetadata{Name: "name", Type: "varchar(255)"}, "varchar", func(v any) bool { s, ok := v.(string); return ok && len(s) > 0 }},
		{driver.ColumnMetadata{Name: "note", Type: "text"}, "text", func(v any) bool { _, ok := v.(string); return ok }},
		{driver.ColumnMetadata{Name: "dob", Type: "date"}, "date", func(v any) bool { s, ok := v.(string); return ok && len(s) == 10 }},
		{driver.ColumnMetadata{Name: "ts", Type: "timestamp"}, "timestamp", func(v any) bool { _, ok := v.(string); return ok }},
		{driver.ColumnMetadata{Name: "tsz", Type: "timestamptz"}, "timestamptz", func(v any) bool { s, ok := v.(string); return ok && s[len(s)-1] == 'Z' }},
		{driver.ColumnMetadata{Name: "meta", Type: "json"}, "json", func(v any) bool { s, ok := v.(string); return ok && s[0] == '{' }},
		{driver.ColumnMetadata{Name: "data", Type: "blob"}, "blob", func(v any) bool { s, ok := v.(string); return ok && s == "0xDEADBEEF" }},
		{driver.ColumnMetadata{Name: "status", Type: "enum('active','inactive','pending')"}, "enum", func(v any) bool {
			s, ok := v.(string)
			return ok && (s == "active" || s == "inactive" || s == "pending")
		}},
		{driver.ColumnMetadata{Name: "weird", Type: "custom_type"}, "default", func(v any) bool { s, ok := v.(string); return ok && len(s) > 0 }},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			v := generateValue(tc.col, nil, nil, &seq, r)
			if !tc.want(v) {
				t.Errorf("generateValue(%q) = %v (%T), want passing predicate", tc.col.Type, v, v)
			}
		})
	}
}

func TestGenerateValueSelfRef(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	seq := 0
	col := driver.ColumnMetadata{Name: "manager_id", Type: "int"}
	isSelfRef := map[string]bool{"manager_id": true}
	v := generateValue(col, nil, isSelfRef, &seq, r)
	if v != nil {
		t.Errorf("self-ref column should return nil, got %v", v)
	}
}

func TestGenerateValueFKPool(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	seq := 0
	col := driver.ColumnMetadata{Name: "category_id", Type: "int"}
	pool := map[string][]any{"category_id": {1, 2, 3}}
	v := generateValue(col, pool, nil, &seq, r)
	found := false
	for _, p := range pool["category_id"] {
		if v == p {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FK pool value %v not in pool %v", v, pool["category_id"])
	}
}

func TestRandUUID(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	u := randUUID(r)
	if len(u) != 36 {
		t.Errorf("UUID length = %d, want 36; got %q", len(u), u)
	}
	// Must have hyphens at positions 8, 13, 18, 23.
	for _, pos := range []int{8, 13, 18, 23} {
		if u[pos] != '-' {
			t.Errorf("UUID missing hyphen at position %d: %q", pos, u)
		}
	}
}

// TestTopoSort validates that a two-table dependency (child → parent) produces
// the right insertion order when topological sorting is applied.
// ponytail: Inline Kahn's rather than shipping the function; tests the concept, not the unexported impl.
func TestTopoSort(t *testing.T) {
	// Simulate: "orders" has FK → "customers".
	// Kahn's: in-degree of customers = 0, orders = 1.
	// Expected order: customers first, then orders.
	deps := map[string][]string{
		"orders":    {"customers"},
		"customers": {},
	}
	order := kahnSort(deps)
	if len(order) != 2 {
		t.Fatalf("expected 2 tables, got %d: %v", len(order), order)
	}
	custIdx, ordIdx := -1, -1
	for i, n := range order {
		if n == "customers" {
			custIdx = i
		}
		if n == "orders" {
			ordIdx = i
		}
	}
	if custIdx >= ordIdx {
		t.Errorf("customers (%d) should come before orders (%d) in sort", custIdx, ordIdx)
	}
}

// kahnSort is a minimal Kahn's algorithm for test verification.
func kahnSort(deps map[string][]string) []string {
	inDegree := map[string]int{}
	for node := range deps {
		if _, ok := inDegree[node]; !ok {
			inDegree[node] = 0
		}
		for _, dep := range deps[node] {
			inDegree[dep] = inDegree[dep] // ensure exists
			inDegree[node]++              // node depends on dep → dep has lower in-degree
		}
	}
	// Re-compute correctly: in-degree = number of OTHER nodes that list this as dependency.
	inDegree = map[string]int{}
	for node := range deps {
		inDegree[node] = 0
	}
	for node, depList := range deps {
		for _, dep := range depList {
			_ = node
			inDegree[dep] = inDegree[dep] // dep has no additional incoming
		}
		inDegree[node] += len(depList)
	}

	var queue, result []string
	for node, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, node)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		result = append(result, n)
		for other, depList := range deps {
			for _, dep := range depList {
				if dep == n {
					inDegree[other]--
					if inDegree[other] == 0 {
						queue = append(queue, other)
					}
				}
			}
		}
	}
	return result
}

func TestGenerate_RepeatedRunsDoNotCollideOnPK(t *testing.T) {
	ctx := context.Background()
	sess, err := driver.Connect(ctx, driver.Config{Driver: "sqlite", DatabaseName: filepath.Join(t.TempDir(), "m.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := sess.ExecRaw(ctx, "CREATE TABLE items (id BIGINT PRIMARY KEY, name VARCHAR(40) NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		if _, err := Generate(ctx, sess, Config{DriverName: "sqlite", Table: "items", Rows: 3000}); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
	}
	rows, err := sess.QueryRaw(ctx, "SELECT COUNT(*), COUNT(DISTINCT id) FROM items")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][0] != int64(6000) || rows[0][1] != int64(6000) {
		t.Fatalf("expected 6000 distinct ids, got %v", rows[0])
	}
}
