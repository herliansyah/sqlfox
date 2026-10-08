# Unified Operational Notes Inbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a unified Vault-persisted Operational Notes Inbox in SqlFox with raw snapshot capture, developer side notes, database object tagging, scheduled follow-up reminder alarms, table-filtered feed views, and interactive AI Copilot `@` mentions.

**Architecture:** Consolidates fragmented notes into a single SQLite table (`operational_notes`) in `config.db`. Serves CRUD and reminder endpoints via pure Go stdlib HTTP handlers. Implements a responsive slide-over drawer UI with an overdue reminder poller, interactive toast alerts with audio chime via Web Audio API, and `@` mention autocomplete in AI Copilot.

**Tech Stack:** Go 1.21+ (standard library `database/sql`, `net/http`, `encoding/json`), SQLite (via `modernc.org/sqlite`), Vanilla HTML5/Tailwind CSS/JavaScript (embedded SPA).

**Spec:** [docs/adr/0044-unified-operational-notes-inbox-and-followup-lifecycle.md](../../adr/0044-unified-operational-notes-inbox-and-followup-lifecycle.md)

## Global Constraints

- Zero new external dependencies (pure Go stdlib, zero-daemon, pure web standards).
- Single-binary zero-dependency Ponytail standard: keep code minimal, edge-case correct, and readable.
- All notes persist in the local SQLite Vault (`config.db`).
- Preserve backward compatibility for table note lookup in AI and table viewers.

## Review Focus

1. **Empty / Nil fields**: Creating notes with empty tags, empty side notes, or nil reminder timestamps must serialize cleanly without SQLite null pointer crashes.
2. **Reminder dismissal and snooze**: Snoozing a reminder by 10 minutes must correctly bump `reminder_at` and clear overdue visual state immediately.
3. **Table tag filtering**: Sub-tab in Table Viewer must match `db://{connId}/{dbName}/{tableName}` safely, case-insensitively, handling URL encoding.
4. **AI `@` mention resolution**: If user types `[@note:My Slow Query]`, Copilot prompt assembler must inject both the raw snapshot and developer side notes without leaking sensitive credentials.
5. **Vault archive export/import**: Exporting `.sqlfox-vault` must include `operational_notes` and import them idempotently without crashing on primary key collisions.

---

### Task 1: Vault Storage & Data Model (`operational_notes`)

**Files:**
- Modify: `internal/vault/vault.go`
- Create: `internal/vault/operational_notes.go`
- Create: `internal/vault/operational_notes_test.go`
- Modify: `internal/vault/export.go`

- [x] Step 1: Write failing test in `internal/vault/operational_notes_test.go` for CRUD, reminder queries, and tag filtering.
- [x] Step 2: Add `operational_notes` table definition in `internal/vault/vault.go`.
- [x] Step 3: Implement `OperationalNote` struct and repository methods in `internal/vault/operational_notes.go`.
- [x] Step 4: Update `internal/vault/export.go` to include `OperationalNotes` in archive export/import.
- [x] Step 5: Run tests `go test -v ./internal/vault` and ensure all pass.

---

### Task 2: Backend HTTP API Endpoints

**Files:**
- Create: `internal/server/handlers_notes.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`

- [x] Step 1: Write failing test in `internal/server/server_test.go` for Notes REST endpoints (`GET /api/notes`, `POST /api/notes`, `PUT /api/notes/{id}`, `DELETE /api/notes/{id}`, `GET /api/notes/reminders/due`, `POST /api/notes/{id}/snooze`, `POST /api/notes/{id}/resolve`).
- [x] Step 2: Implement handlers in `internal/server/handlers_notes.go`.
- [x] Step 3: Register routes in `internal/server/server.go`.
- [x] Step 4: Run server tests `go test -v ./internal/server` and ensure all pass.

---

### Task 3: AI Copilot `@` Mention Context Injection

**Files:**
- Modify: `internal/ai/assistant.go`
- Modify: `internal/server/handlers_ai.go`
- Modify: `internal/ai/assistant_test.go`

- [x] Step 1: Write failing test in `internal/ai/assistant_test.go` verifying that mentioned notes (`[@note:...]` or note summaries) are injected with snapshots and side notes.
- [x] Step 2: Update `AssemblePrompts` in `internal/ai/assistant.go` to assemble referenced notes into `=== REFERENCED OPERATIONAL NOTES ===`.
- [x] Step 3: Update `internal/server/handlers_ai.go` to load referenced operational notes by IDs or tags.
- [x] Step 4: Run AI tests `go test -v ./internal/ai` and ensure all pass.

---

### Task 4: Frontend UI - Notes Inbox Drawer & Overdue Reminder Poller

**Files:**
- Modify: `web/dist/index.html`

- [x] Step 1: Add Header Navigation button `[📝 Notes]` with overdue badge count `#notes-inbox-badge`.
- [x] Step 2: Build the slide-over drawer `#notes-inbox-drawer` with search, filter tabs, note card list, and note detail/edit modal.
- [x] Step 3: Implement Web Audio chime synthesizer and 30-second reminder poller with interactive in-app toast (`Open Note`, `Snooze 10m`, `Mark Resolved`).
- [x] Step 4: Update Quick Capture handlers (`saveToInternalNotes`, Processlist action, AI Copilot save action) to create `operational_notes` directly with instant toast feedback.

---

### Task 5: Frontend UI - Table Viewer Sub-Tab Feed & AI `@` Autocomplete

**Files:**
- Modify: `web/dist/index.html`

- [x] Step 1: Update Table Viewer sub-tab `[📝 Notes & Runbook]` to render a filtered list of notes matching `db://{connId}/{dbName}/{tableName}` with pinned runbooks at top and `+ Add Note` button.
- [x] Step 2: Add interactive `@` mention autocomplete popup dropdown in AI Copilot textarea.
- [x] Step 3: Verify end-to-end flow and run full test suite `go test ./...`.
