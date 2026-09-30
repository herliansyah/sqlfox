package ai

import (
	"fmt"
	"regexp"
	"strings"

	"sqlfox/internal/driver"
)

// Firewall ensures zero data leakage and curates minimal schema context.
type Firewall struct {
	MaskIdentifiers bool
}

func NewFirewall(maskIdentifiers bool) *Firewall {
	return &Firewall{MaskIdentifiers: maskIdentifiers}
}

// ponytail: Heuristic table match: direct substring or singular stem match.
func tableMatchesQuery(tblName, queryUpper string) bool {
	u := strings.ToUpper(tblName)
	return strings.Contains(queryUpper, u) || (strings.HasSuffix(u, "S") && len(u) > 3 && strings.Contains(queryUpper, strings.TrimSuffix(u, "S")))
}

// BuildSchemaPrompt constructs a comprehensive DDL context for tables in the schema.
// For standard-sized schemas (<= 30 tables), it includes all tables so AI has complete context
// regardless of query wording, Indonesian terms, or join relationships.
func (f *Firewall) BuildSchemaPrompt(schema *driver.SchemaMetadata, referencedQuery string) string {
	if schema == nil || len(schema.Tables) == 0 {
		return "No database schema metadata available."
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

	var sb strings.Builder
	sb.WriteString("Database Schema Context (Metadata Only - No Row Data Included):\n\n")

	for _, tbl := range activeTables {
		sb.WriteString(fmt.Sprintf("Table `%s` (%d estimated rows):\n", tbl.Name, tbl.RowCount))
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
			sb.WriteString(fmt.Sprintf("    - `%s` %s%s%s\n", col.Name, col.Type, nullStr, pkStr))
		}
		if len(tbl.Indexes) > 0 {
			sb.WriteString("  Indexes:\n")
			for _, idx := range tbl.Indexes {
				uniqueStr := ""
				if idx.IsUnique {
					uniqueStr = "UNIQUE "
				}
				sb.WriteString(fmt.Sprintf("    - %sINDEX `%s` (%s)\n", uniqueStr, idx.Name, strings.Join(idx.Columns, ", ")))
			}
		}
		sb.WriteString("\n")
	}

	if len(otherTableNames) > 0 {
		sb.WriteString(fmt.Sprintf("Other Tables in Database (columns omitted for brevity):\n%s\n\n", strings.Join(otherTableNames, ", ")))
	}

	result := sb.String()
	if f.MaskIdentifiers {
		result = f.applyMasking(result)
	}

	return result
}

var tokenRegex = regexp.MustCompile("`[a-zA-Z0-9_]+`")

func (f *Firewall) applyMasking(ddl string) string {
	mapping := make(map[string]string)
	colCounter := 1

	return tokenRegex.ReplaceAllStringFunc(ddl, func(match string) string {
		clean := strings.Trim(match, "`")
		if val, exists := mapping[clean]; exists {
			return "`" + val + "`"
		}
		masked := fmt.Sprintf("masked_%d", colCounter)
		colCounter++
		mapping[clean] = masked
		return "`" + masked + "`"
	})
}
