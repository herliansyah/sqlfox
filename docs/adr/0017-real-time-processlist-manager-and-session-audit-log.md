# 0017: Real-Time Processlist Manager and Session Audit Log

To equip DBAs with real-time operational diagnostics and auditability, SqlFox introduces the Processlist Manager with Lock Dependency Tree analysis alongside an asynchronous Session Audit Log backed by the local SQLite vault.

## Context & Problem

In production database administration, unexpected transaction lock cascades, runaway slow queries, and unmonitored sessions can cause database outages. Previous versions of SqlFox supported query cancellation for the active client query only (`CancelQuery`), but provided no visibility into:
1. Other active server threads, client host origins, or thread runtimes.
2. In-flight transaction lock contention, blocked victim queries, or the root blocker transaction (`sys.innodb_lock_waits`).
3. Historical chronological ledgers of executed statements and their performance metrics across session lifetimes.

## Considered Options

- **WebSocket Push & Live Canvas Graphs**: Pushing thread snapshots continuously over persistent WebSockets and rendering interactive node-edge diagrams with D3.js or HTML5 Canvas. While visually elaborate, this introduces heavy graph rendering overhead and violates the stdlib HTTP transport model.
- **Client-Only History (In-Memory Ring Buffer)**: Retaining query history solely in browser memory. Zero disk overhead, but lost entirely whenever the browser tab is reloaded or closed.
- **Two-Tier Fallback Processlist with Hybrid Vault Persistence (Chosen)**:
  - **Processlist Manager**: Dedicated tab `[⚡ Processlist]` with client-side polling (`Pause / 1s / 3s / 5s`), slow query threshold highlighting (amber `>5s`, red `>30s`), and two-tier lock inspection: queries `sys.innodb_lock_waits` with calculation of `is_root_blocker`, falling back to `SHOW FULL PROCESSLIST` state analysis on restricted accounts.
  - **Safety Gate**: Out-of-band `Kill Query` and `Kill Connection` requiring native confirmation (`showNativeConfirm`), protecting the user's active connection (`[You]`) and system threads (`system user`, `event_scheduler`) from accidental termination.
  - **Session Audit Log**: Asynchronous persistence via `internal/vault` to SQLite `audit_logs` table upon completion of the Safe Execution Pipeline, exposed through an expandable bottom drawer with search, CSV export, and clear actions.

## Decision

1. **Driver Interface Extension**: Extended `driver.Session` with `GetProcesslist(ctx)` returning `ProcessItem` and `LockWait` arrays, and `KillProcess(ctx, threadID, killConnection)`.
2. **Safe Execution Pipeline Hook**: Hooked `ActiveSession.Execute` to record non-blocking audit entries (`SessionID`, `ConnectionID`, `QuerySQL`, `DurationMs`, `RowsAffected`, `Error`, `ExecutedAt`).
3. **Engine-Specific Visibility**: Processlist tab and action triggers are dynamically rendered for MySQL/MariaDB engines and hidden for single-file embedded engines (SQLite).
4. **Interactive Root Blocker UX**: When lock waits are detected, a prominent `🚨 ROOT BLOCKER` banner isolates the culprit holding locks and provides 1-click `Kill Blocker` remediation.
5. **DBA Best Practice Filtering & State Persistence**: Toggleable `Hide Sleep` (connection pool idle threads) and `Hide System` (event scheduler, binlog dump) filters persisted in browser `localStorage`.
6. **Thread Query Diagnostic Seams**:
   - `⚡ Explain`: Direct pre-flight `EXPLAIN` execution plan evaluation via visual Impact Preview modal.
   - `📝 To Tab`: Opens target SQL in a clean Query Editor tab for isolated debugging.
   - `🤖 Copilot Draft Mode`: Injects SQL with optimization prompt template into AI prompt input without auto-execution, guarded by confirmation dialog if existing user draft text is dirty.
   - `📑 Catatan (Internal Notes)`: Appends statement into `Notes.md` scratchpad with timestamps for persistent runbook documentation.
   - `📋 Copy`: Instant SQL copy to clipboard.

## Consequences

- Full observability into active threads and lock wait graphs without third-party agent dependencies.
- Zero latency impact on query execution due to async vault recording.
- Full auditability surviving browser restarts with Excel-safe CSV export capability.
- Frictionless transition from thread diagnosis to AI optimization, query tuning, and documentation without copy-paste overhead.
