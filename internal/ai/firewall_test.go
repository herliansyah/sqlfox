package ai

import (
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
