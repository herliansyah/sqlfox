# 0052: Processlist Batch Operations & Safety Guardrails

## Status
Accepted

## Context
SqlFox equipped DBAs with a real-time Processlist Manager (ADR 0017) to inspect active threads, detect slow queries, and mitigate lock contention. However, during high-concurrency incidents—such as connection spikes, runaway batch jobs, or cascading transaction locks—operators needed to terminate or triage multiple rogue threads simultaneously.

Previously, killing threads was limited to one-by-one execution with individual confirmation prompts. When dealing with dozens of runaway threads, this created operational friction and delay. Furthermore, operators needed safe operational bulk utilities (such as copying thread IDs, exporting snapshots, or saving queries to Notes) and ironclad guardrails to prevent accidental self-termination or terminating critical system/replication threads, especially on production environments.

## Decisions

### 1. Quick Filter Ergonomics
We augment the Processlist Manager toolbar with single-click filter chips alongside the existing text search and slow query threshold slider:
- `All`: Default view displaying all threads.
- `Active`: Threads actively executing commands (`command != 'Sleep'`).
- `Locked`: Threads actively engaged in lock contention (root blockers and waiting threads).
- `Slow`: Threads with elapsed execution time $\ge 5$ seconds.
- `Sleep`: Idle client connections sleeping $\ge 60$ seconds.

### 2. Multi-Select Model & Protected Process Threads (Immunity Guardrails)
- **Table Checkbox Column**: Adds a multi-select checkbox column to the thread table with a `<th/>` select-all checkbox targeting all visible, non-immune threads.
- **Immune Thread Enforcement**:
  - Own connection (`isSelf` / `currentThreadID`) and system/replication threads (`isSystem`: `system user`, `event_scheduler`, `Binlog Dump`) are strictly immune: checkboxes are disabled with explanatory tooltips, and backend validation unconditionally rejects termination attempts targeting these IDs.
- **Polling Resilience**: Selected thread IDs are retained in a client-side `Set<number>` across real-time polling ticks (1s/3s/5s). Disappeared threads are automatically pruned from the selection.

### 3. Ephemeral Floating Dock (`Processlist Batch Action Bar`)
When one or more threads are selected, an ephemeral floating dock appears anchored at the bottom-center of the screen (`fixed bottom-6 z-30`):
- Selection counter badge (e.g. `3 Threads Selected`).
- Destructive commands: `⚡ Kill Query` (`KILL QUERY <id>`) and `💀 Kill Connection` (`KILL <id>` / `pg_terminate_backend`).
- Non-destructive operational utilities: `📋 Copy IDs` (CSV to clipboard), `💾 Export (JSON/CSV)`, and `📝 Save to Notes` (adds incident notes with SQL queries to Notes Inbox).
- Clear button (`✕ Deselect`).

### 4. Batch Termination Guardrail Modal & Safety Challenge
Before executing batch terminations:
- Polling auto-refresh is temporarily paused.
- A dedicated modal displays the targeted threads with IDs, users, databases, durations, and query previews.
- **Safety Challenge**: If the session environment is `Production` OR the batch targets $> 5$ threads, the execution button is locked until the operator explicitly types the word `KILL` into a confirmation field.
- **Emergency Operational Bypass**: Batch terminations are permitted on Read-Only sessions when the safety challenge is verified, ensuring DBAs can mitigate server outages while recording a high-priority entry in the Session Audit Log.

### 5. Dedicated Backend Batch Termination Endpoint
We introduce `POST /api/session/{id}/batch-kill`:
- Payload: `{ "thread_ids": [10, 12, 15], "mode": "query" | "connection" }`.
- Validates against immune thread IDs (`currentThreadID` and system accounts).
- Executes terminations with best-effort iteration per ID, ensuring that already-completed threads do not abort subsequent terminations.
- Returns granular execution results: `{ "total": 3, "success_count": 2, "failed_count": 1, "results": [...] }`.
- Records a consolidated audit entry in the local SQLite vault.

## Consequences
- DBAs can immediately triage and batch-kill cascading runaway queries during database emergencies in seconds.
- Accidental self-termination or disruption of database replication is structurally impossible at both UI and server API layers.
- Operational safety challenge prevents reflexive keystroke confirmation on production workloads.
