# 53. Processlist Two-Tier Header Layout Architecture

Date: 2026-10-19

## Status
Accepted

## Context
The Processlist Manager previously squeezed 9 distinct groups of interactive controls into a single, horizontally crowded header toolbar (`h-10 overflow-x-auto`):
1. Trio Segmented Switcher (`Active Threads`, `Statement Digest`, `Deadlock History`)
2. Auto-refresh interval selectors (`Pause`, `1s`, `3s`, `5s`) + countdown bar
3. Immediate manual refresh button (`Refresh Now`)
4. Quick filter chips (`All`, `Active`, `Locked`, `Slow`, `Sleep`)
5. Slow query execution threshold input (`Slow > N sec`)
6. Multifield textual search input (`Search user, host, db, state, or SQL...`)
7. Visibility checkboxes (`Hide Sleep`, `Hide System`)
8. Thread count and slow query summary metric badge (`Threads: N`)
9. Root blocker lock contention alert button and activity trend toggle button

This created excessive horizontal clutter ("melebar banget"), forced horizontal scrolling even on standard desktop displays (1366px–1920px), and exposed thread-specific search and filter controls while viewing unrelated sub-views like `Statement Digest` and `Deadlock History`.

## Decision
We refactored the Processlist Manager header into a clean, hierarchical **Two-Tier Layout**:

1. **Processlist Primary Toolbar (`#pl-primary-toolbar`)** (`h-10 bg-app-surface`):
   - **Left Section**: Trio Segmented View Switcher, vertical separator, live thread counter pill (`#pl-summary-text`), and incident-triage root blocker button (`#pl-lock-summary`).
   - **Right Section**: Global telemetry operations: auto-refresh interval selector (`#pl-refresh-control-group`), manual refresh button (`#pl-refresh-now-btn`), and activity trend panel toggle (`#pl-toggle-trend-btn`).

2. **Processlist Filter Subtoolbar (`#pl-filter-subtoolbar`)** (`h-9 bg-slate-900/70`):
   - **Left Section**: Quick filter chips (`#pl-filter-chips`: `All`, `Active`, `Locked`, `Slow`, `Sleep`).
   - **Center Section**: Full flexible multifield search input (`#pl-search-input` with `flex-1 min-w-[180px]`).
   - **Right Section**: `Hide Sleep` and `Hide System` checkboxes alongside `Slow > [N] sec` threshold input.
   - **Contextual Visibility**: Automatically hidden when switching to `Statement Digest` or `Deadlock History` views via `switchProcesslistView()`, reducing cognitive load and visual noise.

## Consequences
- **Ergonomics & Readability**: Thread filtering, query searching, and telemetry actions are cleanly separated by purpose. Search input has ample horizontal room to display complex SQL query fragments.
- **Contextual Relevance**: Statement Digest and Deadlock History views are no longer cluttered with inactive thread filters.
- **Zero DOM Disruption**: All legacy element IDs and event handlers (`fetchProcesslist`, `setProcesslistFilterChip`, `onProcesslistFilterChange`, `onProcesslistSearchInput`, `renderProcesslistUI`) are completely preserved.
- **Verified Resilience**: Automated regression test `TestProcesslistTwoTierToolbarLayout` guarantees DOM hierarchy, styling tokens, and script integrity.
