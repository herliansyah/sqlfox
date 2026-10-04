# Effort Map: Heuristic Index Advisor & Query Tuner

## Destination
A lightweight, zero-dependency heuristic index recommendation engine across MySQL (including MySQL 5.7 and 8.0+), PostgreSQL, and SQLite that analyzes AST query predicates (ESR rule), existing table indexes, and EXPLAIN plans to generate safe DDL index recommendations, warns clearly on index bloat, and provides single-query UI advice and batch workload CLI analysis.

## Notes
- Domain: `Query Optimization & Index Tuning` in `CONTEXT.md`
- Skills: `/grilling`, `/domain-modeling`, `/research`, `/prototype`
- Constraints: Pure Go, zero new dependencies, non-blocking DDL (`ALGORITHM=INPLACE, LOCK=NONE` on MySQL 5.7/8.0, `CONCURRENTLY` on PostgreSQL), explicit Index Bloat Warning when secondary indexes > 5 while continuing to display recommendations.

## Decisions so far

<!-- the index — one line per closed ticket: enough to judge relevance, then zoom the link for the detail the ticket holds -->

- [MySQL 5.7 and Cross-Dialect Index DDL & EXPLAIN Introspection Compatibility](file:///home/ian/sqlfox-aspidochelone/.scratch/index-advisor-and-query-tuner/issues/01-mysql-57-and-cross-dialect-compatibility.md) — Confirmed non-blocking DDL (`ALGORITHM=INPLACE, LOCK=NONE` on 5.7), recursive JSON EXPLAIN traversal, and isolated catalog introspection fixes.
- [ESR Rule Engine & AST Parser for Composite Index Formulation](file:///home/ian/sqlfox-aspidochelone/.scratch/index-advisor-and-query-tuner/issues/02-esr-rule-engine-and-ast-parser.md) — Implemented ESR column ordering (Equality -> Sort -> Range), 4-column cap, and left-prefix redundancy detection.
- [Index Bloat Guard & Storage Overhead Heuristics](file:///home/ian/sqlfox-aspidochelone/.scratch/index-advisor-and-query-tuner/issues/03-index-bloat-guard-and-storage-heuristics.md) — Implemented index bloat warning (>5 secondary indexes) with unsuppressed candidate recommendations and consolidation advice.
- [UI Surface Integration in Visual EXPLAIN Modal & Query Editor](file:///home/ian/sqlfox-aspidochelone/.scratch/index-advisor-and-query-tuner/issues/04-ui-integration-explain-modal-and-editor.md) — Integrated `💡 Advisor` tab in EXPLAIN modal with amber bloat banner, ESR badges, safe DDL copy, and 1-click editor injection.
- [Workload Batch Analyzer for Session Audit Log & Headless CLI](file:///home/ian/sqlfox-aspidochelone/.scratch/index-advisor-and-query-tuner/issues/05-workload-batch-analyzer-and-headless-cli.md) — Implemented `sqlfox advise` CLI subcommand supporting `--dsn`, `--sql`, `--file`, and JSON output.

## Not yet specified

- **Covering Index Heuristics & `INCLUDE` Clause**: Determining when a recommendation should promote columns from `SELECT` into a covering index vs keeping the index narrow to minimize index leaf node write amplification.
- **Hypothetical Optimizer Feedback**: Investigating whether virtual/hypothetical index extensions (e.g. `hypopg` for PostgreSQL) can be opportunistically queried without hard dependencies.

## Out of scope

- AI / LLM based index advice (must remain deterministic, local AST & catalog analysis).
- Automatic execution / background auto-indexing without explicit human approval.
- Specialized non-relational or vector indexes.
