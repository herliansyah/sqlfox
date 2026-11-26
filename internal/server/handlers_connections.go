package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/herliansyah/sqlfox/internal/ai"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/vault"
)

// validateConnectionPayload enforces path and TLS security boundaries for HTTP-supplied connections:
//   - Blocks opening SqlFox's own vault database or directory as a SQLite target.
//   - Rejects arbitrary host filesystem SQLite paths from remote (LAN) requests.
//   - Requires inline PEM content (not host filesystem paths) for SSLCA/SSLCert/SSLKey from the HTTP API.
func (s *Server) validateConnectionPayload(conn *vault.ConnectionRecord, isRemote bool) error {
	if conn == nil {
		return errors.New("connection payload is required")
	}
	checkPEM := func(field, val string) error {
		v := strings.TrimSpace(val)
		if v == "" {
			return nil
		}
		if !strings.Contains(v, "-----BEGIN ") {
			return errors.New(field + " must be inline PEM certificate/key content, not a server filesystem path")
		}
		return nil
	}
	if err := checkPEM("ssl_ca", conn.SSLCA); err != nil {
		return err
	}
	if err := checkPEM("ssl_cert", conn.SSLCert); err != nil {
		return err
	}
	if err := checkPEM("ssl_key", conn.SSLKey); err != nil {
		return err
	}

	drv := strings.ToLower(strings.TrimSpace(conn.Driver))
	if drv == "sqlite" || drv == "sqlite3" {
		dbPath := strings.TrimSpace(conn.DatabaseName)
		if dbPath == "" {
			dbPath = strings.TrimSpace(conn.Host)
		}
		if dbPath != "" && dbPath != ":memory:" {
			if isRemote {
				return errors.New("remote requests cannot open arbitrary server SQLite filesystem paths")
			}
			// Normalize SQLite file: URI (authority, query, fragment, percent-encoding) and resolve symlinks
			// so file:///.../config%2edb, file://localhost/.../config.db, or symlinks cannot bypass the vault block.
			rawPath := normalizeSQLiteTargetPath(dbPath)
			if rawPath != "" && rawPath != ":memory:" && s.vault != nil && s.vault.Path() != "" {
				absTarget, err1 := canonicalAbsPath(rawPath)
				absVault, err2 := canonicalAbsPath(s.vault.Path())
				if err1 == nil && err2 == nil {
					masterKeyPath := filepath.Join(filepath.Dir(absVault), "master.key")
					if absTarget == absVault || strings.HasPrefix(absTarget, absVault+"-") || absTarget == masterKeyPath {
						return errors.New("cannot open SqlFox's own vault database or key file as a SQLite connection")
					}
				}
			}
		}
	}
	return nil
}

func normalizeSQLiteTargetPath(dbPath string) string {
	rawPath := strings.TrimSpace(dbPath)
	if !strings.HasPrefix(strings.ToLower(rawPath), "file:") {
		return rawPath
	}
	uriPath := strings.ReplaceAll(rawPath, `\`, "/")
	if strings.HasPrefix(strings.ToLower(uriPath), "file://localhost") {
		remainder := uriPath[len("file://localhost"):]
		if isWindowsDrivePath(remainder) {
			uriPath = "file:///" + remainder
		}
	} else if strings.HasPrefix(strings.ToLower(uriPath), "file://") {
		remainder := uriPath[len("file://"):]
		if isWindowsDrivePath(remainder) {
			uriPath = "file:///" + remainder
		}
	}
	if u, err := url.Parse(uriPath); err == nil {
		p := u.Path
		if len(u.Host) == 2 && u.Host[1] == ':' && isASCIIAlpha(u.Host[0]) {
			p = u.Host + p
		} else if len(p) >= 4 && p[0] == '/' && isWindowsDrivePath(p[1:]) {
			p = p[1:]
		}
		if p == "" {
			p = u.Opaque
		}
		if unescaped, uErr := url.PathUnescape(p); uErr == nil {
			p = unescaped
		}
		if p != "" {
			return p
		}
	}
	// Fallback manual strip for non-standard file: forms
	rawPath = rawPath[5:]
	if qIdx := strings.IndexAny(rawPath, "?#"); qIdx >= 0 {
		rawPath = rawPath[:qIdx]
	}
	if strings.HasPrefix(strings.ToLower(rawPath), "//localhost/") {
		rawPath = rawPath[len("//localhost"):]
	}
	if unescaped, err := url.PathUnescape(rawPath); err == nil {
		rawPath = unescaped
	}
	return rawPath
}

func isWindowsDrivePath(path string) bool {
	return len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && path[2] == '/'
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func canonicalAbsPath(p string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", err
	}
	if resolved, rErr := filepath.EvalSymlinks(abs); rErr == nil {
		return resolved, nil
	}
	// Target file may not exist yet; resolve parent directory symlinks so a symlinked dir cannot bypass checks.
	if resolvedDir, rErr := filepath.EvalSymlinks(filepath.Dir(abs)); rErr == nil {
		return filepath.Join(resolvedDir, filepath.Base(abs)), nil
	}
	return abs, nil
}

func (s *Server) handleListConnections(w http.ResponseWriter, r *http.Request) {
	list, err := s.vault.ListConnections()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*vault.ConnectionRecord{}
	}
	// Never expose raw or sealed SSL private keys in connection list responses.
	for _, c := range list {
		if c != nil {
			c.Password = ""
			c.SSLKey = ""
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleSaveConnection(w http.ResponseWriter, r *http.Request) {
	var conn vault.ConnectionRecord
	if err := json.NewDecoder(r.Body).Decode(&conn); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if conn.Name == "" {
		writeError(w, http.StatusBadRequest, "connection name is required")
		return
	}
	if conn.Driver == "" {
		conn.Driver = "mysql"
	}
	if conn.Environment == "" {
		conn.Environment = "Development"
	}
	if err := s.validateConnectionPayload(&conn, s.IsRemoteRequest(r)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.vault.SaveConnection(&conn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save connection: "+err.Error())
		return
	}

	conn.Password = ""
	conn.SSLKey = ""
	writeJSON(w, http.StatusOK, conn)
}

func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "connection id is required")
		return
	}

	_ = s.sessionMgr.CloseSession(id)
	s.sessionMgr.RemoveReadOnly(id)
	if err := s.vault.DeleteConnection(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var conn *vault.ConnectionRecord
	var err error

	if id != "" && id != "test-transient" {
		conn, err = s.vault.GetConnection(id)
		if err != nil {
			writeError(w, http.StatusNotFound, "connection not found: "+err.Error())
			return
		}
	} else {
		// Test transient payload from form before saving
		conn = &vault.ConnectionRecord{}
		if err := json.NewDecoder(r.Body).Decode(conn); err != nil {
			writeError(w, http.StatusBadRequest, "invalid connection payload: "+err.Error())
			return
		}
		if conn.Password == "" && conn.ID != "" {
			if existing, err := s.vault.GetConnection(conn.ID); err == nil {
				if !strings.EqualFold(conn.Host, existing.Host) || conn.Port != existing.Port || !strings.EqualFold(conn.Driver, existing.Driver) {
					writeError(w, http.StatusBadRequest, "password must be re-entered when changing host, port, or driver")
					return
				}
				conn.Password = existing.Password
			}
		}
		if conn.UseSSL && conn.SSLKey == "" && conn.ID != "" {
			if existing, err := s.vault.GetConnection(conn.ID); err == nil {
				if strings.EqualFold(conn.Host, existing.Host) && conn.Port == existing.Port && strings.EqualFold(conn.Driver, existing.Driver) {
					conn.SSLKey = existing.SSLKey
				}
			}
		}
		if err := s.validateConnectionPayload(conn, s.IsRemoteRequest(r)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cfg := driver.Config{
		Driver:       conn.Driver,
		Host:         conn.Host,
		Port:         conn.Port,
		DatabaseName: conn.DatabaseName,
		User:         conn.User,
		Password:     conn.Password,
		UseSSL:       conn.UseSSL,
		SSLCA:        conn.SSLCA,
		SSLCert:      conn.SSLCert,
		SSLKey:       conn.SSLKey,
	}

	sess, err := driver.Connect(ctx, cfg)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer sess.Close()

	latency, err := sess.Ping(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	info, _ := sess.ServerInfo(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"latency_ms": latency.Milliseconds(),
		"server":     info,
	})
}

func (s *Server) handleExportVault(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExportPassphrase      string `json:"export_passphrase"`
		CurrentMasterPassword string `json:"current_master_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExportPassphrase == "" {
		writeError(w, http.StatusBadRequest, "export_passphrase is required")
		return
	}
	if strings.TrimSpace(req.CurrentMasterPassword) == "" {
		writeError(w, http.StatusUnauthorized, "current_master_password is required to export vault secrets")
		return
	}
	if s.vault == nil || !s.vault.ValidateMasterPassword(req.CurrentMasterPassword) {
		writeError(w, http.StatusUnauthorized, "incorrect current master password")
		return
	}

	archiveBytes, err := vault.ExportVault(s.vault, req.ExportPassphrase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"sqlfox-connections.sqlfox-vault\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archiveBytes)
}

func (s *Server) handleImportVault(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExportPassphrase string          `json:"export_passphrase"`
		Archive          json.RawMessage `json:"archive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExportPassphrase == "" {
		writeError(w, http.StatusBadRequest, "export_passphrase and archive are required")
		return
	}

	isRemote := s.IsRemoteRequest(r)
	count, err := vault.ImportVaultWithValidation(s.vault, req.Archive, req.ExportPassphrase, func(payload *vault.ExportPayload) error {
		for _, conn := range payload.Connections {
			if err := s.validateConnectionPayload(conn, isRemote); err != nil {
				return err
			}
		}
		for _, p := range payload.AIProfiles {
			if p == nil {
				return errors.New("ai profile payload is required")
			}
			if err := validateAIEndpoint(p.BaseURL, ai.Provider(p.Provider), isRemote); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "import failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported_count": count,
		"message":        "Connections successfully imported into vault.",
	})
}
