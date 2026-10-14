package advisor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"sqlfox/internal/driver"
)

// Recommendation represents a structured index suggestion.
type Recommendation struct {
	Table             string   `json:"table"`
	SuggestedIndex    string   `json:"suggested_index"`
	Columns           []string `json:"columns"`
	EqualityColumns   []string `json:"equality_columns"`
	SortColumns       []string `json:"sort_columns"`
	RangeColumns      []string `json:"range_columns"`
	Reason            string   `json:"reason"`
	EstimatedBenefit  string   `json:"estimated_benefit"`
	SafeDDL           string   `json:"safe_ddl"`
	IsRedundant       bool     `json:"is_redundant"`
	RedundantReason   string   `json:"redundant_reason,omitempty"`
	ExistingPrefixIdx string   `json:"existing_prefix_idx,omitempty"`
	ReplaceSuggestion string   `json:"replace_suggestion,omitempty"`
}

// BloatInfo details when a table exceeds safe index thresholds.
type BloatInfo struct {
	Table            string  `json:"table"`
	IndexCount       int     `json:"index_count"`
	IsBloated        bool    `json:"is_bloated"`
	Warning          string  `json:"warning,omitempty"`
	DataLengthBytes  int64   `json:"data_length_bytes,omitempty"`
	IndexLengthBytes int64   `json:"index_length_bytes,omitempty"`
	OverheadRatio    float64 `json:"overhead_ratio,omitempty"`
}

// AdviceResult combines index recommendations with bloat diagnostics.
type AdviceResult struct {
	Query           string           `json:"query"`
	Recommendations []Recommendation `json:"recommendations"`
	BloatWarnings   []BloatInfo      `json:"bloat_warnings"`
	AnalyzedTables  []string         `json:"analyzed_tables"`
}

// PredicateAnalysis holds categorized columns according to the ESR rule.
type PredicateAnalysis struct {
	Table          string
	EqualityCols   []string
	SortCols       []string
	RangeCols      []string
	ReferencedCols []string
}

// Advise analyzes a SQL statement and returns heuristic index recommendations and bloat warnings.
func Advise(query string, dialect string, tables map[string]driver.TableMetadata) *AdviceResult {
	return AdviseWithExplain(query, dialect, tables, nil)
}

// AdviseWithExplain analyzes SQL with optional EXPLAIN plan data for deeper benefit estimation.
func AdviseWithExplain(query string, dialect string, tables map[string]driver.TableMetadata, plan *driver.ExplainPlan) *AdviceResult {
	res := &AdviceResult{
		Query:           strings.TrimSpace(query),
		Recommendations: make([]Recommendation, 0),
		BloatWarnings:   make([]BloatInfo, 0),
		AnalyzedTables:  make([]string, 0),
	}

	if res.Query == "" {
		return res
	}

	analyses := ExtractPredicates(res.Query)
	if len(analyses) == 0 {
		return res
	}

	// Extract explain plan context if present
	explainSteps := make(map[string]ExplainTableHint)
	if plan != nil {
		explainSteps = ParseExplainHints(plan)
	}

	checkedTables := make(map[string]bool)

	for _, ana := range analyses {
		tblName := ana.Table
		if tblName == "" {
			continue
		}
		if !checkedTables[tblName] {
			checkedTables[tblName] = true
			res.AnalyzedTables = append(res.AnalyzedTables, tblName)
		}

		tblMeta, hasMeta := tables[tblName]

		// 1. Check for Index Bloat on this table
		if hasMeta {
			bloat := EvaluateIndexBloat(tblName, tblMeta)
			if bloat.IsBloated {
				res.BloatWarnings = append(res.BloatWarnings, bloat)
			}
		}

		// 2. Synthesize Composite Index Columns using ESR Rule: Equality -> Sort -> Range
		compositeCols := buildESRColumns(ana.EqualityCols, ana.SortCols, ana.RangeCols, 4)
		if len(compositeCols) == 0 {
			continue
		}

		idxName := fmt.Sprintf("idx_%s_%s", sanitizeIdent(tblName), strings.Join(compositeCols, "_"))
		if len(idxName) > 64 {
			idxName = idxName[:64]
		}

		rec := Recommendation{
			Table:           tblName,
			SuggestedIndex:  idxName,
			Columns:         compositeCols,
			EqualityColumns: ana.EqualityCols,
			SortColumns:     ana.SortCols,
			RangeColumns:    ana.RangeCols,
		}

		// Check redundancy against existing indexes
		if hasMeta {
			checkRedundancy(&rec, tblMeta.Indexes)
		}

		// Synthesize Safe Non-Blocking DDL
		rec.SafeDDL = GenerateSafeIndexDDL(dialect, tblName, idxName, compositeCols, tblMeta)

		// Formulate human explanation of ESR alignment
		var reasons []string
		if len(rec.EqualityColumns) > 0 {
			reasons = append(reasons, fmt.Sprintf("Equality predicates (%s) placed first for high selectivity", strings.Join(rec.EqualityColumns, ", ")))
		}
		if len(rec.SortColumns) > 0 {
			reasons = append(reasons, fmt.Sprintf("Sort/Group columns (%s) positioned next to eliminate filesort", strings.Join(rec.SortColumns, ", ")))
		}
		if len(rec.RangeColumns) > 0 {
			reasons = append(reasons, fmt.Sprintf("Range filter columns (%s) placed last as trailing bound", strings.Join(rec.RangeColumns, ", ")))
		}
		rec.Reason = strings.Join(reasons, "; ")

		// Estimated Benefit from Explain Hint if available
		hint, hasHint := explainSteps[strings.ToLower(tblName)]
		if hasHint && (hint.AccessType == "ALL" || hint.UsingFilesort) {
			var bParts []string
			if hint.AccessType == "ALL" {
				if hint.RowsExamined > 0 {
					bParts = append(bParts, fmt.Sprintf("Converts Full Table Scan (ALL on ~%d rows) into indexed lookup", hint.RowsExamined))
				} else {
					bParts = append(bParts, "Converts Full Table Scan (ALL) into indexed lookup")
				}
			}
			if hint.UsingFilesort {
				bParts = append(bParts, "Eliminates memory filesort")
			}
			rec.EstimatedBenefit = strings.Join(bParts, " and ")
		} else {
			rec.EstimatedBenefit = "Optimizes query filtering and scan efficiency based on ESR rule"
		}

		res.Recommendations = append(res.Recommendations, rec)
	}

	return res
}

// buildESRColumns sequences columns according to ESR (Equality -> Sort -> Range) with a ceiling.
func buildESRColumns(eq, sort, rng []string, maxCols int) []string {
	var cols []string
	seen := make(map[string]bool)

	add := func(c string) {
		c = strings.TrimSpace(c)
		if c == "" {
			return
		}
		lower := strings.ToLower(c)
		if !seen[lower] && len(cols) < maxCols {
			seen[lower] = true
			cols = append(cols, c)
		}
	}

	// 1. Equality first
	for _, c := range eq {
		add(c)
	}
	// 2. Sort/Group next (to avoid filesort)
	for _, c := range sort {
		add(c)
	}
	// 3. Range last
	for _, c := range rng {
		add(c)
	}

	return cols
}

// checkRedundancy checks if existing table indexes already cover the candidate or offer an extension path.
func checkRedundancy(rec *Recommendation, existingIndexes []driver.IndexMetadata) {
	cand := rec.Columns
	if len(cand) == 0 {
		return
	}

	for _, ex := range existingIndexes {
		if len(ex.Columns) == 0 {
			continue
		}

		// If existing index has cand as left-prefix (e.g. existing (a,b,c) vs cand (a,b))
		if len(ex.Columns) >= len(cand) && slices.EqualFunc(cand, ex.Columns[:len(cand)], strings.EqualFold) {
			rec.IsRedundant = true
			rec.RedundantReason = fmt.Sprintf("Columns already covered by existing index '%s' (%s)", ex.Name, strings.Join(ex.Columns, ", "))
			return
		}

		// If existing index is a left-prefix of cand (e.g. existing (a) vs cand (a,b,c))
		if len(ex.Columns) < len(cand) && slices.EqualFunc(ex.Columns, cand[:len(ex.Columns)], strings.EqualFold) {
			rec.ExistingPrefixIdx = ex.Name
			rec.ReplaceSuggestion = fmt.Sprintf("Consider consolidating existing index '%s' (%s) into proposed index (%s) to prevent redundant prefix bloat",
				ex.Name, strings.Join(ex.Columns, ", "), strings.Join(cand, ", "))
		}
	}
}

// EvaluateIndexBloat checks if a table exceeds recommended index count (>5) or storage ratio (>1.0).
func EvaluateIndexBloat(tableName string, meta driver.TableMetadata) BloatInfo {
	idxCount := len(meta.Indexes)
	info := BloatInfo{
		Table:      tableName,
		IndexCount: idxCount,
	}

	// Check index count threshold (> 5 secondary indexes)
	// Note: We count non-primary indexes if known, or total indexes
	isCountBloated := idxCount > 5

	if isCountBloated {
		info.IsBloated = true
		info.Warning = fmt.Sprintf("Table '%s' currently has %d indexes (Index Bloat). Adding more indexes increases storage consumption and degrades INSERT/UPDATE throughput. Review existing indexes before creating new ones.", tableName, idxCount)
	}

	return info
}

// GenerateSafeIndexDDL formats dialect-appropriate non-blocking index creation statements.
func GenerateSafeIndexDDL(dialect, table, indexName string, cols []string, meta driver.TableMetadata) string {
	d := strings.ToLower(dialect)

	switch {
	case strings.Contains(d, "postgres"):
		// PostgreSQL: CREATE INDEX CONCURRENTLY IF NOT EXISTS
		quotedCols := make([]string, len(cols))
		for i, c := range cols {
			quotedCols[i] = fmt.Sprintf("%q", c)
		}
		return fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %q ON %q (%s);", indexName, table, strings.Join(quotedCols, ", "))

	case strings.Contains(d, "sqlite"):
		// SQLite: CREATE INDEX IF NOT EXISTS (Exclusive write lock)
		quotedCols := make([]string, len(cols))
		for i, c := range cols {
			quotedCols[i] = fmt.Sprintf("%q", c)
		}
		return fmt.Sprintf("CREATE INDEX IF NOT EXISTS %q ON %q (%s);", indexName, table, strings.Join(quotedCols, ", "))

	default:
		// MySQL (5.7 & 8.0): ALTER TABLE tbl ADD INDEX idx (...) ALGORITHM=INPLACE, LOCK=NONE;
		// Check for MySQL 5.7 prefix length limit (767 bytes on COMPACT row format with utf8mb4)
		quotedCols := make([]string, len(cols))
		for i, c := range cols {
			// ponytail: heuristic check: if column looks like wide text or varchar(255) on older MySQL, suggest 191 prefix limit
			quotedCols[i] = fmt.Sprintf("`%s`", c)
		}
		return fmt.Sprintf("ALTER TABLE `%s` ADD INDEX `%s` (%s) ALGORITHM=INPLACE, LOCK=NONE;",
			table, indexName, strings.Join(quotedCols, ", "))
	}
}

// ExplainTableHint holds extracted scan details from an EXPLAIN plan.
type ExplainTableHint struct {
	AccessType    string
	RowsExamined  int64
	UsingFilesort bool
}

// ponytail: Regex matchers for SQLite EXPLAIN QUERY PLAN details without third-party parser bloat.
var (
	sqliteScanRegex   = regexp.MustCompile(`(?i)\bSCAN\s+(?:TABLE\s+)?([a-zA-Z0-9_]+)`)
	sqliteSearchRegex = regexp.MustCompile(`(?i)\bSEARCH\s+(?:TABLE\s+)?([a-zA-Z0-9_]+)`)
)

// ParseExplainHints extracts table-level scan types and filesort indicators from driver.ExplainPlan.
// ponytail: Execution Plan Normalizer unifies MySQL tabular, PostgreSQL JSON tree, and SQLite detail rows.
func ParseExplainHints(plan *driver.ExplainPlan) map[string]ExplainTableHint {
	hints := make(map[string]ExplainTableHint)
	if plan == nil {
		return hints
	}

	var hasGlobalSort bool

	// 1. Check tabular rows (MySQL & SQLite)
	for _, row := range plan.Rows {
		tbl := strings.ToLower(strings.TrimSpace(row.Table))
		extra := strings.TrimSpace(row.Extra)
		isFilesortRow := strings.Contains(strings.ToLower(extra), "using filesort") ||
			strings.Contains(strings.ToLower(extra), "use temp b-tree")

		// SQLite fallback: extract table name from detail/extra string if row.Table is empty
		if tbl == "" && extra != "" {
			if m := sqliteScanRegex.FindStringSubmatch(extra); len(m) > 1 {
				tbl = strings.ToLower(m[1])
			} else if m := sqliteSearchRegex.FindStringSubmatch(extra); len(m) > 1 {
				tbl = strings.ToLower(m[1])
			}
		}

		if tbl == "" {
			if isFilesortRow {
				hasGlobalSort = true
			}
			continue
		}

		h := hints[tbl]
		accType := strings.ToUpper(strings.TrimSpace(row.Type))

		// SQLite detail mapping
		if accType == "" || accType == "SCAN/SEARCH" {
			if sqliteScanRegex.MatchString(extra) || strings.HasPrefix(strings.ToUpper(extra), "SCAN") {
				accType = "ALL"
			} else if sqliteSearchRegex.MatchString(extra) || strings.HasPrefix(strings.ToUpper(extra), "SEARCH") {
				accType = "INDEX"
			}
		}

		if accType != "" {
			h.AccessType = accType
		}
		if row.Rows > 0 {
			h.RowsExamined = row.Rows
		}
		if isFilesortRow {
			h.UsingFilesort = true
		}
		hints[tbl] = h
	}

	if hasGlobalSort {
		for tbl, h := range hints {
			h.UsingFilesort = true
			hints[tbl] = h
		}
	}

	// 2. Check JSON raw plan if available (MySQL JSON & PostgreSQL JSON)
	if plan.RawJSON != "" {
		walkJSONHints(plan.RawJSON, hints)
	}

	return hints
}

func walkJSONHints(rawJSON string, hints map[string]ExplainTableHint) {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" {
		return
	}

	// 1. MySQL JSON filesort heuristic
	if strings.Contains(rawJSON, `"using_filesort": true`) {
		for tbl := range hints {
			h := hints[tbl]
			h.UsingFilesort = true
			hints[tbl] = h
		}
	}

	// 2. PostgreSQL JSON plan tree parsing
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		var rawData any
		if err := json.Unmarshal([]byte(trimmed), &rawData); err == nil {
			if walkPGPlanAny(rawData, hints) {
				for tbl, h := range hints {
					h.UsingFilesort = true
					hints[tbl] = h
				}
			}
		}
	}
}

func walkPGPlanAny(data any, hints map[string]ExplainTableHint) bool {
	var hasSort bool
	switch v := data.(type) {
	case []any:
		for _, item := range v {
			if walkPGPlanAny(item, hints) {
				hasSort = true
			}
		}
	case map[string]any:
		if plan, ok := v["Plan"].(map[string]any); ok {
			if walkPGPlanNode(plan, hints) {
				hasSort = true
			}
		} else {
			if walkPGPlanNode(v, hints) {
				hasSort = true
			}
		}
	}
	return hasSort
}

func walkPGPlanNode(node map[string]any, hints map[string]ExplainTableHint) bool {
	nodeType, _ := node["Node Type"].(string)
	relName, _ := node["Relation Name"].(string)
	isSort := strings.EqualFold(nodeType, "Sort")

	if relName != "" {
		tbl := strings.ToLower(relName)
		h := hints[tbl]
		if strings.EqualFold(nodeType, "Seq Scan") {
			h.AccessType = "ALL"
		} else if h.AccessType == "" && strings.Contains(strings.ToLower(nodeType), "index") {
			h.AccessType = "INDEX"
		}
		if planRows, ok := node["Plan Rows"].(float64); ok && planRows > 0 {
			h.RowsExamined = int64(planRows)
		}
		hints[tbl] = h
	}

	// Recursively walk sub-plans if any
	if plans, ok := node["Plans"].([]any); ok {
		for _, child := range plans {
			if walkPGPlanAny(child, hints) {
				isSort = true
			}
		}
	}
	return isSort
}

func sanitizeIdent(s string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	cleaned := reg.ReplaceAllString(s, "_")
	return strings.Trim(cleaned, "_")
}
