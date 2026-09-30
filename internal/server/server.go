package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"sqlfox/internal/safety"
	"sqlfox/internal/vault"

	_ "sqlfox/internal/driver/mysql"
	_ "sqlfox/internal/driver/sqlite"
)

// Server coordinates HTTP API routing and embedded web frontend delivery.
type Server struct {
	handler        http.Handler
	vault          *vault.Vault
	guard          *safety.SafetyGuard
	backupMgr      *safety.BackupManager
	sessionMgr     *SessionManager
	authMgr        *AuthManager
	masterPassword string
	assets         fs.FS
}

// NewServer initializes HTTP routes and application state.
func NewServer(v *vault.Vault, assets fs.FS, backupDir string, masterPassword string) *Server {
	guard := safety.NewSafetyGuard()
	backupMgr := safety.NewBackupManager(backupDir)
	sessionMgr := NewSessionManager(v, guard, backupMgr)
	authMgr := NewAuthManager()

	s := &Server{
		vault:          v,
		guard:          guard,
		backupMgr:      backupMgr,
		sessionMgr:     sessionMgr,
		authMgr:        authMgr,
		masterPassword: masterPassword,
		assets:         assets,
	}

	s.routes()
	return s
}

func (s *Server) Router() http.Handler {
	return s.handler
}

func (s *Server) routes() {
	mux := http.NewServeMux()

	// Remote access authentication
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/unlock", s.handleAuthUnlock)
	mux.HandleFunc("GET /api/auth/master-key", s.handleGetMasterPassword)

	// Connection vault endpoints
	mux.HandleFunc("GET /api/connections", s.handleListConnections)
	mux.HandleFunc("POST /api/connections", s.handleSaveConnection)
	mux.HandleFunc("DELETE /api/connections/{id}", s.handleDeleteConnection)
	mux.HandleFunc("POST /api/connections/{id}/test", s.handleTestConnection)
	mux.HandleFunc("POST /api/vault/export", s.handleExportVault)
	mux.HandleFunc("POST /api/vault/import", s.handleImportVault)

	// Query snippets endpoints
	mux.HandleFunc("GET /api/snippets", s.handleListSnippets)
	mux.HandleFunc("POST /api/snippets", s.handleSaveSnippet)
	mux.HandleFunc("DELETE /api/snippets/{id}", s.handleDeleteSnippet)

	// Session exploration and actions
	mux.HandleFunc("POST /api/session/connect/{id}", s.handleConnectSession)
	mux.HandleFunc("GET /api/session/{id}/schema", s.handleGetSchema)
	mux.HandleFunc("GET /api/session/{id}/ddl/{table}", s.handleGetTableDDL)
	mux.HandleFunc("GET /api/session/{id}/table/{table}/rows", s.handleGetTableRows)
	mux.HandleFunc("GET /api/session/{id}/notes/{table}", s.handleGetTableNote)
	mux.HandleFunc("POST /api/session/{id}/notes/{table}", s.handleSaveTableNote)
	mux.HandleFunc("POST /api/session/{id}/table/{table}/import-csv", s.handleImportCSV)

	// Session commands dispatcher
	mux.HandleFunc("POST /api/session/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("action") {
		case "select-db":
			s.handleSelectDatabase(w, r)
		case "refresh-schema":
			s.handleRefreshSchema(w, r)
		case "cell-edit":
			s.handleCellEdit(w, r)
		case "toggle-readonly":
			s.handleToggleReadOnly(w, r)
		case "inspect":
			s.handleInspectQuery(w, r)
		case "explain":
			s.handleExplainQuery(w, r)
		case "execute":
			s.handleExecuteQuery(w, r)
		case "cancel":
			s.handleCancelQuery(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	// AI Assistant endpoints
	mux.HandleFunc("POST /api/ai/chat", s.handleAIChat)
	mux.HandleFunc("GET /api/ai/ollama-check", s.handleAIOllamaCheck)

	// Embedded Static Frontend
	if s.assets != nil {
		fileServer := http.FileServer(http.FS(s.assets))
		mux.HandleFunc("GET /", func(w http.ResponseWriter, req *http.Request) {
			path := strings.TrimPrefix(req.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			f, err := s.assets.Open(path)
			if err != nil {
				// Fallback to index.html for SPA routing
				req.URL.Path = "/"
			} else {
				_ = f.Close()
			}
			fileServer.ServeHTTP(w, req)
		})
	}

	// ponytail: stdlib panic recoverer and remote access guard replace chi router & middleware
	s.handler = recoverer(s.RemoteAccessGuardMiddleware(mux))
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil && rvr != http.ErrAbortHandler {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
