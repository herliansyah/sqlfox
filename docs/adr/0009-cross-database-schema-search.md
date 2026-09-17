# 0009: In-Memory Cross-Database Schema Search and Quick Navigation

To enable developers to instantly locate tables, views, and columns across multi-database environments without hunting through sidebar trees, SqlFox adopts an in-memory client-side Schema Search modal (`Ctrl+P` / `Cmd+P`) integrated with the existing `multiDBCatalogCache`.

## Considered Options

- **Backend Query on Every Keystroke (`/api/session/{id}/search?q=...`)**:
  - Requires querying `information_schema.COLUMNS` or `sqlite_master` on each debounce.
  - Adds network latency, database locks, and degrades UX on remote/SSH connections.
- **Dedicated External Search Index (e.g. Bleve/SQLite FTS)**:
  - Overkill and introduces unnecessary memory overhead for transient schema metadata.
- **Client-Side In-Memory Catalog Traversal (`multiDBCatalogCache`) (Chosen)**:
  - As schemas are already lazily cached in memory by the cross-database autocomplete engine, searching across all databases, tables, views, and columns takes <1ms locally.
  - Zero roundtrip latency, instantaneous fuzzy/substring filtering as the user types.

## Dual Action Model

When an item is selected from the search results:
1. **Enter / Primary Click**: Navigates immediately to that object (switches active database if needed and opens Table Viewer in Structure/Data mode).
2. **Shift+Enter / Insert Button**: Pastes the fully qualified identifier (`database.table.column`) directly at the caret position in the active Query Editor.

## Consequences

- Instant keyboard-driven navigation (`Ctrl+P`) aligned with developer muscle memory from modern IDEs (VS Code Quick Open).
- Zero server load during schema searches.
- Direct synergy with the multi-database catalog caching layer.
