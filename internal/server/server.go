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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server coordinates HTTP API routing and embedded web frontend delivery.
type Server struct {
	router         *chi.Mux
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
		router:         chi.NewRouter(),
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
	return s.router
}

func (s *Server) routes() {
	r := s.router

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(s.RemoteAccessGuardMiddleware)

	// API Namespace
	r.Route("/api", func(api chi.Router) {
		// Remote access authentication
		api.Get("/auth/status", s.handleAuthStatus)
		api.Post("/auth/unlock", s.handleAuthUnlock)
		api.Get("/auth/master-key", s.handleGetMasterPassword)
		// Connection vault endpoints
		api.Get("/connections", s.handleListConnections)
		api.Post("/connections", s.handleSaveConnection)
		api.Delete("/connections/{id}", s.handleDeleteConnection)
		api.Post("/connections/{id}/test", s.handleTestConnection)
		api.Post("/vault/export", s.handleExportVault)
		api.Post("/vault/import", s.handleImportVault)

		// Session and database exploration
		api.Post("/session/connect/{id}", s.handleConnectSession)
		api.Get("/session/{id}/schema", s.handleGetSchema)
		api.Post("/session/{id}/refresh-schema", s.handleRefreshSchema)
		api.Get("/session/{id}/ddl/{table}", s.handleGetTableDDL)
		api.Get("/session/{id}/table/{table}/rows", s.handleGetTableRows)
		api.Post("/session/{id}/cell-edit", s.handleCellEdit)
		api.Post("/session/{id}/toggle-readonly", s.handleToggleReadOnly)

		// Query execution, safety, and streaming
		api.Post("/session/{id}/inspect", s.handleInspectQuery)
		api.Post("/session/{id}/explain", s.handleExplainQuery)
		api.Post("/session/{id}/execute", s.handleExecuteQuery)
		api.Post("/session/{id}/cancel", s.handleCancelQuery)
		api.HandleFunc("/session/{id}/stream", s.handleStreamWebSocket)

		// AI Assistant endpoints
		api.Post("/ai/chat", s.handleAIChat)
		api.Get("/ai/ollama-check", s.handleAIOllamaCheck)
	})

	// Embedded Static Frontend
	if s.assets != nil {
		fileServer := http.FileServer(http.FS(s.assets))
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
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
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
