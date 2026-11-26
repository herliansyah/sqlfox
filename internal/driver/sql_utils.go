package driver

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

func QuoteIdentifier(name, driverName string) string {
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.Trim(name, "`\"")
	if strings.EqualFold(driverName, "sqlite") || strings.EqualFold(driverName, "sqlite3") ||
		strings.EqualFold(driverName, "postgres") || strings.EqualFold(driverName, "postgresql") ||
		strings.EqualFold(driverName, "pg") || strings.EqualFold(driverName, "pgx") {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Placeholder returns the parameter placeholder for the given driver and 1-based index.
func Placeholder(driverName string, index int) string {
	d := strings.ToLower(driverName)
	if strings.HasPrefix(d, "postgre") || d == "pg" || d == "pgx" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

var (
	reLimitClause  = regexp.MustCompile(`(?i)\b(LIMIT\s+(\d+|ALL\b|\?|\$\d+)|FETCH\s+(FIRST|NEXT)\b)`)
	reLockingTail  = regexp.MustCompile(`(?i)\b(FOR\s+(UPDATE|SHARE|NO\s+KEY\s+UPDATE|KEY\s+SHARE)|LOCK\s+IN\s+SHARE\s+MODE)\b`)
	reTopLevelInto = regexp.MustCompile(`(?i)\bINTO\b`)
)

// mysqlExecCommentPrefixLen returns the prefix byte length (3 for "/*!", 4 for "/*M!" / "/*m!")
// if s[i:] starts a MySQL or MariaDB executable comment, or 0 otherwise.
func mysqlExecCommentPrefixLen(s string, i int) int {
	if i+2 >= len(s) || s[i] != '/' || s[i+1] != '*' {
		return 0
	}
	if s[i+2] == '!' {
		return 3
	}
	if i+3 < len(s) && (s[i+2] == 'M' || s[i+2] == 'm') && s[i+3] == '!' {
		return 4
	}
	return 0
}

func hasMySQLExecutableComment(s string) bool {
	return strings.Contains(s, "/*!") || strings.Contains(s, "/*M!") || strings.Contains(s, "/*m!")
}

func isIdentCharByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// maskNonTopLevel blanks comments, quoted literals/identifiers and everything inside parentheses
// (byte offsets preserved), so keyword scans only see the outermost statement: a LIMIT inside a
// subquery, string, dollar-quote or comment must not count as the statement's own LIMIT.
func maskNonTopLevel(q string) string {
	b := []byte(q)
	depth := 0
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '-' && i+1 < len(b) && b[i+1] == '-':
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case b[i] == '#' && !(i+1 < len(b) && (b[i+1] == '>' || b[i+1] == '-')):
			// Skip MySQL # comments while preserving PostgreSQL JSONB operators (#>, #>>, #-).
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			if pLen := mysqlExecCommentPrefixLen(q, i); pLen > 0 {
				// MySQL/MariaDB executable comment: blank the /*!NNNNN or /*M!NNNNN wrapper, keep body visible.
				for k := 0; k < pLen; k++ {
					b[i+k] = ' '
				}
				j := i + pLen
				for j < len(b) && b[j] >= '0' && b[j] <= '9' {
					b[j] = ' '
					j++
				}
				i = j - 1
				continue
			}
			// Support nested /* /* */ */ block comments (PostgreSQL).
			cDepth := 1
			b[i], b[i+1] = ' ', ' '
			i += 2
			for i < len(b) && cDepth > 0 {
				if b[i] == '/' && i+1 < len(b) && b[i+1] == '*' {
					cDepth++
					b[i], b[i+1] = ' ', ' '
					i += 2
					continue
				}
				if b[i] == '*' && i+1 < len(b) && b[i+1] == '/' {
					cDepth--
					b[i], b[i+1] = ' ', ' '
					i += 2
					continue
				}
				b[i] = ' '
				i++
			}
			i--
		case b[i] == '*' && i+1 < len(b) && b[i+1] == '/':
			// Trailing closer from an unwrapped /*! ... */ comment.
			b[i], b[i+1] = ' ', ' '
			i++
		case b[i] == '$':
			// PostgreSQL dollar-quoted string $$...$$ or $tag$...$tag$.
			j := i + 1
			for j < len(b) && (b[j] == '_' || (b[j] >= 'a' && b[j] <= 'z') || (b[j] >= 'A' && b[j] <= 'Z') || (j > i+1 && b[j] >= '0' && b[j] <= '9')) {
				j++
			}
			if j < len(b) && b[j] == '$' && (i == 0 || !(b[i-1] == '_' || b[i-1] == '$' || (b[i-1] >= 'a' && b[i-1] <= 'z') || (b[i-1] >= 'A' && b[i-1] <= 'Z') || (b[i-1] >= '0' && b[i-1] <= '9'))) {
				tag := q[i : j+1]
				end := strings.Index(q[j+1:], tag)
				stop := len(b)
				if end >= 0 {
					stop = j + 1 + end + len(tag)
				}
				for ; i < stop; i++ {
					b[i] = ' '
				}
				i--
				continue
			}
			if depth > 0 {
				b[i] = ' '
			}
		case b[i] == '\'' || b[i] == '"' || b[i] == '`':
			quote := b[i]
			b[i] = ' '
			for i++; i < len(b); i++ {
				if b[i] == quote {
					if i+1 < len(b) && b[i+1] == quote {
						b[i], b[i+1] = ' ', ' '
						i++
						continue
					}
					b[i] = ' '
					break
				}
				if b[i] == '\\' && quote == '\'' && i+1 < len(b) && b[i+1] != '\'' {
					b[i], b[i+1] = ' ', ' '
					i++
					continue
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

// ApplyLimitClause adds a LIMIT to SELECT (and read-only WITH ... SELECT / parenthesized SELECT)
// queries that have no outermost limit and limit > 0.
func ApplyLimitClause(querySQL string, limit int) string {
	if limit <= 0 {
		return querySQL
	}
	trimmed := strings.TrimSpace(querySQL)
	cleanUpper := strings.ToUpper(StripLeadingComments(trimmed))
	isCapable := strings.HasPrefix(cleanUpper, "SELECT") ||
		(strings.HasPrefix(cleanUpper, "WITH") && IsQueryStatement(trimmed)) ||
		(strings.HasPrefix(cleanUpper, "(") && IsQueryStatement(trimmed))
	if !isCapable || HasLimitClause(trimmed) {
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
// Tracks nested block comments (PostgreSQL) and unwraps MySQL /*! ... */ executable comments.
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
			// MySQL/MariaDB executes /*! ... */ and /*M! ... */ bodies, so unwrap them instead of
			// discarding; otherwise callers (e.g. the Safety Guard) would miss the real statement.
			if pLen := mysqlExecCommentPrefixLen(s, 0); pLen > 0 {
				idx := strings.Index(s[pLen:], "*/")
				if idx == -1 {
					body := strings.TrimLeft(s[pLen:], "0123456789")
					s = strings.TrimSpace(body)
					continue
				}
				body := strings.TrimLeft(s[pLen:pLen+idx], "0123456789")
				s = strings.TrimSpace(body + " " + s[pLen+idx+2:])
				continue
			}
			// Track nested block comment depth (valid in PostgreSQL: /* /* */ */).
			depth := 1
			i := 2
			for i < len(s) && depth > 0 {
				if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
					depth++
					i += 2
					continue
				}
				if s[i] == '*' && i+1 < len(s) && s[i+1] == '/' {
					depth--
					i += 2
					continue
				}
				i++
			}
			if depth > 0 {
				return ""
			}
			s = strings.TrimSpace(s[i:])
			continue
		}
		break
	}
	return s
}

// UnwrapMySQLExecutableComments removes /*!NNNNN, /*M!NNNNN, and matching */ wrappers outside
// string literals so downstream lexers, splitters, and safety guards see the exact SQL MySQL/MariaDB will execute.
func UnwrapMySQLExecutableComments(sql string) string {
	if !hasMySQLExecutableComment(sql) {
		return sql
	}
	var b strings.Builder
	b.Grow(len(sql))
	n := len(sql)
	execDepth := 0
	for i := 0; i < n; i++ {
		c := sql[i]
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			b.WriteByte(c)
			i++
			for i < n {
				b.WriteByte(sql[i])
				if sql[i] == '\\' && quote != '`' && i+1 < n {
					i++
					b.WriteByte(sql[i])
					i++
					continue
				}
				if sql[i] == quote {
					if i+1 < n && sql[i+1] == quote {
						i++
						b.WriteByte(sql[i])
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}
		if c == '-' && i+1 < n && sql[i+1] == '-' {
			for i < n && sql[i] != '\n' {
				b.WriteByte(sql[i])
				i++
			}
			if i < n {
				b.WriteByte(sql[i])
			}
			continue
		}
		if c == '#' && !(i+1 < n && (sql[i+1] == '>' || sql[i+1] == '-')) {
			for i < n && sql[i] != '\n' {
				b.WriteByte(sql[i])
				i++
			}
			if i < n {
				b.WriteByte(sql[i])
			}
			continue
		}
		if pLen := mysqlExecCommentPrefixLen(sql, i); pLen > 0 {
			execDepth++
			b.WriteByte(' ')
			i += pLen
			for i < n && sql[i] >= '0' && sql[i] <= '9' {
				i++
			}
			i--
			continue
		}
		if execDepth > 0 && c == '*' && i+1 < n && sql[i+1] == '/' {
			execDepth--
			b.WriteByte(' ')
			i++
			continue
		}
		if c == '/' && i+1 < n && sql[i+1] == '*' {
			b.WriteString("/*")
			i += 2
			for i < n {
				if sql[i] == '*' && i+1 < n && sql[i+1] == '/' {
					b.WriteString("*/")
					i++
					break
				}
				b.WriteByte(sql[i])
				i++
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// ErrExplainTarget is returned when SQL cannot be explained without risking its execution.
var ErrExplainTarget = errors.New("explain requires exactly one statement without EXPLAIN ANALYZE/options (they are applied automatically)")

var reExplainPrefix = regexp.MustCompile(`(?is)^EXPLAIN(\s+QUERY\s+PLAN)?\s+`)

// ExplainTarget validates the statements of an Explain request and returns the bare statement to plan.
func ExplainTarget(stmts []string) (string, error) {
	if len(stmts) != 1 {
		return "", ErrExplainTarget
	}
	// Reject hidden multi-statements smuggled inside MySQL /*! ... ; ... */ executable comments.
	if len(splitDefaultDelimiter(UnwrapMySQLExecutableComments(stmts[0]), true)) != 1 {
		return "", ErrExplainTarget
	}
	s := strings.TrimRight(strings.TrimSpace(StripLeadingComments(stmts[0])), "; \t\r\n")
	if loc := reExplainPrefix.FindStringIndex(s); loc != nil {
		// Strip comments again after EXPLAIN so "EXPLAIN /*x*/ ANALYZE ..." cannot bypass the check.
		s = strings.TrimSpace(StripLeadingComments(s[loc[1]:]))
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

var (
	reCTEMutationVerb     = regexp.MustCompile(`(?i)\b(?:INSERT|UPDATE|DELETE|MERGE|DROP|ALTER|TRUNCATE|CREATE)\b`)
	reSelectIntoFile      = regexp.MustCompile(`(?i)\bINTO\s+(?:OUTFILE|DUMPFILE)\b`)
	reSelectIntoTopLevel  = regexp.MustCompile(`(?i)\bINTO\b`)
	reExplainAnalyzeToken = regexp.MustCompile(`(?i)\bANALY[ZS]E\b`)
	rePragmaCallName      = regexp.MustCompile(`(?is)^PRAGMA\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

var readOnlySQLitePragmas = map[string]bool{
	"table_info": true, "table_xinfo": true, "index_list": true, "index_info": true, "index_xinfo": true,
	"foreign_key_list": true, "database_list": true, "collation_list": true, "compile_options": true,
	"function_list": true, "module_list": true, "pragma_list": true, "integrity_check": true,
	"quick_check": true, "foreign_key_check": true, "data_version": true, "schema_version": true,
	"user_version": true, "application_id": true, "encoding": true, "page_count": true,
	"page_size": true, "freelist_count": true,
}

// stripSQLCommentsAndLiterals replaces string/quoted literals and comments with spaces
// so keyword checks cannot be tricked by literals or bypassed by inline comments like DELETE/*x*/FROM.
// When mysqlEscapes is false, PostgreSQL standard_conforming_strings is honored ('\' ends the literal).
func stripSQLCommentsAndLiterals(sql string, mysqlEscapes bool) string {
	sql = UnwrapMySQLExecutableComments(sql)
	var b strings.Builder
	b.Grow(len(sql))
	n := len(sql)
	for i := 0; i < n; i++ {
		c := sql[i]
		// PostgreSQL E'...' / e'...' escape string literal (always honors backslash escapes).
		if !mysqlEscapes && (c == 'E' || c == 'e') && i+1 < n && sql[i+1] == '\'' && (i == 0 || !isIdentCharByte(sql[i-1])) {
			b.WriteByte(' ')
			i += 2
			for i < n {
				if sql[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			b.WriteByte(' ')
			i++
			for i < n {
				if mysqlEscapes && sql[i] == '\\' && quote != '`' && i+1 < n {
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
			for j < n && (sql[j] == '_' || (sql[j] >= 'a' && sql[j] <= 'z') || (sql[j] >= 'A' && sql[j] <= 'Z') || (j > i+1 && sql[j] >= '0' && sql[j] <= '9')) {
				j++
			}
			if j < n && sql[j] == '$' && (i == 0 || !(sql[i-1] == '_' || sql[i-1] == '$' || (sql[i-1] >= 'a' && sql[i-1] <= 'z') || (sql[i-1] >= 'A' && sql[i-1] <= 'Z') || (sql[i-1] >= '0' && sql[i-1] <= '9'))) {
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
		if mysqlEscapes && c == '#' {
			b.WriteByte(' ')
			for i < n && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && sql[i+1] == '*' {
			b.WriteByte(' ')
			depth := 1
			i += 2
			for i < n && depth > 0 {
				if sql[i] == '/' && i+1 < n && sql[i+1] == '*' {
					depth++
					i += 2
					continue
				}
				if sql[i] == '*' && i+1 < n && sql[i+1] == '/' {
					depth--
					i += 2
					continue
				}
				i++
			}
			i--
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// IsQueryStatement reports whether a SQL statement is a pure read-only query that returns rows.
// Rejects SELECT ... INTO OUTFILE/DUMPFILE, mutating PRAGMA (=), hidden multi-statements in /*! ... */,
// and data-modifying CTEs under both MySQL and PostgreSQL string-escaping rules.
func IsQueryStatement(sql string) bool {
	// Reject hidden multi-statement smuggling via MySQL/MariaDB /*! ... ; ... */ or /*M! ... ; ... */ executable comments.
	if hasMySQLExecutableComment(sql) && len(splitDefaultDelimiter(UnwrapMySQLExecutableComments(sql), true)) > 1 {
		return false
	}
	clean := StripLeadingComments(sql)
	clean = strings.TrimLeft(clean, "(\t\r\n ")
	upper := strings.ToUpper(clean)
	if strings.HasPrefix(upper, "SELECT") {
		if reSelectIntoFile.MatchString(stripSQLCommentsAndLiterals(clean, true)) ||
			reSelectIntoFile.MatchString(stripSQLCommentsAndLiterals(clean, false)) {
			return false
		}
		// Reject SELECT ... INTO <new_table> (PostgreSQL table creation via SELECT INTO).
		if reSelectIntoTopLevel.MatchString(maskNonTopLevel(clean)) {
			return false
		}
		return true
	}
	if strings.HasPrefix(upper, "PRAGMA") {
		// Only read-only PRAGMAs (no assignment '=' and no mutating function-call PRAGMA)
		// may be classified as query statements.
		stripped := stripSQLCommentsAndLiterals(clean, false)
		if strings.Contains(stripped, "=") {
			return false
		}
		if m := rePragmaCallName.FindStringSubmatch(strings.TrimSpace(stripped)); len(m) == 2 {
			return readOnlySQLitePragmas[strings.ToLower(m[1])]
		}
		return true
	}
	if strings.HasPrefix(upper, "EXPLAIN") {
		stripped := stripSQLCommentsAndLiterals(clean, false)
		if reExplainAnalyzeToken.MatchString(stripped) {
			// EXPLAIN ANALYZE / ANALYSE actually executes the inner statement; fail closed if
			// any mutation/DDL verb appears or if the inner statement cannot be proven read-only.
			if reCTEMutationVerb.MatchString(stripped) || reCTEMutationVerb.MatchString(stripSQLCommentsAndLiterals(clean, true)) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESC") ||
		strings.HasPrefix(upper, "VALUES") ||
		strings.HasPrefix(upper, "TABLE") ||
		strings.HasPrefix(upper, "CHECK TABLE") {
		return true
	}
	if strings.HasPrefix(upper, "WITH") {
		// Fail closed across both MySQL (\-escaped) and PostgreSQL (standard_conforming_strings) lexers.
		if reCTEMutationVerb.MatchString(stripSQLCommentsAndLiterals(clean, false)) {
			return false
		}
		if reCTEMutationVerb.MatchString(stripSQLCommentsAndLiterals(clean, true)) {
			return false
		}
		if reSelectIntoFile.MatchString(stripSQLCommentsAndLiterals(clean, false)) ||
			reSelectIntoFile.MatchString(stripSQLCommentsAndLiterals(clean, true)) {
			return false
		}
		return true
	}
	return false
}

// SplitSQLStatements splits SQL text into executable statements. It honors MySQL `DELIMITER` lines
// and, under the default ";" terminator, is quote/comment aware: '...' (with ” and, for MySQL,
// backslash escapes), "...", `...`, -- and /* */ comments (# too for MySQL), and PostgreSQL
// dollar quotes ($$...$$ and $tag$...$tag$). Comment-only fragments are dropped.

var reValidPrivilegeName = regexp.MustCompile(`^[A-Za-z]+( [A-Za-z]+)*$`)

// ValidPrivilegeName reports whether priv contains only bare SQL privilege keywords.
func ValidPrivilegeName(priv string) bool {
	return reValidPrivilegeName.MatchString(priv)
}

// ColumnsFromTypes converts database/sql ColumnType metadata into driver.ColumnDefinition slices.
func ColumnsFromTypes(colTypes []*sql.ColumnType) []ColumnDefinition {
	cols := make([]ColumnDefinition, len(colTypes))
	for i, ct := range colTypes {
		nullable, _ := ct.Nullable()
		cols[i] = ColumnDefinition{
			Name:     ct.Name(),
			Type:     ct.DatabaseTypeName(),
			Nullable: nullable,
		}
	}
	return cols
}

// ConnStateMap stores per-driver.Conn state with a bounded capacity so recycled pool
// connections do not accumulate unboundedly over long-running sessions.
type ConnStateMap[T any] struct {
	mu      sync.Mutex
	maxSize int
	entries map[any]*T
}

// NewConnStateMap returns a bounded ConnStateMap. Default capacity is 64 when maxSize <= 0.
func NewConnStateMap[T any](maxSize int) *ConnStateMap[T] {
	if maxSize <= 0 {
		maxSize = 64
	}
	return &ConnStateMap[T]{
		maxSize: maxSize,
		entries: make(map[any]*T),
	}
}

// Get returns the state associated with conn's underlying driver.Conn, creating it if absent.
func (m *ConnStateMap[T]) Get(conn *sql.Conn) *T {
	if conn == nil {
		return nil
	}
	var key any
	_ = conn.Raw(func(dc any) error {
		key = dc
		return nil
	})
	if key == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.maxSize <= 0 {
		m.maxSize = 64
	}
	if m.entries == nil {
		m.entries = make(map[any]*T)
	}
	if st, ok := m.entries[key]; ok {
		return st
	}
	// Evict stale entries when exceeding bounded pool capacity.
	if len(m.entries) >= m.maxSize {
		clear(m.entries)
	}
	st := new(T)
	m.entries[key] = st
	return st
}

// Delete removes any cached state associated with conn's underlying driver.Conn.
func (m *ConnStateMap[T]) Delete(conn *sql.Conn) {
	if conn == nil || m == nil {
		return
	}
	var key any
	_ = conn.Raw(func(dc any) error {
		key = dc
		return nil
	})
	if key == nil {
		return
	}
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
}
