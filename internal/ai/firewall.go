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

// BuildSchemaPrompt constructs a compact DDL context strictly for tables referenced in query or prompt.
func (f *Firewall) BuildSchemaPrompt(schema *driver.SchemaMetadata, referencedQuery string) string {
	if schema == nil || len(schema.Tables) == 0 {
		return "No database schema metadata available."
	}

	// Extract table names mentioned in query or prompt
	queryUpper := strings.ToUpper(referencedQuery)
	var activeTables []driver.TableMetadata

	for _, tbl := range schema.Tables {
		tblUpper := strings.ToUpper(tbl.Name)
		// Check if table name appears in query, or if active tables is empty and total tables < 10 include all
		if strings.Contains(queryUpper, tblUpper) || len(schema.Tables) <= 5 {
			activeTables = append(activeTables, tbl)
		}
	}

	if len(activeTables) == 0 {
		// Fallback to top 10 tables
		limit := len(schema.Tables)
		if limit > 10 {
			limit = 10
		}
		activeTables = schema.Tables[:limit]
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
