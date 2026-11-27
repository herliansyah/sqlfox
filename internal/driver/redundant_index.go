package driver

import (
	"fmt"
	"slices"
	"strings"
)

// isPrimaryOrSystemIndex identifies constraint-backed primary/auto indexes across MySQL,
// PostgreSQL (*_pkey), and SQLite (sqlite_autoindex_*) that must never be recommended for DROP INDEX.
func isPrimaryOrSystemIndex(idx IndexMetadata, tblName string, pkCols []string) bool {
	lower := strings.ToLower(strings.TrimSpace(idx.Name))
	if lower == "primary" || strings.HasPrefix(lower, "sqlite_autoindex_") {
		return true
	}
	if idx.IsUnique && len(pkCols) > 0 && isSameColumns(idx.Columns, pkCols) {
		if strings.HasSuffix(lower, "_pkey") || lower == strings.ToLower(tblName)+"_pk" {
			return true
		}
	}
	return false
}

// DetectRedundantIndexes analyzes tables to discover duplicate, shadowed, and redundant primary key indexes.
// ponytail: Pure metadata heuristic scan avoiding redundant database queries.
func DetectRedundantIndexes(dialect, database string, tables []TableMetadata) []RedundantIndex {
	var results []RedundantIndex

	for _, tbl := range tables {
		if len(tbl.Indexes) == 0 {
			continue
		}

		// Collect Primary Key columns
		var pkCols []string
		for _, col := range tbl.Columns {
			if col.IsPrimaryKey {
				pkCols = append(pkCols, col.Name)
			}
		}

		// 1. Check for redundant PK index
		// If a secondary index consists strictly of the PK column(s) in order, it is redundant with PRIMARY.
		for _, idx := range tbl.Indexes {
			if isPrimaryOrSystemIndex(idx, tbl.Name, pkCols) {
				continue
			}
			if len(pkCols) > 0 && isSameColumns(idx.Columns, pkCols) {
				results = append(results, RedundantIndex{
					Table:          tbl.Name,
					RedundantIndex: idx.Name,
					RedundantCols:  idx.Columns,
					ShadowedBy:     "PRIMARY",
					ShadowedCols:   pkCols,
					Reason:         fmt.Sprintf("Primary key already indexes (%s)", strings.Join(pkCols, ", ")),
					DropSQL:        generateDropIndexSQL(dialect, database, tbl.Name, idx.Name),
				})
			}
		}

		// 2. Check for Left-Prefix Shadows and Exact Duplicates
		for i := 0; i < len(tbl.Indexes); i++ {
			idxA := tbl.Indexes[i]
			if isPrimaryOrSystemIndex(idxA, tbl.Name, pkCols) {
				continue
			}

			for j := 0; j < len(tbl.Indexes); j++ {
				if i == j {
					continue
				}
				idxB := tbl.Indexes[j]
				if isPrimaryOrSystemIndex(idxB, tbl.Name, pkCols) {
					continue
				}

				// Never compare indexes of different access methods (e.g., FULLTEXT/GIN/HASH vs BTREE).
				if idxA.Type != "" && idxB.Type != "" && !strings.EqualFold(idxA.Type, idxB.Type) {
					continue
				}

				// Check exact duplicate: same columns in same order
				if isSameColumns(idxA.Columns, idxB.Columns) {
					// Always keep the UNIQUE one; among equals, drop the lexically greater name.
					if (idxB.IsUnique && !idxA.IsUnique) ||
						(idxA.IsUnique == idxB.IsUnique && (idxA.Name > idxB.Name || (idxA.Name == idxB.Name && i > j))) {
						results = append(results, RedundantIndex{
							Table:          tbl.Name,
							RedundantIndex: idxA.Name,
							RedundantCols:  idxA.Columns,
							ShadowedBy:     idxB.Name,
							ShadowedCols:   idxB.Columns,
							Reason:         fmt.Sprintf("Exact duplicate of index '%s'", idxB.Name),
							DropSQL:        generateDropIndexSQL(dialect, database, tbl.Name, idxA.Name),
						})
						break
					}
				} else if isPrefixColumns(idxA.Columns, idxB.Columns) {
					// idxA is a strict left-prefix of idxB.
					// If idxA is NOT unique, queries on idxA.Columns are satisfied by idxB leftmost prefix.
					if !idxA.IsUnique {
						results = append(results, RedundantIndex{
							Table:          tbl.Name,
							RedundantIndex: idxA.Name,
							RedundantCols:  idxA.Columns,
							ShadowedBy:     idxB.Name,
							ShadowedCols:   idxB.Columns,
							Reason:         fmt.Sprintf("Left-prefix of composite index '%s' (%s)", idxB.Name, strings.Join(idxB.Columns, ", ")),
							DropSQL:        generateDropIndexSQL(dialect, database, tbl.Name, idxA.Name),
						})
						break
					}
				}
			}
		}
	}

	if results == nil {
		results = []RedundantIndex{}
	}
	return results
}

func isSameColumns(a, b []string) bool {
	return slices.EqualFunc(a, b, strings.EqualFold)
}

func isPrefixColumns(prefix, full []string) bool {
	return len(prefix) > 0 && len(prefix) < len(full) && slices.EqualFunc(prefix, full[:len(prefix)], strings.EqualFold)
}

func generateDropIndexSQL(dialect, database, table, index string) string {
	if IsSqliteDriver(dialect) {
		return fmt.Sprintf("DROP INDEX %s;", QuoteIdentifier(index, "sqlite"))
	}
	if IsPgDriver(dialect) {
		if database != "" {
			return fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s.%s;", QuoteIdentifier(database, "postgres"), QuoteIdentifier(index, "postgres"))
		}
		return fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s;", QuoteIdentifier(index, "postgres"))
	}
	if database != "" {
		return fmt.Sprintf("ALTER TABLE %s.%s DROP INDEX %s;", QuoteIdentifier(database, "mysql"), QuoteIdentifier(table, "mysql"), QuoteIdentifier(index, "mysql"))
	}
	return fmt.Sprintf("ALTER TABLE %s DROP INDEX %s;", QuoteIdentifier(table, "mysql"), QuoteIdentifier(index, "mysql"))
}
