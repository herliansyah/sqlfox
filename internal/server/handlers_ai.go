package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"sqlfox/internal/ai"
	"sqlfox/internal/driver"
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
	Language        string      `json:"language,omitempty"`       // "id" (Bahasa Indonesia), "en", "auto"
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
	var schema *driver.SchemaMetadata
	var tableNotes []ai.TableNoteSummary

	includeSchema := req.IncludeSchema == nil || *req.IncludeSchema
	if includeSchema {
		sess, err := s.sessionMgr.GetActiveOrSingleSession(req.ConnectionID)
		if err == nil && sess != nil {
			dialect = sess.DriverName
			// ponytail: lazy schema inspection ensures prompt has column metadata even if initial connection inspection was incomplete
			if (sess.CachedSchema == nil || len(sess.CachedSchema.Tables) == 0) && sess.Session != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				if sc, err := sess.Session.InspectSchema(ctx, ""); err == nil {
					sess.CachedSchema = sc
				}
				cancel()
			}
			schema = sess.CachedSchema

			// Fetch table notes for the connection
			if notes, err := s.vault.ListTableNotesForConnection(sess.ConnectionID); err == nil && len(notes) > 0 {
				for _, n := range notes {
					tableNotes = append(tableNotes, ai.TableNoteSummary{
						TableName: n.TableName,
						Content:   n.Content,
					})
				}
			}
		}
	}

	var operationalNotes []ai.OperationalNoteSummary
	if s.vault != nil {
		allOpNotes, _ := s.vault.ListAllOperationalNotes()
		queryAndPrompt := strings.ToLower(req.Prompt + " " + req.Query)

		for _, on := range allOpNotes {
			// Check if specifically mentioned via @note:<id>, @note:<title>, [@note:...] or title mention
			isMentioned := strings.Contains(queryAndPrompt, "@note:"+strings.ToLower(on.ID)) ||
				strings.Contains(queryAndPrompt, strings.ToLower(on.Title))

			// Or if tagged to active table being queried
			isTagRelevant := false
			for _, tag := range on.ObjectTags {
				parts := strings.Split(tag, "/")
				if len(parts) > 0 {
					lastPart := strings.ToLower(parts[len(parts)-1])
					if lastPart != "" && strings.Contains(queryAndPrompt, lastPart) {
						isTagRelevant = true
						break
					}
				}
			}

			if isMentioned || isTagRelevant {
				operationalNotes = append(operationalNotes, ai.OperationalNoteSummary{
					ID:         on.ID,
					Title:      on.Title,
					Content:    on.Content,
					SideNote:   on.SideNote,
					ObjectTags: on.ObjectTags,
					UserTags:   on.UserTags,
				})
			}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := ai.Assist(ctx, ai.AssistRequest{
		Action:           req.Action,
		Query:            req.Query,
		Plan:             req.Plan,
		Prompt:           req.Prompt,
		ErrorMessage:     req.ErrorMessage,
		Dialect:          dialect,
		Schema:           schema,
		TableNotes:       tableNotes,
		OperationalNotes: operationalNotes,
		Provider:         req.Provider,
		APIKey:           req.APIKey,
		Model:            req.Model,
		Endpoint:         req.Endpoint,
		MaskIdentifiers:  req.MaskIdentifiers,
		Language:         req.Language,
		ResponseStyle:    req.ResponseStyle,
		CustomPrompt:     req.CustomPrompt,
		IncludeSchema:    includeSchema,
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
