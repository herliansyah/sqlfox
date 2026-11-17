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

// StatementEstimate summarizes the estimated pre-execution blast radius of a statement in a batch.
type StatementEstimate struct {
	StatementIndex    int    `json:"statement_index"`
	SQL               string `json:"sql"`
	Operation         string `json:"operation"`
	TargetTable       string `json:"target_table"`
	TargetDatabase    string `json:"target_database,omitempty"`
	HasWhere          bool   `json:"has_where"`
	CountQuery        string `json:"count_query,omitempty"`
	EstimatedRows     *int64 `json:"estimated_rows,omitempty"`
	EstimatedIsExact  bool   `json:"estimated_is_exact,omitempty"`
	RequiresChallenge bool   `json:"requires_challenge"`
	Reason            string `json:"reason,omitempty"`
}

// InspectionResult details the safety evaluation of a SQL query.
type InspectionResult struct {
	RiskLevel           RiskLevel                      `json:"risk_level"`
	IsDestructive       bool                           `json:"is_destructive"`
	RequiresChallenge   bool                           `json:"requires_challenge"`
	RequiresAffirmation bool                           `json:"requires_affirmation,omitempty"`
	SchemaScopeMismatch bool                           `json:"schema_scope_mismatch,omitempty"`
	TargetDatabase      string                         `json:"target_database,omitempty"`
	TargetTable         string                         `json:"target_table,omitempty"`
	TargetType          string                         `json:"target_type,omitempty"`
	Operation           string                         `json:"operation,omitempty"`
	Reason              string                         `json:"reason,omitempty"`
	Suggestion          string                         `json:"suggestion,omitempty"`
	TableLocks          []driver.TableLockInfo         `json:"table_locks,omitempty"`
	Relationships       []driver.TableRelationshipInfo `json:"relationships,omitempty"`
	HasWhere            bool                           `json:"has_where,omitempty"`
	CountQuery          string                         `json:"count_query,omitempty"`
	EstimatedRows       *int64                         `json:"estimated_rows,omitempty"`
	EstimatedIsExact    bool                           `json:"estimated_is_exact,omitempty"`
	StatementEstimates  []StatementEstimate            `json:"statement_estimates,omitempty"`
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

	// Multi-statement inspection: if query contains multiple SQL statements, inspect every
	// statement so destructive statements hidden after a benign statement (e.g. "SELECT 1; DROP TABLE users;")
	// cannot slip through!
	stmts := driver.SplitSQLStatements(clean, "")
	if len(stmts) > 1 {
		return inspectMultiStatements(stmts, environment)
	}

	// The Vitess parser has no CTE support, so inspect both CTE sub-bodies (for data-modifying CTEs)
	// and the main statement behind WITH ... AS (...).
	if cteStmts, ok := extractCTEStatements(clean); ok {
		return inspectMultiStatements(cteStmts, environment)
	}

	// Try AST parsing first (Vitess parser)
	stmt, err := sqlparser.Parse(clean)
	if err == nil {
		return inspectAST(stmt, environment, clean)
	}

	// ponytail: Lexical fallback for statements Vitess parser doesn't cover (e.g. multi-statements or SQLite specific syntax); upgrade path: dedicated SQLite lexer
	return inspectLexicalFallback(clean, environment)
}

func inspectAST(stmt sqlparser.Statement, environment, query string) *InspectionResult {
	isProd := strings.EqualFold(environment, "Production")
	isStaging := strings.EqualFold(environment, "Staging")

	switch node := stmt.(type) {
	case *sqlparser.Delete:
		tableName := sqlparser.String(node.TableExprs)
		db, targetTbl := extractTargetFromTableExprs(node.TableExprs, tableName)
		if node.Where == nil || !hasWhereOutsideLiterals(query) {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetDatabase:    db,
				TargetTable:       targetTbl,
				Operation:         "DELETE",
				Reason:            fmt.Sprintf("Unqualified DELETE statement on '%s' without a WHERE clause will delete ALL rows.", targetTbl),
				Suggestion:        "Add a specific WHERE clause targeting rows (e.g. WHERE id = ?).",
				CountQuery:        fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName),
			}
		}
		whereClause := strings.TrimSpace(sqlparser.String(node.Where))
		countQ := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", tableName, whereClause)
		res := &InspectionResult{
			IsDestructive:  true,
			HasWhere:       true,
			TargetDatabase: db,
			TargetTable:    targetTbl,
			Operation:      "DELETE",
			CountQuery:     countQ,
		}
		if isProd {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("DELETE query executing on PRODUCTION environment on table '%s'.", targetTbl)
		} else if isStaging {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("DELETE query executing on STAGING environment on table '%s'.", targetTbl)
		} else {
			res.RiskLevel = RiskNone
			res.Reason = fmt.Sprintf("DELETE query targeting table '%s'.", targetTbl)
		}
		return res

	case *sqlparser.Update:
		tableName := sqlparser.String(node.TableExprs)
		db, targetTbl := extractTargetFromTableExprs(node.TableExprs, tableName)
		if node.Where == nil || !hasWhereOutsideLiterals(query) {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetDatabase:    db,
				TargetTable:       targetTbl,
				Operation:         "UPDATE",
				Reason:            fmt.Sprintf("Unqualified UPDATE statement on '%s' without a WHERE clause will modify ALL rows.", targetTbl),
				Suggestion:        "Add a specific WHERE clause targeting rows (e.g. WHERE id = ?).",
				CountQuery:        fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName),
			}
		}
		whereClause := strings.TrimSpace(sqlparser.String(node.Where))
		countQ := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", tableName, whereClause)
		res := &InspectionResult{
			IsDestructive:  true,
			HasWhere:       true,
			TargetDatabase: db,
			TargetTable:    targetTbl,
			Operation:      "UPDATE",
			CountQuery:     countQ,
		}
		if isProd {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("UPDATE query executing on PRODUCTION environment on table '%s'.", targetTbl)
		} else if isStaging {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("UPDATE query executing on STAGING environment on table '%s'.", targetTbl)
		} else {
			res.RiskLevel = RiskNone
			res.Reason = fmt.Sprintf("UPDATE query targeting table '%s'.", targetTbl)
		}
		return res

	case *sqlparser.DDL:
		tableName := node.Table.Name.String()
		if tableName == "" {
			tableName = node.NewName.Name.String()
		}
		qualifier := node.Table.Qualifier.String()
		if qualifier == "" {
			qualifier = node.NewName.Qualifier.String()
		}
		action := strings.ToLower(node.Action)

		switch action {
		case "create":
			return &InspectionResult{
				RiskLevel:           RiskWarning,
				IsDestructive:       false,
				RequiresChallenge:   isProd,
				RequiresAffirmation: true,
				TargetDatabase:      qualifier,
				TargetTable:         tableName,
				TargetType:          "TABLE",
				Operation:           "CREATE",
				Reason:              fmt.Sprintf("CREATE TABLE will create schema object '%s'.", tableName),
			}
		case "drop":
			return &InspectionResult{
				RiskLevel:           RiskCritical,
				IsDestructive:       true,
				RequiresChallenge:   true,
				RequiresAffirmation: true,
				TargetDatabase:      qualifier,
				TargetTable:         tableName,
				TargetType:          "TABLE",
				Operation:           "DROP",
				Reason:              fmt.Sprintf("DROP statement will permanently delete table or database '%s'.", tableName),
				Suggestion:          "Ensure backups exist before dropping schema objects.",
			}
		case "truncate":
			return &InspectionResult{
				RiskLevel:           RiskCritical,
				IsDestructive:       true,
				RequiresChallenge:   true,
				RequiresAffirmation: true,
				TargetDatabase:      qualifier,
				TargetTable:         tableName,
				TargetType:          "TABLE",
				Operation:           "TRUNCATE",
				Reason:              fmt.Sprintf("TRUNCATE statement will empty ALL records from '%s'.", tableName),
				Suggestion:          "Consider soft-deleting or creating a backup snapshot before truncating.",
			}
		case "alter":
			return &InspectionResult{
				RiskLevel:           RiskDangerous,
				IsDestructive:       true,
				RequiresChallenge:   isProd,
				RequiresAffirmation: true,
				TargetDatabase:      qualifier,
				TargetTable:         tableName,
				TargetType:          "TABLE",
				Operation:           "ALTER",
				Reason:              fmt.Sprintf("ALTER TABLE modifies schema structure on '%s'.", tableName),
			}
		case "rename":
			return &InspectionResult{
				RiskLevel:           RiskDangerous,
				IsDestructive:       true,
				RequiresChallenge:   isProd,
				RequiresAffirmation: true,
				TargetDatabase:      qualifier,
				TargetTable:         tableName,
				TargetType:          "TABLE",
				Operation:           "RENAME",
				Reason:              fmt.Sprintf("RENAME TABLE modifies schema object '%s'.", tableName),
			}
		}

	case *sqlparser.Insert:
		if strings.EqualFold(node.Action, sqlparser.ReplaceStr) {
			qualifier := node.Table.Qualifier.String()
			tableName := node.Table.Name.String()
			res := &InspectionResult{
				IsDestructive:  true,
				TargetDatabase: qualifier,
				TargetTable:    tableName,
				TargetType:     "TABLE",
				Operation:      "REPLACE",
			}
			if isProd || isStaging {
				res.RiskLevel = RiskWarning
				res.RequiresAffirmation = true
				res.Reason = fmt.Sprintf("REPLACE INTO on %s environment targeting table '%s' (deletes and re-inserts conflicting rows).", strings.ToUpper(environment), tableName)
			} else {
				res.RiskLevel = RiskNone
				res.Reason = fmt.Sprintf("REPLACE INTO targeting table '%s'.", tableName)
			}
			return res
		}

	case *sqlparser.DBDDL:
		if strings.EqualFold(node.Action, sqlparser.DropStr) {
			return &InspectionResult{
				RiskLevel:           RiskCritical,
				IsDestructive:       true,
				RequiresChallenge:   true,
				RequiresAffirmation: true,
				TargetDatabase:      node.DBName,
				TargetTable:         node.DBName,
				TargetType:          "DATABASE",
				Operation:           "DROP",
				Reason:              fmt.Sprintf("DROP DATABASE will permanently delete database '%s' and every object in it.", node.DBName),
				Suggestion:          "Ensure a full dump exists before dropping a database.",
			}
		}
		if strings.EqualFold(node.Action, sqlparser.CreateStr) {
			return &InspectionResult{
				RiskLevel:           RiskWarning,
				RequiresChallenge:   isProd,
				RequiresAffirmation: true,
				TargetDatabase:      node.DBName,
				TargetTable:         node.DBName,
				TargetType:          "DATABASE",
				Operation:           "CREATE",
				Reason:              fmt.Sprintf("CREATE DATABASE will create database '%s'.", node.DBName),
			}
		}
	}

	return &InspectionResult{RiskLevel: RiskNone}
}

func inspectMultiStatements(stmts []string, environment string) *InspectionResult {
	var highestRes *InspectionResult
	var estimates []StatementEstimate
	hasDestructive := false
	hasChallenge := false
	hasAffirmation := false

	for idx, s := range stmts {
		res := Inspect(s, environment)
		if res.IsDestructive {
			hasDestructive = true
		}
		if res.RequiresChallenge {
			hasChallenge = true
		}
		if res.RequiresAffirmation {
			hasAffirmation = true
		}

		if res.IsDestructive || res.RequiresChallenge || res.RequiresAffirmation {
			estimates = append(estimates, StatementEstimate{
				StatementIndex:    idx + 1,
				SQL:               s,
				Operation:         res.Operation,
				TargetTable:       res.TargetTable,
				TargetDatabase:    res.TargetDatabase,
				HasWhere:          res.HasWhere,
				CountQuery:        res.CountQuery,
				EstimatedRows:     res.EstimatedRows,
				EstimatedIsExact:  res.EstimatedIsExact,
				RequiresChallenge: res.RequiresChallenge,
				Reason:            res.Reason,
			})
		}

		if highestRes == nil || riskPrecedence(res.RiskLevel) > riskPrecedence(highestRes.RiskLevel) {
			highestRes = res
		}
	}

	if highestRes == nil {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	cloned := *highestRes
	cloned.IsDestructive = hasDestructive || cloned.IsDestructive
	cloned.RequiresChallenge = hasChallenge || cloned.RequiresChallenge
	cloned.RequiresAffirmation = hasAffirmation || cloned.RequiresAffirmation
	if len(estimates) > 0 {
		cloned.StatementEstimates = estimates
	}
	return &cloned
}

func riskPrecedence(r RiskLevel) int {
	switch r {
	case RiskCritical:
		return 4
	case RiskDangerous:
		return 3
	case RiskWarning:
		return 2
	case RiskNone:
		return 1
	default:
		return 0
	}
}

func extractTargetFromTableExprs(exprs sqlparser.TableExprs, fallbackRaw string) (string, string) {
	if len(exprs) > 0 {
		if aliased, ok := exprs[0].(*sqlparser.AliasedTableExpr); ok {
			if tn, ok := aliased.Expr.(sqlparser.TableName); ok {
				return tn.Qualifier.String(), tn.Name.String()
			}
		}
	}
	return extractDBAndTable(fallbackRaw)
}

var (
	reDrop     = regexp.MustCompile(`(?i)^\s*DROP\s+(TABLE|DATABASE|SCHEMA|VIEW|MATERIALIZED\s+VIEW|INDEX|SEQUENCE|FUNCTION|PROCEDURE|TRIGGER|EVENT|TYPE|DOMAIN|EXTENSION)\s+(IF\s+EXISTS\s+)?([` + "`" + `"'\[\]\w\.\-]+)`)
	reTruncate = regexp.MustCompile(`(?i)^\s*TRUNCATE\s+(TABLE\s+)?(?:ONLY\s+)?([` + "`" + `"'\[\]\w\.\-]+)`)
	reDelete   = regexp.MustCompile(`(?i)^\s*DELETE\s+FROM\s+(?:ONLY\s+)?([` + "`" + `"'\[\]\w\.\-]+)`)
	reUpdate   = regexp.MustCompile(`(?i)^\s*UPDATE\s+(?:ONLY\s+)?([` + "`" + `"'\[\]\w\.\-]+)`)
	reAlter    = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+(?:ONLY\s+)?([` + "`" + `"'\[\]\w\.\-]+)`)
	reRename   = regexp.MustCompile(`(?i)^\s*RENAME\s+TABLE\s+([` + "`" + `"'\[\]\w\.\-]+)`)
	reReplace  = regexp.MustCompile(`(?i)^\s*REPLACE\s+(?:INTO\s+)?([` + "`" + `"'\[\]\w\.\-]+)`)
	reCreate   = regexp.MustCompile(`(?i)^\s*CREATE\s+(TABLE|DATABASE|SCHEMA|VIEW|INDEX|UNIQUE\s+INDEX)\s+(IF\s+NOT\s+EXISTS\s+)?([` + "`" + `"'\[\]\w\.\-]+)`)
	reWhere    = regexp.MustCompile(`(?i)\bWHERE\b`)
)

// maskCommentsAndLiterals blanks out comments, quoted strings and quoted identifiers so keyword
// regexes (e.g. WHERE) only see real SQL tokens. The guard has no dialect, so callers use
// hasWhereOutsideLiterals, which requires a WHERE to survive both MySQL-style lexing (backslash
// escapes, # comments) and standard-SQL lexing (PostgreSQL/SQLite: no backslash escapes, nested block
// comments, $tag$ dollar quotes). Masking more than the real dialect only makes the guard stricter.
func maskCommentsAndLiterals(q string) string { return maskSQL(q, true) }

func maskSQL(q string, mysqlLexing bool) string {
	b := []byte(q)
	blank := func(from, to int) {
		for k := from; k < to && k < len(b); k++ {
			b[k] = ' '
		}
	}
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '-' && i+1 < len(b) && b[i+1] == '-', mysqlLexing && b[i] == '#':
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			depth, j := 1, i+2
			for j < len(b) && depth > 0 {
				if b[j] == '*' && j+1 < len(b) && b[j+1] == '/' {
					depth--
					j += 2
					continue
				}
				if !mysqlLexing && b[j] == '/' && j+1 < len(b) && b[j+1] == '*' {
					depth++
					j += 2
					continue
				}
				j++
			}
			blank(i, j)
			i = j - 1
		case !mysqlLexing && b[i] == '$':
			// PostgreSQL dollar quote: $$...$$ or $tag$...$tag$
			j := i + 1
			for j < len(b) && (b[j] == '_' || b[j] >= 'a' && b[j] <= 'z' || b[j] >= 'A' && b[j] <= 'Z' || b[j] >= '0' && b[j] <= '9') {
				j++
			}
			if j < len(b) && b[j] == '$' && (j == i+1 || !(b[i+1] >= '0' && b[i+1] <= '9')) {
				tag := q[i : j+1]
				end := strings.Index(q[j+1:], tag)
				stop := len(b)
				if end >= 0 {
					stop = j + 1 + end + len(tag)
				}
				blank(i, stop)
				i = stop - 1
			}
		case b[i] == '[':
			// SQLite / MS SQL bracket-quoted identifier: [my table]
			b[i] = ' '
			for i++; i < len(b); i++ {
				if b[i] == ']' {
					b[i] = ' '
					break
				}
				b[i] = ' '
			}
		case b[i] == '\'' || b[i] == '"' || b[i] == '`':
			quote := b[i]
			b[i] = ' '
			for i++; i < len(b); i++ {
				if mysqlLexing && b[i] == '\\' && quote != '`' && i+1 < len(b) {
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

// hasWhereOutsideLiterals reports whether a real WHERE keyword exists under every supported lexing.
func hasWhereOutsideLiterals(q string) bool {
	return reWhere.MatchString(maskSQL(q, true)) && reWhere.MatchString(maskSQL(q, false))
}

var reReturning = regexp.MustCompile(`(?i)\bRETURNING\b.*$`)

func buildLexicalCountQuery(table, query string) string {
	masked := maskCommentsAndLiterals(query)
	whereLoc := reWhere.FindStringIndex(masked)
	if whereLoc == nil {
		return fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	}
	clause := strings.TrimSpace(query[whereLoc[0]:])
	clause = reReturning.ReplaceAllString(clause, "")
	clause = strings.TrimRight(clause, "; \t\r\n")
	return fmt.Sprintf("SELECT COUNT(*) FROM %s %s", table, clause)
}

// collapseInlineComments replaces SQL comments outside quotes with spaces so keyword
// regexes cannot be bypassed via inline comments such as DELETE/*x*/FROM.
func collapseInlineComments(q string) string {
	b := []byte(q)
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			for i++; i < len(b); i++ {
				if b[i] == '\\' && quote != '`' && i+1 < len(b) {
					i++
					continue
				}
				if b[i] == quote {
					break
				}
			}
			continue
		}
		if (c == '-' && i+1 < len(b) && b[i+1] == '-') || c == '#' {
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
			continue
		}
		if c == '/' && i+1 < len(b) && b[i+1] == '*' {
			b[i], b[i+1] = ' ', ' '
			i += 2
			for i < len(b) {
				if b[i] == '*' && i+1 < len(b) && b[i+1] == '/' {
					b[i], b[i+1] = ' ', ' '
					i++
					break
				}
				b[i] = ' '
				i++
			}
		}
	}
	return string(b)
}

func inspectLexicalFallback(query string, environment string) *InspectionResult {
	if cteStmts, ok := extractCTEStatements(query); ok {
		return inspectMultiStatements(cteStmts, environment)
	}

	origQuery := query
	query = collapseInlineComments(query)

	isProd := strings.EqualFold(environment, "Production")
	isStaging := strings.EqualFold(environment, "Staging")

	if match := reDrop.FindStringSubmatch(query); len(match) > 3 {
		targetType := strings.ToUpper(match[1])
		db, target := extractDBAndTable(match[3])
		if targetType == "DATABASE" || targetType == "SCHEMA" {
			db = target
		}
		return &InspectionResult{
			RiskLevel:           RiskCritical,
			IsDestructive:       true,
			RequiresChallenge:   true,
			RequiresAffirmation: true,
			TargetDatabase:      db,
			TargetTable:         target,
			TargetType:          targetType,
			Operation:           "DROP",
			Reason:              fmt.Sprintf("DROP statement will permanently delete '%s'.", target),
		}
	}

	if match := reTruncate.FindStringSubmatch(query); len(match) > 2 {
		db, target := extractDBAndTable(match[2])
		return &InspectionResult{
			RiskLevel:           RiskCritical,
			IsDestructive:       true,
			RequiresChallenge:   true,
			RequiresAffirmation: true,
			TargetDatabase:      db,
			TargetTable:         target,
			TargetType:          "TABLE",
			Operation:           "TRUNCATE",
			Reason:              fmt.Sprintf("TRUNCATE statement will permanently empty '%s'.", target),
		}
	}

	if match := reDelete.FindStringSubmatch(query); len(match) > 1 {
		db, target := extractDBAndTable(match[1])
		hasWhere := hasWhereOutsideLiterals(origQuery)
		cleanTable := match[1]
		if !hasWhere {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetDatabase:    db,
				TargetTable:       target,
				Operation:         "DELETE",
				Reason:            fmt.Sprintf("Unqualified DELETE statement on '%s' without a WHERE clause will delete ALL rows.", target),
				CountQuery:        fmt.Sprintf("SELECT COUNT(*) FROM %s", cleanTable),
			}
		}
		countQ := buildLexicalCountQuery(cleanTable, origQuery)
		res := &InspectionResult{
			IsDestructive:  true,
			HasWhere:       true,
			TargetDatabase: db,
			TargetTable:    target,
			Operation:      "DELETE",
			CountQuery:     countQ,
		}
		if isProd {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("DELETE on PRODUCTION environment targeting '%s'.", target)
		} else if isStaging {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("DELETE on STAGING environment targeting '%s'.", target)
		} else {
			res.RiskLevel = RiskNone
			res.Reason = fmt.Sprintf("DELETE query targeting '%s'.", target)
		}
		return res
	}

	if match := reUpdate.FindStringSubmatch(query); len(match) > 1 {
		db, target := extractDBAndTable(match[1])
		hasWhere := hasWhereOutsideLiterals(origQuery)
		cleanTable := match[1]
		if !hasWhere {
			return &InspectionResult{
				RiskLevel:         RiskCritical,
				IsDestructive:     true,
				RequiresChallenge: true,
				TargetDatabase:    db,
				TargetTable:       target,
				Operation:         "UPDATE",
				Reason:            fmt.Sprintf("Unqualified UPDATE statement on '%s' without a WHERE clause will modify ALL rows.", target),
				CountQuery:        fmt.Sprintf("SELECT COUNT(*) FROM %s", cleanTable),
			}
		}
		countQ := buildLexicalCountQuery(cleanTable, origQuery)
		res := &InspectionResult{
			IsDestructive:  true,
			HasWhere:       true,
			TargetDatabase: db,
			TargetTable:    target,
			Operation:      "UPDATE",
			CountQuery:     countQ,
		}
		if isProd {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("UPDATE on PRODUCTION environment targeting '%s'.", target)
		} else if isStaging {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("UPDATE on STAGING environment targeting '%s'.", target)
		} else {
			res.RiskLevel = RiskNone
			res.Reason = fmt.Sprintf("UPDATE query targeting '%s'.", target)
		}
		return res
	}

	if match := reAlter.FindStringSubmatch(query); len(match) > 1 {
		db, target := extractDBAndTable(match[1])
		return &InspectionResult{
			RiskLevel:           RiskDangerous,
			IsDestructive:       true,
			RequiresChallenge:   isProd,
			RequiresAffirmation: true,
			TargetDatabase:      db,
			TargetTable:         target,
			TargetType:          "TABLE",
			Operation:           "ALTER",
			Reason:              fmt.Sprintf("ALTER TABLE modifies schema structure on '%s'.", target),
		}
	}

	if match := reRename.FindStringSubmatch(query); len(match) > 1 {
		db, target := extractDBAndTable(match[1])
		return &InspectionResult{
			RiskLevel:           RiskDangerous,
			IsDestructive:       true,
			RequiresChallenge:   isProd,
			RequiresAffirmation: true,
			TargetDatabase:      db,
			TargetTable:         target,
			TargetType:          "TABLE",
			Operation:           "RENAME",
			Reason:              fmt.Sprintf("RENAME TABLE modifies schema object '%s'.", target),
		}
	}

	if match := reReplace.FindStringSubmatch(query); len(match) > 1 {
		db, target := extractDBAndTable(match[1])
		res := &InspectionResult{
			IsDestructive:  true,
			TargetDatabase: db,
			TargetTable:    target,
			TargetType:     "TABLE",
			Operation:      "REPLACE",
		}
		if isProd || isStaging {
			res.RiskLevel = RiskWarning
			res.RequiresAffirmation = true
			res.Reason = fmt.Sprintf("REPLACE INTO on %s environment targeting '%s'.", strings.ToUpper(environment), target)
		} else {
			res.RiskLevel = RiskNone
			res.Reason = fmt.Sprintf("REPLACE INTO targeting '%s'.", target)
		}
		return res
	}

	if match := reCreate.FindStringSubmatch(query); len(match) > 3 {
		targetType := strings.ToUpper(match[1])
		db, target := extractDBAndTable(match[3])
		if targetType == "DATABASE" || targetType == "SCHEMA" {
			db = target
		}
		return &InspectionResult{
			RiskLevel:           RiskWarning,
			IsDestructive:       false,
			RequiresChallenge:   isProd,
			RequiresAffirmation: true,
			TargetDatabase:      db,
			TargetTable:         target,
			TargetType:          targetType,
			Operation:           "CREATE",
			Reason:              fmt.Sprintf("CREATE %s will create new schema object '%s'.", targetType, target),
		}
	}

	return &InspectionResult{RiskLevel: RiskNone}
}

func extractDBAndTable(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	parts := strings.Split(s, ".")
	if len(parts) >= 2 {
		db := strings.Trim(parts[len(parts)-2], "`'\"[]")
		tbl := strings.Trim(parts[len(parts)-1], "`'\"[]")
		return db, tbl
	}
	return "", cleanTableName(raw)
}

// InspectWithScope analyzes a query string against the target environment and scoped database.
func InspectWithScope(query string, environment string, scopedDatabase string) *InspectionResult {
	res := Inspect(query, environment)
	isDatabaseTarget := strings.EqualFold(res.TargetType, "DATABASE") || strings.EqualFold(res.TargetType, "SCHEMA")
	if !isDatabaseTarget && res.TargetDatabase != "" && scopedDatabase != "" && !strings.EqualFold(res.TargetDatabase, scopedDatabase) {
		res.SchemaScopeMismatch = true
		res.RiskLevel = RiskDangerous
		res.RequiresChallenge = true
		res.Reason = fmt.Sprintf("Schema Scope Mismatch: query explicitly targets database '%s', but active tab is set to '%s'.", res.TargetDatabase, scopedDatabase)
	} else if res.TargetDatabase == "" && scopedDatabase != "" && !isDatabaseTarget {
		res.TargetDatabase = scopedDatabase
	}
	if scopedDatabase != "" && !isDatabaseTarget {
		for i := range res.StatementEstimates {
			if res.StatementEstimates[i].TargetDatabase == "" {
				res.StatementEstimates[i].TargetDatabase = scopedDatabase
			}
		}
	}
	return res
}

func cleanTableName(raw string) string {
	s := strings.TrimSpace(raw)
	parts := strings.Split(s, ".")
	last := parts[len(parts)-1]
	return strings.Trim(last, "`'\"[]")
}

var reCTEStart = regexp.MustCompile(`(?i)^WITH\b`)

// extractCTEStatements extracts each top-level CTE body `( ... )` and the main statement
// following a WITH clause so data-modifying CTEs (e.g. WITH d AS (DELETE FROM t) SELECT * FROM d)
// are inspected alongside the outer statement.
func extractCTEStatements(q string) ([]string, bool) {
	if !reCTEStart.MatchString(q) {
		return nil, false
	}
	depth := 0
	var quote byte
	var bodyStart int
	var stmts []string
	n := len(q)
	for i := 4; i < n; i++ {
		c := q[i]

		if quote != 0 {
			if c == '\\' && quote != '`' && i+1 < n {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}

		// PostgreSQL dollar quote ($$...$$ or $tag$...$tag$)
		if c == '$' {
			j := i + 1
			for j < n && isIdentByte(q[j]) {
				j++
			}
			if j < n && q[j] == '$' && (j == i+1 || !(q[i+1] >= '0' && q[i+1] <= '9')) {
				tag := q[i : j+1]
				if end := strings.Index(q[j+1:], tag); end >= 0 {
					i = j + 1 + end + len(tag) - 1
					continue
				}
				return nil, false
			}
		}

		// Single-line comment (-- or #)
		if (c == '-' && i+1 < n && q[i+1] == '-') || c == '#' {
			for i < n && q[i] != '\n' {
				i++
			}
			continue
		}

		// Multi-line comment (/* ... */)
		if c == '/' && i+1 < n && q[i+1] == '*' {
			i += 2
			for i+1 < n && !(q[i] == '*' && q[i+1] == '/') {
				i++
			}
			i++ // skip '/'
			continue
		}

		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			if depth == 0 {
				bodyStart = i + 1
			}
			depth++
		case ')':
			if depth > 0 {
				depth--
				if depth == 0 && bodyStart > 0 && i > bodyStart {
					stmts = append(stmts, q[bodyStart:i])
					bodyStart = 0
				}
			}
		default:
			if depth != 0 || !isIdentByte(c) || isIdentByte(q[i-1]) {
				continue
			}
			j := i
			for j < n && isIdentByte(q[j]) {
				j++
			}
			switch strings.ToUpper(q[i:j]) {
			case "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE":
				stmts = append(stmts, q[i:])
				return stmts, true
			}
			i = j - 1
		}
	}
	return nil, false
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
