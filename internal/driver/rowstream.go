package driver

import (
	"database/sql"
	"io"
)

// SQLRowStream provides canonical chunked row streaming across all standard SQL drivers.
type SQLRowStream struct {
	rows *sql.Rows
	cols []ColumnDefinition
}

// NewSQLRowStream creates a shared RowStream adapter from standard sql.Rows.
func NewSQLRowStream(rows *sql.Rows, cols []ColumnDefinition) RowStream {
	return &SQLRowStream{
		rows: rows,
		cols: cols,
	}
}

func (r *SQLRowStream) Columns() []ColumnDefinition {
	return r.cols
}

func (r *SQLRowStream) NextChunk(batchSize int) ([][]any, error) {
	var chunk [][]any
	count := len(r.cols)

	for len(chunk) < batchSize {
		if !r.rows.Next() {
			if len(chunk) > 0 {
				return chunk, nil
			}
			return nil, io.EOF
		}

		row := make([]any, count)
		valuePtrs := make([]any, count)
		for i := range row {
			valuePtrs[i] = &row[i]
		}

		if err := r.rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		// Convert raw []byte to string in-place for JSON safety
		for i, val := range row {
			if b, ok := val.([]byte); ok {
				row[i] = string(b)
			}
		}
		chunk = append(chunk, row)
	}

	return chunk, nil
}

func (r *SQLRowStream) Close() error {
	return r.rows.Close()
}
