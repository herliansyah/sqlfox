package driver

import (
	"fmt"
	"strings"
)

func SequenceResetStatements(tbl TableMetadata, driverName string) []string {
	d := strings.ToLower(driverName)
	if d != "postgres" && d != "postgresql" && d != "pg" {
		return nil
	}
	var out []string
	qTable := QuoteIdentifier(tbl.Name, d)
	for _, col := range tbl.Columns {
		if !strings.HasPrefix(strings.ToLower(col.DefaultValue), "nextval(") {
			continue
		}
		qCol := QuoteIdentifier(col.Name, d)
		out = append(out, fmt.Sprintf("SELECT setval(pg_get_serial_sequence(%s, %s), COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false);",
			EscapeSQLString(qTable, d), EscapeSQLString(col.Name, d), qCol, qTable))
	}
	return out
}

// TopologicalSortTables orders tables so that referenced parent tables appear before dependent child tables.
// ponytail: Kahn's algorithm resolves FK dependency DAG with deterministic cycle fallback.
func TopologicalSortTables(tables []TableMetadata) []TableMetadata {
	tableMap := make(map[string]TableMetadata, len(tables))
	for _, t := range tables {
		tableMap[strings.ToLower(t.Name)] = t
	}

	dependents := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if _, ok := inDegree[lowName]; !ok {
			inDegree[lowName] = 0
		}
		for _, fk := range t.ForeignKeys {
			refTable := strings.ToLower(fk.ReferencedTable)
			if refTable != "" && refTable != lowName {
				if _, exists := tableMap[refTable]; exists {
					dependents[refTable] = append(dependents[refTable], lowName)
					inDegree[lowName]++
				}
			}
		}
	}

	var queue []string
	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if inDegree[lowName] == 0 {
			queue = append(queue, lowName)
		}
	}

	var sortedNames []string
	visited := make(map[string]bool)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if visited[curr] {
			continue
		}
		visited[curr] = true
		sortedNames = append(sortedNames, curr)

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 && !visited[dep] {
				queue = append(queue, dep)
			}
		}
	}

	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if !visited[lowName] {
			sortedNames = append(sortedNames, lowName)
			visited[lowName] = true
		}
	}

	var result []TableMetadata
	for _, name := range sortedNames {
		if t, ok := tableMap[name]; ok {
			result = append(result, t)
		}
	}
	return result
}
