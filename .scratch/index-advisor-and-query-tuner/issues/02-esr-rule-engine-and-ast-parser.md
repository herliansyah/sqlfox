# Issue: ESR Rule Engine & AST Parser for Composite Index Formulation

Status: resolved
Type: prototype
Blocked by: 01

## Question
How should the parser extract predicate columns from SQL statements (`WHERE`, `JOIN ... ON`, `GROUP BY`, `ORDER BY`), categorize them into Equality (`=`, `IS NULL`), Sort/Group (`ORDER BY`, `GROUP BY`), and Range (`>`, `<`, `BETWEEN`, `LIKE 'prefix%'`), and synthesize the optimal composite index definition with overlap detection against existing table indexes?

Specifically:
1. How does the AST parser handle multi-table queries and qualify column names to their respective tables?
2. What algorithm checks whether an existing index already covers the prefix of the proposed index (e.g. existing index `(a, b)` vs proposed `(a, b, c)`), and how does it generate an `ALTER/REPLACE` suggestion instead of adding a redundant index?
3. What is the maximum column limit cutoff (e.g. 4-5 columns) to avoid generating excessively wide composite indexes?

## Answer
1. **Multi-Table AST & Lexical Parsing**: Implemented in `internal/advisor/parser.go` using `sqlparser` AST inspection with table alias resolution (`AliasedTableExpr` and `JoinTableExpr`) mapping qualified columns to their origin table, backed by regex fallback for non-standard dialect statements.
2. **ESR Composite Formulation**: Equality predicates (`=`, `<=>`, `IS NULL`) are placed first, followed by Sort/Group columns (`ORDER BY`, `GROUP BY`) to eliminate filesort, and trailing with Range predicates (`>`, `<`, `BETWEEN`, `LIKE`).
3. **Prefix Overlap & Redundancy Guard**: `checkRedundancy()` checks existing table indexes from catalog cache. If an existing index covers the proposed columns as a left-prefix, it marks `IsRedundant: true` and suppresses redundant index creation. If an existing index is a prefix of the proposed index, it generates a consolidation suggestion (`Consider consolidating existing index...`).
4. **Column Ceiling**: Bounded to a maximum of 4 columns to prevent wide composite index overhead.
