package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/herliansyah/sqlfox/internal/ai"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/vault"
)

type AIChatRequest struct {
	Action          string      `json:"action"` // explain_query, explain_plan, optimize, nl_to_sql, chat, fix_query
	ProfileID       string      `json:"profile_id,omitempty"`
	Query           string      `json:"query"`
	Plan            string      `json:"plan"`
	Prompt          string      `json:"prompt"`
	ErrorMessage    string      `json:"error_message,omitempty"`
	ConnectionID    string      `json:"connection_id"`
	Provider        ai.Provider `json:"provider"`
	APIKey          string      `json:"api_key"`
	Model           string      `json:"model"`
	Endpoint        string      `json:"endpoint"`
	MaskIdentifiers *bool       `json:"mask_identifiers,omitempty"` // default true (secure-by-default)
	Language        string      `json:"language,omitempty"`         // "id" (Bahasa Indonesia), "en", "auto"
	ResponseStyle   string      `json:"response_style,omitempty"`   // "concise", "detailed", "normal"
	CustomPrompt    string      `json:"custom_prompt,omitempty"`    // custom persona / system instructions
	IncludeSchema   *bool       `json:"include_schema,omitempty"`   // default true
	Temperature     float32     `json:"temperature,omitempty"`
}

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	var req AIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if s.vault != nil {
		var profile *vault.AIProfile
		var err error
		if req.ProfileID != "" {
			profile, err = s.vault.GetAIProfile(req.ProfileID)
		} else if req.APIKey == "" || req.Provider == "" {
			profile, err = s.vault.GetActiveAIProfile()
		}
		if err == nil && profile != nil {
			if req.Provider == "" {
				req.Provider = ai.Provider(profile.Provider)
			}
			if req.APIKey == "" {
				req.APIKey = profile.APIKey
			}
			if req.Model == "" {
				req.Model = profile.ModelName
			}
			if req.Endpoint == "" {
				req.Endpoint = profile.BaseURL
			}
			if req.Temperature == 0 && profile.Temperature != 0 {
				req.Temperature = profile.Temperature
			}
			if req.Language == "" {
				req.Language = profile.Language
			}
			if req.ResponseStyle == "" {
				req.ResponseStyle = profile.ResponseStyle
			}
			if req.CustomPrompt == "" {
				req.CustomPrompt = profile.CustomPrompt
			}
			if req.IncludeSchema == nil {
				req.IncludeSchema = &profile.IncludeSchema
			}
			if req.MaskIdentifiers == nil {
				req.MaskIdentifiers = &profile.MaskIdentifiers
			}
		}
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
			if (sess.Schema() == nil || len(sess.Schema().Tables) == 0) && sess.Session != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				if sc, err := sess.Session.InspectSchema(ctx, ""); err == nil {
					sess.SetSchema(sc)
				}
				cancel()
			}
			schema = sess.Schema()

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
		MaskIdentifiers:  req.MaskIdentifiers == nil || *req.MaskIdentifiers,
		Language:         req.Language,
		ResponseStyle:    req.ResponseStyle,
		CustomPrompt:     req.CustomPrompt,
		IncludeSchema:    includeSchema,
		Temperature:      req.Temperature,
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

func (s *Server) handleListAIProfiles(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeJSON(w, http.StatusOK, []*vault.AIProfile{})
		return
	}
	list, err := s.vault.ListAIProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list AI profiles: "+err.Error())
		return
	}
	if list == nil {
		list = []*vault.AIProfile{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not initialized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "profile id is required")
		return
	}
	p, err := s.vault.GetAIProfile(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreateAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not initialized")
		return
	}
	var p vault.AIProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		writeError(w, http.StatusBadRequest, "profile name is required")
		return
	}
	if p.Provider == "" {
		p.Provider = "openai"
	}
	p.ModelName = strings.TrimSpace(p.ModelName)
	if p.ModelName == "" {
		writeError(w, http.StatusBadRequest, "model name is required")
		return
	}

	if err := s.vault.SaveAIProfile(&p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save AI profile: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdateAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not initialized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "profile id is required")
		return
	}
	var p vault.AIProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	p.ID = id
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		writeError(w, http.StatusBadRequest, "profile name is required")
		return
	}
	p.ModelName = strings.TrimSpace(p.ModelName)
	if p.ModelName == "" {
		writeError(w, http.StatusBadRequest, "model name is required")
		return
	}

	if err := s.vault.SaveAIProfile(&p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update AI profile: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not initialized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "profile id is required")
		return
	}
	if err := s.vault.DeleteAIProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

func (s *Server) handleSetActiveAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not initialized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "profile id is required")
		return
	}
	if err := s.vault.SetActiveAIProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "active_id": id})
}
