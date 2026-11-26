package dataimport

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/herliansyah/sqlfox/internal/driver"
)

// ImportOptions defines configuration for the CSV Ingestion Engine.
type ImportOptions struct {
	Delimiter   rune              `json:"delimiter,omitempty"`
	EmptyAsNull bool              `json:"empty_as_null"`
	SkipErrors  bool              `json:"skip_errors"`
	Mapping     map[string]string `json:"mapping,omitempty"`
	DriverName  string            `json:"driver_name,omitempty"`
	TxChunkSize int               `json:"tx_chunk_size,omitempty"`
}

// ImportResult contains statistics and errors from a completed import.
type ImportResult struct {
	Success        bool     `json:"success"`
	PartialSuccess bool     `json:"partial_success,omitempty"`
	Delimiter      string   `json:"delimiter"`
	TotalRows      int      `json:"total_rows"`
	ImportedRows   int      `json:"imported_rows"`
	SkippedRows    int      `json:"skipped_rows"`
	Errors         []string `json:"errors"`
}

// SniffDelimiter detects the delimiter among standard CSV separators while ignoring characters inside quotes.
// ponytail: Quote-aware frequency scan across up to 5 logical lines.
func SniffDelimiter(sample []byte) rune {
	delims := []rune{',', ';', '\t', '|'}
	counts := make(map[rune]int, len(delims))

	s := string(sample)
	inQuotes := false
	linesSeen := 0
	for i := 0; i < len(s) && linesSeen < 5; i++ {
		c := s[i]
		if c == '"' {
			if inQuotes && i+1 < len(s) && s[i+1] == '"' {
				i++ // escaped quote
				continue
			}
			inQuotes = !inQuotes
			continue
		}
		if inQuotes {
			continue
		}
		if c == '\n' {
			linesSeen++
			continue
		}
		switch c {
		case ',', ';', '\t', '|':
			counts[rune(c)]++
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

// Import reads CSV records from r and inserts them into tableName on the given driver session.
func Import(ctx context.Context, sess driver.Session, tableName string, r io.Reader, opts ImportOptions) (*ImportResult, error) {
	if tableName == "" {
		return nil, fmt.Errorf("table name is required")
	}

	// 1. Read full initial prefix (up to 4096 bytes) for delimiter sniffing and BOM stripping.
	// Using io.ReadFull avoids short-read splits on chunked HTTP streams.
	prefix := make([]byte, 4096)
	n, readErr := io.ReadFull(r, prefix)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read CSV stream: %w", readErr)
	}
	prefix = prefix[:n]

	// Strip UTF-8 BOM if present
	prefix = bytes.TrimPrefix(prefix, []byte("\xef\xbb\xbf"))

	// 2. Resolve delimiter
	comma := opts.Delimiter
	if comma == 0 {
		comma = SniffDelimiter(prefix)
	}

	// 3. Recompose stream: prefix + remaining body
	fullReader := io.MultiReader(bytes.NewReader(prefix), r)
	csvReader := csv.NewReader(fullReader)
	csvReader.Comma = comma
	csvReader.LazyQuotes = true
	csvReader.FieldsPerRecord = -1

	// 4. Read CSV header row
	headers, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}

	// 5. Parse Column Mapping Matrix
	type mappedCol struct {
		csvIdx    int
		targetCol string
	}
	var activeCols []mappedCol
	for i, h := range headers {
		target := ""
		// Priority 1: mapping by numeric index "0", "1"
		if val, ok := opts.Mapping[strconv.Itoa(i)]; ok {
			target = strings.TrimSpace(val)
		} else if val, ok := opts.Mapping[h]; ok {
			// Priority 2: mapping by header text
			target = strings.TrimSpace(val)
		} else if len(opts.Mapping) == 0 {
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
		return nil, fmt.Errorf("no columns mapped for import")
	}

	targetColNames := make([]string, len(activeCols))
	for i, c := range activeCols {
		targetColNames[i] = c.targetCol
	}

	// 6. Dynamic Batch Configuration
	// ponytail: 100 rows per batch ensures parameter count stays well under SQLite 999 max limit.
	batchLimit := 100
	if len(activeCols) > 9 {
		batchLimit = 900 / len(activeCols)
		if batchLimit < 1 {
			batchLimit = 1
		}
	}

	// Fail-fast imports run in bounded chunked transactions (default 5000 rows per tx) so small imports
	// remain atomic while multi-million-row imports do not exhaust undo/WAL logs or hold unbounded locks.
	// skip_errors imports stay autocommit: a failed row would abort a PostgreSQL transaction.
	txChunkSize := opts.TxChunkSize
	if txChunkSize <= 0 {
		txChunkSize = 5000
	}

	var activeTx driver.TxSession
	txRows := 0
	rollbackTx := func() {
		if activeTx != nil {
			_ = activeTx.Rollback()
			activeTx = nil
			txRows = 0
		}
	}
	defer rollbackTx()

	ensureTx := func() (execer, error) {
		if opts.SkipErrors {
			return sess, nil
		}
		if activeTx == nil {
			tx, err := sess.BeginTx(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to begin import transaction: %w", err)
			}
			activeTx = tx
			txRows = 0
		}
		return activeTx, nil
	}

	commitChunk := func() error {
		if activeTx == nil {
			return nil
		}
		if err := activeTx.Commit(); err != nil {
			rollbackTx()
			return err
		}
		activeTx = nil
		txRows = 0
		return nil
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
		target, err := ensureTx()
		if err != nil {
			return err
		}
		imported, errs := executeBatchInsert(ctx, target, tableName, targetColNames, currentBatch, opts.SkipErrors, opts.DriverName)
		importedRows += imported
		skippedRows += len(currentBatch) - imported
		if len(errs) > 0 {
			for _, e := range errs {
				if len(errorMessages) < 20 {
					errorMessages = append(errorMessages, e)
				}
			}
			if !opts.SkipErrors {
				rollbackTx()
				return fmt.Errorf("%s", errs[0])
			}
		}
		batchLen := len(currentBatch)
		currentBatch = currentBatch[:0]
		if !opts.SkipErrors {
			txRows += batchLen
			if txRows >= txChunkSize {
				if err := commitChunk(); err != nil {
					return fmt.Errorf("failed to commit import chunk: %w", err)
				}
			}
		}
		return nil
	}

	// 7. Stream Records
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
			if !opts.SkipErrors {
				rollbackTx()
				return nil, fmt.Errorf("error on row %d: %w", totalRows, err)
			}
			continue
		}

		rowVals := make([]any, len(activeCols))
		for cIdx, col := range activeCols {
			if col.csvIdx < len(record) {
				raw := record[col.csvIdx]
				// Only truly empty strings map to NULL; preserve intentional whitespace such as "   ".
				if opts.EmptyAsNull && raw == "" {
					rowVals[cIdx] = nil
				} else {
					// Raw string preserves leading zeros (e.g. "08123456789") and intentional whitespace.
					rowVals[cIdx] = raw
				}
			} else {
				if opts.EmptyAsNull {
					rowVals[cIdx] = nil
				} else {
					rowVals[cIdx] = ""
				}
			}
		}

		currentBatch = append(currentBatch, rowVals)
		if len(currentBatch) >= batchLimit {
			if err := flushBatch(); err != nil {
				return nil, fmt.Errorf("batch import failed: %w", err)
			}
		}
	}

	if len(currentBatch) > 0 {
		if err := flushBatch(); err != nil {
			return nil, fmt.Errorf("batch import failed: %w", err)
		}
	}

	if err := commitChunk(); err != nil {
		return nil, fmt.Errorf("failed to commit import: %w", err)
	}

	hasFailures := skippedRows > 0 || len(errorMessages) > 0
	return &ImportResult{
		Success:        !hasFailures,
		PartialSuccess: importedRows > 0 && hasFailures,
		Delimiter:      string(comma),
		TotalRows:      totalRows,
		ImportedRows:   importedRows,
		SkippedRows:    skippedRows,
		Errors:         errorMessages,
	}, nil
}

// quoteTableIdent quotes both unqualified ("users") and schema-qualified ("public.users") table names.
func quoteTableIdent(tableName, drv string) string {
	trimmed := strings.TrimSpace(tableName)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) == 1 {
		return driver.QuoteIdentifier(trimmed, drv)
	}
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		quoted = append(quoted, driver.QuoteIdentifier(p, drv))
	}
	if len(quoted) == 0 {
		return driver.QuoteIdentifier(trimmed, drv)
	}
	return strings.Join(quoted, ".")
}

// executeBatchInsert executes a multi-row parameterized INSERT.
// If skipErrors is true and the batch fails, it falls back to row-by-row insertion.
// execer is satisfied by both driver.Session and driver.TxSession.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (*driver.ExecResult, error)
}

func executeBatchInsert(ctx context.Context, sess execer, tableName string, colNames []string, batch [][]any, skipErrors bool, driverName ...string) (int, []string) {
	if len(batch) == 0 {
		return 0, nil
	}

	drv := "mysql"
	if len(driverName) > 0 && driverName[0] != "" {
		drv = driverName[0]
	}

	quotedTable := quoteTableIdent(tableName, drv)

	var sb strings.Builder
	sb.WriteString("INSERT INTO ")
	sb.WriteString(quotedTable)
	sb.WriteString(" (")
	for i, c := range colNames {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(driver.QuoteIdentifier(c, drv))
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
				sb.WriteString(", ")
			}
			args = append(args, val)
			sb.WriteString(driver.Placeholder(drv, len(args)))
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
	singleSb.WriteString("INSERT INTO ")
	singleSb.WriteString(quotedTable)
	singleSb.WriteString(" (")
	for i, c := range colNames {
		if i > 0 {
			singleSb.WriteString(", ")
		}
		singleSb.WriteString(driver.QuoteIdentifier(c, drv))
	}
	singleSb.WriteString(") VALUES (")
	for i := range colNames {
		if i > 0 {
			singleSb.WriteString(", ")
		}
		singleSb.WriteString(driver.Placeholder(drv, i+1))
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
