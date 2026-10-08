package postgres

import (
	"context"
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

