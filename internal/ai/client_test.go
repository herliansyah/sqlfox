package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteOpenAI_StreamingResponseFromProxy(t *testing.T) {
	// Simulate 9router / proxy returning SSE stream with multiple chunks
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var reqBody map[string]any
		_ = json.Unmarshal(bodyBytes, &reqBody)

		// Check if stream was explicitly set to false
		if reqBody["stream"] != false {
			t.Errorf("expected stream: false in payload, got %v", reqBody["stream"])
		}

		// Simulate proxy returning SSE chunks that produced the user's exact error
		// User error: invalid character 'd' after top-level value
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("{\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\ndata: {\"id\":\"chatcmpl-2\",\"choices\":[{\"delta\":{\"content\":\" from 9router\"}}]}\ndata: [DONE]\n"))
	}))
	defer ts.Close()

	client := NewAIClient(ClientConfig{
		Provider: ProviderOpenAI,
		Endpoint: ts.URL,
		Model:    "test-model",
	})

	resp, err := client.Complete(context.Background(), CompletionRequest{
		UserPrompt: "test",
	})
	if err != nil {
		t.Fatalf("❌ AI Proxy test failed: %v", err)
	}

	if !strings.Contains(resp.Content, "hello") || !strings.Contains(resp.Content, "from 9router") {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
}

func TestCompleteOpenAI_StandardSSEStream(t *testing.T) {
	// Simulate pure standard SSE stream starting with data:
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" 9router\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer ts.Close()

	client := NewAIClient(ClientConfig{
		Provider: ProviderOpenAI,
		Endpoint: ts.URL,
		Model:    "test-model",
	})

	resp, err := client.Complete(context.Background(), CompletionRequest{
		UserPrompt: "test",
	})
	if err != nil {
		t.Fatalf("❌ AI Proxy test failed: %v", err)
	}

	if resp.Content != "hello 9router" {
		t.Fatalf("expected 'hello 9router', got %q", resp.Content)
	}
}
