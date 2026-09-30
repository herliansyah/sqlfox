package safety

import (
	"fmt"
	"regexp"
	"strings"

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
	RiskLevel         RiskLevel `json:"risk_level"`
	IsDestructive     bool      `json:"is_destructive"`
	RequiresChallenge bool      `json:"requires_challenge"`
	TargetTable       string    `json:"target_table,omitempty"`
	Operation         string    `json:"operation,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	Suggestion        string    `json:"suggestion,omitempty"`
}

// ponytail: Inspect is a pure deterministic package function; SafetyGuard struct is kept as lightweight alias.
type SafetyGuard struct{}

func NewSafetyGuard() *SafetyGuard {
	return &SafetyGuard{}
}

func (g *SafetyGuard) Inspect(query string, environment string) *InspectionResult {
	return Inspect(query, environment)
}

// Inspect analyzes a query string against the target environment (Production, Staging, Development).
func Inspect(query string, environment string) *InspectionResult {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	// Try AST parsing first (Vitess parser)
	stmt, err := sqlparser.Parse(trimmed)
	if err == nil {
		return inspectAST(stmt, environment)
	}

	// ponytail: Lexical fallback for statements Vitess parser doesn't cover (e.g. multi-statements or SQLite specific syntax); upgrade path: dedicated SQLite lexer
	return inspectLexicalFallback(trimmed, environment)
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
	reDrop     = regexp.MustCompile(`(?i)^\s*DROP\s+(TABLE|DATABASE|VIEW)\s+(IF\s+EXISTS\s+)?([` + "`" + `"'\w\.]+)`)
	reTruncate = regexp.MustCompile(`(?i)^\s*TRUNCATE\s+(TABLE\s+)?([` + "`" + `"'\w\.]+)`)
	reDelete   = regexp.MustCompile(`(?i)^\s*DELETE\s+FROM\s+([` + "`" + `"'\w\.]+)`)
	reUpdate   = regexp.MustCompile(`(?i)^\s*UPDATE\s+([` + "`" + `"'\w\.]+)`)
	reWhere    = regexp.MustCompile(`(?i)\bWHERE\b`)
)

func inspectLexicalFallback(query string, environment string) *InspectionResult {
	isProd := strings.EqualFold(environment, "Production")

	if match := reDrop.FindStringSubmatch(query); len(match) > 3 {
		target := cleanTableName(match[3])
		return &InspectionResult{
			RiskLevel:         RiskCritical,
			IsDestructive:     true,
			RequiresChallenge: true,
			TargetTable:       target,
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
		if !reWhere.MatchString(query) {
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
		if !reWhere.MatchString(query) {
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

	return &InspectionResult{RiskLevel: RiskNone}
}

func cleanTableName(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "`'\"")
	parts := strings.Split(s, ".")
	return parts[len(parts)-1]
}
