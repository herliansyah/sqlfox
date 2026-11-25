package driver

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
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
	inPGBackslashString := false // true while inside PostgreSQL E'...' / e'...' escape string
	inLineComment := false
	blockCommentDepth := 0
	dollarTag := "" // non-empty while inside a dollar-quoted body

	runes := []rune(content)
	n := len(runes)

	isQuoteEscaped := func(i int) bool {
		if !isMySQL && !inPGBackslashString {
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
		if blockCommentDepth > 0 {
			current.WriteRune(r)
			if !isMySQL && r == '/' && i+1 < n && runes[i+1] == '*' {
				blockCommentDepth++
				current.WriteRune('*')
				i++
				continue
			}
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				blockCommentDepth--
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
				// In MySQL/MariaDB, /*! ... */ and /*M! ... */ are executable SQL: do NOT treat them
				// as opaque comments that hide semicolons.
				if isMySQL && i+2 < n && runes[i+2] == '!' {
					current.WriteString("/*!")
					i += 2
					continue
				}
				if isMySQL && i+3 < n && (runes[i+2] == 'M' || runes[i+2] == 'm') && runes[i+3] == '!' {
					current.WriteString("/*")
					current.WriteRune(runes[i+2])
					current.WriteRune('!')
					i += 3
					continue
				}
				blockCommentDepth = 1
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
				// PostgreSQL E'...' / e'...' escape string literal
				if (r == 'E' || r == 'e') && i+1 < n && runes[i+1] == '\'' && (i == 0 || !(runes[i-1] == '_' || unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]))) {
					inSingleQuote = true
					inPGBackslashString = true
					current.WriteRune(r)
					current.WriteRune('\'')
					i++
					continue
				}
			}
		}

		switch {
		case r == '\'' && !inDoubleQuote && !inBacktick:
			if inSingleQuote && isQuoteEscaped(i) {
				// Backslash-escaped literal quote (\' in MySQL or PostgreSQL E'...'):
				// stays inside string and must NOT pair with a following closing quote (e.g. E'\'').
				break
			}
			if inSingleQuote && i+1 < n && runes[i+1] == '\'' {
				current.WriteString("''")
				i++
				continue
			}
			inSingleQuote = !inSingleQuote
			if !inSingleQuote {
				inPGBackslashString = false
			}
		case r == '"' && !inSingleQuote && !inBacktick:
			if inDoubleQuote && isQuoteEscaped(i) {
				break
			}
			if inDoubleQuote && i+1 < n && runes[i+1] == '"' {
				current.WriteString(`""`)
				i++
				continue
			}
			inDoubleQuote = !inDoubleQuote
		case r == '`' && !inSingleQuote && !inDoubleQuote:
			if inBacktick && i+1 < n && runes[i+1] == '`' {
				current.WriteString("``")
				i++
				continue
			}
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

// errStopScan is an internal sentinel to halt streaming early without reporting an error.
var ErrStopScan = errors.New("stop scan")

// ScanSQLStatements streams SQL statements from r one-by-one into fn using a bounded bufio.Reader
// instead of loading the entire script into memory or converting it to []rune.
// Honors quotes, comments, MySQL /*! / /*M! executable comments, PostgreSQL dollar-quoting and E'...',
// and MySQL DELIMITER directives. Returning ErrStopScan from fn halts scanning cleanly with nil error.
func ScanSQLStatements(r io.Reader, driverName string, fn func(stmt string) error) error {
	isMySQL := !strings.EqualFold(driverName, "postgres") && !strings.EqualFold(driverName, "postgresql") && !strings.EqualFold(driverName, "sqlite") && !strings.EqualFold(driverName, "sqlite3")
	br := bufio.NewReaderSize(r, 64*1024)

	// Strip UTF-8 BOM if present at start of stream.
	if bom, err := br.Peek(3); err == nil && len(bom) == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		_, _ = br.Discard(3)
	}

	var current strings.Builder
	delim := ";"
	delimLastRune := ';'

	inSingleQuote, inDoubleQuote, inBacktick := false, false, false
	inPGBackslashString := false
	inLineComment := false
	blockCommentDepth := 0
	dollarTag := ""
	backslashCount := 0
	prevRune := rune(0)
	atLineStart := true

	emitCurrent := func() error {
		stmt := strings.TrimSpace(current.String())
		current.Reset()
		if stmt != "" && HasExecutableSQL(stmt, isMySQL) {
			if err := fn(stmt); err != nil {
				return err
			}
		}
		return nil
	}

	peekString := func(n int) string {
		b, _ := br.Peek(n)
		return string(b)
	}

	readDollarTag := func() string {
		// Current rune '$' was already consumed. Peek up to 64 bytes for optional tag + closing '$'.
		b, _ := br.Peek(64)
		if len(b) == 0 {
			return ""
		}
		// Fast path for $$
		if b[0] == '$' {
			_, _ = br.ReadByte()
			return "$$"
		}
		// Walk runes in peeked slice
		off := 0
		first := true
		for off < len(b) {
			ru, sz := utf8.DecodeRune(b[off:])
			if ru == '$' {
				tag := "$" + string(b[:off+sz])
				_, _ = br.Discard(off + sz)
				return tag
			}
			valid := ru == '_' || unicode.IsLetter(ru) || (!first && unicode.IsDigit(ru))
			if !valid {
				return ""
			}
			first = false
			off += sz
		}
		return ""
	}

	for {
		// Check for DELIMITER directive at start of line when outside quotes/comments/dollar-quotes.
		if atLineStart && !inSingleQuote && !inDoubleQuote && !inBacktick && !inLineComment && blockCommentDepth == 0 && dollarTag == "" {
			// Skip leading horizontal spaces/tabs to detect indented DELIMITER lines without losing them if not DELIMITER.
			peek := peekString(32)
			trimmedLeft := strings.TrimLeft(peek, " \t")
			if len(trimmedLeft) >= len("DELIMITER ") &&
				strings.EqualFold(trimmedLeft[:len("DELIMITER")], "DELIMITER") &&
				(trimmedLeft[len("DELIMITER")] == ' ' || trimmedLeft[len("DELIMITER")] == '\t') {
				line, lineErr := br.ReadString('\n')
				if err := emitCurrent(); err != nil {
					if errors.Is(err, ErrStopScan) {
						return nil
					}
					return err
				}
				fields := strings.Fields(strings.TrimSpace(line)[len("DELIMITER"):])
				if len(fields) > 0 && fields[0] != "" {
					delim = fields[0]
					lastR, _ := utf8.DecodeLastRuneInString(delim)
					delimLastRune = lastR
				}
				backslashCount = 0
				prevRune = '\n'
				atLineStart = true
				if lineErr == io.EOF {
					break
				}
				if lineErr != nil {
					return lineErr
				}
				continue
			}
		}

		r, _, err := br.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if r == '\n' {
			atLineStart = true
		} else if r != '\r' {
			atLineStart = false
		}

		if inLineComment {
			current.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			backslashCount = 0
			prevRune = r
			continue
		}

		if blockCommentDepth > 0 {
			current.WriteRune(r)
			if !isMySQL && r == '/' && strings.HasPrefix(peekString(1), "*") {
				_, _, _ = br.ReadRune()
				current.WriteRune('*')
				blockCommentDepth++
				prevRune = '*'
				backslashCount = 0
				continue
			}
			if r == '*' && strings.HasPrefix(peekString(1), "/") {
				_, _, _ = br.ReadRune()
				current.WriteRune('/')
				blockCommentDepth--
				prevRune = '/'
				backslashCount = 0
				continue
			}
			backslashCount = 0
			prevRune = r
			continue
		}

		if dollarTag != "" {
			if r == '$' && len(dollarTag) >= 2 && strings.HasPrefix(peekString(len(dollarTag)-1), dollarTag[1:]) {
				_, _ = br.Discard(len(dollarTag) - 1)
				current.WriteString(dollarTag)
				dollarTag = ""
				backslashCount = 0
				prevRune = '$'
				continue
			}
			current.WriteRune(r)
			backslashCount = 0
			prevRune = r
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && strings.HasPrefix(peekString(1), "-") {
				_, _, _ = br.ReadRune()
				inLineComment = true
				current.WriteString("--")
				backslashCount = 0
				prevRune = '-'
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				current.WriteRune('#')
				backslashCount = 0
				prevRune = '#'
				continue
			}
			if r == '/' && strings.HasPrefix(peekString(1), "*") {
				_, _, _ = br.ReadRune() // consume '*'
				// MySQL/MariaDB executable comments /*! ... */ and /*M! ... */
				if isMySQL {
					p2 := peekString(2)
					if strings.HasPrefix(p2, "!") {
						_, _, _ = br.ReadRune()
						current.WriteString("/*!")
						backslashCount = 0
						prevRune = '!'
						continue
					}
					if len(p2) >= 2 && (p2[0] == 'M' || p2[0] == 'm') && p2[1] == '!' {
						mRune, _, _ := br.ReadRune()
						_, _, _ = br.ReadRune()
						current.WriteString("/*")
						current.WriteRune(mRune)
						current.WriteRune('!')
						backslashCount = 0
						prevRune = '!'
						continue
					}
				}
				blockCommentDepth = 1
				current.WriteString("/*")
				backslashCount = 0
				prevRune = '*'
				continue
			}
			if !isMySQL {
				if r == '$' {
					if tag := readDollarTag(); tag != "" {
						dollarTag = tag
						current.WriteString(tag)
						backslashCount = 0
						prevRune = '$'
						continue
					}
				}
				if (r == 'E' || r == 'e') && strings.HasPrefix(peekString(1), "'") &&
					(prevRune == 0 || !(prevRune == '_' || unicode.IsLetter(prevRune) || unicode.IsDigit(prevRune))) {
					_, _, _ = br.ReadRune() // consume '\''
					inSingleQuote = true
					inPGBackslashString = true
					current.WriteRune(r)
					current.WriteRune('\'')
					backslashCount = 0
					prevRune = '\''
					continue
				}
			}
		}

		isEscaped := (isMySQL || inPGBackslashString) && (backslashCount%2 == 1)
		switch {
		case r == '\'' && !inDoubleQuote && !inBacktick:
			if inSingleQuote && isEscaped {
				// Backslash-escaped literal quote
			} else if inSingleQuote && strings.HasPrefix(peekString(1), "'") {
				_, _, _ = br.ReadRune()
				current.WriteString("''")
				backslashCount = 0
				prevRune = '\''
				continue
			} else {
				inSingleQuote = !inSingleQuote
				if !inSingleQuote {
					inPGBackslashString = false
				}
			}
		case r == '"' && !inSingleQuote && !inBacktick:
			if inDoubleQuote && isEscaped {
				// Backslash-escaped literal double quote
			} else if inDoubleQuote && strings.HasPrefix(peekString(1), `"`) {
				_, _, _ = br.ReadRune()
				current.WriteString(`""`)
				backslashCount = 0
				prevRune = '"'
				continue
			} else {
				inDoubleQuote = !inDoubleQuote
			}
		case r == '`' && !inSingleQuote && !inDoubleQuote:
			if inBacktick && strings.HasPrefix(peekString(1), "`") {
				_, _, _ = br.ReadRune()
				current.WriteString("``")
				backslashCount = 0
				prevRune = '`'
				continue
			}
			inBacktick = !inBacktick
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if delim == ";" && r == ';' {
				if err := emitCurrent(); err != nil {
					if errors.Is(err, ErrStopScan) {
						return nil
					}
					return err
				}
				backslashCount = 0
				prevRune = r
				continue
			}
		}

		current.WriteRune(r)

		if !inSingleQuote && !inDoubleQuote && !inBacktick && delim != ";" && r == delimLastRune {
			curStr := current.String()
			if strings.HasSuffix(curStr, delim) {
				trimmed := strings.TrimSpace(curStr[:len(curStr)-len(delim)])
				current.Reset()
				if trimmed != "" && HasExecutableSQL(trimmed, isMySQL) {
					if err := fn(trimmed); err != nil {
						if errors.Is(err, ErrStopScan) {
							return nil
						}
						return err
					}
				}
				backslashCount = 0
				prevRune = r
				continue
			}
		}

		if r == '\\' {
			backslashCount++
		} else {
			backslashCount = 0
		}
		prevRune = r
	}

	if err := emitCurrent(); err != nil && !errors.Is(err, ErrStopScan) {
		return err
	}
	return nil
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
				if isMySQL && i+3 < n && (runes[i+2] == 'M' || runes[i+2] == 'm') && runes[i+3] == '!' {
					// In MariaDB, /*M! ... */ is an executable comment containing SQL
					tokenCount++
					i += 3
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
				// Double single-quote works identically under both default MySQL sql_mode
				// and NO_BACKSLASH_ESCAPES (ANSI mode), preventing quote-escape SQLi.
				sb.WriteString(`''`)
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

// hexLiteral renders raw bytes as a dialect-correct binary literal (PostgreSQL X'' is a bit string, not bytea).
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
