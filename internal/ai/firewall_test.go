package ai

import (
	"fmt"
	"strings"
	"testing"

	"sqlfox/internal/driver"
)

func TestFirewall_BuildSchemaPrompt(t *testing.T) {
	schema := &driver.SchemaMetadata{
		DatabaseName: "ecommerce",
		Tables: []driver.TableMetadata{
			{
				Name:     "users",
				RowCount: 50000,
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INT", IsPrimaryKey: true},
					{Name: "email", Type: "VARCHAR(255)", IsNullable: false},
					{Name: "status", Type: "VARCHAR(50)"},
				},
				Indexes: []driver.IndexMetadata{
					{Name: "idx_email", Columns: []string{"email"}, IsUnique: true},
				},
			},
			{
				Name:     "orders",
				RowCount: 250000,
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "BIGINT", IsPrimaryKey: true},
					{Name: "user_id", Type: "INT"},
					{Name: "total", Type: "DECIMAL(10,2)"},
				},
			},
			{
				Name:     "sensitive_payroll",
				RowCount: 500,
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INT"},
					{Name: "salary", Type: "DECIMAL(10,2)"},
				},
			},
		},
	}

	firewall := NewFirewall(false)

	// Case 1: Query references only `users` and `orders`
	prompt := firewall.BuildSchemaPrompt(schema, "SELECT u.email, o.total FROM users u JOIN orders o ON u.id = o.user_id")

	if !strings.Contains(prompt, "Table `users`") {
		t.Fatal("expected `users` table in prompt")
	}
	if !strings.Contains(prompt, "Table `orders`") {
		t.Fatal("expected `orders` table in prompt")
	}

	// Case 2: Masking enabled
	maskingFirewall := NewFirewall(true)
	maskedPrompt := maskingFirewall.BuildSchemaPrompt(schema, "SELECT * FROM users")

	if strings.Contains(maskedPrompt, "`email`") {
		t.Fatal("expected identifiers to be masked, found `email`")
	}
	if !strings.Contains(maskedPrompt, "`masked_") {
		t.Fatal("expected masked_ token prefix")
	}
}

func TestAIClient_EndpointNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://192.168.1.50:8000", "http://192.168.1.50:8000/v1/chat/completions"},
		{"http://192.168.1.50:8000/v1", "http://192.168.1.50:8000/v1/chat/completions"},
		{"http://192.168.1.50:8000/v1/", "http://192.168.1.50:8000/v1/chat/completions"},
		{"http://192.168.1.50:8000/v1/chat/completions", "http://192.168.1.50:8000/v1/chat/completions"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
	}

	for _, tt := range tests {
		client := NewAIClient(ClientConfig{
			Provider: ProviderOpenAI,
			Endpoint: tt.input,
		})
		if client.cfg.Endpoint != tt.expected {
			t.Errorf("for input %q, expected endpoint %q, got %q", tt.input, tt.expected, client.cfg.Endpoint)
		}
	}
}

func TestFirewall_BuildSchemaPrompt_UnrelatedQueryInEditor(t *testing.T) {
	schema := &driver.SchemaMetadata{
		DatabaseName: "shop",
		Tables: []driver.TableMetadata{
			{
				Name: "users",
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INT", IsPrimaryKey: true},
					{Name: "email", Type: "VARCHAR(255)"},
				},
			},
			{
				Name: "orders",
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INT", IsPrimaryKey: true},
					{Name: "total_amount", Type: "DECIMAL(10,2)"},
				},
			},
			{
				Name: "products",
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INT", IsPrimaryKey: true},
					{Name: "sku", Type: "VARCHAR(50)"},
					{Name: "price", Type: "DECIMAL(10,2)"},
				},
			},
			{Name: "categories", Columns: []driver.ColumnMetadata{{Name: "id", Type: "INT"}}},
			{Name: "audit_logs", Columns: []driver.ColumnMetadata{{Name: "id", Type: "INT"}}},
			{Name: "settings", Columns: []driver.ColumnMetadata{{Name: "id", Type: "INT"}}},
		},
	}

	fw := NewFirewall(false)

	// User has an existing query in the editor (audit_logs), but asks AI a natural language question about products
	referencedInput := "SELECT * FROM audit_logs LIMIT 50; buatkan query menampilkan produk dan harganya"
	prompt := fw.BuildSchemaPrompt(schema, referencedInput)

	if !strings.Contains(prompt, "Table `products`") {
		t.Fatal("expected `products` table in prompt even when editor has unrelated query")
	}
	if !strings.Contains(prompt, "`sku`") || !strings.Contains(prompt, "`price`") {
		t.Fatal("expected `sku` and `price` columns in prompt")
	}
}

func TestFirewall_BuildSchemaPrompt_StemMatching(t *testing.T) {
	// Table is plural "users", prompt mentions singular "user"
	schema := &driver.SchemaMetadata{
		DatabaseName: "testdb",
		Tables: []driver.TableMetadata{
			{
				Name: "users",
				Columns: []driver.ColumnMetadata{
					{Name: "username", Type: "VARCHAR(50)"},
					{Name: "email", Type: "VARCHAR(100)"},
				},
			},
		},
	}
	fw := NewFirewall(false)
	prompt := fw.BuildSchemaPrompt(schema, "tampilkan data user aktif")
	if !strings.Contains(prompt, "`username`") || !strings.Contains(prompt, "`email`") {
		t.Fatal("expected users columns to be included via stem matching")
	}
}

func TestFirewall_BuildSchemaPrompt_LargeSchema(t *testing.T) {
	var tables []driver.TableMetadata
	for i := 1; i <= 40; i++ {
		tables = append(tables, driver.TableMetadata{
			Name: fmt.Sprintf("table_%02d", i),
			Columns: []driver.ColumnMetadata{
				{Name: "id", Type: "INT"},
				{Name: "val", Type: "VARCHAR(50)"},
			},
		})
	}
	schema := &driver.SchemaMetadata{Tables: tables}
	fw := NewFirewall(false)
	prompt := fw.BuildSchemaPrompt(schema, "SELECT * FROM table_35")

	// table_35 should be active and include columns
	if !strings.Contains(prompt, "Table `table_35`") {
		t.Fatal("expected table_35 to be active")
	}
	// Other tables should be listed in summary
	if !strings.Contains(prompt, "Other Tables in Database") {
		t.Fatal("expected summary list of other tables")
	}
}


