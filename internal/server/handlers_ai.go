package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"sqlfox/internal/ai"
)

type AIChatRequest struct {
	Action          string      `json:"action"` // explain_query, explain_plan, optimize, nl_to_sql, chat, fix_query
	Query           string      `json:"query"`
	Plan            string      `json:"plan"`
	Prompt          string      `json:"prompt"`
	ErrorMessage    string      `json:"error_message,omitempty"`
	ConnectionID    string      `json:"connection_id"`
	Provider        ai.Provider `json:"provider"`
	APIKey          string      `json:"api_key"`
	Model           string      `json:"model"`
	Endpoint        string      `json:"endpoint"`
	MaskIdentifiers bool        `json:"mask_identifiers"`
	Language        string      `json:"language,omitempty"`       // "id", "en", "auto"
	ResponseStyle   string      `json:"response_style,omitempty"` // "concise", "detailed", "normal"
	CustomPrompt    string      `json:"custom_prompt,omitempty"`  // custom persona / system instructions
	IncludeSchema   *bool       `json:"include_schema,omitempty"` // default true
}

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	var req AIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	dialect := "SQL"
	var schemaPrompt string

	includeSchema := req.IncludeSchema == nil || *req.IncludeSchema
	if includeSchema {
		sess, err := s.sessionMgr.GetActiveOrSingleSession(req.ConnectionID)
		if err == nil && sess != nil {
			dialect = sess.DriverName
			// ponytail: lazy schema inspection ensures prompt has column metadata even if initial connection inspection was incomplete
			if (sess.CachedSchema == nil || len(sess.CachedSchema.Tables) == 0) && sess.Session != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				if schema, err := sess.Session.InspectSchema(ctx, ""); err == nil {
					sess.CachedSchema = schema
				}
				cancel()
			}
			firewall := ai.NewFirewall(req.MaskIdentifiers)
			schemaPrompt = firewall.BuildSchemaPrompt(sess.CachedSchema, req.Query+" "+req.Prompt+" "+req.ErrorMessage)
		}
	}

	systemPrompt := ai.BaseSystemPrompt(dialect, schemaPrompt)

	// Append language preferences
	if req.Language == "id" {
		systemPrompt += "\nLanguage Instruction:\nIMPORTANT: You MUST answer strictly in Bahasa Indonesia (Indonesian language) throughout your explanation, comments, and answers.\n"
	} else if req.Language == "en" {
		systemPrompt += "\nLanguage Instruction:\nIMPORTANT: You MUST answer in English.\n"
	}

	// Append response style preferences
	if req.ResponseStyle == "concise" {
		systemPrompt += "\nStyle Instruction:\nBe extremely concise. Directly give the SQL query and a brief 1-2 sentence explanation without any verbose introductions or conclusions.\n"
	} else if req.ResponseStyle == "detailed" {
		systemPrompt += "\nStyle Instruction:\nProvide an in-depth technical explanation, discussing query execution semantics, indexing implications, and performance optimizations.\n"
	}

	// Append user custom persona / system instructions
	if req.CustomPrompt != "" {
		systemPrompt += "\nCustom System Instructions:\n" + req.CustomPrompt + "\n"
	}

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
	case "fix_query":
		userPrompt = ai.BuildDebugErrorPrompt(req.Query, req.ErrorMessage)
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
