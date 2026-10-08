package ai

import (
	"context"
	"fmt"
	"strings"

	"sqlfox/internal/driver"
)

// TableNoteSummary carries table documentation for context window injection.
type TableNoteSummary struct {
	TableName string `json:"table_name"`
	Content   string `json:"content"`
}

// OperationalNoteSummary carries an operational note for context window injection.
type OperationalNoteSummary struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	SideNote   string   `json:"side_note,omitempty"`
	ObjectTags []string `json:"object_tags,omitempty"`
	UserTags   []string `json:"user_tags,omitempty"`
}

// AssistRequest defines all parameters for Copilot Assistant reasoning.
type AssistRequest struct {
	Action           string                   `json:"action"` // explain_query, explain_plan, optimize, nl_to_sql, chat, fix_query
	Query            string                   `json:"query"`
	Plan             string                   `json:"plan"`
	Prompt           string                   `json:"prompt"`
	ErrorMessage     string                   `json:"error_message,omitempty"`
	Dialect          string                   `json:"dialect,omitempty"`
	Schema           *driver.SchemaMetadata   `json:"schema,omitempty"`
	TableNotes       []TableNoteSummary       `json:"table_notes,omitempty"`
	OperationalNotes []OperationalNoteSummary `json:"operational_notes,omitempty"`
	Provider         Provider                 `json:"provider"`
	APIKey           string                   `json:"api_key"`
	Model            string                   `json:"model"`
	Endpoint         string                   `json:"endpoint"`
	MaskIdentifiers  bool                     `json:"mask_identifiers"`
	Language         string                   `json:"language,omitempty"`       // "id", "en", "auto"
	ResponseStyle    string                   `json:"response_style,omitempty"` // "concise", "detailed", "normal"
	CustomPrompt     string                   `json:"custom_prompt,omitempty"`  // custom persona / system instructions
	IncludeSchema    bool                     `json:"include_schema"`
}


// AssemblePrompts compiles the system prompt (incorporating Schema Context Window,
// Table Knowledge Notes, AI Persona, and Firewall masking) and user prompt.
func AssemblePrompts(req AssistRequest) (systemPrompt string, userPrompt string) {
	dialect := req.Dialect
	if dialect == "" {
		dialect = "SQL"
	}

	var schemaPrompt string
	if req.IncludeSchema && req.Schema != nil {
		schemaPrompt = BuildSchemaPrompt(req.Schema, req.Query+" "+req.Prompt+" "+req.ErrorMessage, req.MaskIdentifiers)

		// Table Notes Injection: check if any referenced tables have table notes
		if len(req.TableNotes) > 0 {
			queryAndPrompt := strings.ToLower(req.Query + " " + req.Prompt + " " + req.ErrorMessage)
			var relevantNotes []string
			for _, n := range req.TableNotes {
				if n.Content != "" && (queryAndPrompt == "  " || strings.Contains(queryAndPrompt, strings.ToLower(n.TableName))) {
					relevantNotes = append(relevantNotes, fmt.Sprintf("Table `%s` Notes & Business Rules:\n%s", n.TableName, n.Content))
				}
			}
			if len(relevantNotes) > 0 {
				schemaPrompt += "\n\n=== RELEVANT TABLE NOTES & BUSINESS RULES ===\n" + strings.Join(relevantNotes, "\n\n")
			}
		}
	}

	systemPrompt = BaseSystemPrompt(dialect, schemaPrompt)

	// Operational Notes Injection: check if any notes are referenced or mentioned
	if len(req.OperationalNotes) > 0 {
		var noteEntries []string
		for _, on := range req.OperationalNotes {
			entry := fmt.Sprintf("### Operational Note: %s\n- **Captured Content / Snapshot:**\n%s", on.Title, on.Content)
			if on.SideNote != "" {
				entry += fmt.Sprintf("\n- **Developer Notes & Action Items:**\n%s", on.SideNote)
			}
			if len(on.ObjectTags) > 0 {
				entry += fmt.Sprintf("\n- **Tagged Database Objects:** %s", strings.Join(on.ObjectTags, ", "))
			}
			noteEntries = append(noteEntries, entry)
		}
		if len(noteEntries) > 0 {
			systemPrompt += "\n\n=== REFERENCED OPERATIONAL NOTES ===\n" + strings.Join(noteEntries, "\n\n")
		}
	}

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

	// Append custom persona instructions
	if req.CustomPrompt != "" {
		systemPrompt += "\nCustom System Instructions:\n" + req.CustomPrompt + "\n"
	}

	// User prompt routing
	switch req.Action {
	case "explain_query":
		userPrompt = BuildExplainQueryPrompt(req.Query)
	case "explain_plan":
		userPrompt = BuildExplainPlanPrompt(req.Query, req.Plan)
	case "optimize":
		userPrompt = BuildOptimizeQueryPrompt(req.Query, req.Plan)
	case "nl_to_sql":
		userPrompt = BuildNLToSQLPrompt(req.Prompt)
	case "fix_query":
		userPrompt = BuildDebugErrorPrompt(req.Query, req.ErrorMessage)
	default:
		userPrompt = req.Prompt
	}

	return systemPrompt, userPrompt
}

// Assist orchestrates prompt compilation and execution through the appropriate AI provider.
func Assist(ctx context.Context, req AssistRequest) (*CompletionResponse, error) {
	systemPrompt, userPrompt := AssemblePrompts(req)

	provider := req.Provider
	if provider == "" {
		provider = ProviderOllama
	}

	client := NewAIClient(ClientConfig{
		Provider: provider,
		APIKey:   req.APIKey,
		Model:    req.Model,
		Endpoint: req.Endpoint,
	})

	return client.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
}
