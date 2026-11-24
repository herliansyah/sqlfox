package safety

import (
	"fmt"
	"strings"

	"github.com/herliansyah/sqlfox/internal/driver"
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

	// Multi-statement inspection: inspect every statement across both MySQL (including
	// unwrapped /*! ... */ executable comments) and standard-SQL splitting so destructive
	// statements hidden after a benign statement cannot slip through.
	unwrapped := driver.UnwrapMySQLExecutableComments(clean)
	stmtsMySQL := driver.SplitSQLStatements(unwrapped, "mysql")
	stmtsStd := driver.SplitSQLStatements(clean, "postgres")
	if len(stmtsMySQL) > 1 || len(stmtsStd) > 1 {
		var resMySQL, resStd *InspectionResult
		if len(stmtsMySQL) > 1 {
			resMySQL = inspectMultiStatements(stmtsMySQL, environment)
		} else {
			resMySQL = inspectSingleStatement(unwrapped, environment)
		}
		if len(stmtsStd) > 1 {
			resStd = inspectMultiStatements(stmtsStd, environment)
		} else {
			resStd = inspectSingleStatement(clean, environment)
		}
		return pickHigherRisk(resMySQL, resStd)
	}

	return inspectSingleStatement(clean, environment)
}

func inspectSingleStatement(clean string, environment string) *InspectionResult {
	clean = driver.StripLeadingComments(strings.TrimSpace(clean))
	if clean == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	// Unwrap balanced outer parentheses, e.g. "(DELETE FROM users)" or "(WITH d AS (...) SELECT * FROM d)".
	clean = unwrapOuterParens(clean)
	if clean == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	// Inspect CTE sub-bodies under BOTH standard SQL (PostgreSQL standard_conforming_strings +
	// named $tag$ dollar-quotes) and MySQL backslash-escape lexing, picking the stricter result.
	var best *InspectionResult
	for _, mysqlMode := range []bool{false, true} {
		if cteStmts, ok := extractCTEStatementsMode(clean, mysqlMode); ok {
			best = pickHigherRisk(best, inspectMultiStatements(cteStmts, environment))
		}
		best = pickHigherRisk(best, inspectTokensMode(clean, environment, mysqlMode))
	}
	if best == nil {
		return &InspectionResult{RiskLevel: RiskNone}
	}
	return best
}

func pickHigherRisk(a, b *InspectionResult) *InspectionResult {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if riskPrecedence(b.RiskLevel) > riskPrecedence(a.RiskLevel) {
		return b
	}
	if riskPrecedence(b.RiskLevel) == riskPrecedence(a.RiskLevel) {
		if b.RequiresChallenge && !a.RequiresChallenge {
			return b
		}
		if b.IsDestructive && !a.IsDestructive {
			return b
		}
		if len(b.StatementEstimates) > len(a.StatementEstimates) {
			return b
		}
	}
	return a
}

func unwrapOuterParens(q string) string {
	for {
		trimmed := strings.TrimSpace(q)
		if len(trimmed) < 2 || trimmed[0] != '(' || trimmed[len(trimmed)-1] != ')' {
			return trimmed
		}
		tokens := tokenizeSQL(trimmed, !strings.Contains(trimmed, "$$"))
		if len(tokens) < 2 || tokens[0].kind != tokLParen || tokens[len(tokens)-1].kind != tokRParen {
			return trimmed
		}
		// Verify that the opening '(' at index 0 matches the closing ')' at the very last token.
		depth := 0
		matchedEnd := -1
		for i, tok := range tokens {
			if tok.kind == tokLParen {
				depth++
			} else if tok.kind == tokRParen {
				depth--
				if depth == 0 {
					matchedEnd = i
					break
				}
			}
		}
		if matchedEnd != len(tokens)-1 {
			return trimmed
		}
		q = trimmed[1 : len(trimmed)-1]
	}
}

func inspectTokens(query string, environment string) *InspectionResult {
	return inspectTokensMode(query, environment, !strings.Contains(query, "$$"))
}

func inspectTokensMode(query string, environment string, mysqlEscapes bool) *InspectionResult {
	tokens := tokenizeSQL(query, mysqlEscapes)
	if len(tokens) == 0 || tokens[0].kind != tokWord {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	isProd := strings.EqualFold(environment, "Production")
	isStaging := strings.EqualFold(environment, "Staging")

	switch tokens[0].upper {
	case "EXPLAIN":
		return inspectExplain(tokens, query, environment)
	case "DO", "CALL", "EXECUTE", "EXEC":
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         tokens[0].upper,
			Reason:            fmt.Sprintf("%s executes procedural/dynamic SQL that may mutate data.", tokens[0].upper),
			Suggestion:        "Verify the body or parameters of the procedure/block before executing.",
		}
	case "COPY":
		// PostgreSQL COPY ... TO/FROM PROGRAM executes OS commands on the DB server; file COPY reads/writes server disk.
		for _, tok := range tokens[1:] {
			if tok.kind == tokWord && tok.upper == "PROGRAM" {
				return &InspectionResult{
					RiskLevel:           RiskCritical,
					IsDestructive:       true,
					RequiresChallenge:   true,
					RequiresAffirmation: true,
					Operation:           "COPY",
					Reason:              "COPY ... PROGRAM executes arbitrary OS commands on the database server.",
					Suggestion:          "Avoid COPY PROGRAM; use client-side import/export instead.",
				}
			}
		}
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: isProd,
			Operation:         "COPY",
			Reason:            "COPY bulk-imports or exports server files/data.",
			Suggestion:        "Verify target table and file permissions before running COPY.",
		}
	case "ATTACH", "DETACH":
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         tokens[0].upper,
			Reason:            fmt.Sprintf("%s DATABASE accesses external database files on the host filesystem.", tokens[0].upper),
			Suggestion:        "Use separate SqlFox connections instead of ATTACH DATABASE.",
		}
	case "VACUUM":
		for _, tok := range tokens[1:] {
			if tok.kind == tokWord && tok.upper == "INTO" {
				return &InspectionResult{
					RiskLevel:         RiskDangerous,
					IsDestructive:     true,
					RequiresChallenge: true,
					Operation:         "VACUUM",
					Reason:            "VACUUM INTO writes a database copy to the host filesystem.",
					Suggestion:        "Use SqlFox Export Dump instead of VACUUM INTO.",
				}
			}
			if tok.kind == tokWord && tok.upper == "FULL" {
				return &InspectionResult{
					RiskLevel:           RiskDangerous,
					IsDestructive:       true,
					RequiresChallenge:   isProd,
					RequiresAffirmation: isProd || isStaging,
					Operation:           "VACUUM",
					Reason:              "VACUUM FULL acquires an ACCESS EXCLUSIVE table lock and rewrites the entire table.",
					Suggestion:          "Avoid VACUUM FULL on active production tables during peak traffic.",
				}
			}
		}
		return &InspectionResult{RiskLevel: RiskNone}
	case "LOAD":
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "LOAD",
			Reason:            "LOAD DATA bulk-loads files directly into database tables.",
			Suggestion:        "Verify source file path and target table before running LOAD DATA.",
		}
	case "GRANT", "REVOKE":
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: isProd,
			Operation:         tokens[0].upper,
			Reason:            fmt.Sprintf("%s modifies database access privileges.", tokens[0].upper),
			Suggestion:        "Verify target account and privilege scope before applying.",
		}
	case "PRAGMA":
		// Mutating PRAGMAs contain '=' (e.g. PRAGMA writable_schema = ON) or call non-readonly
		// function-form PRAGMAs like PRAGMA writable_schema(1) / PRAGMA query_only(0).
		isMutatingPragma := false
		for i := 1; i < len(tokens); i++ {
			if tokens[i].raw == "=" {
				isMutatingPragma = true
				break
			}
			if tokens[i].kind == tokWord && i+1 < len(tokens) && tokens[i+1].kind == tokLParen {
				switch strings.ToLower(tokens[i].unquoted) {
				case "table_info", "table_xinfo", "index_list", "index_info", "index_xinfo",
					"foreign_key_list", "database_list", "collation_list", "compile_options",
					"function_list", "module_list", "pragma_list", "integrity_check",
					"quick_check", "foreign_key_check", "data_version", "schema_version",
					"user_version", "application_id", "encoding", "page_count", "page_size", "freelist_count":
				default:
					isMutatingPragma = true
				}
				break
			}
		}
		if isMutatingPragma {
			return &InspectionResult{
				RiskLevel:         RiskDangerous,
				IsDestructive:     true,
				RequiresChallenge: isProd,
				Operation:         "PRAGMA",
				Reason:            "Mutating PRAGMA alters database engine configuration or schema safety.",
				Suggestion:        "Verify the PRAGMA setting before executing.",
			}
		}
		return &InspectionResult{RiskLevel: RiskNone}
	case "INSERT":
		for i := 1; i < len(tokens); i++ {
			if tokens[i].kind == tokWord && (tokens[i].upper == "DUPLICATE" || tokens[i].upper == "CONFLICT") {
				return &InspectionResult{
					RiskLevel:           RiskWarning,
					IsDestructive:       true,
					RequiresAffirmation: isProd || isStaging,
					Operation:           "INSERT",
					Reason:              "Upsert (INSERT ... ON DUPLICATE/CONFLICT) may overwrite existing rows.",
				}
			}
		}
		return &InspectionResult{RiskLevel: RiskNone}
	case "SET":
		for _, tok := range tokens[1:] {
			if tok.kind == tokWord && (tok.upper == "GLOBAL" || tok.upper == "PERSIST" || tok.upper == "PERSIST_ONLY" || tok.upper == "ROLE" || tok.upper == "AUTHORIZATION" || tok.upper == "PASSWORD") {
				return &InspectionResult{
					RiskLevel:           RiskDangerous,
					IsDestructive:       true,
					RequiresChallenge:   isProd,
					RequiresAffirmation: true,
					Operation:           "SET",
					Reason:              fmt.Sprintf("SET %s modifies server-wide configuration, role, or credentials.", tok.upper),
				}
			}
		}
		return &InspectionResult{RiskLevel: RiskNone}
	case "SHUTDOWN", "KILL", "FLUSH", "LOCK", "RESET", "PURGE", "STOP", "HANDLER", "PREPARE", "REASSIGN", "DISCARD", "REINDEX", "CLUSTER":
		return &InspectionResult{
			RiskLevel:           RiskDangerous,
			IsDestructive:       true,
			RequiresChallenge:   true,
			RequiresAffirmation: true,
			Operation:           tokens[0].upper,
			Reason:              fmt.Sprintf("%s performs a high-impact administrative or locking operation.", tokens[0].upper),
		}
	case "OPTIMIZE", "REPAIR":
		return &InspectionResult{
			RiskLevel:           RiskWarning,
			IsDestructive:       true,
			RequiresChallenge:   isProd,
			RequiresAffirmation: isProd || isStaging,
			Operation:           tokens[0].upper,
			Reason:              fmt.Sprintf("%s TABLE may acquire heavy table locks and rewrite storage.", tokens[0].upper),
		}
	case "SELECT":
		for i := 1; i+1 < len(tokens); i++ {
			if tokens[i].kind == tokWord && tokens[i].upper == "INTO" &&
				tokens[i+1].kind == tokWord && (tokens[i+1].upper == "OUTFILE" || tokens[i+1].upper == "DUMPFILE") {
				return &InspectionResult{
					RiskLevel:           RiskCritical,
					IsDestructive:       true,
					RequiresChallenge:   true,
					RequiresAffirmation: true,
					Operation:           "SELECT",
					Reason:              fmt.Sprintf("SELECT ... INTO %s writes directly to the database server filesystem.", tokens[i+1].upper),
					Suggestion:          "Use SqlFox client-side CSV/SQL export instead.",
				}
			}
		}
		if _, _, hasTopInto := findTopLevelKeyword(tokens, 1, "INTO"); hasTopInto {
			return &InspectionResult{
				RiskLevel:           RiskWarning,
				IsDestructive:       true,
				RequiresChallenge:   isProd,
				RequiresAffirmation: isProd || isStaging,
				Operation:           "SELECT",
				Reason:              "SELECT ... INTO creates a new table from query results.",
			}
		}
		return &InspectionResult{RiskLevel: RiskNone}
	case "DROP":
		return inspectDrop(tokens, isProd)
	case "TRUNCATE":
		return inspectTruncate(tokens)
	case "DELETE":
		return inspectDelete(tokens, query, environment, isProd, isStaging)
	case "UPDATE":
		return inspectUpdate(tokens, query, environment, isProd, isStaging)
	case "ALTER":
		return inspectAlter(tokens, isProd)
	case "RENAME":
		return inspectRename(tokens, isProd)
	case "REPLACE":
		return inspectReplace(tokens, environment, isProd, isStaging)
	case "CREATE":
		return inspectCreate(tokens, isProd)
	case "MERGE":
		return inspectMerge(tokens, environment, isProd, isStaging)
	default:
		return &InspectionResult{RiskLevel: RiskNone}
	}
}

// inspectExplain inspects EXPLAIN [ANALYZE] statements. Because EXPLAIN ANALYZE actually executes
// the underlying query in PostgreSQL and MySQL 8+, its inner statement must be safety-checked.
func inspectExplain(tokens []sqlToken, query string, environment string) *InspectionResult {
	hasAnalyze := false
	idx := 1
	// Skip optional MySQL/Postgres EXPLAIN modifiers before the inner statement.
	for idx < len(tokens) {
		tok := tokens[idx]
		if tok.kind == tokLParen {
			depth := 1
			idx++
			for idx < len(tokens) && depth > 0 {
				if tokens[idx].kind == tokLParen {
					depth++
				} else if tokens[idx].kind == tokRParen {
					depth--
				} else if tokens[idx].kind == tokWord && (tokens[idx].upper == "ANALYZE" || tokens[idx].upper == "ANALYSE") {
					hasAnalyze = true
				}
				idx++
			}
			continue
		}
		if tok.kind != tokWord {
			break
		}
		switch tok.upper {
		case "ANALYZE", "ANALYSE":
			hasAnalyze = true
			idx++
		case "VERBOSE", "COSTS", "SETTINGS", "BUFFERS", "WAL", "TIMING", "SUMMARY",
			"EXTENDED", "PARTITIONS", "FORMAT", "TREE", "JSON", "TEXT", "XML", "YAML", "TRADITIONAL":
			idx++
			// Handle FORMAT = TREE / FORMAT = JSON
			if idx < len(tokens) && tokens[idx].kind == tokOther && query[tokens[idx].start:tokens[idx].end] == "=" {
				idx++
				if idx < len(tokens) {
					idx++
				}
			}
		default:
			goto doneModifiers
		}
	}
doneModifiers:
	if !hasAnalyze || idx >= len(tokens) {
		return &InspectionResult{RiskLevel: RiskNone}
	}
	inner := strings.TrimSpace(query[tokens[idx].start:])
	return Inspect(inner, environment)
}

func collectCommaSeparatedTargets(tokens []sqlToken, idx int) (firstDB string, targets []string) {
	db, target, _, nextIdx := parseQualifiedTarget(tokens, idx)
	if target == "" {
		return "", nil
	}
	firstDB = db
	targets = append(targets, target)
	for nextIdx < len(tokens) && tokens[nextIdx].kind == tokComma {
		_, nextTarget, _, afterNext := parseQualifiedTarget(tokens, nextIdx+1)
		if nextTarget == "" {
			break
		}
		targets = append(targets, nextTarget)
		nextIdx = afterNext
	}
	return firstDB, targets
}

func inspectDrop(tokens []sqlToken, isProd bool) *InspectionResult {
	idx := 1
	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "TEMPORARY" {
		idx++
	}
	if idx >= len(tokens) || tokens[idx].kind != tokWord {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "DROP",
			Reason:            "Unparsed DROP statement could permanently delete database objects (fail-closed).",
		}
	}

	targetType := tokens[idx].upper
	if targetType == "MATERIALIZED" && idx+1 < len(tokens) && tokens[idx+1].kind == tokWord && tokens[idx+1].upper == "VIEW" {
		targetType = "MATERIALIZED VIEW"
		idx += 2
	} else {
		switch targetType {
		case "TABLE", "DATABASE", "SCHEMA", "VIEW", "INDEX", "SEQUENCE",
			"FUNCTION", "PROCEDURE", "TRIGGER", "EVENT", "TYPE", "DOMAIN", "EXTENSION",
			"ROLE", "USER", "OWNED", "POLICY", "TABLESPACE":
			idx++
		default:
			return &InspectionResult{
				RiskLevel:         RiskDangerous,
				IsDestructive:     true,
				RequiresChallenge: true,
				Operation:         "DROP",
				Reason:            fmt.Sprintf("DROP %s statement may permanently delete database objects (fail-closed).", targetType),
			}
		}
	}

	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "CONCURRENTLY" {
		idx++
	}
	if idx+1 < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "IF" &&
		tokens[idx+1].kind == tokWord && tokens[idx+1].upper == "EXISTS" {
		idx += 2
	}

	db, targets := collectCommaSeparatedTargets(tokens, idx)
	if len(targets) == 0 {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "DROP",
			Reason:            "DROP statement with unparsed target requires confirmation (fail-closed).",
		}
	}
	target := strings.Join(targets, ", ")
	if targetType == "DATABASE" || targetType == "SCHEMA" {
		db = targets[0]
	}

	reason := fmt.Sprintf("DROP statement will permanently delete '%s'.", target)
	suggestion := "Ensure backups exist before dropping schema objects."
	if targetType == "DATABASE" {
		reason = fmt.Sprintf("DROP DATABASE will permanently delete database '%s' and every object in it.", target)
		suggestion = "Ensure a full dump exists before dropping a database."
	}
	_ = isProd

	return &InspectionResult{
		RiskLevel:           RiskCritical,
		IsDestructive:       true,
		RequiresChallenge:   true,
		RequiresAffirmation: true,
		TargetDatabase:      db,
		TargetTable:         target,
		TargetType:          targetType,
		Operation:           "DROP",
		Reason:              reason,
		Suggestion:          suggestion,
	}
}

func inspectTruncate(tokens []sqlToken) *InspectionResult {
	idx := 1
	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "TABLE" {
		idx++
	}
	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "ONLY" {
		idx++
	}
	db, targets := collectCommaSeparatedTargets(tokens, idx)
	if len(targets) == 0 {
		return &InspectionResult{
			RiskLevel:         RiskCritical,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "TRUNCATE",
			Reason:            "TRUNCATE statement with unparsed target will permanently empty table data (fail-closed).",
		}
	}
	target := strings.Join(targets, ", ")
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
		Suggestion:          "Consider soft-deleting or creating a backup snapshot before truncating.",
	}
}

func inspectDelete(tokens []sqlToken, query, environment string, isProd, isStaging bool) *InspectionResult {
	idx := 1
	for idx < len(tokens) && tokens[idx].kind == tokWord {
		switch tokens[idx].upper {
		case "LOW_PRIORITY", "QUICK", "IGNORE":
			idx++
			continue
		}
		break
	}

	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "FROM" {
		idx++
	} else if fromIdx, _, ok := findTopLevelKeyword(tokens, idx, "FROM"); ok {
		// MySQL multi-table delete: DELETE t1 FROM t1 JOIN t2 ...
		idx = fromIdx + 1
	}

	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "ONLY" {
		idx++
	}

	db, target, rawTable, nextIdx := parseQualifiedTarget(tokens, idx)
	if target == "" {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "UPDATE",
			Reason:            "UPDATE statement with unparsed target requires confirmation (fail-closed).",
		}
	}

	whereIdx, whereByteStart, hasWhere := findTopLevelKeyword(tokens, nextIdx, "WHERE")
	if hasWhere {
		// Require WHERE to exist under standard-SQL (non-backslash-escape) lexing too,
		// preventing backslash-quote smuggling from faking a WHERE clause across dialects.
		stdTokens := tokenizeSQL(query, false)
		_, _, hasStdWhere := findTopLevelKeyword(stdTokens, 0, "WHERE")
		hasWhere = hasStdWhere
	}
	if !hasWhere {
		return &InspectionResult{
			RiskLevel:         RiskCritical,
			IsDestructive:     true,
			RequiresChallenge: true,
			TargetDatabase:    db,
			TargetTable:       target,
			Operation:         "DELETE",
			Reason:            fmt.Sprintf("Unqualified DELETE statement on '%s' without a WHERE clause will delete ALL rows.", target),
			Suggestion:        "Add a specific WHERE clause targeting rows (e.g. WHERE id = ?).",
			CountQuery:        fmt.Sprintf("SELECT COUNT(*) FROM %s", rawTable),
		}
	}

	countQ := buildTokenCountQuery(rawTable, query, tokens, whereIdx, whereByteStart)
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
		res.Reason = fmt.Sprintf("DELETE query executing on PRODUCTION environment on table '%s'.", target)
	} else if isStaging {
		res.RiskLevel = RiskWarning
		res.RequiresAffirmation = true
		res.Reason = fmt.Sprintf("DELETE query executing on STAGING environment on table '%s'.", target)
	} else {
		res.RiskLevel = RiskNone
		res.Reason = fmt.Sprintf("DELETE query targeting table '%s'.", target)
	}
	return res
}

func inspectUpdate(tokens []sqlToken, query, environment string, isProd, isStaging bool) *InspectionResult {
	idx := 1
	for idx < len(tokens) && tokens[idx].kind == tokWord {
		switch tokens[idx].upper {
		case "LOW_PRIORITY", "IGNORE":
			idx++
			continue
		}
		break
	}
	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "ONLY" {
		idx++
	}

	db, target, rawTable, nextIdx := parseQualifiedTarget(tokens, idx)
	if target == "" {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "UPDATE",
			Reason:            "UPDATE statement with unparsed target requires confirmation (fail-closed).",
		}
	}

	whereIdx, whereByteStart, hasWhere := findTopLevelKeyword(tokens, nextIdx, "WHERE")
	if hasWhere {
		// Require WHERE to exist under standard-SQL (non-backslash-escape) lexing too,
		// preventing backslash-quote smuggling from faking a WHERE clause across dialects.
		stdTokens := tokenizeSQL(query, false)
		_, _, hasStdWhere := findTopLevelKeyword(stdTokens, 0, "WHERE")
		hasWhere = hasStdWhere
	}
	if !hasWhere {
		return &InspectionResult{
			RiskLevel:         RiskCritical,
			IsDestructive:     true,
			RequiresChallenge: true,
			TargetDatabase:    db,
			TargetTable:       target,
			Operation:         "UPDATE",
			Reason:            fmt.Sprintf("Unqualified UPDATE statement on '%s' without a WHERE clause will modify ALL rows.", target),
			Suggestion:        "Add a specific WHERE clause targeting rows (e.g. WHERE id = ?).",
			CountQuery:        fmt.Sprintf("SELECT COUNT(*) FROM %s", rawTable),
		}
	}

	countQ := buildTokenCountQuery(rawTable, query, tokens, whereIdx, whereByteStart)
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
		res.Reason = fmt.Sprintf("UPDATE query executing on PRODUCTION environment on table '%s'.", target)
	} else if isStaging {
		res.RiskLevel = RiskWarning
		res.RequiresAffirmation = true
		res.Reason = fmt.Sprintf("UPDATE query executing on STAGING environment on table '%s'.", target)
	} else {
		res.RiskLevel = RiskNone
		res.Reason = fmt.Sprintf("UPDATE query targeting table '%s'.", target)
	}
	return res
}

func inspectAlter(tokens []sqlToken, isProd bool) *InspectionResult {
	idx := 1
	if idx >= len(tokens) || tokens[idx].kind != tokWord {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "ALTER",
			Reason:            "Unparsed ALTER statement requires confirmation (fail-closed).",
		}
	}
	objType := tokens[idx].upper
	if objType == "SYSTEM" {
		return &InspectionResult{
			RiskLevel:           RiskCritical,
			IsDestructive:       true,
			RequiresChallenge:   true,
			RequiresAffirmation: true,
			TargetType:          "SYSTEM",
			Operation:           "ALTER",
			Reason:              "ALTER SYSTEM modifies global database server configuration.",
		}
	}
	if objType != "TABLE" {
		switch objType {
		case "USER", "ROLE", "DATABASE", "SCHEMA", "FUNCTION", "PROCEDURE", "TRIGGER", "VIEW", "EVENT", "POLICY", "INDEX", "SEQUENCE", "TYPE", "DOMAIN", "EXTENSION", "TABLESPACE":
			idx++
			if idx+1 < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "IF" && tokens[idx+1].kind == tokWord && tokens[idx+1].upper == "EXISTS" {
				idx += 2
			}
			db, target, _, _ := parseQualifiedTarget(tokens, idx)
			if objType == "DATABASE" || objType == "SCHEMA" {
				db = target
			}
			return &InspectionResult{
				RiskLevel:           RiskDangerous,
				IsDestructive:       true,
				RequiresChallenge:   isProd || objType == "USER" || objType == "ROLE",
				RequiresAffirmation: true,
				TargetDatabase:      db,
				TargetTable:         target,
				TargetType:          objType,
				Operation:           "ALTER",
				Reason:              fmt.Sprintf("ALTER %s modifies database object or privileges.", objType),
			}
		default:
			return &InspectionResult{
				RiskLevel:         RiskDangerous,
				IsDestructive:     true,
				RequiresChallenge: isProd,
				Operation:         "ALTER",
				Reason:            fmt.Sprintf("ALTER %s modifies database configuration or schema.", objType),
			}
		}
	}
	idx++
	if idx+1 < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "IF" &&
		tokens[idx+1].kind == tokWord && tokens[idx+1].upper == "EXISTS" {
		idx += 2
	}
	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "ONLY" {
		idx++
	}
	db, target, _, _ := parseQualifiedTarget(tokens, idx)
	if target == "" {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "ALTER",
			Reason:            "Unparsed ALTER TABLE target requires confirmation (fail-closed).",
		}
	}
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

func inspectRename(tokens []sqlToken, isProd bool) *InspectionResult {
	idx := 1
	if idx >= len(tokens) || tokens[idx].kind != tokWord {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "RENAME",
			Reason:            "Unparsed RENAME statement requires confirmation (fail-closed).",
		}
	}
	if tokens[idx].upper == "USER" {
		return &InspectionResult{
			RiskLevel:           RiskDangerous,
			IsDestructive:       true,
			RequiresChallenge:   true,
			RequiresAffirmation: true,
			TargetType:          "USER",
			Operation:           "RENAME",
			Reason:              "RENAME USER modifies database user accounts.",
		}
	}
	if tokens[idx].upper != "TABLE" {
		return &InspectionResult{RiskLevel: RiskNone}
	}
	db, target, _, _ := parseQualifiedTarget(tokens, idx+1)
	if target == "" {
		return &InspectionResult{
			RiskLevel:         RiskDangerous,
			IsDestructive:     true,
			RequiresChallenge: true,
			Operation:         "RENAME",
			Reason:            "Unparsed RENAME TABLE target requires confirmation (fail-closed).",
		}
	}
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

func inspectReplace(tokens []sqlToken, environment string, isProd, isStaging bool) *InspectionResult {
	idx := 1
	for idx < len(tokens) && tokens[idx].kind == tokWord {
		switch tokens[idx].upper {
		case "LOW_PRIORITY", "DELAYED", "INTO":
			idx++
			continue
		}
		break
	}
	db, target, _, _ := parseQualifiedTarget(tokens, idx)
	if target == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}
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

func inspectCreate(tokens []sqlToken, isProd bool) *InspectionResult {
	idx := 1
	if idx+1 < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "OR" &&
		tokens[idx+1].kind == tokWord && tokens[idx+1].upper == "REPLACE" {
		idx += 2
	}
	for idx < len(tokens) && tokens[idx].kind == tokWord {
		switch tokens[idx].upper {
		case "TEMPORARY", "TEMP", "UNIQUE", "UNLOGGED", "FULLTEXT", "SPATIAL":
			idx++
			continue
		}
		break
	}
	// Skip MySQL DEFINER = user@host before VIEW/FUNCTION/PROCEDURE/TRIGGER/EVENT.
	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "DEFINER" {
		for idx < len(tokens) {
			if tokens[idx].kind == tokWord {
				switch tokens[idx].upper {
				case "VIEW", "FUNCTION", "PROCEDURE", "TRIGGER", "EVENT":
					goto foundCreateTarget
				}
			}
			idx++
		}
	}
foundCreateTarget:
	if idx >= len(tokens) || tokens[idx].kind != tokWord {
		return &InspectionResult{RiskLevel: RiskNone}
	}

	targetType := tokens[idx].upper
	isPrivilegedCreate := false
	if targetType == "MATERIALIZED" && idx+1 < len(tokens) && tokens[idx+1].kind == tokWord && tokens[idx+1].upper == "VIEW" {
		targetType = "MATERIALIZED VIEW"
		idx += 2
	} else {
		switch targetType {
		case "TABLE", "DATABASE", "SCHEMA", "VIEW", "INDEX":
			idx++
		case "FUNCTION", "PROCEDURE", "TRIGGER", "EVENT", "USER", "ROLE", "EXTENSION", "SERVER", "FOREIGN", "RULE", "POLICY":
			isPrivilegedCreate = true
			idx++
		default:
			return &InspectionResult{RiskLevel: RiskNone}
		}
	}

	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "CONCURRENTLY" {
		idx++
	}
	if idx+2 < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "IF" &&
		tokens[idx+1].kind == tokWord && tokens[idx+1].upper == "NOT" &&
		tokens[idx+2].kind == tokWord && tokens[idx+2].upper == "EXISTS" {
		idx += 3
	}

	db, target, _, _ := parseQualifiedTarget(tokens, idx)
	if target == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}
	if targetType == "DATABASE" || targetType == "SCHEMA" {
		db = target
	}
	risk := RiskWarning
	if isPrivilegedCreate {
		risk = RiskDangerous
	}
	return &InspectionResult{
		RiskLevel:           risk,
		IsDestructive:       isPrivilegedCreate,
		RequiresChallenge:   isProd,
		RequiresAffirmation: true,
		TargetDatabase:      db,
		TargetTable:         target,
		TargetType:          targetType,
		Operation:           "CREATE",
		Reason:              fmt.Sprintf("CREATE %s will create new schema object '%s'.", targetType, target),
	}
}

func inspectMerge(tokens []sqlToken, environment string, isProd, isStaging bool) *InspectionResult {
	idx := 1
	if idx < len(tokens) && tokens[idx].kind == tokWord && tokens[idx].upper == "INTO" {
		idx++
	}
	db, target, _, _ := parseQualifiedTarget(tokens, idx)
	if target == "" {
		return &InspectionResult{RiskLevel: RiskNone}
	}
	res := &InspectionResult{
		IsDestructive:  true,
		TargetDatabase: db,
		TargetTable:    target,
		TargetType:     "TABLE",
		Operation:      "MERGE",
	}
	if isProd || isStaging {
		res.RiskLevel = RiskWarning
		res.RequiresAffirmation = true
		res.Reason = fmt.Sprintf("MERGE on %s environment targeting '%s'.", strings.ToUpper(environment), target)
	} else {
		res.RiskLevel = RiskNone
		res.Reason = fmt.Sprintf("MERGE targeting '%s'.", target)
	}
	return res
}

func buildTokenCountQuery(rawTable, query string, tokens []sqlToken, whereIdx, whereByteStart int) string {
	endByte := len(query)
	if _, retByte, ok := findTopLevelKeyword(tokens, whereIdx+1, "RETURNING"); ok && retByte < endByte {
		endByte = retByte
	}
	clause := strings.TrimSpace(query[whereByteStart:endByte])
	clause = strings.TrimRight(clause, "; \t\r\n")
	if clause == "" {
		return fmt.Sprintf("SELECT COUNT(*) FROM %s", rawTable)
	}
	return fmt.Sprintf("SELECT COUNT(*) FROM %s %s", rawTable, clause)
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

// InspectWithScope analyzes a query string against the target environment and scoped database.
func InspectWithScope(query string, environment string, scopedDatabase string) *InspectionResult {
	res := Inspect(query, environment)
	isDatabaseTarget := strings.EqualFold(res.TargetType, "DATABASE") || strings.EqualFold(res.TargetType, "SCHEMA")
	if !isDatabaseTarget && res.TargetDatabase != "" && scopedDatabase != "" && !strings.EqualFold(res.TargetDatabase, scopedDatabase) {
		res.SchemaScopeMismatch = true
		if riskPrecedence(RiskDangerous) > riskPrecedence(res.RiskLevel) {
			res.RiskLevel = RiskDangerous
		}
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

// extractCTEStatements extracts each top-level CTE body `( ... )` and the main statement
// following a WITH clause so data-modifying CTEs (e.g. WITH d AS (DELETE FROM t) SELECT * FROM d)
// are inspected alongside the outer statement.
func extractCTEStatements(q string) ([]string, bool) {
	return extractCTEStatementsMode(q, !strings.Contains(q, "$$"))
}

func extractCTEStatementsMode(q string, mysqlEscapes bool) ([]string, bool) {
	tokens := tokenizeSQL(q, mysqlEscapes)
	if len(tokens) == 0 || tokens[0].kind != tokWord || tokens[0].upper != "WITH" {
		return nil, false
	}

	depth := 0
	bodyStart := -1
	var stmts []string
	sawCTEBody := false

	for i := 1; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok.kind {
		case tokLParen:
			if depth == 0 {
				if sawCTEBody && i > 0 && tokens[i-1].kind == tokRParen {
					// Outer parenthesized main query: WITH d AS (...) (SELECT * FROM d)
					stmts = append(stmts, q[tok.start:])
					return stmts, true
				}
				bodyStart = tok.end
			}
			depth++
		case tokRParen:
			if depth > 0 {
				depth--
				if depth == 0 && bodyStart >= 0 && tok.start > bodyStart {
					stmts = append(stmts, q[bodyStart:tok.start])
					bodyStart = -1
					sawCTEBody = true
				}
			}
		case tokWord:
			if depth == 0 {
				switch tok.upper {
				case "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE", "TABLE", "VALUES":
					stmts = append(stmts, q[tok.start:])
					return stmts, true
				}
			}
		}
	}
	return nil, false
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
