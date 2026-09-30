package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sqlfox/internal/driver"

	"github.com/go-chi/chi/v5"
)

// SniffCSVDelimiter detects the delimiter among standard CSV separators.
// ponytail: Heuristic frequency scan across initial bytes. Upgrade path: dialect/grammar parser if exotic files encountered.
func SniffCSVDelimiter(sample []byte) rune {
	delims := []rune{',', ';', '\t', '|'}
	counts := make(map[rune]int)

	lines := strings.Split(string(sample), "\n")
	sampleLines := lines
	if len(sampleLines) > 5 {
		sampleLines = sampleLines[:5]
	}
	for _, line := range sampleLines {
		for _, d := range delims {
			counts[d] += strings.Count(line, string(d))
		}
	}

	best := ','
	bestCount := 0
	for _, d := range delims {
		if counts[d] > bestCount {
			bestCount = counts[d]
			best = d
		}
	}
	return best
}

func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	connID := chi.URLParam(r, "id")
	tableName := chi.URLParam(r, "table")
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

	// Read initial prefix (up to 4096 bytes) for delimiter sniffing and BOM stripping
	prefix := make([]byte, 4096)
	n, _ := fileReader.Read(prefix)
	prefix = prefix[:n]

	// Strip UTF-8 BOM if present
	prefix = bytes.TrimPrefix(prefix, []byte("\xef\xbb\xbf"))

	// Determine delimiter
	var comma rune
	if delimiterParam == "" || delimiterParam == "auto" {
		comma = SniffCSVDelimiter(prefix)
	} else {
		runes := []rune(delimiterParam)
		if len(runes) > 0 {
			comma = runes[0]
		} else {
			comma = ','
		}
	}

	// Recompose stream: prefix + remaining body
	fullReader := io.MultiReader(bytes.NewReader(prefix), fileReader)
	csvReader := csv.NewReader(fullReader)
	csvReader.Comma = comma
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	// Read CSV header row
	headers, err := csvReader.Read()
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read CSV headers: "+err.Error())
		return
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}

	// Parse column mapping
	mapping := make(map[string]string)
	if mappingJSON != "" {
		_ = json.Unmarshal([]byte(mappingJSON), &mapping)
	}

	type mappedCol struct {
		csvIdx    int
		targetCol string
	}
	var activeCols []mappedCol
	for i, h := range headers {
		target := ""
		// Priority 1: mapping by numeric index "0", "1"
		if val, ok := mapping[strconv.Itoa(i)]; ok {
			target = strings.TrimSpace(val)
		} else if val, ok := mapping[h]; ok {
			// Priority 2: mapping by header text
			target = strings.TrimSpace(val)
		} else if len(mapping) == 0 {
			// Default 1:1 if no mapping map supplied
			target = h
		}

		if target != "" && strings.ToUpper(target) != "SKIP" && target != "-" {
			activeCols = append(activeCols, mappedCol{
				csvIdx:    i,
				targetCol: target,
			})
		}
	}

	if len(activeCols) == 0 {
		writeError(w, http.StatusBadRequest, "no columns mapped for import")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	if dbName != "" {
		_ = sess.Session.SelectDatabase(ctx, dbName)
	}

	targetColNames := make([]string, len(activeCols))
	for i, c := range activeCols {
		targetColNames[i] = c.targetCol
	}

	// Batch configuration
	// ponytail: 100 rows per batch ensures parameter count stays well under SQLite 999 max limit.
	batchLimit := 100
	if len(activeCols) > 9 {
		batchLimit = 900 / len(activeCols)
		if batchLimit < 10 {
			batchLimit = 10
		}
	}

	totalRows := 0
	importedRows := 0
	skippedRows := 0
	var errorMessages []string

	var currentBatch [][]any

	flushBatch := func() error {
		if len(currentBatch) == 0 {
			return nil
		}
		imported, errs := executeBatchInsert(ctx, sess.Session, tableName, targetColNames, currentBatch, skipErrorsParam)
		importedRows += imported
		skippedRows += len(currentBatch) - imported
		if len(errs) > 0 {
			for _, e := range errs {
				if len(errorMessages) < 20 {
					errorMessages = append(errorMessages, e)
				}
			}
			if !skipErrorsParam {
				return fmt.Errorf("%s", errs[0])
			}
		}
		currentBatch = currentBatch[:0]
		return nil
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		totalRows++
		if err != nil {
			skippedRows++
			if len(errorMessages) < 20 {
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d parse error: %v", totalRows, err))
			}
			if !skipErrorsParam {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("error on row %d: %v", totalRows, err))
				return
			}
			continue
		}

		rowVals := make([]any, len(activeCols))
		for cIdx, col := range activeCols {
			if col.csvIdx < len(record) {
				raw := record[col.csvIdx]
				if emptyAsNullParam && strings.TrimSpace(raw) == "" {
					rowVals[cIdx] = nil
				} else {
					// Raw string preserves leading zeros (e.g. "08123456789")
					rowVals[cIdx] = raw
				}
			} else {
				if emptyAsNullParam {
					rowVals[cIdx] = nil
				} else {
					rowVals[cIdx] = ""
				}
			}
		}

		currentBatch = append(currentBatch, rowVals)
		if len(currentBatch) >= batchLimit {
			if err := flushBatch(); err != nil {
				writeError(w, http.StatusInternalServerError, "batch import failed: "+err.Error())
				return
			}
		}
	}

	if len(currentBatch) > 0 {
		if err := flushBatch(); err != nil {
			writeError(w, http.StatusInternalServerError, "batch import failed: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       importedRows > 0 || totalRows == 0,
		"delimiter":     string(comma),
		"total_rows":    totalRows,
		"imported_rows": importedRows,
		"skipped_rows":  skippedRows,
		"errors":        errorMessages,
	})
}

// executeBatchInsert executes a multi-row parameterized INSERT.
// If skipErrors is true and the batch fails, it falls back to row-by-row insertion.
func executeBatchInsert(ctx context.Context, sess driver.Session, tableName string, colNames []string, batch [][]any, skipErrors bool) (int, []string) {
	if len(batch) == 0 {
		return 0, nil
	}

	var sb strings.Builder
	sb.WriteString("INSERT INTO `")
	sb.WriteString(strings.ReplaceAll(tableName, "`", "``"))
	sb.WriteString("` (")
	for i, c := range colNames {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("`")
		sb.WriteString(strings.ReplaceAll(c, "`", "``"))
		sb.WriteString("`")
	}
	sb.WriteString(") VALUES ")

	var args []any
	for rIdx, row := range batch {
		if rIdx > 0 {
			sb.WriteString(", (")
		} else {
			sb.WriteString("(")
		}
		for cIdx, val := range row {
			if cIdx > 0 {
				sb.WriteString(", ?")
			} else {
				sb.WriteString("?")
			}
			args = append(args, val)
		}
		sb.WriteString(")")
	}

	_, err := sess.Exec(ctx, sb.String(), args...)
	if err == nil {
		return len(batch), nil
	}

	if !skipErrors {
		return 0, []string{err.Error()}
	}

	// Fallback to row-by-row execution for this batch
	var rowErrors []string
	imported := 0

	var singleSb strings.Builder
	singleSb.WriteString("INSERT INTO `")
	singleSb.WriteString(strings.ReplaceAll(tableName, "`", "``"))
	singleSb.WriteString("` (")
	for i, c := range colNames {
		if i > 0 {
			singleSb.WriteString(", ")
		}
		singleSb.WriteString("`")
		singleSb.WriteString(strings.ReplaceAll(c, "`", "``"))
		singleSb.WriteString("`")
	}
	singleSb.WriteString(") VALUES (")
	for i := range colNames {
		if i > 0 {
			singleSb.WriteString(", ?")
		} else {
			singleSb.WriteString("?")
		}
	}
	singleSb.WriteString(")")
	singleSQL := singleSb.String()

	for _, row := range batch {
		_, rErr := sess.Exec(ctx, singleSQL, row...)
		if rErr == nil {
			imported++
		} else {
			if len(rowErrors) < 10 {
				rowErrors = append(rowErrors, rErr.Error())
			}
		}
	}

	return imported, rowErrors
}
