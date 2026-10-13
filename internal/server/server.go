package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"sqlfox/internal/driver"
	"sqlfox/internal/logger"
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
	logger            *slog.Logger
	updateAPIURL      string
	targetExePath     string
	restartFn         func() error
	cloudSyncMu       sync.Mutex
	cloudSyncTimer    *time.Timer
	lanEnabled        bool
	allowedHosts      []string
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

	// Operational Notes Inbox & Follow-up endpoints
	mux.HandleFunc("GET /api/notes", s.handleListOperationalNotes)
	mux.HandleFunc("POST /api/notes", s.handleCreateOperationalNote)
	mux.HandleFunc("GET /api/notes/reminders/due", s.handleListDueReminders)
	mux.HandleFunc("GET /api/notes/{id}", s.handleGetOperationalNote)
	mux.HandleFunc("PUT /api/notes/{id}", s.handleUpdateOperationalNote)
	mux.HandleFunc("DELETE /api/notes/{id}", s.handleDeleteOperationalNote)
	mux.HandleFunc("POST /api/notes/{id}/snooze", s.handleSnoozeReminder)
	mux.HandleFunc("POST /api/notes/{id}/resolve", s.handleResolveOperationalNote)

	// Cloud Sync & Portability (Notes Bundle & GitHub Sync)
	mux.HandleFunc("GET /api/cloud-sync/config", s.handleGetCloudSyncConfig)
	mux.HandleFunc("POST /api/cloud-sync/config", s.handleSaveCloudSyncConfig)
	mux.HandleFunc("POST /api/cloud-sync/test", s.handleTestCloudSync)
	mux.HandleFunc("POST /api/cloud-sync/run", s.handleRunCloudSync)
	mux.HandleFunc("GET /api/cloud-sync/status", s.handleGetCloudSyncStatus)
	mux.HandleFunc("GET /api/notes/bundle/export", s.handleExportNotesBundle)
	mux.HandleFunc("POST /api/notes/bundle/import", s.handleImportNotesBundle)

	// Session exploration and actions
	mux.HandleFunc("POST /api/session/connect/{id}", s.handleConnectSession)
	mux.HandleFunc("GET /api/session/{id}/schema", s.handleGetSchema)
	mux.HandleFunc("GET /api/session/{id}/fast-catalog", s.handleGetFastCatalog)
	mux.HandleFunc("GET /api/session/{id}/active-threads", s.handleGetActiveThreads)
	mux.HandleFunc("GET /api/session/{id}/ddl/{table}", s.handleGetTableDDL)
	mux.HandleFunc("GET /api/session/{id}/ddl/routine/{name}", s.handleGetRoutineDDL)
	mux.HandleFunc("GET /api/session/{id}/ddl/event/{name}", s.handleGetEventDDL)
	mux.HandleFunc("POST /api/session/{id}/event/{name}/toggle", s.handleToggleEvent)
	mux.HandleFunc("GET /api/session/{id}/partitions/{table}", s.handleGetTablePartitions)
	mux.HandleFunc("GET /api/session/{id}/preflight-ddl/{table}", s.handlePreflightDDL)
	mux.HandleFunc("GET /api/session/{id}/table/{table}/rows", s.handleGetTableRows)
	mux.HandleFunc("GET /api/session/{id}/notes/{table}", s.handleGetTableNote)
	mux.HandleFunc("POST /api/session/{id}/notes/{table}", s.handleSaveTableNote)
	mux.HandleFunc("POST /api/session/{id}/table/{table}/mock-data", s.handleGenerateMockData)
	mux.HandleFunc("POST /api/session/{id}/table/{table}/import-csv", s.handleImportCSV)


	// Processlist & DBA operations
	mux.HandleFunc("GET /api/session/{id}/processlist", s.handleGetProcesslist)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/deadlock", s.handleGetDeadlock)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/statement-digest", s.handleGetStatementDigest)
	mux.HandleFunc("POST /api/session/{id}/diagnostics/statement-digest/reset", s.handleResetStatementDigest)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/cluster", s.handleGetClusterHealth)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/variables", s.handleGetServerVariables)
	mux.HandleFunc("GET /api/session/{id}/diagnostics/databases", s.handleGetDatabaseDiagnostics)
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

	// Observability & Live Activity Stream
	mux.HandleFunc("GET /api/logs/stream", s.handleLogsStream)

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
				isRemote := !isLoopback(req.RemoteAddr)
				token := extractToken(req)
				unlocked := !isRemote || s.authMgr.ValidateToken(token)
				if isRemote && !unlocked {
					content, err := fs.ReadFile(s.assets, "index.html")
					if err == nil {
						// ponytail: unhide remote-lock-modal and mark body vault-locked so streaming / Ctrl+F5 suppresses inner UI
						modified := strings.Replace(string(content), `<body class="`, `<body class="vault-locked `, 1)
						modified = strings.Replace(modified, `id="remote-lock-modal" class="fixed inset-0 bg-slate-950/95 backdrop-blur-md flex items-center justify-center p-4 z-[100] hidden"`, `id="remote-lock-modal" class="fixed inset-0 bg-slate-950/95 backdrop-blur-md flex items-center justify-center p-4 z-[100]"`, 1)
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						_, _ = w.Write([]byte(modified))
						return
					}
				}
			}
			fileServer.ServeHTTP(w, req)
		})
	}

	// ponytail: stdlib panic recoverer, logger, and ingress + remote access guards replace chi router & middleware
	baseHandler := recoverer(s.IngressGuardMiddleware(s.RemoteAccessGuardMiddleware(mux)))
	if s.logger != nil {
		s.handler = logger.HTTPMiddleware(s.logger)(baseHandler)
	} else {
		s.handler = baseHandler
	}
}

// SetLANEnabled toggles whether the server is configured to accept LAN connections and display Master PIN.
func (s *Server) SetLANEnabled(enabled bool) {
	s.lanEnabled = enabled
}

// IsLANEnabled returns whether LAN mode is currently active.
func (s *Server) IsLANEnabled() bool {
	return s.lanEnabled
}

// SetAllowedHosts sets explicit hostnames or IPs accepted by the DNS Rebinding Guard.
func (s *Server) SetAllowedHosts(hosts []string) {
	s.allowedHosts = hosts
}

// Close gracefully shuts down all active database sessions and background managers.
func (s *Server) Close() {
	if s.sessionMgr != nil {
		s.sessionMgr.CloseAll()
	}
}

// SetLogger attaches the Diagnostic Logger and wraps HTTP routing with logging telemetry.
func (s *Server) SetLogger(l *slog.Logger) {
	s.logger = l
	if s.handler != nil {
		s.handler = logger.HTTPMiddleware(l)(s.handler)
	}
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

