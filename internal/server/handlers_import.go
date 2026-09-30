package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"sqlfox/internal/dataimport"
)

// SniffCSVDelimiter delegates to dataimport.SniffDelimiter.
func SniffCSVDelimiter(sample []byte) rune {
	return dataimport.SniffDelimiter(sample)
}

func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")
	dbName := r.URL.Query().Get("db")

	if connID == "" || tableName == "" {
		writeError(w, http.StatusBadRequest, "connection id and table name are required")
		return
	}

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if sess.ReadOnly {
		writeError(w, http.StatusForbidden, "cannot import data: session is in read-only mode")
		return
	}

	// Parse multipart form (max 32MB in memory, rest streams to temp disk)
	err = r.ParseMultipartForm(32 << 20)
	var fileReader io.Reader
	var delimiterParam string
	var emptyAsNullParam bool = true
	var skipErrorsParam bool = false
	var mappingJSON string

	if err == nil && r.MultipartForm != nil {
		files := r.MultipartForm.File["file"]
		if len(files) == 0 {
			writeError(w, http.StatusBadRequest, "no file uploaded in 'file' form field")
			return
		}
		f, err := files[0].Open()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to open uploaded file: "+err.Error())
			return
		}
		defer f.Close()
		fileReader = f

		delimiterParam = r.FormValue("delimiter")
		if r.FormValue("empty_as_null") == "false" {
			emptyAsNullParam = false
		}
		if r.FormValue("skip_errors") == "true" {
			skipErrorsParam = true
		}
		mappingJSON = r.FormValue("mapping")
	} else {
		// Fallback to raw request body if sent directly
		fileReader = r.Body
		delimiterParam = r.URL.Query().Get("delimiter")
		if r.URL.Query().Get("empty_as_null") == "false" {
			emptyAsNullParam = false
		}
		if r.URL.Query().Get("skip_errors") == "true" {
			skipErrorsParam = true
		}
		mappingJSON = r.URL.Query().Get("mapping")
	}

	var delimiter rune
	if delimiterParam != "" && delimiterParam != "auto" {
		runes := []rune(delimiterParam)
		if len(runes) > 0 {
			delimiter = runes[0]
		}
	}

	mapping := make(map[string]string)
	if mappingJSON != "" {
		_ = json.Unmarshal([]byte(mappingJSON), &mapping)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	if dbName != "" && sess.Session != nil {
		_ = sess.Session.SelectDatabase(ctx, dbName)
	}

	importer := dataimport.NewCSVImporter()
	res, err := importer.Import(ctx, sess.Session, tableName, fileReader, dataimport.ImportOptions{
		Delimiter:   delimiter,
		EmptyAsNull: emptyAsNullParam,
		SkipErrors:  skipErrorsParam,
		Mapping:     mapping,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}
