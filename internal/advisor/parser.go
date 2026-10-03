package advisor

import (
	"regexp"
	"strings"

	"github.com/xwb1989/sqlparser"
)

// ExtractPredicates parses a SQL statement and returns ESR categorized columns per table.
func ExtractPredicates(query string) []PredicateAnalysis {
	stmt, err := sqlparser.Parse(query)
	if err == nil {
		return extractFromAST(stmt)
	}

	// Fallback to lexical regex parsing for non-standard dialect statements
	return extractLexicalFallback(query)
}

func extractFromAST(stmt sqlparser.Statement) []PredicateAnalysis {
	var analyses []PredicateAnalysis

	switch node := stmt.(type) {
	case *sqlparser.Select:
		// Map aliases to table names
		aliasToTable := make(map[string]string)
		tables := make([]string, 0)

		for _, fromExpr := range node.From {
			tbls, aliases := extractTablesAndAliases(fromExpr)
			tables = append(tables, tbls...)
			for a, t := range aliases {
				aliasToTable[a] = t
			}
		}

		if len(tables) == 0 {
			return nil
		}

		// Analysis per table
		tableMap := make(map[string]*PredicateAnalysis)
		for _, t := range tables {
			tableMap[t] = &PredicateAnalysis{Table: t}
		}

		// 1. Walk WHERE clause
		if node.Where != nil {
			walkExpr(node.Where.Expr, tableMap, aliasToTable, tables[0])
		}

		// 2. Walk GROUP BY
		for _, grp := range node.GroupBy {
			colName, tbl := resolveColTable(grp, aliasToTable, tables[0])
			if colName != "" {
				if ana, ok := tableMap[tbl]; ok {
					if !containsStr(ana.SortCols, colName) {
						ana.SortCols = append(ana.SortCols, colName)
					}
				}
			}
		}

		// 3. Walk ORDER BY
		for _, ord := range node.OrderBy {
			colName, tbl := resolveColTable(ord.Expr, aliasToTable, tables[0])
			if colName != "" {
				if ana, ok := tableMap[tbl]; ok {
					if !containsStr(ana.SortCols, colName) {
						ana.SortCols = append(ana.SortCols, colName)
					}
				}
			}
		}

		for _, t := range tables {
			if ana, ok := tableMap[t]; ok {
				analyses = append(analyses, *ana)
			}
		}

	case *sqlparser.Update:
		tblName := sqlparser.String(node.TableExprs)
		tblName = cleanTableName(tblName)
		ana := &PredicateAnalysis{Table: tblName}
		tableMap := map[string]*PredicateAnalysis{tblName: ana}
		aliasToTable := map[string]string{tblName: tblName}

		if node.Where != nil {
			walkExpr(node.Where.Expr, tableMap, aliasToTable, tblName)
		}
		if node.OrderBy != nil {
			for _, ord := range node.OrderBy {
				colName, tbl := resolveColTable(ord.Expr, aliasToTable, tblName)
				if colName != "" && tbl == tblName && !containsStr(ana.SortCols, colName) {
					ana.SortCols = append(ana.SortCols, colName)
				}
			}
		}
		analyses = append(analyses, *ana)

	case *sqlparser.Delete:
		tblName := sqlparser.String(node.TableExprs)
		tblName = cleanTableName(tblName)
		ana := &PredicateAnalysis{Table: tblName}
		tableMap := map[string]*PredicateAnalysis{tblName: ana}
		aliasToTable := map[string]string{tblName: tblName}

		if node.Where != nil {
			walkExpr(node.Where.Expr, tableMap, aliasToTable, tblName)
		}
		if node.OrderBy != nil {
			for _, ord := range node.OrderBy {
				colName, tbl := resolveColTable(ord.Expr, aliasToTable, tblName)
				if colName != "" && tbl == tblName && !containsStr(ana.SortCols, colName) {
					ana.SortCols = append(ana.SortCols, colName)
				}
			}
		}
		analyses = append(analyses, *ana)
	}

	return analyses
}

func extractTablesAndAliases(tableExpr sqlparser.TableExpr) ([]string, map[string]string) {
	var tables []string
	aliases := make(map[string]string)

	switch t := tableExpr.(type) {
	case *sqlparser.AliasedTableExpr:
		rawName := sqlparser.String(t.Expr)
		tblName := cleanTableName(rawName)
		tables = append(tables, tblName)
		if !t.As.IsEmpty() {
			aliases[t.As.String()] = tblName
		}
		aliases[tblName] = tblName

	case *sqlparser.JoinTableExpr:
		leftTbls, leftAliases := extractTablesAndAliases(t.LeftExpr)
		rightTbls, rightAliases := extractTablesAndAliases(t.RightExpr)
		tables = append(tables, leftTbls...)
		tables = append(tables, rightTbls...)
		for k, v := range leftAliases {
			aliases[k] = v
		}
		for k, v := range rightAliases {
			aliases[k] = v
		}
	}

	return tables, aliases
}

func walkExpr(expr sqlparser.Expr, tableMap map[string]*PredicateAnalysis, aliasToTable map[string]string, defaultTable string) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *sqlparser.AndExpr:
		walkExpr(e.Left, tableMap, aliasToTable, defaultTable)
		walkExpr(e.Right, tableMap, aliasToTable, defaultTable)

	case *sqlparser.ComparisonExpr:
		op := strings.ToUpper(e.Operator)
		colName, tbl := resolveColTable(e.Left, aliasToTable, defaultTable)
		if colName == "" {
			colName, tbl = resolveColTable(e.Right, aliasToTable, defaultTable)
		}
		if colName == "" {
			return
		}

		ana, ok := tableMap[tbl]
		if !ok {
			return
		}

		switch op {
		case "=", "<=>":
			if !containsStr(ana.EqualityCols, colName) {
				ana.EqualityCols = append(ana.EqualityCols, colName)
			}
		case ">", "<", ">=", "<=", "!=", "<>", "LIKE", "NOT LIKE", "REGEXP", "IN", "NOT IN":
			if !containsStr(ana.RangeCols, colName) {
				ana.RangeCols = append(ana.RangeCols, colName)
			}
		}

	case *sqlparser.RangeCond:
		colName, tbl := resolveColTable(e.Left, aliasToTable, defaultTable)
		if colName != "" {
			if ana, ok := tableMap[tbl]; ok {
				if !containsStr(ana.RangeCols, colName) {
					ana.RangeCols = append(ana.RangeCols, colName)
				}
			}
		}

	case *sqlparser.IsExpr:
		colName, tbl := resolveColTable(e.Expr, aliasToTable, defaultTable)
		if colName != "" {
			if ana, ok := tableMap[tbl]; ok {
				if e.Operator == sqlparser.IsNullStr {
					if !containsStr(ana.EqualityCols, colName) {
						ana.EqualityCols = append(ana.EqualityCols, colName)
					}
				} else {
					if !containsStr(ana.RangeCols, colName) {
						ana.RangeCols = append(ana.RangeCols, colName)
					}
				}
			}
		}
	}
}

func resolveColTable(expr sqlparser.Expr, aliasToTable map[string]string, defaultTable string) (string, string) {
	if col, ok := expr.(*sqlparser.ColName); ok {
		colName := col.Name.String()
		if !col.Qualifier.IsEmpty() {
			qual := col.Qualifier.Name.String()
			if mapped, ok := aliasToTable[qual]; ok {
				return colName, mapped
			}
			return colName, qual
		}
		return colName, defaultTable
	}
	return "", ""
}

func cleanTableName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`\"'")
	// If schema.table format, strip schema
	if parts := strings.Split(raw, "."); len(parts) > 1 {
		return strings.Trim(parts[len(parts)-1], "`\"'")
	}
	return raw
}

func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}

// extractLexicalFallback parses SQL using regex heuristics for non-standard queries.
func extractLexicalFallback(query string) []PredicateAnalysis {
	tblRegex := regexp.MustCompile(`(?i)\bFROM\s+([a-zA-Z0-9_` + "`" + `"]+)`)
	m := tblRegex.FindStringSubmatch(query)
	if len(m) < 2 {
		return nil
	}

	tbl := cleanTableName(m[1])
	ana := PredicateAnalysis{Table: tbl}

	// Find WHERE clause
	whereIdx := strings.Index(strings.ToUpper(query), " WHERE ")
	if whereIdx != -1 {
		clause := query[whereIdx+7:]
		// Cut off ORDER BY or GROUP BY
		endIdx := len(clause)
		if idx := strings.Index(strings.ToUpper(clause), " ORDER BY "); idx != -1 && idx < endIdx {
			endIdx = idx
		}
		if idx := strings.Index(strings.ToUpper(clause), " GROUP BY "); idx != -1 && idx < endIdx {
			endIdx = idx
		}
		if idx := strings.Index(strings.ToUpper(clause), " LIMIT "); idx != -1 && idx < endIdx {
			endIdx = idx
		}
		whereBody := clause[:endIdx]

		// Extract equality col = val
		eqRegex := regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*=\s*`)
		for _, eqMatch := range eqRegex.FindAllStringSubmatch(whereBody, -1) {
			if len(eqMatch) >= 2 && !containsStr(ana.EqualityCols, eqMatch[1]) {
				ana.EqualityCols = append(ana.EqualityCols, eqMatch[1])
			}
		}

		// Extract range col > / < / BETWEEN
		rangeRegex := regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*(?:>|<|>=|<=|BETWEEN|LIKE)\s*`)
		for _, rngMatch := range rangeRegex.FindAllStringSubmatch(whereBody, -1) {
			if len(rngMatch) >= 2 && !containsStr(ana.EqualityCols, rngMatch[1]) && !containsStr(ana.RangeCols, rngMatch[1]) {
				ana.RangeCols = append(ana.RangeCols, rngMatch[1])
			}
		}
	}

	// Find ORDER BY
	orderIdx := strings.Index(strings.ToUpper(query), " ORDER BY ")
	if orderIdx != -1 {
		orderBody := query[orderIdx+10:]
		if limitIdx := strings.Index(strings.ToUpper(orderBody), " LIMIT "); limitIdx != -1 {
			orderBody = orderBody[:limitIdx]
		}
		parts := strings.Split(orderBody, ",")
		for _, p := range parts {
			col := strings.Fields(strings.TrimSpace(p))
			if len(col) > 0 {
				c := strings.Trim(col[0], "`\"'")
				if !containsStr(ana.SortCols, c) {
					ana.SortCols = append(ana.SortCols, c)
				}
			}
		}
	}

	return []PredicateAnalysis{ana}
}
