package ai

import (
	"strings"
	"testing"

	"sqlfox/internal/driver"
)

func TestAssistant_AssemblePrompts(t *testing.T) {

	schema := &driver.SchemaMetadata{
		Tables: []driver.TableMetadata{
			{
				Name: "orders",
				Columns: []driver.ColumnMetadata{
					{Name: "id", Type: "INTEGER", IsNullable: false},
					{Name: "user_id", Type: "INTEGER", IsNullable: false},
					{Name: "total_amount", Type: "REAL", IsNullable: false},
				},
			},
			{
				Name: "secrets",
				Columns: []driver.ColumnMetadata{
					{Name: "api_key", Type: "TEXT", IsNullable: false},
				},
			},
		},
	}

	notes := []TableNoteSummary{
		{
			TableName: "orders",
			Content:   "Orders must have total_amount >= 0. Deleted orders are archived.",
		},
		{
			TableName: "unrelated_table",
			Content:   "This note should not be in the prompt.",
		},
	}

	req := AssistRequest{
		Action:        "explain_query",
		Query:         "SELECT id, total_amount FROM orders WHERE user_id = 42;",
		Dialect:       "sqlite",
		Schema:        schema,
		TableNotes:    notes,
		IncludeSchema: true,
		Language:      "id",
		ResponseStyle: "concise",
		CustomPrompt:  "Always check indexes.",
	}

	systemPrompt, userPrompt := AssemblePrompts(req)

	// 1. Verify Dialect & Schema injection
	if !strings.Contains(systemPrompt, "sqlite") {
		t.Fatalf("expected sqlite dialect in system prompt, got: %s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "orders") {
		t.Fatalf("expected orders table schema in system prompt, got: %s", systemPrompt)
	}

	// 2. Verify Table Notes filtering
	if !strings.Contains(systemPrompt, "Orders must have total_amount >= 0") {
		t.Fatalf("expected relevant orders table note in system prompt, got: %s", systemPrompt)
	}
	if strings.Contains(systemPrompt, "unrelated_table") || strings.Contains(systemPrompt, "This note should not be in the prompt") {
		t.Fatalf("expected irrelevant table note to be excluded from system prompt, got: %s", systemPrompt)
	}

	// 3. Verify Indonesian Language instruction
	if !strings.Contains(systemPrompt, "Bahasa Indonesia") {
		t.Fatalf("expected Indonesian language instruction, got: %s", systemPrompt)
	}

	// 4. Verify Style instruction
	if !strings.Contains(systemPrompt, "Be extremely concise") {
		t.Fatalf("expected concise style instruction, got: %s", systemPrompt)
	}

	// 5. Verify Custom instruction
	if !strings.Contains(systemPrompt, "Always check indexes") {
		t.Fatalf("expected custom persona instruction, got: %s", systemPrompt)
	}

	// 6. Verify User Prompt action
	if !strings.Contains(userPrompt, "Explain the following SQL query") || !strings.Contains(userPrompt, req.Query) {
		t.Fatalf("expected explain query prompt, got: %s", userPrompt)
	}
}

func TestAssistant_ActionRouting(t *testing.T) {
	tests := []struct {
		action   string
		req      AssistRequest
		expected string
	}{
		{
			action:   "nl_to_sql",
			req:      AssistRequest{Action: "nl_to_sql", Prompt: "show me all active users"},
			expected: "Generate a syntactically correct, highly optimal SQL query",
		},
		{
			action:   "fix_query",
			req:      AssistRequest{Action: "fix_query", Query: "SELEC * FROM t", ErrorMessage: "syntax error near SELEC"},
			expected: "The following SQL query produced a database error",
		},
		{
			action:   "optimize",
			req:      AssistRequest{Action: "optimize", Query: "SELECT * FROM large", Plan: "SCAN TABLE"},
			expected: "Optimize the following query for performance",
		},
		{
			action:   "chat",
			req:      AssistRequest{Action: "chat", Prompt: "What is an index?"},
			expected: "What is an index?",
		},
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			_, userPrompt := AssemblePrompts(tc.req)
			if !strings.Contains(userPrompt, tc.expected) {
				t.Fatalf("expected user prompt to contain %q, got: %s", tc.expected, userPrompt)
			}
		})
	}
}
