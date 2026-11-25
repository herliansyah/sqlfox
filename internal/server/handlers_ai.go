package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/herliansyah/sqlfox/internal/ai"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/vault"
)

var cgnatRange = func() *net.IPNet {
	_, cidr, _ := net.ParseCIDR("100.64.0.0/10")
	return cidr
}()

var nat64Range = func() *net.IPNet {
	_, cidr, _ := net.ParseCIDR("64:ff9b::/96")
	return cidr
}()

var awsIPv6IMDS = net.ParseIP("fd00:ec2::254")

// validateAIEndpoint enforces SSRF boundaries on user-supplied AI endpoints.
// Cloud metadata IPs/hosts and CGNAT (100.64.0.0/10) are always blocked; private/loopback
// IPs are blocked unless the provider is local Ollama and the request originates from localhost.
func validateAIEndpoint(raw string, provider ai.Provider, isRemote bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid AI endpoint URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("AI endpoint must use http or https scheme")
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(u.Hostname())), ".")
	if host == "" {
		return errors.New("AI endpoint host is required")
	}
	if host == "metadata.google.internal" || host == "169.254.169.254" || host == "100.100.100.200" {
		return errors.New("AI endpoint targets a blocked metadata host")
	}

	allowLoopback := !isRemote
	allowPrivate := provider == ai.ProviderOllama && !isRemote
	checkIP := func(ip net.IP) error {
		if ip == nil {
			return nil
		}
		if ip4 := ip.To4(); ip4 != nil {
			ip = ip4
			if ip[0] == 0 {
				return errors.New("AI endpoint targets a blocked 0.0.0.0/8 IP")
			}
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return errors.New("AI endpoint targets a blocked link-local, multicast, or unspecified IP")
		}
		if cgnatRange != nil && cgnatRange.Contains(ip) {
			return errors.New("AI endpoint targets a blocked CGNAT/metadata IP range")
		}
		if nat64Range != nil && nat64Range.Contains(ip) {
			return errors.New("AI endpoint targets a blocked NAT64 IP range")
		}
		if awsIPv6IMDS != nil && ip.Equal(awsIPv6IMDS) {
			return errors.New("AI endpoint targets a blocked cloud metadata IP")
		}
		if ip.IsLoopback() && !allowLoopback {
			return errors.New("AI endpoint targets a loopback IP")
		}
		if ip.IsPrivate() && !allowPrivate {
			return errors.New("AI endpoint targets a private IP")
		}
		return nil
	}

	if ip := net.ParseIP(host); ip != nil {
		if err := checkIP(ip); err != nil {
			return err
		}
		if u.Scheme == "http" && provider != ai.ProviderOllama && !ip.IsLoopback() {
			return errors.New("cloud AI endpoints must use https")
		}
		return nil
	}
	if !allowLoopback && (host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local")) {
		return errors.New("AI endpoint targets a local hostname")
	}
	if u.Scheme == "http" && provider != ai.ProviderOllama && host != "localhost" {
		return errors.New("cloud AI endpoints must use https")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// Fail closed when DNS resolution fails (unless running offline unit tests with loopback).
		return fmt.Errorf("AI endpoint host resolution failed: %w", err)
	}
	for _, ip := range ips {
		if err := checkIP(ip); err != nil {
			return err
		}
	}
	return nil
}

// sanitizeAIProfileForResponse strips secret key material before sending an AI profile over HTTP.
func sanitizeAIProfileForResponse(p *vault.AIProfile) *vault.AIProfile {
	if p == nil {
		return nil
	}
	cp := *p
	cp.HasAPIKey = cp.HasAPIKey || cp.APIKey != "" || cp.EncryptedAPIKey != ""
	cp.APIKey = ""
	cp.EncryptedAPIKey = ""
	return &cp
}

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
			// Prevent stored API key exfiltration: if the request relies on the vault's stored APIKey,
			// reject any caller-supplied endpoint override that differs from the saved profile's BaseURL.
			if req.APIKey == "" && profile.APIKey != "" && req.Endpoint != "" &&
				!strings.EqualFold(strings.TrimRight(strings.TrimSpace(req.Endpoint), "/"), strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")) {
				writeError(w, http.StatusBadRequest, "cannot override endpoint when using a stored AI profile API key")
				return
			}
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

	if err := validateAIEndpoint(req.Endpoint, req.Provider, s.IsRemoteRequest(r)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	dialect := "SQL"
	var schema *driver.SchemaMetadata
	var tableNotes []ai.TableNoteSummary

	includeSchema := req.IncludeSchema == nil || *req.IncludeSchema
	maskIdentifiers := req.MaskIdentifiers == nil || *req.MaskIdentifiers
	if includeSchema || maskIdentifiers {
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

			// Fetch table notes for the connection (only when schema is included)
			if includeSchema {
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
	}

	var operationalNotes []ai.OperationalNoteSummary
	if s.vault != nil {
		allOpNotes, _ := s.vault.ListAllOperationalNotes()
		queryAndPrompt := strings.ToLower(req.Prompt + " " + req.Query)

		for _, on := range allOpNotes {
			// Check if specifically mentioned via @note:<id>, or a non-trivial title (>= 3 chars).
			titleLower := strings.ToLower(strings.TrimSpace(on.Title))
			isMentioned := strings.Contains(queryAndPrompt, "@note:"+strings.ToLower(on.ID)) ||
				(len(titleLower) >= 3 && strings.Contains(queryAndPrompt, titleLower))

			// Or if tagged to active table being queried
			isTagRelevant := false
			for _, tag := range on.ObjectTags {
				parts := strings.Split(tag, "/")
				if len(parts) > 0 {
					lastPart := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
					if len(lastPart) >= 2 && strings.Contains(queryAndPrompt, lastPart) {
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
		MaskIdentifiers:  maskIdentifiers,
		Language:         req.Language,
		ResponseStyle:    req.ResponseStyle,
		CustomPrompt:     req.CustomPrompt,
		IncludeSchema:    includeSchema,
		Temperature:      req.Temperature,
		RemoteRequest:    s.IsRemoteRequest(r),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAIOllamaCheck(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Query().Get("endpoint")
	isRemote := s.IsRemoteRequest(r)
	if err := validateAIEndpoint(endpoint, ai.ProviderOllama, isRemote); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	available := ai.DetectOllama(ctx, endpoint, isRemote)
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
	sanitized := make([]*vault.AIProfile, len(list))
	for i, p := range list {
		sanitized[i] = sanitizeAIProfileForResponse(p)
	}
	writeJSON(w, http.StatusOK, sanitized)
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
	writeJSON(w, http.StatusOK, sanitizeAIProfileForResponse(p))
}

func (s *Server) handleCreateAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not initialized")
		return
	}
	var dto struct {
		vault.AIProfile
		IncludeSchema   *bool `json:"include_schema,omitempty"`
		MaskIdentifiers *bool `json:"mask_identifiers,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	p := dto.AIProfile
	// Secure-by-default: new profiles enable identifier masking and schema context unless explicitly disabled.
	p.IncludeSchema = dto.IncludeSchema == nil || *dto.IncludeSchema
	p.MaskIdentifiers = dto.MaskIdentifiers == nil || *dto.MaskIdentifiers

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

	if err := validateAIEndpoint(p.BaseURL, ai.Provider(p.Provider), s.IsRemoteRequest(r)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.vault.SaveAIProfile(&p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save AI profile: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, sanitizeAIProfileForResponse(&p))
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

	if err := validateAIEndpoint(p.BaseURL, ai.Provider(p.Provider), s.IsRemoteRequest(r)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Prevent stored API key exfiltration: if BaseURL or Provider changed and no new APIKey is provided,
	// clear any existing encrypted key so it cannot be redirected to a new endpoint.
	if p.APIKey == "" {
		if existing, err := s.vault.GetAIProfile(id); err == nil && existing != nil {
			if !strings.EqualFold(strings.TrimRight(strings.TrimSpace(p.BaseURL), "/"), strings.TrimRight(strings.TrimSpace(existing.BaseURL), "/")) ||
				!strings.EqualFold(strings.TrimSpace(p.Provider), strings.TrimSpace(existing.Provider)) {
				p.ClearAPIKey = true
				p.EncryptedAPIKey = ""
				p.HasAPIKey = false
			} else {
				p.EncryptedAPIKey = existing.EncryptedAPIKey
			}
		}
	}

	if err := s.vault.SaveAIProfile(&p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update AI profile: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, sanitizeAIProfileForResponse(&p))
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
