# Effort Map: MySQL & MariaDB Object CRUD & Designer Suite (Views, Triggers, Routines, Events)

## Destination

A complete, ergonomic, production-grade CRUD workflow and visual designer suite for Views, Triggers, Stored Routines (Procedures & Functions), and Scheduled Events in SqlFox on MySQL and MariaDB: featuring interactive modals with embedded CodeMirror SQL editors, parameter builders, schedule selectors, safe DDL execution with safety guards, and 1-click "Open in Query Tab" handoff.

## Notes

- Target Dialect: MySQL 5.7, 8.0+, MariaDB 10+
- Domain: Schema Object Introspection & DDL Execution in `CONTEXT.md`
- Skills: `/wayfinder`, `/prototype`, `/research`, `/tdd`
- Architecture Pattern: Hybrid Ergonomic Designer (Structured modal with embedded CodeMirror editor + 1-click handoff to full Query Editor tab)
- Zero-Dependency Invariant: Pure embedded vanilla JavaScript and standard Go library; no external npm or web packages.

## Decisions so far

<!-- the index — one line per closed ticket: enough to judge relevance, then zoom the link for the detail the ticket holds -->

- [MySQL DDL Delimiters, Definers, and Safe Object Recreation Contracts](file:///home/ian/orca/workspaces/sqlfox/marlin/.scratch/mysql-objects-crud-suite/issues/01-mysql-ddl-delimiters-and-safe-recreation-contracts.md) — Standardized bare DDL execution without DELIMITER, DEFINER stripping, atomic CREATE OR REPLACE VIEW, and sequential DROP IF EXISTS + CREATE for triggers, routines, and events.
- [CodeMirror Modal Embedding and Hybrid Tab Handoff Architecture](file:///home/ian/orca/workspaces/sqlfox/marlin/.scratch/mysql-objects-crud-suite/issues/02-codemirror-modal-and-hybrid-tab-handoff.md) — Proven zero-height elimination via post-showModal refresh, instance reuse on container._cmInstance, and 1-click openInQueryEditor tab handoff.
- [View and Trigger CRUD Hardening and CodeMirror Integration](file:///home/ian/orca/workspaces/sqlfox/marlin/.scratch/mysql-objects-crud-suite/issues/03-view-and-trigger-crud-hardening.md) — Hardened View (auto-fetch DDL on edit) and Trigger (DROP TRIGGER IF EXISTS prepended) with embedded CodeMirror and 1-click tab handoff.
- [Stored Routine Designer and Drop Management Suite](file:///home/ian/orca/workspaces/sqlfox/marlin/.scratch/mysql-objects-crud-suite/issues/04-routine-designer-and-drop-suite.md) — Implemented Routine Designer modal with Parameter Grid builder, DETERMINISTIC toggle, embedded CodeMirror, 1-click drop action, and sidebar integration.
- [Scheduled Event Designer and Lifecycle Management Suite](file:///home/ian/orca/workspaces/sqlfox/marlin/.scratch/mysql-objects-crud-suite/issues/05-scheduled-event-designer-and-drop-suite.md) — Implemented Scheduled Event Designer modal with recurring/one-time schedule picker, PRESERVE toggle, embedded CodeMirror, 1-click drop action, and sidebar integration.
- [Functional Hardening and Edge Case Verification Suite](file:///home/ian/orca/workspaces/sqlfox/marlin/.scratch/mysql-objects-crud-suite/issues/06-functional-hardening-and-edge-case-verification.md) — End-to-end verification covering targeted DEFINER stripping without whitespace flattening, full schema search filter coverage, trailing semicolon normalization, and rename lifecycle safety.

## Not yet specified

- **PostgreSQL Parity for Functions & Triggers**: Expanding the same visual designer suite to PostgreSQL PL/pgSQL functions, procedures, and row/statement-level triggers.
- **SQLite Trigger & View Alter Workarounds**: Special handling for SQLite's lack of `ALTER TRIGGER` or `CREATE OR REPLACE VIEW`.

## Out of scope

- Visual GUI diagramming for Trigger execution flowcharts.
- External debugger engine (e.g. step-by-step PL/SQL breakpoint debugging).
- Non-relational event schedulers or Redis/MongoDB triggers.
