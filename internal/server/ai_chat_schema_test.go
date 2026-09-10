package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sqlfox/internal/vault"
)

func TestServer_AIChat_IncludesRealColumnsWhenFirewallOff(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-ai-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "config.db")
	targetDB := filepath.Join(tempDir, "target.db")

	v, err := vault.OpenVault(vaultDB, "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "test-master")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// 1. Create a Connection
	createPayload := vault.ConnectionRecord{
		Name:         "Shop SQLite",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: targetDB,
	}
	body, _ := json.Marshal(createPayload)

	client := ts.Client()
	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/connections failed: %v, status: %d", err, resp.StatusCode)
	}
	var savedConn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&savedConn)
	resp.Body.Close()

	// 2. Connect Session
	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, savedConn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/session/connect failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Create tables: users, orders, products
	createSQL := map[string]any{
		"sql": `
			CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT NOT NULL, email TEXT NOT NULL);
			CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, total_amount REAL);
			CREATE TABLE products (id INTEGER PRIMARY KEY, title TEXT, price REAL);
		`,
	}
	sqlBody, _ := json.Marshal(createSQL)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, savedConn.ID), "application/json", bytes.NewReader(sqlBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Execute CREATE TABLE failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Mock AI Provider to intercept and inspect SystemPrompt
	var capturedSystemPrompt string
	var mu sync.Mutex

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBytes, _ := io.ReadAll(r.Body)
		var aiReq struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(reqBytes, &aiReq)

		mu.Lock()
		for _, msg := range aiReq.Messages {
			if msg.Role == "system" {
				capturedSystemPrompt = msg.Content
			}
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "```sql\nSELECT id, username, email FROM users;\n```",
					},
				},
			},
		})
	}))
	defer mockAI.Close()

	// 5. Invoke /api/ai/chat with Privacy Firewall OFF, include_schema: true, and Indonesian prompt
	includeSchema := true
	aiChatPayload := AIChatRequest{
		Action:          "chat",
		Prompt:          "buatkan query untuk menampilkan data pengguna",
		Query:           "SELECT * FROM products LIMIT 50;", // Unrelated table in editor
		ConnectionID:    savedConn.ID,
		Provider:        "openai",
		APIKey:          "test-key",
		Model:           "gpt-4o-mini",
		Endpoint:        mockAI.URL,
		MaskIdentifiers: false,
		IncludeSchema:   &includeSchema,
	}
	aiBody, _ := json.Marshal(aiChatPayload)

	resp, err = client.Post(ts.URL+"/api/ai/chat", "application/json", bytes.NewReader(aiBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/ai/chat failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Verify the AI prompt contains real table and column names
	mu.Lock()
	sysPrompt := capturedSystemPrompt
	mu.Unlock()

	if !strings.Contains(sysPrompt, "Table `users`") {
		t.Fatalf("expected `users` table in system prompt, got:\n%s", sysPrompt)
	}
	if !strings.Contains(sysPrompt, "`username`") || !strings.Contains(sysPrompt, "`email`") {
		t.Fatalf("expected real column names `username` and `email` in system prompt, got:\n%s", sysPrompt)
	}
	if !strings.Contains(sysPrompt, "`total_amount`") {
		t.Fatalf("expected `orders` column `total_amount` in system prompt, got:\n%s", sysPrompt)
	}

	// 6. Test with empty ConnectionID (verifying fallback to single active session)
	capturedSystemPrompt = ""
	aiChatPayload.ConnectionID = ""
	aiBodyEmptyConn, _ := json.Marshal(aiChatPayload)
	resp, err = client.Post(ts.URL+"/api/ai/chat", "application/json", bytes.NewReader(aiBodyEmptyConn))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/ai/chat with empty ConnectionID failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	mu.Lock()
	sysPromptFallback := capturedSystemPrompt
	mu.Unlock()

	if !strings.Contains(sysPromptFallback, "Table `users`") || !strings.Contains(sysPromptFallback, "`username`") {
		t.Fatalf("expected active session fallback to include users schema, got:\n%s", sysPromptFallback)
	}
}
