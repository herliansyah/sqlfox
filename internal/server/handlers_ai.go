package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"sqlfox/internal/ai"
)

type AIChatRequest struct {
	Action          string      `json:"action"` // explain_query, explain_plan, optimize, nl_to_sql, chat
	Query           string      `json:"query"`
	Plan            string      `json:"plan"`
	Prompt          string      `json:"prompt"`
	ConnectionID    string      `json:"connection_id"`
	Provider        ai.Provider `json:"provider"`
	APIKey          string      `json:"api_key"`
	Model           string      `json:"model"`
	Endpoint        string      `json:"endpoint"`
	MaskIdentifiers bool        `json:"mask_identifiers"`
}

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	var req AIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	dialect := "SQL"
	var schemaPrompt string

	if req.ConnectionID != "" {
		if sess, err := s.sessionMgr.GetSession(req.ConnectionID); err == nil {
			dialect = sess.DriverName
			firewall := ai.NewFirewall(req.MaskIdentifiers)
			schemaPrompt = firewall.BuildSchemaPrompt(sess.CachedSchema, req.Query+" "+req.Prompt)
		}
	}

	systemPrompt := ai.BaseSystemPrompt(dialect, schemaPrompt)
	var userPrompt string

	switch req.Action {
	case "explain_query":
		userPrompt = ai.BuildExplainQueryPrompt(req.Query)
	case "explain_plan":
		userPrompt = ai.BuildExplainPlanPrompt(req.Query, req.Plan)
	case "optimize":
		userPrompt = ai.BuildOptimizeQueryPrompt(req.Query, req.Plan)
	case "nl_to_sql":
		userPrompt = ai.BuildNLToSQLPrompt(req.Prompt)
	default:
		userPrompt = req.Prompt
	}

	if req.Provider == "" {
		req.Provider = ai.ProviderOllama
	}

	client := ai.NewAIClient(ai.ClientConfig{
		Provider: req.Provider,
		APIKey:   req.APIKey,
		Model:    req.Model,
		Endpoint: req.Endpoint,
	})

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := client.Complete(ctx, ai.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAIOllamaCheck(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Query().Get("endpoint")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	available := ai.DetectOllama(ctx, endpoint)
	writeJSON(w, http.StatusOK, map[string]bool{
		"available": available,
	})
}
