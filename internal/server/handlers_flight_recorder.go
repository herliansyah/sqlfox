package server

import (
	"encoding/json"
	"net/http"

	"github.com/herliansyah/sqlfox/internal/vault"
)

type startFlightRecordingReq struct {
	Name       string `json:"name"`
	IntervalMs int    `json:"interval_ms"`
}

func (s *Server) handleFlightRecorderStart(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req startFlightRecordingReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	rec, err := s.flightRecorderMgr.StartRecording(sess.ID, connID, req.Name, req.IntervalMs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":      "Flight recording started",
		"recording_id": rec.ID,
		"name":         rec.Name,
		"started_at":   rec.StartedAt,
		"interval_ms":  rec.Interval.Milliseconds(),
	})
}

func (s *Server) handleFlightRecorderStop(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	finished, err := s.flightRecorderMgr.StopRecording(sess.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, finished)
}

func (s *Server) handleFlightRecorderStatus(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	status, err := s.flightRecorderMgr.GetStatus(sess.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleFlightRecorderListSessions(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	list, err := s.vault.ListFlightRecordingSessions(sess.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (s *Server) flightRecordingOwnedBy(connID string, rec *vault.FlightRecordingSession) bool {
	if connID == "" {
		return true
	}
	if rec == nil {
		return false
	}
	if rec.ConnectionID == connID || rec.SessionID == connID {
		return true
	}
	if s.sessionMgr != nil {
		if sess, err := s.sessionMgr.GetSession(connID); err == nil && sess != nil {
			if rec.SessionID == sess.ID || rec.ConnectionID == sess.ID {
				return true
			}
		}
	}
	return false
}

func (s *Server) handleFlightRecorderGetSession(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	recID := r.PathValue("recId")
	detail, err := s.vault.GetFlightRecordingSessionWithTicks(recID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if detail == nil || !s.flightRecordingOwnedBy(connID, detail.Session) {
		writeError(w, http.StatusNotFound, "flight recording not found for this session")
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleFlightRecorderDeleteSession(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	recID := r.PathValue("recId")
	detail, err := s.vault.GetFlightRecordingSessionWithTicks(recID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if detail == nil || !s.flightRecordingOwnedBy(connID, detail.Session) {
		writeError(w, http.StatusNotFound, "flight recording not found for this session")
		return
	}
	if err := s.vault.DeleteFlightRecordingSession(recID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Flight recording session deleted",
	})
}
