package server

import (
	"encoding/json"
	"net/http"

	"sqlfox/internal/vault"
)

func (s *Server) handleListSnippets(w http.ResponseWriter, r *http.Request) {
	list, err := s.vault.ListSnippets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*vault.Snippet{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleSaveSnippet(w http.ResponseWriter, r *http.Request) {
	var snip vault.Snippet
	if err := json.NewDecoder(r.Body).Decode(&snip); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if snip.Title == "" {
		writeError(w, http.StatusBadRequest, "snippet title is required")
		return
	}
	if snip.SQLContent == "" {
		writeError(w, http.StatusBadRequest, "snippet SQL content is required")
		return
	}

	if err := s.vault.SaveSnippet(&snip); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save snippet: "+err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, snip)
}

func (s *Server) handleDeleteSnippet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "snippet id is required")
		return
	}

	if err := s.vault.DeleteSnippet(id); err != nil {
		if err == vault.ErrSnippetNotFound {
			writeError(w, http.StatusNotFound, "snippet not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
