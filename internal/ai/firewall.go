package ai

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/herliansyah/sqlfox/internal/driver"
)

// ponytail: Heuristic table match: direct substring or singular stem match.
func tableMatchesQuery(tblName, queryUpper string) bool {
	u := strings.ToUpper(tblName)
	return strings.Contains(queryUpper, u) || (strings.HasSuffix(u, "S") && len(u) > 3 && strings.Contains(queryUpper, strings.TrimSuffix(u, "S")))
}

// BuildSchemaPrompt constructs a comprehensive DDL context for tables in the schema.
// For standard-sized schemas (<= 30 tables), it includes all tables so AI has complete context
// regardless of query wording, Indonesian terms, or join relationships.
// ponytail: Pure stateless function replacing single-method Firewall wrapper.
// BuildSchemaPrompt generates a schema metadata summary, optionally masking identifiers.
func BuildSchemaPrompt(schema *driver.SchemaMetadata, referencedQuery string, maskIdentifiers bool) string {
	prompt, _ := BuildSchemaPromptWithMap(schema, referencedQuery, maskIdentifiers)
	return prompt
}

// BuildSchemaPromptWithMap generates a schema metadata summary and returns the real->masked identifier map.
func BuildSchemaPromptWithMap(schema *driver.SchemaMetadata, referencedQuery string, maskIdentifiers bool) (string, map[string]string) {
	if schema == nil || len(schema.Tables) == 0 {
		return "No database schema metadata available.", nil
	}

	queryUpper := strings.ToUpper(referencedQuery)
	var activeTables []driver.TableMetadata
	var otherTableNames []string

	// If the schema is reasonably sized (<= 30 tables), include all tables with full columns.
	// This ensures AI has full relationship and join context without missing columns due to prompt wording.
	if len(schema.Tables) <= 30 {
		activeTables = schema.Tables
	} else {
		matchedMap := make(map[string]bool)
		for _, tbl := range schema.Tables {
			if tableMatchesQuery(tbl.Name, queryUpper) {
				activeTables = append(activeTables, tbl)
				matchedMap[tbl.Name] = true
			}
		}

		// Ensure at least 15 tables are presented for rich context
		if len(activeTables) < 15 {
			for _, tbl := range schema.Tables {
				if !matchedMap[tbl.Name] {
					activeTables = append(activeTables, tbl)
					matchedMap[tbl.Name] = true
					if len(activeTables) >= 15 {
						break
					}
				}
			}
		}

		// List remaining tables concisely so AI knows of their existence
		for _, tbl := range schema.Tables {
			if !matchedMap[tbl.Name] {
				otherTableNames = append(otherTableNames, tbl.Name)
			}
		}
	}

	// Masking is applied to every identifier as the prompt is built (table, column, index and
	// index-column names, and the "other tables" list), so no real name can slip through a
	// post-hoc regex that only recognised backticked ASCII tokens.
	mapping := make(map[string]string)
	ident := func(name string) string {
		if !maskIdentifiers {
			return name
		}
		if m, ok := mapping[name]; ok {
			return m
		}
		m := fmt.Sprintf("masked_%d", len(mapping)+1)
		mapping[name] = m
		return m
	}

	var sb strings.Builder
	sb.WriteString("Database Schema Context (Metadata Only - No Row Data Included):\n\n")

	for _, tbl := range activeTables {
		sb.WriteString(fmt.Sprintf("Table `%s` (%d estimated rows):\n", ident(tbl.Name), tbl.RowCount))
		sb.WriteString("  Columns:\n")
		for _, col := range tbl.Columns {
			pkStr := ""
			if col.IsPrimaryKey {
				pkStr = " [PRIMARY KEY]"
			}
			nullStr := " NOT NULL"
			if col.IsNullable {
				nullStr = " NULL"
			}
			colType := col.Type
			if maskIdentifiers {
				upperType := strings.ToUpper(strings.TrimSpace(colType))
				if strings.HasPrefix(upperType, "ENUM(") {
					colType = "ENUM([MASKED])"
				} else if strings.HasPrefix(upperType, "SET(") {
					colType = "SET([MASKED])"
				}
			}
			sb.WriteString(fmt.Sprintf("    - `%s` %s%s%s\n", ident(col.Name), colType, nullStr, pkStr))
		}
		if len(tbl.Indexes) > 0 {
			sb.WriteString("  Indexes:\n")
			for _, idx := range tbl.Indexes {
				uniqueStr := ""
				if idx.IsUnique {
					uniqueStr = "UNIQUE "
				}
				cols := make([]string, len(idx.Columns))
				for i, c := range idx.Columns {
					cols[i] = "`" + ident(c) + "`"
				}
				sb.WriteString(fmt.Sprintf("    - %sINDEX `%s` (%s)\n", uniqueStr, ident(idx.Name), strings.Join(cols, ", ")))
			}
		}
		sb.WriteString("\n")
	}

	if len(otherTableNames) > 0 {
		names := make([]string, len(otherTableNames))
		for i, n := range otherTableNames {
			names[i] = ident(n)
		}
		sb.WriteString(fmt.Sprintf("Other Tables in Database (columns omitted for brevity):\n%s\n\n", strings.Join(names, ", ")))
	}

	return sb.String(), mapping
}

var sqlKeywordsToPreserve = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "ORDER": true, "GROUP": true, "BY": true,
	"HAVING": true, "LIMIT": true, "OFFSET": true, "JOIN": true, "INNER": true, "LEFT": true,
	"RIGHT": true, "FULL": true, "OUTER": true, "CROSS": true, "ON": true, "AS": true,
	"AND": true, "OR": true, "NOT": true, "NULL": true, "IS": true, "IN": true, "LIKE": true,
	"ILIKE": true, "BETWEEN": true, "EXISTS": true, "CASE": true, "WHEN": true, "THEN": true,
	"ELSE": true, "END": true, "DISTINCT": true, "UNION": true, "ALL": true, "ASC": true,
	"DESC": true, "COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
	"INSERT": true, "INTO": true, "VALUES": true, "UPDATE": true, "SET": true, "DELETE": true,
	"CREATE": true, "ALTER": true, "DROP": true, "TABLE": true, "INDEX": true, "VIEW": true,
	"KEY": true, "PRIMARY": true, "FOREIGN": true, "REFERENCES": true, "DEFAULT": true,
	"WITH": true, "EXPLAIN": true, "ANALYZE": true,
}

// MaskIdentifiersInText replaces real schema identifiers in SQL, plans, errors, and prompts.
// Case-insensitive fallback is used only when a lowercase name maps to one identifier.
func MaskIdentifiersInText(text string, mapping map[string]string) string {
	if text == "" || len(mapping) == 0 {
		return text
	}
	keys := make([]string, 0, len(mapping))
	exactLookup := make(map[string]string, len(mapping))
	lowerLookup := make(map[string]string, len(mapping))
	ambiguousLower := make(map[string]bool)
	for k, v := range mapping {
		if k == "" {
			continue
		}
		keys = append(keys, k)
		exactLookup[k] = v
		lower := strings.ToLower(k)
		if prev, ok := lowerLookup[lower]; ok && prev != v {
			ambiguousLower[lower] = true
		} else {
			lowerLookup[lower] = v
		}
	}
	if len(keys) == 0 {
		return text
	}
	// Sort longest-first so e.g. user_accounts is matched before user in regex alternation.
	slices.SortFunc(keys, func(a, b string) int {
		if len(b) != len(a) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = regexp.QuoteMeta(k)
	}
	// Match quoted (`ident` or "ident") or bare word identifiers; never mask unquoted SQL keywords.
	re := regexp.MustCompile("(?i)(?:`|\")?(?:\\b(?:" + strings.Join(quoted, "|") + ")\\b)(?:`|\")?")
	return re.ReplaceAllStringFunc(text, func(match string) string {
		isQuoted := (strings.HasPrefix(match, "`") && strings.HasSuffix(match, "`")) ||
			(strings.HasPrefix(match, `"`) && strings.HasSuffix(match, `"`))
		bare := strings.Trim(match, "`\"")
		if !isQuoted && sqlKeywordsToPreserve[strings.ToUpper(bare)] {
			return match
		}
		repl, ok := exactLookup[bare]
		if !ok {
			lower := strings.ToLower(bare)
			if ambiguousLower[lower] {
				return match
			}
			repl, ok = lowerLookup[lower]
		}
		if !ok {
			return match
		}
		if isQuoted {
			return match[:1] + repl + match[len(match)-1:]
		}
		return repl
	})
}

// UnmaskIdentifiersInText replaces masked identifiers (e.g. masked_1, masked_2)
// back to their original schema identifiers in a single pass using the provided mapping.
func UnmaskIdentifiersInText(text string, mapping map[string]string) string {
	if text == "" || len(mapping) == 0 {
		return text
	}
	reverse := make(map[string]string, len(mapping))
	maskedKeys := make([]string, 0, len(mapping))
	for realName, maskedName := range mapping {
		if maskedName != "" && realName != "" {
			reverse[maskedName] = realName
			maskedKeys = append(maskedKeys, maskedName)
		}
	}
	if len(maskedKeys) == 0 {
		return text
	}
	slices.SortFunc(maskedKeys, func(a, b string) int {
		if len(b) != len(a) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	quoted := make([]string, len(maskedKeys))
	for i, m := range maskedKeys {
		quoted[i] = regexp.QuoteMeta(m)
	}
	re := regexp.MustCompile(`\b(?:` + strings.Join(quoted, "|") + `)\b`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		if orig, ok := reverse[match]; ok {
			return orig
		}
		return match
	})
}

var (
	reSQLSingleQuotedLiteral = regexp.MustCompile(`'(?:''|\\'|[^'])*'`)
	reEmailPII               = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	rePGDetailKeyValue       = regexp.MustCompile(`(?i)(DETAIL:\s*Key\s*\([^)]+\)\s*=\s*\()([^)]+)(\))`)
)

// RedactSensitiveLiterals strips quoted SQL string literals, PostgreSQL constraint DETAIL values,
// and email addresses before sending queries/errors/plans to external AI providers when privacy
// masking is enabled.
func RedactSensitiveLiterals(text string) string {
	if text == "" {
		return text
	}
	out := rePGDetailKeyValue.ReplaceAllString(text, "${1}[REDACTED]${3}")
	out = reSQLSingleQuotedLiteral.ReplaceAllString(out, "'[REDACTED]'")
	out = reEmailPII.ReplaceAllString(out, "[REDACTED_EMAIL]")
	return out
}
