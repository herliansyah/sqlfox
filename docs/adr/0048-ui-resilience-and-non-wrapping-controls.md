# 0048: UI Resilience and Non-Wrapping Controls

## Status
Accepted

## Context
In dense database administration workspaces (such as the Query Editor toolbar, Top Navbar, Table mode tabs, Data Grid toolbar, and Status Bar), viewport resizing or constrained browser window widths previously caused flexbox containers to compress child controls (`flex-shrink: 1`).
This created three primary visual bugs:
1. Buttons deformed into narrow, tall, or distorted pill shapes.
2. Button and badge labels automatically broke into multiple lines (*word-wrapping*), causing text collisions with icons and vertical height bloating.
3. Adjacent buttons and status controls overlapped or were clipped inappropriately.

## Decisions

### 1. Global Non-Wrapping and Anti-Distortion Rules for Controls
- Every button across the application (`button:not(.allow-wrap)`) strictly enforces `white-space: nowrap !important;` and `flex-shrink: 0;`.
- Controls, badge pills, and tab headers (`.badge-pill`, `.control-label`, `.tab-label`, `.toolbar-item`) are permanently non-wrapping and protected from shrinking out of proportion.
- Long database, table, or column names within headers or status bars continue using bounded truncation (`truncate` with ellipsis) alongside native `title="..."` tooltips to preserve adjacent controls.

### 2. Smooth Horizontal Overflow for Toolbars
- High-density toolbars (`header`, `editor-toolbar`, `table-mode-tabs`, `grid-toolbar`, `footer`, `processlist-toolbar`, `pl-trend-panel` header, Statement Digest, and Deadlock History toolbars) are configured with `overflow-x-auto`, `gap-3`, and `shrink-0` groups.
- When the application window is narrower than the cumulative width of toolbar buttons and filter toggles, the container gracefully scrolls horizontally without collapsing buttons or warping layout hierarchy.

### 3. Clear Separation Between Controls and Content Text
- Text content surfaces (AI Copilot chat answers, Operational Notes bodies, error modal stack traces, and SQL execution diagnostics) retain word wrapping (`break-words`, `whitespace-pre-wrap`) to maintain maximum readability without horizontal clipping.

### 4. Modal Dialog and Floating Menu Viewport Resilience
- High-density modal dialogs (`structure-sync-dialog`, `data-sync-dialog`, `data-transfer-dialog`, `table-creator-dialog`, `smart-seed-dialog`, `run-sql-dialog`, `ddl-review-dialog`) enforce `max-h-[90vh]` alongside `overflow-y-auto`, ensuring action footers (Execute, Apply, Transfer, Create) remain accessible on smaller laptop screens or zoomed displays.
- Floating context menus (`#grid-context-menu`) enforce `max-h-[85vh]` and `overflow-y-auto` with bounded coordinate clamping (`Math.max(10, ...)`) to prevent long option lists (such as table columns with multiple ENUM values) from overflowing the screen boundary.
- Context menu lifecycle enforces mutual exclusion via `closeAllContextMenus()`, preventing ghost menus from lingering when invoking adjacent surface menus.

## Consequences
- Buttons and action controls consistently retain their intended ergonomic geometry across all screen resolutions and zoom levels.
- Zero text wrapping or overlapping within UI toolbars and navigation chrome.
- Zero modal action clipping or unclickable buttons on small or zoomed viewports.
- Zero third-party CSS or JS dependencies; enforced via native CSS stylesheet rules and Tailwind utility containment.
