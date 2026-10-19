# 54. Processlist Column Visibility Picker Architecture

Date: 2026-10-19

## Status
Accepted

## Context
In database administration and real-time thread monitoring, screen real estate and triage focus vary significantly by operator:
- On smaller laptop displays or mobile/tablet LAN endpoints, showing all 10 columns (`Select`, `Action`, `ID`, `User`, `Host`, `Database`, `Command`, `Time`, `State`, `SQL / Query Info`) causes table horizontal scrolling or truncates query text.
- Many operators only care about query duration, user, and the SQL statement, while others require host IP and lock state.
- Completely hiding critical identifiers could lead to accidental query or connection termination due to misidentification.

## Decision
We implemented a **Processlist Column Visibility Picker** in the Processlist Filter Subtoolbar:

1. **Trigger & Dropdown**:
   - Integrated as a compact button `👁️ Cols (X/9) ▾` on `#pl-filter-subtoolbar` next to the slow query threshold input.
   - Triggers an opaque popover `#pl-col-picker-dropdown` with click-outside auto-dismissal integrated into `closeAllContextMenus()`.
   - Includes quick-action presets: `Show All` and `Reset Default`.

2. **Column Policy & Mandatory Immunity**:
   - **Guarded Mandatory Columns**: `Select` (batch checkbox), `ID` (thread identifier, root blocker alert badge), and `SQL / Query Info` (actual executing query). These columns are locked (`mandatory: true`), disabled in the checklist with a `🔒 Mandatory` badge, and can never be hidden.
   - **Optional Toggleable Columns**: `Action`, `User`, `Host`, `Database`, `Command`, `Time (s)`, and `State` can be freely toggled show or hide.

3. **DOM Synchronization & Dynamic Colspan**:
   - Each column is marked with `data-pl-col="..."` on `<th/>` and `<td/>`.
   - Applying visibility toggles updates classes (`hidden`) synchronously without re-fetching network data.
   - Empty state `colspan` dynamically calculates the count of currently visible columns (`visibleColCount`), preventing table border collapse distortion.

4. **Persistence**:
   - Preferences are persisted globally across sessions via browser `localStorage` under key `sqlfox-processlist-hidden-cols`.

## Consequences
- **User Ergonomics**: Operators can customize their processlist view to fit their screen width and operational focus.
- **Safety Guarantee**: Mandatory identification columns remain always visible, preventing dangerous actions on wrong threads.
- **Zero Polling Disruption**: Column visibility changes preserve live selections (`plSelectedThreadIds`), batch action bar state, and auto-refresh intervals.
- **Test Integrity**: Validated with regression tests in `TestProcesslistColumnVisibilityPicker`.
