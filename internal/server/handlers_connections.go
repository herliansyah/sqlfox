package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"sqlfox/internal/driver"
	"sqlfox/internal/vault"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListConnections(w http.ResponseWriter, r *http.Request) {
	list, err := s.vault.ListConnections()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*vault.ConnectionRecord{}
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

	if err := s.vault.SaveConnection(&conn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save connection: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, conn)
}

func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "connection id is required")
		return
	}

	_ = s.sessionMgr.CloseSession(id)
	if err := s.vault.DeleteConnection(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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
				conn.Password = existing.Password
			}
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
		UseSSH:       conn.UseSSH,
		SSHHost:      conn.SSHHost,
		SSHPort:      conn.SSHPort,
		SSHUser:      conn.SSHUser,
		SSHPassword:  conn.SSHPassword,
		SSHKey:       conn.SSHKey,
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
		ExportPassphrase string `json:"export_passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExportPassphrase == "" {
		writeError(w, http.StatusBadRequest, "export_passphrase is required")
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

	count, err := vault.ImportVault(s.vault, req.Archive, req.ExportPassphrase)
	if err != nil {
		writeError(w, http.StatusBadRequest, "import failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported_count": count,
		"message":        "Connections successfully imported into vault.",
	})
}
