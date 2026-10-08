package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"sqlfox/internal/vault"
)

// handleListOperationalNotes lists operational notes with optional filtering.
func (s *Server) handleListOperationalNotes(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	filter := vault.OperationalNoteFilter{
		Status:    r.URL.Query().Get("status"),
		ObjectTag: r.URL.Query().Get("object_tag"),
		UserTag:   r.URL.Query().Get("user_tag"),
		Search:    r.URL.Query().Get("q"),
	}

	if pinnedStr := r.URL.Query().Get("is_pinned"); pinnedStr != "" {
		isPinned := pinnedStr == "true" || pinnedStr == "1"
		filter.IsPinned = &isPinned
	}

	notes, err := s.vault.ListOperationalNotes(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list operational notes: "+err.Error())
		return
	}

	if notes == nil {
		notes = []*vault.OperationalNote{}
	}
	writeJSON(w, http.StatusOK, notes)
}

// handleCreateOperationalNote creates a new operational note.
func (s *Server) handleCreateOperationalNote(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	var note vault.OperationalNote
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if note.Title == "" {
		note.Title = "Untitled Note"
	}
	if note.CapturedAt.IsZero() {
		note.CapturedAt = time.Now()
	}

	if err := s.vault.CreateOperationalNote(&note); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create operational note: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusCreated, note)
}

// handleGetOperationalNote retrieves a single operational note by ID.
func (s *Server) handleGetOperationalNote(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "note id required")
		return
	}

	note, err := s.vault.GetOperationalNote(id)
	if err != nil {
		if errors.Is(err, vault.ErrOperationalNoteNotFound) {
			writeError(w, http.StatusNotFound, "operational note not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, note)
}

// handleUpdateOperationalNote updates an operational note.
func (s *Server) handleUpdateOperationalNote(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "note id required")
		return
	}

	var note vault.OperationalNote
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	note.ID = id

	if err := s.vault.UpdateOperationalNote(&note); err != nil {
		if errors.Is(err, vault.ErrOperationalNoteNotFound) {
			writeError(w, http.StatusNotFound, "operational note not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update operational note: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, note)
}

// handleDeleteOperationalNote deletes an operational note.
func (s *Server) handleDeleteOperationalNote(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "note id required")
		return
	}

	if err := s.vault.DeleteOperationalNote(id); err != nil {
		if errors.Is(err, vault.ErrOperationalNoteNotFound) {
			writeError(w, http.StatusNotFound, "operational note not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete operational note: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleListDueReminders lists all active notes whose reminder is due now.
func (s *Server) handleListDueReminders(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	due, err := s.vault.ListDueReminders(time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list due reminders: "+err.Error())
		return
	}

	if due == nil {
		due = []*vault.OperationalNote{}
	}
	writeJSON(w, http.StatusOK, due)
}

// handleSnoozeReminder postpones the note's reminder by N minutes (default 10).
func (s *Server) handleSnoozeReminder(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "note id required")
		return
	}

	minutes := 10
	var req struct {
		Minutes int `json:"minutes"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Minutes > 0 {
			minutes = req.Minutes
		}
	}
	if qM := r.URL.Query().Get("minutes"); qM != "" {
		if parsed, err := strconv.Atoi(qM); err == nil && parsed > 0 {
			minutes = parsed
		}
	}

	duration := time.Duration(minutes) * time.Minute
	if err := s.vault.SnoozeReminder(id, duration); err != nil {
		if errors.Is(err, vault.ErrOperationalNoteNotFound) {
			writeError(w, http.StatusNotFound, "operational note not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to snooze reminder: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "snoozed",
		"snooze_min":  minutes,
		"reminder_at": time.Now().Add(duration),
	})
}

// handleResolveOperationalNote marks the note as resolved.
func (s *Server) handleResolveOperationalNote(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "vault not initialized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "note id required")
		return
	}

	if err := s.vault.ResolveOperationalNote(id); err != nil {
		if errors.Is(err, vault.ErrOperationalNoteNotFound) {
			writeError(w, http.StatusNotFound, "operational note not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to resolve operational note: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}
