package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGemini    Provider = "gemini"
	ProviderOllama    Provider = "ollama"
)

type ClientConfig struct {
	Provider Provider `json:"provider"`
	APIKey   string   `json:"api_key"`
	Model    string   `json:"model"`
	Endpoint string   `json:"endpoint,omitempty"` // For Ollama or custom OpenAI-compatible proxies
}

type AIClient struct {
	cfg        ClientConfig
	httpClient *http.Client
}

func NewAIClient(cfg ClientConfig) *AIClient {
	if cfg.Endpoint == "" {
		switch cfg.Provider {
		case ProviderOpenAI:
			cfg.Endpoint = "https://api.openai.com/v1/chat/completions"
		case ProviderAnthropic:
			cfg.Endpoint = "https://api.anthropic.com/v1/messages"
		case ProviderGemini:
			cfg.Endpoint = "https://generativelanguage.googleapis.com/v1beta"
		case ProviderOllama:
			cfg.Endpoint = "http://localhost:11434/v1/chat/completions"
		}
	} else {
		// ponytail: normalize custom proxy endpoints (9router, LiteLLM, vLLM, etc.)
		ep := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
		if (cfg.Provider == ProviderOpenAI || cfg.Provider == ProviderOllama || cfg.Provider == "") &&
			!strings.HasSuffix(ep, "/chat/completions") && !strings.HasSuffix(ep, "/messages") {
			if strings.HasSuffix(ep, "/v1") {
				ep += "/chat/completions"
			} else {
				ep += "/v1/chat/completions"
			}
		}
		cfg.Endpoint = ep
	}

	if cfg.Model == "" {
		switch cfg.Provider {
		case ProviderOpenAI:
			cfg.Model = "gpt-4o-mini"
		case ProviderAnthropic:
			cfg.Model = "claude-3-5-sonnet-20241022"
		case ProviderGemini:
			cfg.Model = "gemini-1.5-flash"
		case ProviderOllama:
			cfg.Model = "llama3"
		}
	}

	return &AIClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// DetectOllama tests if an Ollama daemon is reachable locally.
func DetectOllama(ctx context.Context, endpoint string) bool {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	url := strings.TrimSuffix(endpoint, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	client := http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type CompletionRequest struct {
	SystemPrompt string  `json:"system_prompt"`
	UserPrompt   string  `json:"user_prompt"`
	Temperature  float32 `json:"temperature,omitempty"`
}

type CompletionResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

func (c *AIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	switch c.cfg.Provider {
	case ProviderOpenAI, ProviderOllama:
		return c.completeOpenAICompatible(ctx, req)
	case ProviderAnthropic:
		return c.completeAnthropic(ctx, req)
	case ProviderGemini:
		return c.completeGemini(ctx, req)
	default:
		return c.completeOpenAICompatible(ctx, req)
	}
}

func (c *AIClient) completeOpenAICompatible(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"stream": false,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.cfg.Endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("AI provider request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("AI provider returned error HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return parseOpenAIResponse(bodyBytes)
}

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"choices"`
	Model string `json:"model"`
}

func (c *openAIChunk) text() string {
	if len(c.Choices) == 0 {
		return ""
	}
	if c.Choices[0].Delta.Content != "" {
		return c.Choices[0].Delta.Content
	}
	if c.Choices[0].Message.Content != "" {
		return c.Choices[0].Message.Content
	}
	return c.Choices[0].Text
}

// parseOpenAIResponse handles standard JSON objects and streaming SSE chunks (e.g. 9router, LiteLLM)
func parseOpenAIResponse(bodyBytes []byte) (*CompletionResponse, error) {
	var single openAIChunk
	if err := json.Unmarshal(bodyBytes, &single); err == nil && single.text() != "" {
		return &CompletionResponse{Content: single.text(), Model: single.Model}, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(bodyBytes))
	var sb strings.Builder
	var lastModel string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasSuffix(line, "[DONE]") {
			continue
		}
		jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if !strings.HasPrefix(jsonStr, "{") {
			continue
		}

		var chunk openAIChunk
		if err := json.Unmarshal([]byte(jsonStr), &chunk); err == nil {
			if chunk.Model != "" {
				lastModel = chunk.Model
			}
			sb.WriteString(chunk.text())
		}
	}

	if sb.Len() > 0 {
		return &CompletionResponse{Content: sb.String(), Model: lastModel}, nil
	}

	return nil, fmt.Errorf("failed to parse AI response: no valid completion choices found in response (%s)", string(bodyBytes))
}

func (c *AIClient) completeAnthropic(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	payload := map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": 4096,
		"system":     req.SystemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": req.UserPrompt},
		},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.cfg.Endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Anthropic API error HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Model string `json:"model"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, err
	}

	if len(parsed.Content) == 0 {
		return nil, errors.New("Anthropic returned empty content")
	}

	return &CompletionResponse{
		Content: parsed.Content[0].Text,
		Model:   parsed.Model,
	}, nil
}

func (c *AIClient) completeGemini(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"), c.cfg.Model, url.QueryEscape(c.cfg.APIKey))

	payload := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{
				{"text": req.SystemPrompt},
			},
		},
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": req.UserPrompt},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Gemini API error HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, err
	}

	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("Gemini returned empty candidates")
	}

	return &CompletionResponse{
		Content: parsed.Candidates[0].Content.Parts[0].Text,
		Model:   c.cfg.Model,
	}, nil
}
