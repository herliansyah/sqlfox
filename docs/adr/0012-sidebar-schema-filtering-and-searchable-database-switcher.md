# 0012: Sidebar Real-Time Schema Filtering and Searchable Database Switcher

To maintain sub-second exploration speed on databases with hundreds of tables, views, triggers, and schemas without overwhelming the DOM or requiring full tree pre-rendering, SqlFox introduces real-time client-side schema filtering and a searchable database popover switcher.

## Problem

When connecting to servers with dozens of databases or schemas with hundreds of tables:
1. Navigating to specific tables or views required manual visual scanning and scrolling through a long list.
2. The Database Selector was a static native HTML `<select>` element, lacking instant search or keyboard filtering when browsing servers with numerous databases.
3. While `Ctrl+P` provides cross-database deep search, users needed an instant, inline filter right on the sidebar to work within their active database context.

## Solution

1. **Sticky Schema Object Filter (`#schema-filter-input`)**:
   - Fixed sticky position above the schema object tree.
   - Filters tables, views, and triggers simultaneously in real-time as the user types.
   - Dynamic counter updates (e.g. `Tables (3/15)`).
   - Empty sections are collapsed/hidden; empty states display a clear shortcut with `✕` or `Clear filter`.
   - Accessible via keyboard shortcut `/` when focus is outside text editors.
2. **Searchable Database Switcher (`#database-switcher-btn` & `#database-dropdown-menu`)**:
   - Mirrors the ubiquitous interaction pattern of the connection Session Switcher.
   - Compact button displays the currently active database (`🗄️ active_db ▼`).
   - Clicking opens a popover with an autofocus filter input (`#database-search-input`) and scrollable database list.
   - Filters dozens of schemas instantly without lag; selecting a database triggers clean stateless selection and schema reloading.
   - Retains the `+ New` button for direct database creation.

## Consequences

- Instant navigation across hundreds of schema objects directly in the sidebar without page reloads or DOM lag.
- Consistent UI patterns across Session Switcher and Database Switcher.
- Fully compatible with existing automated tests through hidden compatibility elements.
