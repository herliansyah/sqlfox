package ai

import (
	"fmt"
	"regexp"
	"strings"

	"sqlfox/internal/driver"
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
			sb.WriteString(fmt.Sprintf("    - `%s` %s%s%s\n", ident(col.Name), col.Type, nullStr, pkStr))
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

// MaskIdentifiersInText replaces real schema identifiers in free text (SQL, plans, errors)
// using the mapping produced by BuildSchemaPromptWithMap, longest keys first.
func MaskIdentifiersInText(text string, mapping map[string]string) string {
	if text == "" || len(mapping) == 0 {
		return text
	}
	keys := make([]string, 0, len(mapping))
	for k := range mapping {
		if k != "" {
			keys = append(keys, k)
		}
	}
	// Sort longest-first so e.g. user_accounts is replaced before user.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if len(keys[j]) > len(keys[i]) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	out := text
	for _, k := range keys {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(k) + `\b`)
		out = re.ReplaceAllString(out, mapping[k])
	}
	return out
}
