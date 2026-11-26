package driver

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func EscapeSQLString(s string, driverName string) string {
	switch strings.ToLower(driverName) {
	case "mysql", "mariadb":
		return EscapeMySQLStringWithMode(s, false)
	case "postgres", "postgresql", "pgx", "pg":
		// Standard SQL doubles single quotes. When standard_conforming_strings is OFF (legacy
		// servers), a backslash is also an escape, so a value containing '\' must use the explicit
		// escape-string syntax E'...' with both ' and \ doubled/escaped to stay unambiguous under
		// either setting.
		if strings.IndexByte(s, '\\') < 0 {
			return "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
		r := strings.NewReplacer(`'`, `''`, `\`, `\\`)
		return "E'" + r.Replace(s) + "'"
	default:
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
}

// EscapeMySQLStringWithMode escapes a MySQL/MariaDB string literal, respecting NO_BACKSLASH_ESCAPES.
func EscapeMySQLStringWithMode(s string, noBackslashEscapes bool) string {
	if noBackslashEscapes {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	var sb strings.Builder
	sb.Grow(len(s) + 2)
	sb.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch b {
		case 0:
			sb.WriteString(`\0`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\\':
			sb.WriteString(`\\`)
		case '\'':
			sb.WriteString(`''`)
		case '\032':
			sb.WriteString(`\Z`)
		default:
			sb.WriteByte(b)
		}
	}
	sb.WriteByte('\'')
	return sb.String()
}

// FormatSQLValueForDriver formats a Go value into a valid SQL literal for a specific driver dialect.
func FormatSQLValueForDriver(val any, driverName string) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return formatSpecialFloat(f, driverName)
		}
		return strconv.FormatFloat(f, 'f', -1, 32)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return formatSpecialFloat(v, driverName)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	case time.Time:
		switch strings.ToLower(driverName) {
		case "postgres", "postgresql", "pgx", "pg":
			return EscapeSQLString(v.Format(time.RFC3339Nano), driverName)
		default:
			return EscapeSQLString(v.Format("2006-01-02 15:04:05.999999"), driverName)
		}
	case []byte:
		return hexLiteral(v, driverName)
	case string:
		// Streamed rows carry binary columns as Go strings; NUL or non-UTF-8 bytes cannot
		// round-trip through a quoted literal, so emit them as a binary hex literal.
		if strings.IndexByte(v, 0) >= 0 || !utf8.ValidString(v) {
			return hexLiteral([]byte(v), driverName)
		}
		return EscapeSQLString(v, driverName)
	default:
		return EscapeSQLString(fmt.Sprintf("%v", v), driverName)
	}
}

// hexLiteral renders raw bytes as a dialect-correct binary literal (PostgreSQL X” is a bit string, not bytea).
func hexLiteral(b []byte, driverName string) string {
	switch strings.ToLower(driverName) {
	case "postgres", "postgresql", "pgx", "pg":
		return fmt.Sprintf("'\\x%x'::bytea", b)
	default:
		return fmt.Sprintf("X'%X'", b)
	}
}

// FormatSQLValue formats a Go value into a valid SQL literal for dumps, inserts, and filters.
// ponytail: Uses 'f' formatting to avoid scientific notation on large numeric IDs and metrics.
func FormatSQLValue(val any) string {
	return FormatSQLValueForDriver(val, "")
}

// SequenceResetStatements returns PostgreSQL statements that advance each serial/nextval column's
// sequence past the dumped rows, so inserts after a restore don't collide with restored ids.
// Other drivers need nothing (AUTO_INCREMENT / rowid track the max automatically).

func formatSpecialFloat(f float64, driverName string) string {
	d := strings.ToLower(driverName)
	isPG := strings.HasPrefix(d, "postgre") || d == "pg" || d == "pgx"
	if !isPG {
		return "NULL"
	}
	if math.IsNaN(f) {
		return "'NaN'::float8"
	}
	if math.IsInf(f, 1) {
		return "'Infinity'::float8"
	}
	return "'-Infinity'::float8"
}
