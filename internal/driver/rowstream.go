package driver

import (
	"database/sql"
	"encoding/hex"
	"io"
	"strconv"
	"strings"
)

const (
	maxSafeJSInt int64  = 9007199254740991 // 2^53 - 1
	minSafeJSInt int64  = -9007199254740991
	maxSafeJSU64 uint64 = 9007199254740991
)

// isBinaryDBType reports whether a database column type carries opaque binary bytes
// that must remain []byte rather than being coerced to a UTF-8 Go string.
func isBinaryDBType(dbType string) bool {
	up := strings.ToUpper(strings.TrimSpace(dbType))
	if up == "" {
		return false
	}
	return strings.Contains(up, "BLOB") ||
		strings.Contains(up, "BYTEA") ||
		strings.Contains(up, "BINARY")
}

// FormatValueForJSON makes a scanned cell value safe for JavaScript JSON.parse consumers:
//   - 64-bit integers outside IEEE-754's exact integer range ±(2^53-1) are stringified
//     so Snowflake/BIGINT IDs are never silently rounded in the browser.
//   - Raw binary []byte values are rendered as 0x<HEX> strings instead of opaque base64.
func FormatValueForJSON(val any) any {
	switch v := val.(type) {
	case int64:
		if v > maxSafeJSInt || v < minSafeJSInt {
			return strconv.FormatInt(v, 10)
		}
		return v
	case uint64:
		if v > maxSafeJSU64 {
			return strconv.FormatUint(v, 10)
		}
		return v
	case []byte:
		return "0x" + strings.ToUpper(hex.EncodeToString(v))
	default:
		return val
	}
}

// FormatRowForJSON returns a JSON-safe copy/normalized slice for a single row.
func FormatRowForJSON(row []any) []any {
	if len(row) == 0 {
		return row
	}
	out := make([]any, len(row))
	for i, v := range row {
		out[i] = FormatValueForJSON(v)
	}
	return out
}

// FormatRowsForJSON normalizes a batch of rows for browser JSON serialization.
func FormatRowsForJSON(rows [][]any) [][]any {
	if len(rows) == 0 {
		return rows
	}
	out := make([][]any, len(rows))
	for i, r := range rows {
		out[i] = FormatRowForJSON(r)
	}
	return out
}

// SQLRowStream provides canonical chunked row streaming across all standard SQL drivers.
type SQLRowStream struct {
	rows    *sql.Rows
	cols    []ColumnDefinition
	isBin   []bool
	cleanup func()
}

// NewSQLRowStream creates a shared RowStream adapter from standard sql.Rows.
func NewSQLRowStream(rows *sql.Rows, cols []ColumnDefinition, cleanup ...func()) RowStream {
	var cl func()
	if len(cleanup) > 0 {
		cl = cleanup[0]
	}
	isBin := make([]bool, len(cols))
	for i, c := range cols {
		isBin[i] = isBinaryDBType(c.Type)
	}
	return &SQLRowStream{
		rows:    rows,
		cols:    cols,
		isBin:   isBin,
		cleanup: cl,
	}
}

func (r *SQLRowStream) Columns() []ColumnDefinition {
	return r.cols
}

func (r *SQLRowStream) NextChunk(batchSize int) ([][]any, error) {
	if batchSize <= 0 {
		// A non-positive size would return (nil, nil) forever and spin callers that loop until EOF.
		batchSize = 1
	}
	chunk := make([][]any, 0, batchSize)
	count := len(r.cols)
	valuePtrs := make([]any, count)

	for len(chunk) < batchSize {
		if !r.rows.Next() {
			if err := r.rows.Err(); err != nil {
				return chunk, err
			}
			if len(chunk) > 0 {
				return chunk, nil
			}
			return nil, io.EOF
		}

		row := make([]any, count)
		for i := range row {
			valuePtrs[i] = &row[i]
		}

		if err := r.rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		// Preserve raw []byte on explicitly binary columns (BLOB/BYTEA/BINARY) so dumps
		// and cross-database transfers stay lossless; coerce text/decimal []byte to string.
		for i, val := range row {
			if b, ok := val.([]byte); ok {
				if i < len(r.isBin) && r.isBin[i] {
					cp := make([]byte, len(b))
					copy(cp, b)
					row[i] = cp
				} else {
					row[i] = string(b)
				}
			}
		}
		chunk = append(chunk, row)
	}

	return chunk, nil
}

func (r *SQLRowStream) Close() error {
	if r.cleanup != nil {
		defer r.cleanup()
	}
	return r.rows.Close()
}
