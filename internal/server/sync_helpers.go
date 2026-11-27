package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

func isPgDriver(d string) bool {
	return strings.EqualFold(d, "postgres") || strings.EqualFold(d, "postgresql") || strings.EqualFold(d, "pg")
}

func isSqliteDriver(d string) bool {
	return strings.EqualFold(d, "sqlite") || strings.EqualFold(d, "sqlite3")
}

// qualifyTable mirrors the PG guard in handlers_table_data.go: on PostgreSQL the incoming db
// may be the physical database name (not a schema); prefixing "mydb"."users" fails when no
// schema named "mydb" exists, so skip the prefix when it matches the session's active DB or
// the default "public" schema.
func qualifyTable(sess *ActiveSession, db, tbl, drv string) string {
	qTbl := quoteIdent(tbl, drv)
	if db == "" || isSqliteDriver(drv) {
		return qTbl
	}
	if isPgDriver(drv) && (strings.EqualFold(db, sess.GetDatabaseName()) || strings.EqualFold(db, "public")) {
		return qTbl
	}
	return quoteIdent(db, drv) + "." + qTbl
}

var (
	reMySQLTypeModifiers = regexp.MustCompile(`(?i)\s+(unsigned|zerofill|signed)\b`)
	reSafeOnUpdate       = regexp.MustCompile(`(?i)^ON\s+UPDATE\s+(CURRENT_TIMESTAMP|LOCALTIMESTAMP|NOW)(\s*\(\s*\d*\s*\))?$`)
)

// hasUnsafeSQLFragment reports whether s contains a statement terminator (;) or SQL comment (-- or /*)
// outside single-quoted string literals. Used to reject DDL injection from untrusted source metadata.
func hasUnsafeSQLFragment(s string) bool {
	inSingle := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			if inSingle && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inSingle = !inSingle
			continue
		}
		if inSingle {
			if c == '\\' && i+1 < len(s) {
				i++
			}
			continue
		}
		if c == ';' || c == '\x00' {
			return true
		}
		if c == '-' && i+1 < len(s) && s[i+1] == '-' {
			return true
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			return true
		}
	}
	return inSingle
}

// sanitizeColumnType rejects untrusted column types containing DDL injection syntax (semicolons,
// SQL comments, unbalanced quotes/parens) and falls back to TEXT.
func sanitizeColumnType(raw string) string {
	t := strings.TrimSpace(raw)
	if t == "" || hasUnsafeSQLFragment(t) {
		return "TEXT"
	}
	depth := 0
	inSingle := false
	for i := 0; i < len(t); i++ {
		c := t[i]
		if c == '\'' {
			if inSingle && i+1 < len(t) && t[i+1] == '\'' {
				i++
				continue
			}
			inSingle = !inSingle
			continue
		}
		if inSingle {
			if c == '\\' && i+1 < len(t) {
				i++
			}
			continue
		}
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return "TEXT"
			}
		}
	}
	if depth != 0 {
		return "TEXT"
	}
	return t
}

// normalizeColumnType maps a source column type onto a type the target driver accepts.
// ponytail: covers the common MySQL<->PostgreSQL<->SQLite types; exotic types pass through sanitized.
func normalizeColumnType(t string, driverName string) string {
	raw := sanitizeColumnType(t)
	upper := strings.ToUpper(raw)

	if isSqliteDriver(driverName) {
		if strings.Contains(upper, "INT") {
			return "INTEGER"
		}
		if strings.Contains(upper, "TEXT") || strings.Contains(upper, "CHAR") || strings.HasPrefix(upper, "ENUM") || strings.HasPrefix(upper, "SET") || strings.HasPrefix(upper, "JSON") || upper == "UUID" {
			return "TEXT"
		}
		if strings.HasPrefix(upper, "DOUBLE") || strings.HasPrefix(upper, "FLOAT") || strings.HasPrefix(upper, "REAL") {
			return "REAL"
		}
		if strings.HasPrefix(upper, "DECIMAL") || strings.HasPrefix(upper, "NUMERIC") {
			return "NUMERIC"
		}
		if strings.Contains(upper, "BLOB") || strings.Contains(upper, "BINARY") || upper == "BYTEA" {
			return "BLOB"
		}
		return raw
	}

	if isPgDriver(driverName) {
		base := strings.TrimSpace(reMySQLTypeModifiers.ReplaceAllString(upper, ""))
		unsigned := base != upper
		switch {
		case base == "TINYINT(1)":
			return "SMALLINT"
		case strings.HasPrefix(base, "BIGINT"):
			if unsigned {
				return "NUMERIC(20)"
			}
			return "BIGINT"
		case strings.HasPrefix(base, "MEDIUMINT"):
			return "INTEGER"
		case strings.HasPrefix(base, "INT(") || base == "INT" || base == "INTEGER":
			if unsigned {
				return "BIGINT"
			}
			return "INTEGER"
		case strings.HasPrefix(base, "TINYINT") || strings.HasPrefix(base, "SMALLINT"):
			if unsigned && strings.HasPrefix(base, "SMALLINT") {
				return "INTEGER"
			}
			return "SMALLINT"
		case base == "YEAR" || strings.HasPrefix(base, "YEAR("):
			return "SMALLINT"
		case strings.Contains(base, "TEXT") || strings.HasPrefix(base, "ENUM") || strings.HasPrefix(base, "SET("):
			return "TEXT"
		case base == "DATETIME":
			return "TIMESTAMP"
		case strings.HasPrefix(base, "DATETIME("):
			return "TIMESTAMP" + strings.TrimPrefix(base, "DATETIME")
		case base == "DOUBLE" || strings.HasPrefix(base, "DOUBLE("):
			return "DOUBLE PRECISION"
		case base == "FLOAT" || strings.HasPrefix(base, "FLOAT("):
			return "REAL"
		case strings.Contains(base, "BLOB") || strings.Contains(base, "BINARY"):
			return "BYTEA"
		}
		if unsigned {
			return base
		}
		return raw
	}

	// MySQL/MariaDB target: map PostgreSQL information_schema type names (never produced by MySQL).
	switch strings.ToLower(raw) {
	case "integer":
		return "INT"
	case "boolean":
		return "TINYINT(1)"
	case "character varying", "character", "uuid":
		if strings.ToLower(raw) == "uuid" {
			return "CHAR(36)"
		}
		return "TEXT"
	case "timestamp without time zone":
		return "DATETIME(6)"
	case "timestamp with time zone":
		return "TIMESTAMP(6)"
	case "time without time zone", "time with time zone":
		return "TIME(6)"
	case "double precision":
		return "DOUBLE"
	case "real":
		return "FLOAT"
	case "numeric":
		return "DECIMAL(65,30)"
	case "bytea":
		return "LONGBLOB"
	case "jsonb", "json":
		return "JSON"
	}
	return raw
}

var (
	rePgCast          = regexp.MustCompile(`::[A-Za-z_][\w ]*(\[\])?(\(\s*\d+(\s*,\s*\d+)?\s*\))?`)
	reNumericLiteral  = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][-+]?\d+)?$`)
	reFuncCallExpr    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\(.*\)$`) // MariaDB unquoted expression default, e.g. uuid()
	reCurrentTimeExpr = regexp.MustCompile(`(?i)^(CURRENT_TIMESTAMP|CURRENT_DATE|CURRENT_TIME|LOCALTIMESTAMP|LOCALTIME|NOW)\s*(\(\s*\d*\s*\))?$`)
)

// renderColumnDefault returns the DEFAULT expression (without the keyword) for col as read from
// srcDriver, rendered for tgtDriver; "" means no default. MySQL's information_schema returns literal
// defaults unquoted and marks expression defaults with EXTRA=DEFAULT_GENERATED, while PostgreSQL and
// SQLite already return SQL expressions (PostgreSQL with ::type casts).
func renderColumnDefault(col driver.ColumnMetadata, srcDriver, tgtDriver string) string {
	v := strings.TrimSpace(col.DefaultValue)
	if v == "" {
		return ""
	}
	if strings.EqualFold(v, "NULL") {
		return "NULL"
	}
	if reCurrentTimeExpr.MatchString(v) {
		if !strings.EqualFold(srcDriver, tgtDriver) && (isPgDriver(tgtDriver) || isSqliteDriver(tgtDriver)) {
			if strings.HasPrefix(strings.ToUpper(v), "CURRENT_TIMESTAMP") || strings.HasPrefix(strings.ToUpper(v), "NOW") {
				return "CURRENT_TIMESTAMP"
			}
		}
		return v
	}

	srcIsMySQL := !isPgDriver(srcDriver) && !isSqliteDriver(srcDriver)
	if srcIsMySQL {
		if strings.Contains(strings.ToUpper(col.Extra), "DEFAULT_GENERATED") {
			// MySQL 8 expression default: must be parenthesized when re-emitted; reject injection payloads.
			if hasUnsafeSQLFragment(v) {
				return ""
			}
			return "(" + v + ")"
		}
		// MariaDB already returns quoted literals / expressions; bit literals are SQL as-is.
		if strings.HasPrefix(v, "'") || strings.HasPrefix(strings.ToLower(v), "b'") || reFuncCallExpr.MatchString(v) {
			if hasUnsafeSQLFragment(v) {
				return ""
			}
			return v
		}
		if reNumericLiteral.MatchString(v) {
			return v
		}
		return driver.EscapeSQLString(v, tgtDriver)
	}

	// PostgreSQL / SQLite source: already an SQL expression.
	if hasUnsafeSQLFragment(v) {
		return ""
	}
	if isPgDriver(srcDriver) && !isPgDriver(tgtDriver) {
		if strings.HasPrefix(strings.ToLower(v), "nextval(") {
			return "" // sequence default has no portable equivalent (auto-increment is a column property)
		}
		v = strings.TrimSpace(stripPgCasts(v))
		if strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")") && strings.Count(v, "(") == 1 {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
		if strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") && len(v) >= 2 && !isSqliteDriver(tgtDriver) {
			// Re-escape for MySQL (backslashes are escapes there).
			return driver.EscapeSQLString(strings.ReplaceAll(v[1:len(v)-1], "''", "'"), tgtDriver)
		}
	}
	return v
}

// stripPgCasts removes ::type casts outside quoted literals ('a::b'::text keeps 'a::b').
func stripPgCasts(v string) string {
	var out strings.Builder
	for i := 0; i < len(v); {
		if v[i] == '\'' {
			j := i + 1
			for j < len(v) {
				if v[j] == '\'' {
					if j+1 < len(v) && v[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			end := min(j+1, len(v))
			out.WriteString(v[i:end])
			i = end
			continue
		}
		next := strings.IndexByte(v[i:], '\'')
		if next < 0 {
			next = len(v) - i
		}
		out.WriteString(rePgCast.ReplaceAllString(v[i:i+next], ""))
		i += next
	}
	return out.String()
}

// canonDefault folds equivalent current-time spellings so they don't count as a difference.
func canonDefault(d string) string {
	if reCurrentTimeExpr.MatchString(d) && (strings.HasPrefix(strings.ToUpper(d), "CURRENT_TIMESTAMP") || strings.HasPrefix(strings.ToUpper(d), "NOW")) {
		return "CURRENT_TIMESTAMP"
	}
	return d
}

// extraClause keeps only the MySQL EXTRA attributes that are valid in a column definition
// (auto_increment, ON UPDATE ..., INVISIBLE); DEFAULT_GENERATED and VIRTUAL/STORED GENERATED are
// metadata flags, not syntax.
func extraClause(extra string) string {
	e := strings.TrimSpace(extra)
	if e == "" {
		return ""
	}
	upper := strings.ToUpper(e)
	var parts []string
	if strings.Contains(upper, "AUTO_INCREMENT") {
		parts = append(parts, "AUTO_INCREMENT")
	}
	if i := strings.Index(upper, "ON UPDATE "); i >= 0 {
		rest := e[i:]
		if j := strings.Index(strings.ToUpper(rest), " INVISIBLE"); j >= 0 {
			rest = rest[:j]
		}
		rest = strings.TrimSpace(rest)
		if reSafeOnUpdate.MatchString(rest) {
			parts = append(parts, rest)
		}
	}
	if strings.Contains(upper, "INVISIBLE") {
		parts = append(parts, "INVISIBLE")
	}
	return strings.Join(parts, " ")
}

func buildColumnDefinition(col driver.ColumnMetadata, srcDriver, driverName string) string {
	clause := normalizeColumnType(col.Type, driverName)
	if !col.IsNullable {
		clause += " NOT NULL"
	}
	if def := renderColumnDefault(col, srcDriver, driverName); def != "" {
		clause += " DEFAULT " + def
	}
	srcIsMySQL := !isPgDriver(srcDriver) && !isSqliteDriver(srcDriver)
	if srcIsMySQL && !isPgDriver(driverName) && !isSqliteDriver(driverName) {
		if x := extraClause(col.Extra); x != "" {
			clause += " " + x
		}
	}
	return clause
}

func buildCreateTableDDL(tbl driver.TableMetadata, srcDriver, driverName string) string {
	var lines []string
	var pkCols []string

	for _, col := range tbl.Columns {
		def := fmt.Sprintf("  %s %s", quoteIdent(col.Name, driverName), buildColumnDefinition(col, srcDriver, driverName))
		lines = append(lines, def)
		if col.IsPrimaryKey {
			pkCols = append(pkCols, quoteIdent(col.Name, driverName))
		}
	}

	if len(pkCols) > 0 {
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n);", quoteIdent(tbl.Name, driverName), strings.Join(lines, ",\n"))
}

// columnEquals compares a source column with a target column as the target would see them,
// so cross-driver syncs don't flag every column merely for spelling types/defaults differently.
func columnEquals(src, tgt driver.ColumnMetadata, srcDriver, tgtDriver string) bool {
	if !strings.EqualFold(normalizeColumnType(src.Type, tgtDriver), normalizeColumnType(tgt.Type, tgtDriver)) {
		return false
	}
	if src.IsNullable != tgt.IsNullable {
		return false
	}
	return strings.EqualFold(canonDefault(renderColumnDefault(src, srcDriver, tgtDriver)), canonDefault(renderColumnDefault(tgt, tgtDriver, tgtDriver)))
}

// maxJSONSafeInt is the largest integer a browser's JSON.parse (float64) represents exactly.
const maxJSONSafeInt = 1<<53 - 1

// syncWireValue makes a scanned value survive the browser round-trip between compare and apply:
// integers beyond 2^53 travel as decimal strings and non-UTF-8 (binary) values as {"$base64": ...}
// (encoding/json would otherwise replace invalid bytes with U+FFFD). fromSyncWire reverses this.
func syncWireValue(v any) any {
	switch x := v.(type) {
	case int64:
		if x > maxJSONSafeInt || x < -maxJSONSafeInt {
			return strconv.FormatInt(x, 10)
		}
	case uint64:
		if x > maxJSONSafeInt {
			return strconv.FormatUint(x, 10)
		}
	case []byte:
		if !utf8.Valid(x) {
			return map[string]string{"$base64": base64.StdEncoding.EncodeToString(x)}
		}
		return string(x)
	case string:
		if !utf8.ValidString(x) {
			return map[string]string{"$base64": base64.StdEncoding.EncodeToString([]byte(x))}
		}
	}
	return v
}

func syncWireRow(row []any) []any {
	out := make([]any, len(row))
	for i, v := range row {
		out[i] = syncWireValue(v)
	}
	return out
}

var reIntegerColumnType = regexp.MustCompile(`(?i)^((tiny|small|medium|big)?int(eger|[248])?|(small|big)?serial)\b`)

// isIntegerColumnType matches integer types only (not INTERVAL/POINT, which merely contain "INT").
func isIntegerColumnType(t string) bool {
	return reIntegerColumnType.MatchString(strings.TrimSpace(t))
}

// fromSyncWire converts a JSON-decoded (UseNumber) apply value back into a bind parameter for a
// target column of colType. Integers are bound as int64/uint64 so comparisons never go through
// float64 (MySQL compares BIGINT with a string/double argument as DOUBLE and could match a neighbour).
func fromSyncWire(v any, colType string) any {
	switch x := v.(type) {
	case map[string]any:
		if b64, ok := x["$base64"].(string); ok && len(x) == 1 {
			if b, err := base64.StdEncoding.DecodeString(b64); err == nil {
				return b
			}
		}
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if u, err := strconv.ParseUint(string(x), 10, 64); err == nil {
			return u
		}
		return string(x) // decimal text keeps full precision for DECIMAL/NUMERIC columns
	case string:
		if isIntegerColumnType(colType) {
			if i, err := strconv.ParseInt(x, 10, 64); err == nil {
				return i
			}
			if u, err := strconv.ParseUint(x, 10, 64); err == nil {
				return u
			}
		}
	}
	return v
}

// syncValueKey canonicalizes a scanned value for cross-driver comparison: drivers return the same
// logical value as int64, string or []byte, and NULL must never collide with any text value.
func syncValueKey(v any) string {
	switch x := v.(type) {
	case nil:
		return "\x00NULL"
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

func rowValuesEqual(rowA, rowB []any) bool {
	return slices.EqualFunc(rowA, rowB, func(a, b any) bool {
		return syncValueKey(a) == syncValueKey(b)
	})
}
