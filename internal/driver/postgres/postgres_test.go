package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"sqlfox/internal/driver"
)

func TestPostgresDriverRegistration(t *testing.T) {
	d, err := driver.GetDriver("postgres")
	if err != nil {
		t.Fatalf("expected postgres driver to be registered: %v", err)
	}
	if d.Name() != "postgres" {
		t.Errorf("expected driver name 'postgres', got %q", d.Name())
	}

	d2, err := driver.GetDriver("postgresql")
	if err != nil || d2.Name() != "postgres" {
		t.Errorf("expected postgresql alias to resolve to postgres: %v", err)
	}

	d3, err := driver.GetDriver("pg")
	if err != nil || d3.Name() != "postgres" {
		t.Errorf("expected pg alias to resolve to postgres: %v", err)
	}
}

func TestExtractColumnsFromIndexDef(t *testing.T) {
	tests := []struct {
		def      string
		expected []string
	}{
		{
			def:      "CREATE INDEX idx_users_email ON users USING btree (email)",
			expected: []string{"email"},
		},
		{
			def:      `CREATE UNIQUE INDEX idx_orders_cust_date ON orders USING btree ("customer_id", "order_date" DESC)`,
			expected: []string{"customer_id", `order_date" DESC`},
		},
		{
			def:      "invalid definition",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		cols := extractColumnsFromIndexDef(tt.def)
		if len(cols) != len(tt.expected) {
			t.Errorf("def %q: expected %d cols, got %d (%v)", tt.def, len(tt.expected), len(cols), cols)
		}
	}
}

func TestGeneratePostgresCreateDDL(t *testing.T) {
	tbl := driver.TableMetadata{
		Name: "users",
		Columns: []driver.ColumnMetadata{
			{Name: "id", Type: "bigint", IsNullable: false, IsPrimaryKey: true, DefaultValue: "nextval('users_id_seq')"},
			{Name: "email", Type: "character varying(255)", IsNullable: false, DefaultValue: ""},
			{Name: "role_id", Type: "integer", IsNullable: true, DefaultValue: "1"},
		},
		ForeignKeys: []driver.ForeignKeyMetadata{
			{
				Name:             "fk_users_role",
				Column:           "role_id",
				ReferencedTable:  "roles",
				ReferencedColumn: "id",
			},
		},
		Indexes: []driver.IndexMetadata{
			{
				Name:     "users_pkey",
				Columns:  []string{"id"},
				IsUnique: true,
			},
			{
				Name:     "idx_users_email",
				Columns:  []string{"email"},
				IsUnique: true,
			},
		},
	}

	ddl := generatePostgresCreateDDL("public", tbl)
	if !strings.Contains(ddl, `CREATE TABLE "public"."users"`) {
		t.Errorf("expected CREATE TABLE statement, got: %s", ddl)
	}
	if !strings.Contains(ddl, `PRIMARY KEY ("id")`) {
		t.Errorf("expected PRIMARY KEY constraint, got: %s", ddl)
	}
	if !strings.Contains(ddl, `CONSTRAINT "fk_users_role" FOREIGN KEY ("role_id") REFERENCES "roles" ("id")`) {
		t.Errorf("expected FOREIGN KEY constraint, got: %s", ddl)
	}
	if !strings.Contains(ddl, `CREATE UNIQUE INDEX "idx_users_email" ON "public"."users" ("email");`) {
		t.Errorf("expected secondary index creation, got: %s", ddl)
	}
	if strings.Contains(ddl, `CREATE UNIQUE INDEX "users_pkey"`) {
		t.Errorf("should skip creating duplicate pkey index")
	}
}

func TestPostgresConnectFailsGracefullyOnInvalidHost(t *testing.T) {
	d, err := driver.GetDriver("postgres")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err = d.Connect(ctx, driver.Config{
		Driver:       "postgres",
		Host:         "127.0.0.1",
		Port:         59999, // Non-existent port
		DatabaseName: "testdb",
		User:         "testuser",
		Password:     "testpass",
		UseSSL:       false,
	})

	if err == nil {
		t.Errorf("expected connection failure on invalid port, got nil")
	}
}

func TestPostgresRoutineSignatureParsing(t *testing.T) {
	cases := []struct {
		inputName string
		baseName  string
	}{
		{"calculate_tax(numeric, integer)", "calculate_tax"},
		{"get_user_summary()", "get_user_summary"},
		{"simple_proc", "simple_proc"},
	}

	for _, c := range cases {
		base := c.inputName
		if idx := strings.Index(c.inputName, "("); idx > 0 {
			base = strings.TrimSpace(c.inputName[:idx])
		}
		if base != c.baseName {
			t.Errorf("input %s: expected base %s, got %s", c.inputName, c.baseName, base)
		}
	}
}

func TestPostgresCancelQueryFallback(t *testing.T) {
	s := &PostgresSession{
		queryPids: map[string]int{
			"q-specific-456": 12345,
		},
	}
	ctx := context.Background()
	// Should gracefully resolve fallback pid and return nil when db is nil
	if err := s.CancelQuery(ctx, "active"); err != nil {
		t.Fatalf("unexpected CancelQuery error: %v", err)
	}
}

func TestParseBlockingPids(t *testing.T) {
	// Test nil
	if pids := parseBlockingPids(nil); len(pids) != 0 {
		t.Errorf("expected empty for nil, got %v", pids)
	}

	// Test []int32 (pgx int4[] mapping)
	pids32 := parseBlockingPids([]int32{12, 34})
	if len(pids32) != 2 || pids32[0] != 12 || pids32[1] != 34 {
		t.Errorf("expected [12 34], got %v", pids32)
	}

	// Test []int64
	pids64 := parseBlockingPids([]int64{56, 78})
	if len(pids64) != 2 || pids64[0] != 56 || pids64[1] != 78 {
		t.Errorf("expected [56 78], got %v", pids64)
	}

	// Test string representation "{101, 102}"
	pidsStr := parseBlockingPids("{101, 102}")
	if len(pidsStr) != 2 || pidsStr[0] != 101 || pidsStr[1] != 102 {
		t.Errorf("expected [101 102], got %v", pidsStr)
	}

	// Test empty string "{}"
	if pidsEmpty := parseBlockingPids("{}"); len(pidsEmpty) != 0 {
		t.Errorf("expected empty for '{}', got %v", pidsEmpty)
	}
}

func TestPostgresApplyUserGrants_PropagatesError(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://invalid:invalid@127.0.0.1:54329/invaliddb?sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	s := &PostgresSession{
		db:  db,
		cfg: driver.Config{DatabaseName: "testdb"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err = s.ApplyUserGrants(ctx, driver.UserGrantRequest{
		User:       "alice",
		Privileges: []string{"SELECT"},
	})
	if err == nil {
		t.Fatalf("expected error from ApplyUserGrants on unreachable db, got nil")
	}
}

func TestPostgresTx_CloseConn(t *testing.T) {
	tx := &postgresTx{}
	tx.closeConn() // must not panic when conn is nil
	if tx.conn != nil {
		t.Errorf("expected conn to be nil")
	}
}

func TestPostgresSession_BeginTx_FailsGracefully(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://invalid:invalid@127.0.0.1:54329/invaliddb?sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	db.Close()
	s := &PostgresSession{db: db}
	_, err = s.BeginTx(context.Background())
	if err == nil {
		t.Errorf("expected error on closed db Conn, got nil")
	}
}

func TestPostgresKillProcess_Validation(t *testing.T) {
	s := &PostgresSession{}
	ctx := context.Background()

	// 1. Thread ID validation
	if err := s.KillProcess(ctx, 0, true); err == nil || !strings.Contains(err.Error(), "invalid thread id") {
		t.Fatalf("expected invalid thread id error for 0, got %v", err)
	}
	if err := s.KillProcess(ctx, -10, false); err == nil || !strings.Contains(err.Error(), "invalid thread id") {
		t.Fatalf("expected invalid thread id error for negative id, got %v", err)
	}

	// 2. Closed database returns error
	db, err := sql.Open("pgx", "postgres://invalid:invalid@127.0.0.1:54329/invaliddb?sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	db.Close()
	s.db = db
	if err := s.KillProcess(ctx, 42, true); err == nil {
		t.Fatalf("expected error on closed db, got nil")
	}
}
