package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"sqlfox/internal/driver"
	"sqlfox/internal/vault"

	_ "sqlfox/internal/driver/mysql"
	_ "sqlfox/internal/driver/postgres"
	_ "sqlfox/internal/driver/sqlite"
)

// Server coordinates HTTP API routing and embedded web frontend delivery.
type Server struct {
	handler           http.Handler
	vault             *vault.Vault
	backupDir         string
	sessionMgr        *SessionManager
	authMgr           *AuthManager
	flightRecorderMgr *FlightRecorderManager
	masterPassword    string
	assets            fs.FS
	version           string
	updateAPIURL      string
	targetExePath     string
	restartFn         func() error
}

// NewServer initializes HTTP routes and application state.
func NewServer(v *vault.Vault, assets fs.FS, backupDir string, masterPassword string) *Server {
	sessionMgr := NewSessionManager(v, backupDir)
	authMgr := NewAuthManager()
	flightRecorderMgr := NewFlightRecorderManager(v, sessionMgr)

	s := &Server{
		vault:             v,
		backupDir:         backupDir,
		sessionMgr:        sessionMgr,
		authMgr:           authMgr,
		flightRecorderMgr: flightRecorderMgr,
		masterPassword:    masterPassword,
		assets:            assets,
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
	mux.HandleFunc("GET /api/session/{id}/preflight-ddl/{table}", s.handlePreflightDDL)
	mux.HandleFunc("GET /api/session/{id}/table/{table}/rows", s.handleGetTableRows)
	mux.HandleFunc("GET /api/session/{id}/notes/{table}", s.handleGetTableNote)
	mux.HandleFunc("POST /api/session/{id}/notes/{table}", s.handleSaveTableNote)
	mux.HandleFunc("POST /api/session/{id}/table/{table}/mock-data", s.handleGenerateMockData)
	mux.HandleFunc("POST /api/session/{id}/table/{table}/import-csv", s.handleImportCSV)


	// Processlist & DBA operations
	mux.HandleFunc("GET /api/session/{id}/processlist", s.handleGetProcesslist)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/cluster", s.handleGetClusterHealth)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/variables", s.handleGetServerVariables)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/tables", s.handleGetTableDiagnostics)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/redundant-indexes", s.handleGetRedundantIndexes)
	mux.HandleFunc("GET /api/session/{id}/peek", s.handlePeekForeignKey)

	// User & Privilege Management
	mux.HandleFunc("GET /api/session/{id}/users", s.handleListUsers)
	mux.HandleFunc("GET /api/session/{id}/users/grants", s.handleGetUserGrants)

	// Session Audit Log endpoints
	mux.HandleFunc("GET /api/session/{id}/audit-logs", s.handleGetAuditLogs)
	mux.HandleFunc("DELETE /api/session/{id}/audit-logs", s.handleClearAuditLogs)

	// Performance Flight Recorder endpoints
	mux.HandleFunc("POST /api/session/{id}/flight-recorder/start", s.handleFlightRecorderStart)
	mux.HandleFunc("POST /api/session/{id}/flight-recorder/stop", s.handleFlightRecorderStop)
	mux.HandleFunc("GET /api/session/{id}/flight-recorder/status", s.handleFlightRecorderStatus)
	mux.HandleFunc("GET /api/session/{id}/flight-recorder/sessions", s.handleFlightRecorderListSessions)
	mux.HandleFunc("GET /api/session/{id}/flight-recorder/sessions/{recId}", s.handleFlightRecorderGetSession)
	mux.HandleFunc("DELETE /api/session/{id}/flight-recorder/sessions/{recId}", s.handleFlightRecorderDeleteSession)

	// Navicat Power Suite: Dump endpoints
	mux.HandleFunc("GET /api/session/{id}/export-dump", s.handleExportDump)
	mux.HandleFunc("GET /api/session/{id}/export-smart-seed", s.handleExportSmartSeed)

	// Schema & Data Synchronization Engine (Milestone 11)
	mux.HandleFunc("POST /api/sync/structure/compare", s.handleSyncStructureCompare)
	mux.HandleFunc("POST /api/sync/structure/apply", s.handleSyncStructureApply)
	mux.HandleFunc("POST /api/sync/data/transfer", s.handleSyncDataTransfer)
	mux.HandleFunc("POST /api/sync/data/compare", s.handleSyncDataCompare)
	mux.HandleFunc("POST /api/sync/data/apply", s.handleSyncDataApply)

	// Session commands dispatcher
	mux.HandleFunc("POST /api/session/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("action") {
		case "select-db":
			s.handleSelectDatabase(w, r)
		case "refresh-schema":
			s.handleRefreshSchema(w, r)
		case "cell-edit":
			s.handleCellEdit(w, r)
		case "insert-row":
			s.handleInsertRow(w, r)
		case "delete-rows":
			s.handleDeleteRows(w, r)
		case "run-sql-script":
			s.handleRunSQLScript(w, r)
		case "toggle-readonly":
			s.handleToggleReadOnly(w, r)
		case "ping":
			s.handlePingSession(w, r)
		case "inspect":
			s.handleInspectQuery(w, r)
		case "explain":
			s.handleExplainQuery(w, r)
		case "advise":
			s.handleAdviseQuery(w, r)
		case "execute":
			s.handleExecuteQuery(w, r)
		case "cancel":
			s.handleCancelQuery(w, r)
		case "kill":
			s.handleKillProcess(w, r)
		case "maintenance":
			s.handleExecuteMaintenance(w, r)
		case "save-user":
			s.handleSaveUser(w, r)
		case "delete-user":
			s.handleDeleteUser(w, r)
		case "apply-grants":
			s.handleApplyUserGrants(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	// AI Assistant endpoints
	mux.HandleFunc("POST /api/ai/chat", s.handleAIChat)
	mux.HandleFunc("GET /api/ai/ollama-check", s.handleAIOllamaCheck)

	// System / Update Engine endpoints
	mux.HandleFunc("GET /api/system/update/check", s.handleCheckUpdate)
	mux.HandleFunc("POST /api/system/update/apply", s.handleApplyUpdate)
	mux.HandleFunc("GET /api/system/changelog", s.handleGetChangelog)

	// Profiling endpoints (conditional on SQLFOX_PPROF=1)
	s.registerPprof(mux)

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
				path = "index.html"
			} else {
				_ = f.Close()
			}
			if path == "index.html" {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
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

// quoteIdent formats an identifier safely with driver-aware quotes.
func quoteIdent(name, driverName string) string {
	return driver.QuoteIdentifier(name, driverName)
}

