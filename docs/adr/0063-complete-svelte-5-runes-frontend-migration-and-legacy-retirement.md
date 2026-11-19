# 0063: Complete Svelte 5 Runes Frontend Migration and Legacy Pipeline Retirement

## Status
Accepted

## Context
In ADR 0061 and ADR 0062, SqlFox took initial steps to address extreme monolithic technical debt by modularizing the embedded SPA into 15 domain JavaScript files under `web/src/js/` and upgrading CodeMirror 5 to CodeMirror 6 via a standalone esbuild pipeline.

However, several systemic limitations remained:
1. **Procedural Global State Debt**: The 15 JavaScript modules communicated via global variables on `window` (`window.tabs`, `window.currentResults`, `window.stagedEdits`, etc.) and relied on error-prone manual DOM manipulations (`document.getElementById`, `innerHTML`, template literals).
2. **Duplicated & Disjoint Build Pipelines**: Generating the embedded bundle required compiling static Tailwind CSS, running `esbuild` for CodeMirror 6, concatenating 15 modules, validating via `vm.Script`, and injecting strings into HTML skeletons.
3. **Complex Feature Maintenance**: With 149 features implemented across query execution, virtual data grid, DBA telemetry, ERD canvas, and AI Copilot, maintaining reactivity and UI synchronization manually became unsustainable.
4. **Desktop & Multi-Tab Viewport Clipping**: On smaller desktop screens or when managing numerous worksheets, horizontal tab bars overflowed and clipped without a responsive containment mechanism.

Svelte 5 introduced **Runes** (`$state`, `$derived`, `$effect`), providing fine-grained reactivity, zero runtime virtual DOM overhead, and full TypeScript support while compiling to an ultra-lean single bundle. This aligned perfectly with SqlFox's core design invariant: single-binary distribution with a strict < 100MB RAM ceiling.

## Decisions

### 1. Complete Port to Svelte 5 Runes & TypeScript (`web/frontend/`)
- Ported all 149 features across all 15 functional domains into a structured Svelte 5 application under `web/frontend/`:
  - `src/lib/components/`: Domain-specific components for `activity/`, `ai/`, `common/`, `dba/`, `editor/`, `erd/`, `grid/`, `layout/`, `modals/`, `notes/`, and `table/`.
  - `src/lib/stores/appState.svelte.ts`: Centralized reactive state store leveraging Svelte 5 runes (`$state`, `$derived`, `$effect`) for connection profiles, sessions, catalogs, tabs, grid mutations, operational notes, and telemetry.
  - `src/lib/api/client.ts`: Strongly-typed REST client and Server-Sent Events (SSE) stream listener.
  - `src/lib/utils/`: Pure utility modules for SQL statement splitting, formatting, explain plan normalization, clipboard handling, and data type mapping.

### 2. Complete Retirement of Legacy Source Tree
- Permanently retired and deleted:
  - `web/src/js/` (all 15 legacy JavaScript modules).
  - `web/src/index.html` (legacy HTML shell).
  - `web/dist/index.legacy.html` (legacy fallback bundle).
  - `scripts/build_codemirror.js` and `scripts/codemirror6_entry.js`.
- Consolidated the build pipeline so `scripts/build_web.js` acts as a thin wrapper around `npm run build:svelte`, compiling `web/frontend/` via Vite and copying the production distribution to `web/dist/index.html` for Go binary embedding via `//go:embed dist/*`.

### 3. Native CodeMirror 6 Svelte Action (`web/frontend/src/lib/actions/codemirror.ts`)
- Replaced the external standalone esbuild vendor bundle (`web/dist/vendor/codemirror/`) with direct npm dependencies (`@codemirror/view`, `@codemirror/state`, `@codemirror/lang-sql`, `@codemirror/autocomplete`, `@codemirror/search`).
- Implemented as a reusable Svelte action `use:codemirror={options}` with reactive compartment reconfiguration when connection driver dialect changes (MySQL, PostgreSQL, SQLite).

### 4. Query Tab Bar Viewport Containment & Overflow Dropdown (`TabBar.svelte`)
- Implemented dynamic tab strip containment using `ResizeObserver`. Overflowing tabs are automatically moved into a responsive `»` dropdown menu with an overflow badge count.
- Added live search filtering and direct tab closing within the overflow popover.
- Guaranteed active tab visibility by automatically swapping selected hidden tabs into the visible strip.
- Restored right-click tab context menu actions: Rename, Duplicate, Close Others, Close Right, Close, and Close All.

### 5. Virtual Data Grid Staged vs. Auto Edit Modes & Exact Row Count (`VirtualGrid.svelte`)
- Introduced a segmented control toggle between `Staged` (draft edits accumulated in-memory for review and batch commit) and `Auto` (immediate background commit on blur) modes, persisted in `localStorage` (`sqlfox_grid_edit_mode`).
- Added an on-demand `[🔢 Calculate Exact]` button issuing driver-quoted `SELECT COUNT(*)` to obtain precise totals and pagination without stalling fast catalog loading.

### 6. System Daemon Lifecycle Controls (`Header.svelte`, `CommandPaletteModal.svelte`)
- Added native Go daemon actions to the header settings dropdown and Command Palette:
  - Open in Web Browser (`POST /api/system/open-browser`).
  - Copy App URL (with PIN credentials for mobile/tablet pairing).
  - Shut Down Daemon (`POST /api/system/shutdown`) with clean socket teardown and lock release.

### 7. Test Suite Modernization
- Updated `web/embed_test.go` and Node regression test scripts (`scripts/test-*.mjs`) to assert directly against the Svelte distribution (`web/dist/index.html`) and TypeScript sources under `web/frontend/src/`, retiring the legacy Node VM sandbox harness.

## Consequences

### Positive
- **Single Source of Truth**: All UI code lives under `web/frontend/` with end-to-end TypeScript type safety.
- **Ultra-Lean Footprint**: Eliminates ~2 MB of dead embedded assets, maintains zero external CDN dependencies, and easily adheres to the < 100MB RAM budget.
- **Superior Ergonomics**: Tab overflow containment and context menus eliminate horizontal layout breakage and improve productivity.
- **Maintainability**: Component-based structure with Svelte 5 Runes eliminates brittle DOM string manipulations and synchronization races.

### Negative / Neutral
- Build from source requires Node.js 20+ to compile the frontend before `go build`. Handled automatically in `Dockerfile`, `.github/workflows/ci.yml`, and `release.yml`.
