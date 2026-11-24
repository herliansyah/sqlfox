package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func QuoteIdentifier(name, driverName string) string {
	name = strings.Trim(name, "`\"")
	if strings.EqualFold(driverName, "sqlite") || strings.EqualFold(driverName, "postgres") || strings.EqualFold(driverName, "postgresql") || strings.EqualFold(driverName, "pg") {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Placeholder returns the parameter placeholder for the given driver and 1-based index.
func Placeholder(driverName string, index int) string {
	d := strings.ToLower(driverName)
	if strings.HasPrefix(d, "postgre") || d == "pg" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

var (
	reLimitClause  = regexp.MustCompile(`(?i)\b(LIMIT\s+(\d+|ALL\b|\?|\$\d+)|FETCH\s+(FIRST|NEXT)\b)`)
	reLockingTail  = regexp.MustCompile(`(?i)\b(FOR\s+(UPDATE|SHARE|NO\s+KEY\s+UPDATE|KEY\s+SHARE)|LOCK\s+IN\s+SHARE\s+MODE)\b`)
	reTopLevelInto = regexp.MustCompile(`(?i)\bINTO\b`)
)

// maskNonTopLevel blanks comments, quoted literals/identifiers and everything inside parentheses
// (byte offsets preserved), so keyword scans only see the outermost statement: a LIMIT inside a
// subquery, string or comment must not count as the statement's own LIMIT.
// ponytail: backslash is treated as an escape inside quotes (MySQL); a PostgreSQL literal ending in
// a backslash can desync the mask, in which case the cap may be skipped or the query rejected by
// the server — never silently widened.
func maskNonTopLevel(q string) string {
	b := []byte(q)
	depth := 0
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '-' && i+1 < len(b) && b[i+1] == '-':
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(q[i+2:], "*/")
			stop := len(b)
			if end >= 0 {
				stop = i + 2 + end + 2
			}
			for ; i < stop; i++ {
				b[i] = ' '
			}
			i--
		case b[i] == '\'' || b[i] == '"' || b[i] == '`':
			quote := b[i]
			b[i] = ' '
			for i++; i < len(b); i++ {
				if b[i] == '\\' && quote != '`' && i+1 < len(b) {
					b[i], b[i+1] = ' ', ' '
					i++
					continue
				}
				if b[i] == quote {
					b[i] = ' '
					break
				}
				b[i] = ' '
			}
		case b[i] == '(':
			depth++
			b[i] = ' '
		case b[i] == ')':
			if depth > 0 {
				depth--
			}
			b[i] = ' '
		default:
			if depth > 0 {
				b[i] = ' '
			}
		}
	}
	return string(b)
}

// HasLimitClause reports whether the SQL statement's outermost query has a row limit
// (LIMIT n / LIMIT ALL / LIMIT ? / FETCH FIRST|NEXT).
func HasLimitClause(sql string) bool {
	return reLimitClause.MatchString(maskNonTopLevel(sql))
}

// ApplyLimitClause adds a LIMIT to SELECT queries that have no outermost limit and limit > 0.
// It strips leading comments before detecting the SELECT keyword and puts LIMIT on its own line so
// trailing single-line comments cannot swallow it. A locking clause (FOR UPDATE, LOCK IN SHARE MODE)
// must follow LIMIT, so LIMIT is inserted before it; SELECT ... INTO is left untouched.
func ApplyLimitClause(querySQL string, limit int) string {
	if limit <= 0 {
		return querySQL
	}
	trimmed := strings.TrimSpace(querySQL)
	cleanUpper := strings.ToUpper(StripLeadingComments(trimmed))
	if !strings.HasPrefix(cleanUpper, "SELECT") || HasLimitClause(trimmed) {
		return querySQL
	}
	// Drop a trailing terminator followed only by comments/whitespace (e.g. "SELECT 1; -- note")
	if i := strings.LastIndex(trimmed, ";"); i >= 0 && !HasExecutableSQL(trimmed[i+1:], true) {
		trimmed = strings.TrimSpace(trimmed[:i])
	}
	masked := maskNonTopLevel(trimmed)
	if reTopLevelInto.MatchString(masked) {
		return querySQL
	}
	if loc := reLockingTail.FindStringIndex(masked); loc != nil {
		return fmt.Sprintf("%s\nLIMIT %d\n%s", strings.TrimRight(trimmed[:loc[0]], " \t\r\n"), limit, trimmed[loc[0]:])
	}
	return fmt.Sprintf("%s\nLIMIT %d", trimmed, limit)
}

// StripLeadingComments removes leading whitespace and SQL comments (-- , #, and /* ... */).
func StripLeadingComments(sql string) string {
	s := strings.TrimSpace(sql)
	for {
		if strings.HasPrefix(s, "--") {
			idx := strings.IndexByte(s, '\n')
			if idx == -1 {
				return ""
			}
			s = strings.TrimSpace(s[idx+1:])
			continue
		}
		if strings.HasPrefix(s, "#") {
			idx := strings.IndexByte(s, '\n')
			if idx == -1 {
				return ""
			}
			s = strings.TrimSpace(s[idx+1:])
			continue
		}
		if strings.HasPrefix(s, "/*") {
			idx := strings.Index(s, "*/")
			if idx == -1 {
				return ""
			}
			// MySQL executes /*! ... */ and /*!NNNNN ... */ bodies, so unwrap them instead of
			// discarding; otherwise callers (e.g. the Safety Guard) would miss the real statement.
			if strings.HasPrefix(s, "/*!") {
				body := strings.TrimLeft(s[3:idx], "0123456789")
				s = strings.TrimSpace(body + " " + s[idx+2:])
				continue
			}
			s = strings.TrimSpace(s[idx+2:])
			continue
		}
		break
	}
	return s
}

// ErrExplainTarget is returned when SQL cannot be explained without risking its execution.
var ErrExplainTarget = errors.New("explain requires exactly one statement without EXPLAIN ANALYZE/options (they are applied automatically)")

var reExplainPrefix = regexp.MustCompile(`(?is)^EXPLAIN(\s+QUERY\s+PLAN)?\s+`)

// ExplainTarget validates the statements of an Explain request and returns the bare statement to plan.
// stmts must come from a quote-aware splitter. Multiple statements are rejected because drivers that
// accept multi-statement text would execute everything after the first one, and EXPLAIN ANALYZE /
// EXPLAIN (options) are rejected because ANALYZE executes the statement.
func ExplainTarget(stmts []string) (string, error) {
	if len(stmts) != 1 {
		return "", ErrExplainTarget
	}
	s := strings.TrimRight(strings.TrimSpace(StripLeadingComments(stmts[0])), "; \t\r\n")
	if loc := reExplainPrefix.FindStringIndex(s); loc != nil {
		s = strings.TrimSpace(s[loc[1]:])
		up := strings.ToUpper(s)
		if strings.HasPrefix(up, "ANALYZE") || strings.HasPrefix(up, "ANALYSE") || strings.HasPrefix(up, "(") || strings.HasPrefix(up, "FORMAT") {
			return "", ErrExplainTarget
		}
	}
	if s == "" {
		return "", ErrExplainTarget
	}
	return s, nil
}

var reCTEMutationVerb = regexp.MustCompile(`(?i)\b(?:INSERT|UPDATE|DELETE|MERGE|DROP|ALTER|TRUNCATE|CREATE)\b`)

// stripSQLCommentsAndLiterals replaces string/quoted literals and comments with spaces
// so keyword checks cannot be tricked by literals or bypassed by inline comments like DELETE/*x*/FROM.
func stripSQLCommentsAndLiterals(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	n := len(sql)
	for i := 0; i < n; i++ {
		c := sql[i]
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			b.WriteByte(' ')
			i++
			for i < n {
				if sql[i] == '\\' && quote != '`' && i+1 < n {
					i += 2
					continue
				}
				if sql[i] == quote {
					if i+1 < n && sql[i+1] == quote {
						i += 2
						continue
					}
					break
				}
				i++
			}
			continue
		}
		if c == '$' {
			j := i + 1
			for j < n && (sql[j] == '_' || (sql[j] >= 'a' && sql[j] <= 'z') || (sql[j] >= 'A' && sql[j] <= 'Z') || (sql[j] >= '0' && sql[j] <= '9')) {
				j++
			}
			if j < n && sql[j] == '$' && (j == i+1 || !(sql[i+1] >= '0' && sql[i+1] <= '9')) {
				tag := sql[i : j+1]
				b.WriteByte(' ')
				end := strings.Index(sql[j+1:], tag)
				if end >= 0 {
					i = j + 1 + end + len(tag) - 1
				} else {
					i = n - 1
				}
				continue
			}
		}
		if c == '-' && i+1 < n && sql[i+1] == '-' {
			b.WriteByte(' ')
			for i < n && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '#' {
			b.WriteByte(' ')
			for i < n && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && sql[i+1] == '*' {
			b.WriteByte(' ')
			i += 2
			for i+1 < n && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// IsQueryStatement reports whether a SQL statement returns rows (SELECT, SHOW, DESC, EXPLAIN, WITH, PRAGMA, VALUES, TABLE, etc.).
func IsQueryStatement(sql string) bool {
	clean := StripLeadingComments(sql)
	clean = strings.TrimLeft(clean, "(\t\r\n ")
	upper := strings.ToUpper(clean)
	if strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESC") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "VALUES") ||
		strings.HasPrefix(upper, "TABLE") ||
		strings.HasPrefix(upper, "CHECK TABLE") ||
		strings.HasPrefix(upper, "ANALYZE TABLE") ||
		strings.HasPrefix(upper, "OPTIMIZE TABLE") ||
		strings.HasPrefix(upper, "REPAIR TABLE") {
		return true
	}
	if strings.HasPrefix(upper, "WITH") {
		stripped := stripSQLCommentsAndLiterals(clean)
		return !reCTEMutationVerb.MatchString(stripped)
	}
	return false
}

// SplitSQLStatements splits SQL text into executable statements. It honors MySQL `DELIMITER` lines
// and, under the default ";" terminator, is quote/comment aware: '...' (with ” and, for MySQL,
// backslash escapes), "...", `...`, -- and /* */ comments (# too for MySQL), and PostgreSQL
// dollar quotes ($$...$$ and $tag$...$tag$). Comment-only fragments are dropped.
func SplitSQLStatements(sqlText, driverName string) []string {
	isMySQL := strings.EqualFold(driverName, "mysql") || strings.EqualFold(driverName, "mariadb")
	return SplitWithDelimiters(sqlText, isMySQL, func(seg string) []string {
		return splitDefaultDelimiter(seg, isMySQL)
	})
}

// dollarTagAt returns the dollar-quote opener at runes[i] ("$$" or "$tag$"), or "" if none.
// A tag cannot start with a digit, so positional parameters like $1 are never mistaken for one.
func dollarTagAt(runes []rune, i int) string {
	if runes[i] != '$' {
		return ""
	}
	// Like PostgreSQL's lexer: "$" inside an identifier (price$usd) never opens a dollar quote.
	if i > 0 && (runes[i-1] == '_' || runes[i-1] == '$' || unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
		return ""
	}
	j := i + 1
	for j < len(runes) && (runes[j] == '_' || unicode.IsLetter(runes[j]) || (j > i+1 && unicode.IsDigit(runes[j]))) {
		j++
	}
	if j < len(runes) && runes[j] == '$' {
		return string(runes[i : j+1])
	}
	return ""
}

func splitDefaultDelimiter(content string, isMySQL bool) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote, inDoubleQuote, inBacktick := false, false, false
	inLineComment, inBlockComment := false, false
	dollarTag := "" // non-empty while inside a dollar-quoted body

	runes := []rune(content)
	n := len(runes)

	isQuoteEscaped := func(i int) bool {
		if !isMySQL {
			return false
		}
		bs := 0
		for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
			bs++
		}
		return bs%2 == 1
	}
	hasPrefixAt := func(i int, s string) bool {
		rs := []rune(s)
		if i+len(rs) > n {
			return false
		}
		for k, r := range rs {
			if runes[i+k] != r {
				return false
			}
		}
		return true
	}

	for i := 0; i < n; i++ {
		r := runes[i]

		if inLineComment {
			current.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			current.WriteRune(r)
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				inBlockComment = false
				current.WriteRune('/')
				i++
			}
			continue
		}
		if dollarTag != "" {
			if hasPrefixAt(i, dollarTag) {
				current.WriteString(dollarTag)
				i += len([]rune(dollarTag)) - 1
				dollarTag = ""
				continue
			}
			current.WriteRune(r)
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inLineComment = true
				current.WriteString("--")
				i++
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				current.WriteRune('#')
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				inBlockComment = true
				current.WriteString("/*")
				i++
				continue
			}
			if !isMySQL {
				if tag := dollarTagAt(runes, i); tag != "" {
					dollarTag = tag
					current.WriteString(tag)
					i += len([]rune(tag)) - 1
					continue
				}
			}
		}

		switch {
		case r == '\'' && !inDoubleQuote && !inBacktick:
			if inSingleQuote && i+1 < n && runes[i+1] == '\'' {
				current.WriteString("''")
				i++
				continue
			}
			if !isQuoteEscaped(i) {
				inSingleQuote = !inSingleQuote
			}
		case r == '"' && !inSingleQuote && !inBacktick:
			if !isQuoteEscaped(i) {
				inDoubleQuote = !inDoubleQuote
			}
		case r == '`' && !inSingleQuote && !inDoubleQuote:
			inBacktick = !inBacktick
		}

		if r == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			if stmt := strings.TrimSpace(current.String()); stmt != "" && HasExecutableSQL(stmt, isMySQL) {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(r)
	}

	if last := strings.TrimSpace(current.String()); last != "" && HasExecutableSQL(last, isMySQL) {
		statements = append(statements, last)
	}
	return statements
}

// SplitWithDelimiters honors MySQL client `DELIMITER xx` lines (used by mysqldump and routine scripts
// so BEGIN ... END bodies keep their inner semicolons). Segments under the default ";" delimiter go
// through split; segments under a custom delimiter are cut at each occurrence of it.
// ponytail: custom-delimiter segments are cut textually (not quote-aware), same as the mysql client's
// common usage; upgrade path: teach the token-aware splitters a variable terminator.
func SplitWithDelimiters(sqlText string, isMySQL bool, split func(string) []string) []string {
	delim := ";"
	var out []string
	var seg strings.Builder
	flush := func() {
		text := seg.String()
		seg.Reset()
		if delim == ";" {
			out = append(out, split(text)...)
			return
		}
		for _, part := range strings.Split(text, delim) {
			if part = strings.TrimSpace(part); part != "" && HasExecutableSQL(part, isMySQL) {
				out = append(out, part)
			}
		}
	}
	for _, line := range strings.SplitAfter(sqlText, "\n") {
		t := strings.TrimSpace(line)
		if len(t) > len("DELIMITER ") && strings.EqualFold(t[:len("DELIMITER ")], "DELIMITER ") {
			flush()
			delim = strings.Fields(t[len("DELIMITER "):])[0]
			continue
		}
		seg.WriteString(line)
	}
	flush()
	return out
}

// HasExecutableSQL checks whether a SQL statement string contains executable tokens
// (i.e. not entirely comments, whitespace, or empty semicolons).
func HasExecutableSQL(stmt string, isMySQL bool) bool {
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inBlockComment := false
	inLineComment := false

	runes := []rune(stmt)
	n := len(runes)
	tokenCount := 0

	for i := 0; i < n; i++ {
		r := runes[i]
		if inLineComment {
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inLineComment = true
				i++
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				if isMySQL && i+2 < n && runes[i+2] == '!' {
					// In MySQL, /*! ... */ is a versioned executable comment containing SQL
					tokenCount++
					i += 2
					continue
				}
				inBlockComment = true
				i++
				continue
			}
		}
		if r == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
			tokenCount++
			continue
		}
		if r == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
			tokenCount++
			continue
		}
		if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
			tokenCount++
			continue
		}
		if !unicode.IsSpace(r) && r != ';' {
			tokenCount++
		}
	}
	return tokenCount > 0
}

// EscapeSQLString escapes a string literal for the specified SQL driver dialect.
// ponytail: MySQL/MariaDB requires escaping backslashes and control chars; standard SQL doubles single quotes.
func EscapeSQLString(s string, driverName string) string {
	switch strings.ToLower(driverName) {
	case "mysql", "mariadb":
		var sb strings.Builder
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
				sb.WriteString(`\'`)
			case '"':
				sb.WriteString(`\"`)
			case '\032':
				sb.WriteString(`\Z`)
			default:
				sb.WriteByte(b)
			}
		}
		sb.WriteByte('\'')
		return sb.String()
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
			return "NULL"
		}
		return strconv.FormatFloat(f, 'f', -1, 32)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "NULL"
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
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
	case "postgres", "postgresql", "pgx":
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
func SequenceResetStatements(tbl TableMetadata, driverName string) []string {
	d := strings.ToLower(driverName)
	if d != "postgres" && d != "postgresql" && d != "pg" {
		return nil
	}
	var out []string
	qTable := QuoteIdentifier(tbl.Name, d)
	for _, col := range tbl.Columns {
		if !strings.HasPrefix(strings.ToLower(col.DefaultValue), "nextval(") {
			continue
		}
		qCol := QuoteIdentifier(col.Name, d)
		out = append(out, fmt.Sprintf("SELECT setval(pg_get_serial_sequence(%s, %s), COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false);",
			EscapeSQLString(qTable, d), EscapeSQLString(col.Name, d), qCol, qTable))
	}
	return out
}

// TopologicalSortTables orders tables so that referenced parent tables appear before dependent child tables.
// ponytail: Kahn's algorithm resolves FK dependency DAG with deterministic cycle fallback.
func TopologicalSortTables(tables []TableMetadata) []TableMetadata {
	tableMap := make(map[string]TableMetadata, len(tables))
	for _, t := range tables {
		tableMap[strings.ToLower(t.Name)] = t
	}

	dependents := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if _, ok := inDegree[lowName]; !ok {
			inDegree[lowName] = 0
		}
		for _, fk := range t.ForeignKeys {
			refTable := strings.ToLower(fk.ReferencedTable)
			if refTable != "" && refTable != lowName {
				if _, exists := tableMap[refTable]; exists {
					dependents[refTable] = append(dependents[refTable], lowName)
					inDegree[lowName]++
				}
			}
		}
	}

	var queue []string
	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if inDegree[lowName] == 0 {
			queue = append(queue, lowName)
		}
	}

	var sortedNames []string
	visited := make(map[string]bool)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if visited[curr] {
			continue
		}
		visited[curr] = true
		sortedNames = append(sortedNames, curr)

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 && !visited[dep] {
				queue = append(queue, dep)
			}
		}
	}

	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if !visited[lowName] {
			sortedNames = append(sortedNames, lowName)
			visited[lowName] = true
		}
	}

	var result []TableMetadata
	for _, name := range sortedNames {
		if t, ok := tableMap[name]; ok {
			result = append(result, t)
		}
	}
	return result
}
