# 0021: Workspace Ergonomics, Shortcuts Discovery, and Search Standardization

To elevate desktop productivity and align with HeidiSQL and TablePlus usability standards, SqlFox unifies top navbar button sizing, centralizes keyboard shortcuts discovery, implements collapsible schema sections, and standardizes search/filter textboxes across all toolbars and dialogs.

## Context & Problem

As SqlFox evolved through multiple DBA diagnostics and management capabilities, several UI ergonomics issues emerged:
1. **Navbar Clutter & Button Discrepancies**: Top navbar buttons had mismatched heights and contained verbose inline `<kbd>` shortcut tags (e.g. `Ctrl+P`, `Ctrl+K`), producing visual noise and awkward wrapping on smaller displays.
2. **Shortcuts Fragmentation**: Keyboard shortcuts were scattered across disparate button labels rather than accessible from a single centralized cheat sheet.
3. **Uncollapsible Schema Explorer**: Database objects (`Tables`, `Views`, `Triggers`) occupied a flat scrollable tree, creating excessive navigation overhead for schemas with dozens or hundreds of tables.
4. **Sidebar Table Item Visual Noise**: Sidebar table rows contained floating hover icons (`📄 View DDL`, `🗑️ Drop Table`), creating visual distraction and posing the risk of accidental destructive drops during tree navigation.
5. **Session Header Inconsistencies**: The session switcher header had a non-functional duplicate chevron icon, and the Read-Only toggle button contained redundant ` (Locked)` text.
6. **Ephemeral Sidebar Visibility**: Toggling the Left Sidebar or AI Copilot sidebar was not remembered across page refreshes.
7. **Cramped, Inconsistent Search Textboxes**: Search inputs across Processlist, Session Audit Log, Table Data Grid, and Storage Diagnostics were constrained to arbitrary narrow widths (`w-36`, `w-44`, `w-56`), lacked leading search indicators, lacked 1-click clear buttons, and did not consistently respond to the `Escape` key.

## Decision

1. **Standardize Navbar Button Height & Tooltips**:
   - Standardize all top navbar controls (sidebar toggles, palette, schema search, processlist, audit log, diagnostics, privileges, vault, copilot, read-only toggle, PIN badge) to a uniform `h-7` (28px) height.
   - Remove all inline `<kbd>` tags from button labels, moving shortcut hints cleanly to native `title="..."` tooltips.
   - Clean up session switcher header (remove duplicate chevron) and simplify Read-Only button text to `🔒 Read-Only`.

2. **Centralized Keyboard Shortcuts Cheat Sheet**:
   - Implement a dedicated modal dialog (`shortcuts-cheatsheet-modal`) grouping all hotkeys by category (Query Execution, Navigation, Tabs, Tools).
   - Wire discovery to `?`, `F1`, statusbar `⌨️ Shortcuts` footer button, and Command Palette (`Ctrl+K`).

3. **Collapsible Schema Tree Sections with Persistence**:
   - Group sidebar schema items into independent collapsible sections: `Tables`, `Views`, and `Triggers`.
   - Each section header features a toggle chevron, section icon, item count badge, and persists collapsed/expanded state to `localStorage` (`sqlfox_schema_collapse_tables`, `views`, `triggers`).

4. **Clean Sidebar Rows & Relocate Destructive Actions**:
   - Remove floating hover icons from schema table rows; table row counts are flushed to the right margin with `tabular-nums` alignment.
   - Relocate `Drop Table` to the Table Viewer sub-tabs bar right edge and the right-click context menu (`table-context-menu`), protected by the AST Safety Guard Challenge.

5. **Sidebar Visibility Persistence**:
   - Persist Left Sidebar and AI Copilot visibility to `localStorage` (`sqlfox_left_sidebar_visible` and `sqlfox_copilot_visible`, defaulting to `true`).
   - Remove redundant close button `✕` from the AI Copilot header in favor of navbar and shortcut toggles.

6. **Unified Maximized Search & Filter Textbox Standards**:
   - Apply a standardized UX pattern across all search and filter inputs:
     - **Dynamic Horizontal Expansion**: Flexibly fill available toolbar space (`flex-1 min-w-[200px]` or relative flex wrapper) rather than fixed narrow widths.
     - **Leading Search Icon**: Consistent `🔍` icon on the left with appropriate padding (`pl-7` or `pl-8`).
     - **Dynamic Clear Button**: Inline `✕` button on the right that appears dynamically when text is present and hides when cleared.
     - **Native Escape Reset**: Pressing `Escape` inside the input instantly clears the query and resets focus.
   - Applied to: Processlist (`#pl-search-input`), Session Audit Log (`#audit-search-input`), Table Data Grid Raw WHERE filter (`#filter-raw-input`), Storage Diagnostics (`#dbdiag-search-input`), Session Switcher (`#session-search-input`), Database Switcher (`#database-search-input`), and Snippets Manager (`#snippets-filter-input`).

## Consequences

- Clean, uncluttered, professional desktop interface consistent with modern tools like TablePlus and HeidiSQL.
- Significantly faster navigation in large databases through collapsible sections.
- Predictable and unified search/filter behavior across every modal and toolbar in the application.
- Zero external libraries or frontend build step required; all improvements implemented using native vanilla DOM and Tailwind utility classes.
