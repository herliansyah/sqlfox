# 0035: Heuristic Index Advisor & Query Tuner

## Status
Accepted

## Context
Following the completion of the Visual Query Plan & Cost Analyzer (ADR-0023 / Milestone 23) and Large-Scale Hardening (ADR-0034), developers and DBAs can identify slow execution bottlenecks (such as `ALL` Full Table Scans or memory filesorts), but often lack instant, deterministic guidance on how to formulate optimal composite indexes.
Additionally:
1. **ESR Rule Ordering**: Developers frequently guess column ordering in composite indexes, leading to sub-optimal indexes that fail to eliminate filesorts or truncate range scans.
2. **Index Bloat Risk**: Piling on indexes blindly degrades write performance (`INSERT`, `UPDATE`, `DELETE`) and wastes buffer pool cache. Tables with > 5 secondary indexes require explicit warnings while still surfacing tuning opportunities.
3. **Multi-Dialect & MySQL 5.7 Support**: Online non-blocking DDL requires distinct syntax across dialects (`ALGORITHM=INPLACE, LOCK=NONE` on MySQL 5.7/8.0, `CONCURRENTLY` on PostgreSQL, and exclusive write lock protections on SQLite).
4. **Zero New Dependencies**: Must be implemented purely with existing Go standard library and existing `sqlparser` AST inspection without bloated external optimizer dependencies.

## Decisions

### 1. ESR (Equality, Sort, Range) Sequencing Engine
A deterministic rule engine in `internal/advisor/` extracts predicate columns from AST nodes:
- **Equality (`E`)**: Columns filtered by `=`, `<=>`, `IS NULL`, and join equality predicates are sequenced first for maximum selectivity.
- **Sort/Group (`S`)**: `ORDER BY` and `GROUP BY` columns are sequenced immediately following equality columns to allow index-ordered scans that eliminate memory filesorts.
- **Range (`R`)**: Inequality operators (`>`, `<`, `BETWEEN`, `LIKE`, `IN`) are placed as trailing columns since B-Tree traversals cannot utilize subsequent columns after a range condition.
- **Ceiling**: Composite indexes are capped at 4 columns to avoid wide index maintenance overhead.

### 2. Index Bloat Guard & Threshold Heuristics
- When a target table contains > 5 secondary indexes or total index storage length exceeds table data length, the engine triggers an **Index Bloat Warning**.
- The warning is presented prominently in amber (`⚠️ INDEX BLOAT WARNING`) cautioning about write latency penalties, while explicitly **retaining candidate index recommendations** to ensure developer flexibility.
- The engine computes prefix consolidation suggestions (`ReplaceSuggestion`) when an existing index is a prefix of the proposed index, recommending an extension/merge instead of creating duplicate redundant indexes.

### 3. Non-Blocking Safe DDL Generation
- **MySQL 5.7 & 8.0**: Generates `ALTER TABLE tbl ADD INDEX idx (...) ALGORITHM=INPLACE, LOCK=NONE;` with a 3s metadata lock timeout.
- **PostgreSQL**: Generates `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx ON tbl (...);`.
- **SQLite**: Generates `CREATE INDEX IF NOT EXISTS idx ON tbl (...);`.

### 4. Dual-Surface Integration: UI & Headless CLI
- **Visual EXPLAIN Modal**: A new `💡 Advisor` tab in `#impact-preview-modal` displays candidate index cards, ESR breakdown pills, safe DDL blocks, and 1-click `📋 Copy` and `⚡ Editor` buttons.
- **Headless CLI**: `sqlfox advise --dsn ... --sql "..."` (or `--file`) provides non-interactive analysis for scripts and CI/CD pipelines with both human-readable text and `--format json` output.

## Consequences
- Developers get instant, dialect-accurate index recommendations directly from the EXPLAIN modal.
- Prevents database downtime from blocking DDL and prevents index sprawl through prefix consolidation warnings and bloat alerts.
- Zero binary footprint increase and zero external CGo dependencies.
