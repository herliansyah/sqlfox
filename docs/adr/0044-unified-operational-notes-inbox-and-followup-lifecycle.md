# 0044: Unified Operational Notes Inbox and Follow-up Lifecycle

Supersedes: [ADR-0014: Table Knowledge Notes and AI Context Injection](0014-table-knowledge-notes-and-ai-context-injection.md)

To replace fragmented in-memory editor scratchpads and isolated table runbooks with a unified, Vault-persisted Operational Notes Inbox featuring raw snapshot capture, developer side notes, database object tagging, scheduled follow-up alarms, and interactive AI `@` mentions.

## Context and Problem Statement

1. **Fragmented Notes UX**:
   Previously, documentation and notes in SqlFox were fractured across two disparate silos:
   - Client-only in-memory/localStorage editor scratchpad (`Notes.md`, `Internal Notes`).
   - Rigid table-locked SQLite records (`table_notes`, keyed strictly to `(connection_id, database_name, table_name)`).
2. **Absence of Incident & Follow-up Workflow**:
   Developers capturing anomalies (e.g. slow queries from the Processlist Manager, execution plan warnings, schema migration gotchas) had no structured mechanism to record the incident timestamp, attach actionable developer side notes, set target reminder dates, or mark items as resolved.
3. **Restricted AI Context Ingestion**:
   The previous AI Copilot prompt injection only matched exact table names against static table notes. Operators could not interactively reference specific operational findings or incident notes via `@` mentions in prompt conversations.

## Considered Options

- **Option 1: Status Quo (Dual Disjointed Systems)**:
  Retain `table_notes` for tables and `Notes.md` in localStorage.
  *Rejected*: Increases cognitive overhead and fails to support follow-up workflows, reminders, or cross-connection incident tracking.
- **Option 2: Cloud Sync / Multi-User Collaboration Service**:
  Build a dedicated cloud backend or sync daemon.
  *Rejected*: Violates SqlFox's single-binary, local-first, zero-daemon architecture and adds unwanted external infrastructure dependencies.
- **Option 3: Unified Local Vault Operational Notes Inbox (Selected)**:
  - Consolidate all notes into a single persistent Vault table (`operational_notes`) in `config.db`.
  - Maintain a clean boundary between the **Raw Snapshot** (`content`) and **Developer Annotations** (`side_note`).
  - Support soft-link Database Object Tagging (`object_tags` as JSON URI arrays `db://conn/db/table`) and custom user tags (`user_tags`).
  - Introduce a centralized **Notes Inbox** accessible via a header navigation button with an Overdue Reminder badge and a slide-over drawer interface.
  - Provide instant 1-click capture from AI Copilot, Processlist, and Query Editor with toast notification quick-edit.
  - Implement a lightweight in-app follow-up reminder engine with audio chime and interactive actions (`Open Note`, `Snooze 10m`, `Mark Resolved`).
  - Transform Table Viewer's sub-tab `[📝 Notes & Runbook]` into a filtered feed showing pinned runbooks and related incident notes for that specific table.
  - Enable interactive `@` autocomplete in AI Copilot input for seamless note context injection.
  - Fresh start approach: deprecate the legacy `table_notes` table cleanly.

## Decision

Adopt **Option 3**.

## Schema Specification

```sql
CREATE TABLE IF NOT EXISTS operational_notes (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,                   -- Raw Snapshot (query, AI answer, DDL, error log)
    side_note TEXT NOT NULL DEFAULT '',       -- Developer annotations & follow-up action items
    status TEXT NOT NULL DEFAULT 'inbox',     -- 'inbox', 'resolved', 'archived'
    is_pinned INTEGER NOT NULL DEFAULT 0,
    captured_at DATETIME NOT NULL,            -- Timestamp when incident/snapshot was taken
    reminder_at DATETIME,                     -- Scheduled follow-up alarm timestamp (nullable)
    object_tags TEXT NOT NULL DEFAULT '[]',   -- JSON array: ["db://conn/db/table", "@note:uuid"]
    user_tags TEXT NOT NULL DEFAULT '[]',     -- JSON array: ["#incident", "#slow-query"]
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notes_status ON operational_notes(status);
CREATE INDEX IF NOT EXISTS idx_notes_reminder ON operational_notes(reminder_at);
```

## Consequences

- All operational notes, table runbooks, and query incident logs live in a single unified Vault storage.
- Notes are portable, surviving connection reconnections, and exportable via Vault Archives (`.sqlfox-vault`) and Markdown (`.md`).
- Developers can capture slow queries from the Processlist with 1 click, set a reminder for tomorrow morning, and receive an interactive in-app follow-up alarm.
- AI Copilot prompts can ingest complete note context on-demand via interactive `@` mentions.
- The Table Viewer sub-tab gains full incident history alongside pinned runbook documentation.
