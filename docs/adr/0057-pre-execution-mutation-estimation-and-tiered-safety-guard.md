# 0057: Pre-Execution Mutation Estimation & Tiered Safety Guard

## Status
Accepted

## Context
Operators executing data-manipulation queries (`UPDATE`, `DELETE`, `MERGE`) in modern database management tools often suffer from "confirmation blindness" or unexpected mass data alteration:
1. **Uncertain Impact Scope**: Prior to statement execution, operators cannot verify how many rows a `WHERE` predicate actually matches without manually crafting and executing a separate `SELECT COUNT(*) FROM ...` query beforehand.
2. **Binary All-or-Nothing Safety**: Safety guards historically only intercepted completely unqualified mutations (without a `WHERE` clause), allowing queries with flawed, overly broad, or typo-ridden `WHERE` clauses (e.g. `WHERE status = 'archived' OR 1=1`) to mutate millions of rows instantly without warning.
3. **Confirmation Fatigue**: Prompting a heavy, modal-typing challenge for single-row test mutations in local `Development` environments frustrates developers and leads them to seek ways to disable safety systems entirely.
4. **Disjointed Data Grid Safety**: Inline grid edits (`Apply Staged Edits`) and row deletions (`Delete Selected`) used separate, rudimentary browser confirm alerts rather than sharing unified governance with the SQL editor.

## Decisions

### 1. Pre-Execution Row Estimation
Before executing any mutating DML query (`UPDATE`, `DELETE`), the Safe Execution Pipeline performs a non-mutating pre-flight estimation:
- **AST Query Rewrite**: The statement's AST is rewritten into a bounded `SELECT COUNT(*) FROM <table> WHERE <predicate>` and executed with a strict 2-second timeout.
- **Planner Fallback**: If the query reaches the timeout (e.g. massive 50M+ row unindexed table) or features dialect-specific multi-table joins/CTEs beyond deterministic rewrite rules, the pipeline falls back to database query planner statistics (`EXPLAIN` / `Impact Preview`) or displays an informative *Unknown Impact* warning.

### 2. Tiered Affirmation Gate
Confirmation requirements adapt dynamically based on estimated blast radius:
- **Low-Impact Mutations (1–100 rows)**: Present a lightweight 1-click inspection dialog displaying the target database, table, and estimated affected row count with `[Cancel]` and `[Confirm Execution]`.
- **High-Impact Mutations (>100 rows or Unqualified)**: Escalate to the full **Challenge Modal**, requiring the operator to manually type the target table name to authorize execution.

### 3. Environment-Aware Mutation Threshold
- **Development**: Single-row mutations matching a Primary Key or Unique Index execute instantly without modal interruption. Mutations affecting `> 1` row, zero rows, or lacking a `WHERE` clause trigger confirmation.
- **Staging & Production**: Universal pre-execution confirmation is enforced for **all** `UPDATE` and `DELETE` operations regardless of row count.

### 4. Multi-Statement Batch Aggregation
When executing compound SQL scripts containing multiple mutation statements:
- The backend evaluates each statement in the batch and aggregates the impact into a single consolidated review modal listing statement-by-statement counts and the overall total.
- If any statement in the batch exceeds 100 rows or lacks a `WHERE` clause, the entire batch gate escalates to the Challenge Modal.

### 5. Unified Virtual Data Grid Governance
- Grid row deletion (`Delete Selected`) and batch persistence (`Apply Staged Edits`) are unified under the same visual Safety Guard modal system, displaying environment badges, target tables, affected row counts, and escalating to the Challenge Modal if deleting `> 100` rows.

### 6. Exclusion of DML Backup Snapshots
- Automated backup snapshots remain strictly reserved for destructive DDL operations (`DROP` and `TRUNCATE`). DML mutations (`UPDATE`/`DELETE`) omit backup snapshots to maintain zero disk I/O overhead and sub-millisecond execution throughput.

### 7. Environment-Gated Session Mutation Safety Toggle
- Query Editor toolbars expose a `[🛡️ Confirm Mutations]` toggle.
- To prevent accidental accidents in production, the toggle is only switchable in `Development` environments; it is permanently locked and enforced in `Staging` and `Production`.

### 8. Post-Execution Discrepancy Notice
- To handle concurrent mutations without acquiring disruptive write locks during pre-flight, the session manager compares the actual `RowsAffected` with the initial estimation post-execution, presenting a visual notice or warning banner in the status bar if a significant disparity occurs.

## Consequences
- Completely eliminates catastrophic accidental bulk updates or mass deletions caused by broad `WHERE` predicates.
- Prevents confirmation fatigue by keeping 1-row development mutations frictionless while ensuring ironclad governance in production.
- Provides unified, predictable safety behavior across both the SQL Query Editor and the Virtual Data Grid.
