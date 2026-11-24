package advisor

import (
	"strings"
)

type advTokenKind int

const (
	advWord advTokenKind = iota
	advIdent
	advString
	advOp
	advDot
	advComma
	advLParen
	advRParen
	advOther
)

type advToken struct {
	kind     advTokenKind
	raw      string
	upper    string
	unquoted string
}

func tokenizeAdvisorSQL(q string) []advToken {
	n := len(q)
	tokens := make([]advToken, 0, n/6+4)
	for i := 0; i < n; {
		c := q[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' {
			i++
			continue
		}
		if (c == '-' && i+1 < n && q[i+1] == '-') || c == '#' {
			for i < n && q[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && q[i+1] == '*' {
			i += 2
			for i+1 < n && !(q[i] == '*' && q[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		if c == '\'' {
			j := i + 1
			for j < n {
				if q[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				if q[j] == '\'' {
					if j+1 < n && q[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			tokens = append(tokens, advToken{kind: advString, raw: q[i:j]})
			i = j
			continue
		}
		if c == '`' || c == '"' || c == '[' {
			closeChar := c
			if c == '[' {
				closeChar = ']'
			}
			j := i + 1
			for j < n && q[j] != closeChar {
				j++
			}
			if j < n {
				j++
			}
			unq := q[i:j]
			if len(unq) >= 2 && unq[len(unq)-1] == closeChar {
				unq = unq[1 : len(unq)-1]
			}
			tokens = append(tokens, advToken{kind: advIdent, raw: q[i:j], upper: strings.ToUpper(unq), unquoted: unq})
			i = j
			continue
		}
		// Multi-char comparison operators: <=>, <=, >=, !=, <>
		if i+2 < n && q[i:i+3] == "<=>" {
			tokens = append(tokens, advToken{kind: advOp, raw: "<=>", upper: "<=>"})
			i += 3
			continue
		}
		if i+1 < n {
			two := q[i : i+2]
			if two == "<=" || two == ">=" || two == "!=" || two == "<>" {
				tokens = append(tokens, advToken{kind: advOp, raw: two, upper: two})
				i += 2
				continue
			}
		}
		switch c {
		case '=', '<', '>':
			tokens = append(tokens, advToken{kind: advOp, raw: string(c), upper: string(c)})
			i++
		case '.':
			tokens = append(tokens, advToken{kind: advDot, raw: "."})
			i++
		case ',':
			tokens = append(tokens, advToken{kind: advComma, raw: ","})
			i++
		case '(':
			tokens = append(tokens, advToken{kind: advLParen, raw: "("})
			i++
		case ')':
			tokens = append(tokens, advToken{kind: advRParen, raw: ")"})
			i++
		default:
			if isAdvIdentByte(c) || c == '$' {
				j := i + 1
				for j < n && isAdvIdentByte(q[j]) {
					j++
				}
				raw := q[i:j]
				tokens = append(tokens, advToken{kind: advWord, raw: raw, upper: strings.ToUpper(raw), unquoted: raw})
				i = j
			} else {
				tokens = append(tokens, advToken{kind: advOther, raw: string(c)})
				i++
			}
		}
	}
	return tokens
}

func isAdvIdentByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ExtractPredicates parses a SQL statement and returns ESR categorized columns per table.
func ExtractPredicates(query string) []PredicateAnalysis {
	tokens := tokenizeAdvisorSQL(query)
	if len(tokens) == 0 {
		return nil
	}

	// Skip leading WITH ... CTEs if present to reach the main SELECT/UPDATE/DELETE
	startIdx := 0
	if tokens[0].kind == advWord && tokens[0].upper == "WITH" {
		depth := 0
		for i := 1; i < len(tokens); i++ {
			if tokens[i].kind == advLParen {
				depth++
			} else if tokens[i].kind == advRParen && depth > 0 {
				depth--
			} else if depth == 0 && tokens[i].kind == advWord {
				switch tokens[i].upper {
				case "SELECT", "UPDATE", "DELETE":
					startIdx = i
					goto parsedVerb
				}
			}
		}
	}

parsedVerb:
	if startIdx >= len(tokens) || tokens[startIdx].kind != advWord {
		return nil
	}

	var tables []string
	aliasToTable := make(map[string]string)

	// Locate primary table(s)
	switch tokens[startIdx].upper {
	case "SELECT":
		fromIdx := findTopKeyword(tokens, startIdx+1, "FROM")
		if fromIdx < 0 {
			return nil
		}
		tables, aliasToTable = parseFromTables(tokens, fromIdx+1)
	case "UPDATE":
		tbl, alias, _ := parseTableAndAlias(tokens, startIdx+1)
		if tbl != "" {
			tables = append(tables, tbl)
			aliasToTable[tbl] = tbl
			if alias != "" {
				aliasToTable[alias] = tbl
			}
		}
	case "DELETE":
		idx := startIdx + 1
		if idx < len(tokens) && tokens[idx].kind == advWord && tokens[idx].upper == "FROM" {
			idx++
		}
		tbl, alias, _ := parseTableAndAlias(tokens, idx)
		if tbl != "" {
			tables = append(tables, tbl)
			aliasToTable[tbl] = tbl
			if alias != "" {
				aliasToTable[alias] = tbl
			}
		}
	default:
		return nil
	}

	if len(tables) == 0 {
		return nil
	}

	tableMap := make(map[string]*PredicateAnalysis, len(tables))
	for _, t := range tables {
		tableMap[t] = &PredicateAnalysis{Table: t}
	}

	// Walk JOIN ... ON predicates between FROM/JOIN and WHERE/GROUP/ORDER
	if fromIdx := findTopKeyword(tokens, startIdx+1, "FROM"); fromIdx >= 0 {
		fromEnd := findClauseEnd(tokens, fromIdx+1, "WHERE", "GROUP", "HAVING", "ORDER", "LIMIT", "RETURNING", "FOR")
		for j := fromIdx + 1; j < fromEnd; j++ {
			if tokens[j].kind == advWord && tokens[j].upper == "ON" {
				onEnd := findClauseEnd(tokens, j+1, "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS", "NATURAL", "STRAIGHT_JOIN", "WHERE", "GROUP", "HAVING", "ORDER", "LIMIT")
				parseJoinOnTokens(tokens[j+1:onEnd], tableMap, aliasToTable, tables[0])
				j = onEnd - 1
			}
		}
	}

	// Walk WHERE clause predicates
	if whereIdx := findTopKeyword(tokens, startIdx+1, "WHERE"); whereIdx >= 0 {
		endIdx := findClauseEnd(tokens, whereIdx+1, "GROUP", "ORDER", "HAVING", "LIMIT", "RETURNING", "FOR")
		parseWhereTokens(tokens[whereIdx+1:endIdx], tableMap, aliasToTable, tables[0])
	}

	// Walk GROUP BY columns
	if grpIdx := findTopKeyword(tokens, startIdx+1, "GROUP"); grpIdx >= 0 &&
		grpIdx+1 < len(tokens) && tokens[grpIdx+1].kind == advWord && tokens[grpIdx+1].upper == "BY" {
		endIdx := findClauseEnd(tokens, grpIdx+2, "HAVING", "ORDER", "LIMIT", "RETURNING")
		parseSortTokens(tokens[grpIdx+2:endIdx], tableMap, aliasToTable, tables[0])
	}

	// Walk ORDER BY columns
	if ordIdx := findTopKeyword(tokens, startIdx+1, "ORDER"); ordIdx >= 0 &&
		ordIdx+1 < len(tokens) && tokens[ordIdx+1].kind == advWord && tokens[ordIdx+1].upper == "BY" {
		endIdx := findClauseEnd(tokens, ordIdx+2, "LIMIT", "OFFSET", "FETCH", "RETURNING", "FOR")
		parseSortTokens(tokens[ordIdx+2:endIdx], tableMap, aliasToTable, tables[0])
	}

	analyses := make([]PredicateAnalysis, 0, len(tables))
	for _, t := range tables {
		if ana, ok := tableMap[t]; ok {
			analyses = append(analyses, *ana)
		}
	}
	return analyses
}

func findTopKeyword(tokens []advToken, start int, kw string) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].kind {
		case advLParen:
			depth++
		case advRParen:
			if depth > 0 {
				depth--
			}
		case advWord:
			if depth == 0 && tokens[i].upper == kw {
				return i
			}
		}
	}
	return -1
}

func findClauseEnd(tokens []advToken, start int, stopWords ...string) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].kind {
		case advLParen:
			depth++
		case advRParen:
			if depth > 0 {
				depth--
			}
		case advWord:
			if depth == 0 {
				for _, sw := range stopWords {
					if tokens[i].upper == sw {
						return i
					}
				}
			}
		}
	}
	return len(tokens)
}

func parseFromTables(tokens []advToken, start int) ([]string, map[string]string) {
	var tables []string
	aliases := make(map[string]string)
	end := findClauseEnd(tokens, start, "WHERE", "GROUP", "HAVING", "ORDER", "LIMIT", "RETURNING")

	depth := 0
	expectTable := true
	for i := start; i < end; i++ {
		tok := tokens[i]
		if tok.kind == advLParen {
			depth++
			expectTable = false
			continue
		}
		if tok.kind == advRParen {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth != 0 {
			continue
		}
		if tok.kind == advComma || (tok.kind == advWord && tok.upper == "JOIN") {
			expectTable = true
			continue
		}
		if expectTable && (tok.kind == advWord || tok.kind == advIdent) {
			switch tok.upper {
			case "INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS", "NATURAL", "STRAIGHT_JOIN":
				continue
			}
			tbl, alias, nextIdx := parseTableAndAlias(tokens, i)
			if tbl != "" {
				if !containsStr(tables, tbl) {
					tables = append(tables, tbl)
				}
				aliases[tbl] = tbl
				if alias != "" {
					aliases[alias] = tbl
				}
			}
			expectTable = false
			i = nextIdx - 1
		}
	}
	return tables, aliases
}

func parseTableAndAlias(tokens []advToken, idx int) (string, string, int) {
	if idx >= len(tokens) || (tokens[idx].kind != advWord && tokens[idx].kind != advIdent) {
		return "", "", idx + 1
	}
	tbl := tokens[idx].unquoted
	next := idx + 1
	if next+1 < len(tokens) && tokens[next].kind == advDot && (tokens[next+1].kind == advWord || tokens[next+1].kind == advIdent) {
		tbl = tokens[next+1].unquoted
		next += 2
	}
	alias := ""
	if next < len(tokens) && tokens[next].kind == advWord && tokens[next].upper == "AS" {
		next++
		if next < len(tokens) && (tokens[next].kind == advWord || tokens[next].kind == advIdent) {
			alias = tokens[next].unquoted
			next++
		}
	} else if next < len(tokens) && (tokens[next].kind == advWord || tokens[next].kind == advIdent) {
		switch tokens[next].upper {
		case "WHERE", "SET", "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS", "ON", "USING", "GROUP", "ORDER", "LIMIT", "HAVING":
		default:
			alias = tokens[next].unquoted
			next++
		}
	}
	return tbl, alias, next
}

func parseColRef(tokens []advToken, idx int, aliasToTable map[string]string, defaultTable string) (string, string, int) {
	if idx >= len(tokens) || (tokens[idx].kind != advWord && tokens[idx].kind != advIdent) {
		return "", "", idx
	}
	if tokens[idx].kind == advWord {
		switch tokens[idx].upper {
		case "AND", "OR", "NOT", "NULL", "TRUE", "FALSE", "IS", "IN", "BETWEEN", "LIKE", "ILIKE", "REGEXP":
			return "", "", idx
		}
		if tokens[idx].raw[0] >= '0' && tokens[idx].raw[0] <= '9' || tokens[idx].raw[0] == '$' {
			return "", "", idx
		}
	}
	if idx+2 < len(tokens) && tokens[idx+1].kind == advDot && (tokens[idx+2].kind == advWord || tokens[idx+2].kind == advIdent) {
		qual := tokens[idx].unquoted
		col := tokens[idx+2].unquoted
		if mapped, ok := aliasToTable[qual]; ok {
			return col, mapped, idx + 3
		}
		return col, qual, idx + 3
	}
	return tokens[idx].unquoted, defaultTable, idx + 1
}

func parseJoinOnTokens(tokens []advToken, tableMap map[string]*PredicateAnalysis, aliasToTable map[string]string, defaultTable string) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].kind == advLParen || tokens[i].kind == advRParen {
			continue
		}
		col, tbl, afterCol := parseColRef(tokens, i, aliasToTable, defaultTable)
		if col == "" || afterCol >= len(tokens) {
			continue
		}
		if tokens[afterCol].kind == advOp && (tokens[afterCol].upper == "=" || tokens[afterCol].upper == "<=>") {
			if ana, ok := tableMap[tbl]; ok {
				if !containsStr(ana.EqualityCols, col) {
					ana.EqualityCols = append(ana.EqualityCols, col)
				}
				if !containsStr(ana.ReferencedCols, col) {
					ana.ReferencedCols = append(ana.ReferencedCols, col)
				}
			}
			// Also capture the right-hand side column if it's a join column (e.g. a.id = b.a_id)
			if rCol, rTbl, afterR := parseColRef(tokens, afterCol+1, aliasToTable, defaultTable); rCol != "" && rTbl != "" {
				if rAna, ok := tableMap[rTbl]; ok {
					if !containsStr(rAna.EqualityCols, rCol) {
						rAna.EqualityCols = append(rAna.EqualityCols, rCol)
					}
					if !containsStr(rAna.ReferencedCols, rCol) {
						rAna.ReferencedCols = append(rAna.ReferencedCols, rCol)
					}
				}
				i = afterR - 1
				continue
			}
			i = afterCol
		}
	}
}

func parseWhereTokens(tokens []advToken, tableMap map[string]*PredicateAnalysis, aliasToTable map[string]string, defaultTable string) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].kind == advLParen || tokens[i].kind == advRParen {
			continue
		}

		col, tbl, afterCol := parseColRef(tokens, i, aliasToTable, defaultTable)
		if col == "" || afterCol >= len(tokens) {
			continue
		}
		ana, ok := tableMap[tbl]
		if !ok {
			continue
		}
		if !containsStr(ana.ReferencedCols, col) {
			ana.ReferencedCols = append(ana.ReferencedCols, col)
		}

		opTok := tokens[afterCol]
		if opTok.kind == advOp {
			switch opTok.upper {
			case "=", "<=>":
				if !containsStr(ana.EqualityCols, col) {
					ana.EqualityCols = append(ana.EqualityCols, col)
				}
			case ">", "<", ">=", "<=", "!=", "<>":
				if !containsStr(ana.RangeCols, col) {
					ana.RangeCols = append(ana.RangeCols, col)
				}
			}
			i = afterCol
			continue
		}

		if opTok.kind == advWord {
			switch opTok.upper {
			case "IS":
				if afterCol+1 < len(tokens) && tokens[afterCol+1].kind == advWord && tokens[afterCol+1].upper == "NULL" {
					if !containsStr(ana.EqualityCols, col) {
						ana.EqualityCols = append(ana.EqualityCols, col)
					}
				} else {
					if !containsStr(ana.RangeCols, col) {
						ana.RangeCols = append(ana.RangeCols, col)
					}
				}
				i = afterCol
			case "LIKE", "ILIKE", "REGEXP", "BETWEEN", "IN":
				if !containsStr(ana.RangeCols, col) {
					ana.RangeCols = append(ana.RangeCols, col)
				}
				i = afterCol
			case "NOT":
				if afterCol+1 < len(tokens) && tokens[afterCol+1].kind == advWord {
					switch tokens[afterCol+1].upper {
					case "LIKE", "ILIKE", "IN", "BETWEEN":
						if !containsStr(ana.RangeCols, col) {
							ana.RangeCols = append(ana.RangeCols, col)
						}
					}
				}
				i = afterCol + 1
			}
		}
	}
}

func parseSortTokens(tokens []advToken, tableMap map[string]*PredicateAnalysis, aliasToTable map[string]string, defaultTable string) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].kind == advWord && (tokens[i].upper == "ASC" || tokens[i].upper == "DESC" || tokens[i].upper == "NULLS" || tokens[i].upper == "FIRST" || tokens[i].upper == "LAST") {
			continue
		}
		col, tbl, next := parseColRef(tokens, i, aliasToTable, defaultTable)
		if col != "" {
			if ana, ok := tableMap[tbl]; ok && !containsStr(ana.SortCols, col) {
				ana.SortCols = append(ana.SortCols, col)
			}
			i = next - 1
		}
	}
}

func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}
