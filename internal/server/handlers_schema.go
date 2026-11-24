package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/herliansyah/sqlfox/internal/driver"
	"math"
	"net/http"
	"runtime/metrics"
	"time"
)

func (s *Server) handleConnectSession(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	if connID == "" {
		writeError(w, http.StatusBadRequest, "connection id required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	sess, err := s.sessionMgr.ConnectSession(ctx, connID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handlePingSession(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	latency, err := sess.Session.Ping(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ping failed: "+err.Error())
		return
	}
	sess.SetLatency(latency.Milliseconds())

	memSample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(memSample)
	var heapBytes uint64
	if memSample[0].Value.Kind() == metrics.KindUint64 {
		heapBytes = memSample[0].Value.Uint64()
	}
	allocMB := math.Round(float64(heapBytes)/1024/1024*10) / 10
	info, _ := sess.Session.ServerInfo(ctx)
	if info != nil {
		sess.SetInfo(info)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"latency_ms":  latency.Milliseconds(),
		"memory_mb":   allocMB,
		"server_info": sess.Info(),
		"read_only":   sess.IsReadOnly(),
	})
}

func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	targetDB := r.URL.Query().Get("db")
	if targetDB != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		schema, err := sess.Session.InspectSchema(ctx, targetDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
			return
		}
		sess.SetSchema(schema)
		writeJSON(w, http.StatusOK, schema)
		return
	} else if sess.Schema() == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		schema, err := sess.Session.InspectSchema(ctx, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
			return
		}
		sess.SetSchema(schema)
	}

	writeJSON(w, http.StatusOK, sess.Schema())
}

func (s *Server) handleGetFastCatalog(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	catalog, err := sess.Session.InspectFastCatalog(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect fast catalog: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleGetActiveThreads(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	info, err := sess.Session.ServerInfo(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get server info: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"active_threads": info.ActiveThreads,
		"active_conns":   info.ActiveConns,
		"version":        info.Version,
	})
}

func (s *Server) handleSelectDatabase(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		Database string `json:"database"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Database == "" {
		writeError(w, http.StatusBadRequest, "database name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := sess.Session.SelectDatabase(ctx, req.Database); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	schema, err := sess.Session.InspectSchema(ctx, req.Database)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
		return
	}
	sess.SetSchema(schema)

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleRefreshSchema(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	targetDB := r.URL.Query().Get("db")

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	schema, err := sess.Session.InspectSchema(ctx, targetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh schema: "+err.Error())
		return
	}
	sess.SetSchema(schema)

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleGetTableDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	targetDB := r.URL.Query().Get("db")
	if targetDB == "" && sess.DatabaseName != "" {
		targetDB = sess.DatabaseName
	}
	if targetDB != "" {
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	ddl, err := sess.Session.GetTableDDL(ctx, tableName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch DDL: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"table": tableName,
		"ddl":   ddl,
	})
}

func (s *Server) handleGetRoutineDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	routineName := r.PathValue("name")
	routineType := r.URL.Query().Get("type")
	if routineType == "" {
		routineType = "PROCEDURE"
	}

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	ddl, err := sess.GetRoutineDDL(ctx, routineType, routineName)
	if err != nil {
		if errors.Is(err, driver.ErrFeatureNotSupported) {
			ddl = "-- Stored Routines not supported on this engine"
		} else {
			writeError(w, http.StatusInternalServerError, "failed to fetch routine DDL: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"name": routineName,
		"type": routineType,
		"ddl":  ddl,
	})
}

func (s *Server) handleGetEventDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	eventName := r.PathValue("name")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	ddl, err := sess.GetEventDDL(ctx, eventName)
	if err != nil {
		if errors.Is(err, driver.ErrFeatureNotSupported) {
			ddl = "-- Scheduled Events not supported on this engine"
		} else {
			writeError(w, http.StatusInternalServerError, "failed to fetch event DDL: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"name": eventName,
		"ddl":  ddl,
	})
}

func (s *Server) handleToggleEvent(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	eventName := r.PathValue("name")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	var req struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "enable flag required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := sess.ToggleEventStatus(ctx, eventName, req.Enable); err != nil {
		if errors.Is(err, driver.ErrFeatureNotSupported) {
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to toggle event: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleGetTablePartitions(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	database := r.URL.Query().Get("db")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	partitions, err := sess.GetTablePartitions(ctx, database, tableName)
	if err != nil {
		if errors.Is(err, driver.ErrFeatureNotSupported) {
			writeJSON(w, http.StatusOK, []driver.PartitionMetadata{})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch partitions: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, partitions)
}

func (s *Server) handlePreflightDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	targetDB := r.URL.Query().Get("db")
	if targetDB == "" && sess.DatabaseName != "" {
		targetDB = sess.DatabaseName
	}
	if targetDB != "" {
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	var locks []driver.TableLockInfo
	if dp, ok := sess.Session.(driver.DiagnosticsProvider); ok {
		if l, lErr := dp.InspectTableLocks(ctx, targetDB, tableName); lErr == nil {
			locks = l
		}
	}
	if locks == nil {
		locks = []driver.TableLockInfo{}
	}

	var rels []driver.TableRelationshipInfo
	if schema, err := sess.Session.InspectSchema(ctx, targetDB); err == nil {
		rels = driver.InspectTableRelationships(schema, tableName)
	} else {
		rels = []driver.TableRelationshipInfo{}
	}

	alterClause := r.URL.Query().Get("alter")
	if alterClause == "" {
		alterClause = "/* your ALTER TABLE statement */"
	}

	host := "127.0.0.1"
	port := 3306
	user := "root"
	if s.vault != nil && sess.ConnectionID != "" {
		if c, err := s.vault.GetConnection(sess.ConnectionID); err == nil && c != nil {
			if c.Host != "" {
				host = c.Host
			}
			if c.Port > 0 {
				port = c.Port
			}
			if c.User != "" {
				user = c.User
			}
		}
	}

	ghostCmd := driver.GenerateOSCCommand("gh-ost", host, port, user, targetDB, tableName, alterClause)
	ptCmd := driver.GenerateOSCCommand("pt-osc", host, port, user, targetDB, tableName, alterClause)

	writeJSON(w, http.StatusOK, map[string]any{
		"table":         tableName,
		"database":      targetDB,
		"active_locks":  locks,
		"relationships": rels,
		"osc_ghost":     ghostCmd,
		"osc_pt":        ptCmd,
	})
}
