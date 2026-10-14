package safety

import (
	"fmt"
	"regexp"
	"strings"

	"sqlfox/internal/driver"

	"github.com/xwb1989/sqlparser"
)

type RiskLevel string

const (
	RiskNone      RiskLevel = "NONE"
	RiskWarning   RiskLevel = "WARNING"
	RiskDangerous RiskLevel = "DANGEROUS"
	RiskCritical  RiskLevel = "CRITICAL"
)

// InspectionResult details the safety evaluation of a SQL query.
type InspectionResult struct {
	RiskLevel         RiskLevel                      `json:"risk_level"`
	IsDestructive     bool                           `json:"is_destructive"`
	RequiresChallenge bool                           `json:"requires_challenge"`
	TargetTable       string                         `json:"target_table,omitempty"`
	TargetType        string                         `json:"target_type,omitempty"`
	Operation         string                         `json:"operation,omitempty"`
	Reason            string                         `json:"reason,omitempty"`
	Suggestion        string                         `json:"suggestion,omitempty"`
	TableLocks        []driver.TableLockInfo         `json:"table_locks,omitempty"`
	Relationships     []driver.TableRelationshipInfo `json:"relationships,omitempty"`
}

// Inspect analyzes a query string against the target environment (Production, Staging, Development).
func Inspect(query string, environment string) *InspectionResult {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	clean := driver.StripLeadingComments(trimmed)
	if clean == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	// The Vitess parser has no CTE support, so inspect the main statement behind WITH ... AS (...).
	if main, ok := stripLeadingCTE(clean); ok {
		return Inspect(main, environment)
	}

	// Try AST parsing first (Vitess parser)
	stmt, err := sqlparser.Parse(clean)
	if err == nil {
		return inspectAST(stmt, environment)
	}

	// ponytail: Lexical fallback for statements Vitess parser doesn't cover (e.g. multi-statements or SQLite specific syntax); upgrade path: dedicated SQLite lexer
	return inspectLexicalFallback(clean, environment)
}

func inspectAST(stmt sqlparser.Statement, environment string) *InspectionResult {
	isProd := strings.EqualFold(environment, "Production")

	switch node := stmt.(type) {
	case *sqlparser.Delete:
		tableName := sqlparser.String(node.TableExprs)
		if node.Where == nil {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetTable:       cleanTableName(tableName),
				Operation:         "DELETE",
				Reason:            fmt.Sprintf("Unqualified DELETE statement on '%s' without a WHERE clause will delete ALL rows.", tableName),
				Suggestion:        "Add a specific WHERE clause targeting rows (e.g. WHERE id = ?).",
			}
		}
		if isProd {
			return &InspectionResult{
				RiskLevel:     RiskWarning,
				IsDestructive: true,
				TargetTable:   cleanTableName(tableName),
				Operation:     "DELETE",
				Reason:        fmt.Sprintf("DELETE query executing on PRODUCTION environment on table '%s'.", tableName),
			}
		}

	case *sqlparser.Update:
		tableName := sqlparser.String(node.TableExprs)
		if node.Where == nil {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetTable:       cleanTableName(tableName),
				Operation:         "UPDATE",
				Reason:            fmt.Sprintf("Unqualified UPDATE statement on '%s' without a WHERE clause will modify ALL rows.", tableName),
				Suggestion:        "Add a specific WHERE clause targeting rows (e.g. WHERE id = ?).",
			}
		}
		if isProd {
			return &InspectionResult{
				RiskLevel:     RiskWarning,
				IsDestructive: true,
				TargetTable:   cleanTableName(tableName),
				Operation:     "UPDATE",
				Reason:        fmt.Sprintf("UPDATE query executing on PRODUCTION environment on table '%s'.", tableName),
			}
		}

	case *sqlparser.DDL:
		tableName := node.Table.Name.String()
		action := strings.ToLower(node.Action)

		switch action {
		case "drop":
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetTable:       tableName,
				Operation:         "DROP",
				Reason:            fmt.Sprintf("DROP statement will permanently delete table or database '%s'.", tableName),
				Suggestion:        "Ensure backups exist before dropping schema objects.",
			}
		case "truncate":
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetTable:       tableName,
				Operation:         "TRUNCATE",
				Reason:            fmt.Sprintf("TRUNCATE statement will empty ALL records from '%s'.", tableName),
				Suggestion:        "Consider soft-deleting or creating a backup snapshot before truncating.",
			}
		case "alter":
			return &InspectionResult{
				RiskLevel:         RiskDangerous,
				IsDestructive:     true,
				RequiresChallenge: isProd,
				TargetTable:       tableName,
				Operation:         "ALTER",
				Reason:            fmt.Sprintf("ALTER TABLE modifies schema structure on '%s'.", tableName),
			}
		}
	}

	return &InspectionResult{RiskLevel: RiskNone}
}

var (
	reDrop     = regexp.MustCompile(`(?i)^\s*DROP\s+(TABLE|DATABASE|VIEW)\s+(IF\s+EXISTS\s+)?([` + "`" + `"'\w\.\-]+)`)
	reTruncate = regexp.MustCompile(`(?i)^\s*TRUNCATE\s+(TABLE\s+)?([` + "`" + `"'\w\.\-]+)`)
	reDelete   = regexp.MustCompile(`(?i)^\s*DELETE\s+FROM\s+([` + "`" + `"'\w\.\-]+)`)
	reUpdate   = regexp.MustCompile(`(?i)^\s*UPDATE\s+([` + "`" + `"'\w\.\-]+)`)
	reAlter    = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([` + "`" + `"'\w\.\-]+)`)
	reWhere    = regexp.MustCompile(`(?i)\bWHERE\b`)
)

// maskCommentsAndLiterals blanks out comments, quoted strings and quoted identifiers so keyword
// regexes (e.g. WHERE) only see real SQL tokens.
func maskCommentsAndLiterals(q string) string {
	b := []byte(q)
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '-' && i+1 < len(b) && b[i+1] == '-', b[i] == '#':
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
		}
	}
	return string(b)
}

func inspectLexicalFallback(query string, environment string) *InspectionResult {
	isProd := strings.EqualFold(environment, "Production")

	if match := reDrop.FindStringSubmatch(query); len(match) > 3 {
		target := cleanTableName(match[3])
		return &InspectionResult{
			RiskLevel:         RiskCritical,
			IsDestructive:     true,
			RequiresChallenge: true,
			TargetTable:       target,
			TargetType:        strings.ToUpper(match[1]),
			Operation:         "DROP",
			Reason:            fmt.Sprintf("DROP statement will permanently delete '%s'.", target),
		}
	}

	if match := reTruncate.FindStringSubmatch(query); len(match) > 2 {
		target := cleanTableName(match[2])
		return &InspectionResult{
			RiskLevel:         RiskCritical,
			IsDestructive:     true,
			RequiresChallenge: true,
			TargetTable:       target,
			Operation:         "TRUNCATE",
			Reason:            fmt.Sprintf("TRUNCATE statement will permanently empty '%s'.", target),
		}
	}

	if match := reDelete.FindStringSubmatch(query); len(match) > 1 {
		target := cleanTableName(match[1])
		if !reWhere.MatchString(maskCommentsAndLiterals(query)) {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetTable:       target,
				Operation:         "DELETE",
				Reason:            fmt.Sprintf("Unqualified DELETE statement on '%s' without a WHERE clause will delete ALL rows.", target),
			}
		}
		if isProd {
			return &InspectionResult{
				RiskLevel:     RiskWarning,
				IsDestructive: true,
				TargetTable:   target,
				Operation:     "DELETE",
				Reason:        fmt.Sprintf("DELETE on PRODUCTION environment targeting '%s'.", target),
			}
		}
	}

	if match := reUpdate.FindStringSubmatch(query); len(match) > 1 {
		target := cleanTableName(match[1])
		if !reWhere.MatchString(maskCommentsAndLiterals(query)) {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetTable:       target,
				Operation:         "UPDATE",
				Reason:            fmt.Sprintf("Unqualified UPDATE statement on '%s' without a WHERE clause will modify ALL rows.", target),
			}
		}
		if isProd {
			return &InspectionResult{
				RiskLevel:     RiskWarning,
				IsDestructive: true,
				TargetTable:   target,
				Operation:     "UPDATE",
				Reason:        fmt.Sprintf("UPDATE on PRODUCTION environment targeting '%s'.", target),
			}
		}
	}

	if match := reAlter.FindStringSubmatch(query); len(match) > 1 {
		target := cleanTableName(match[1])
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: isProd,
			TargetTable:       target,
			Operation:         "ALTER",
			Reason:            fmt.Sprintf("ALTER TABLE modifies schema structure on '%s'.", target),
		}
	}

	return &InspectionResult{RiskLevel: RiskNone}
}

func cleanTableName(raw string) string {
	s := strings.TrimSpace(raw)
	parts := strings.Split(s, ".")
	last := parts[len(parts)-1]
	return strings.Trim(last, "`'\"")
}

var reCTEStart = regexp.MustCompile(`(?i)^WITH\b`)

// stripLeadingCTE returns the main statement following a WITH clause, i.e. the first
// SELECT/INSERT/UPDATE/DELETE/MERGE keyword found outside parentheses and quotes.
func stripLeadingCTE(q string) (string, bool) {
	if !reCTEStart.MatchString(q) {
		return "", false
	}
	depth := 0
	var quote byte
	for i := 4; i < len(q); i++ {
		c := q[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth != 0 || !isIdentByte(c) || isIdentByte(q[i-1]) {
				continue
			}
			j := i
			for j < len(q) && isIdentByte(q[j]) {
				j++
			}
			switch strings.ToUpper(q[i:j]) {
			case "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE":
				return q[i:], true
			}
			i = j - 1
		}
	}
	return "", false
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
