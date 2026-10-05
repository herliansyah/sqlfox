# 0038: Grid Edit Presets, Type Constraints, and PK-Less Mutations

## Status
Accepted

## Context
Data editing within database data grids often presents challenging edge cases:
1. **Tables Without Primary Keys**: Legacy database schemas or analytics staging tables often lack explicit `PRIMARY KEY` or `UNIQUE` constraints. Traditional web GUI editors either disable row updates entirely or execute unsafe queries that risk updating multiple identical rows.
2. **Repetitive Value Entry**: Entering standard sentinel values (e.g. `NULL`, empty string `''`, `NOW()`, current date `YYYY-MM-DD`, or generated UUIDs) required manual keystrokes and was prone to typographical errors.
3. **Insecure Context Restrictions on Web Crypto**: When accessing SqlFox over local network IP addresses (`http://192.168.x.x:8080`) without TLS certificates, modern browsers restrict `window.crypto.randomUUID()` to Secure Contexts (`https:` or `localhost`), throwing `TypeError: window.crypto.randomUUID is not a function`.
4. **Unconstrained Enum & Boolean Editing**: Free-form text fields allowed typing invalid values into `ENUM` or `BOOLEAN` columns, leading to runtime SQL syntax errors on submission.

## Decisions

### 1. Row Criteria for PK-Less Mutations
When mutating rows in tables lacking a detected primary key:
- The frontend extracts the complete prior state of the row as `row_criteria: Record<string, any>`.
- The backend (`internal/server/handlers_crud.go`) constructs a deterministic parameterized WHERE predicate matching all non-null columns (`WHERE col1 = ? AND col2 = ? ... LIMIT 1`).
- For SQL engines supporting `LIMIT 1` on `UPDATE`/`DELETE` (MySQL, SQLite), the limit is strictly appended.
- Affected row counts are verified to be exactly 1; any ambiguity (0 or >1 affected rows) results in transaction rollback and error notification.

### 2. Multi-Driver Dynamic PK Introspection Fallback
To prevent false negatives where primary keys exist on the server but were omitted from cached metadata:
- A secondary live catalog inspection triggers before falling back to full-row criteria:
  - **MySQL**: Queries `SHOW KEYS FROM \`table\` WHERE Key_name = 'PRIMARY'`.
  - **PostgreSQL**: Queries `pg_index` joined with `pg_attribute` for `indisprimary = true`.
  - **SQLite**: Queries `PRAGMA table_info('table')` inspecting `pk > 0`.

### 3. Floating Presets Pill & Hotkeys
When a grid cell enters edit mode, a compact floating pill toolbar docks adjacent to the input field:
- **Presets**: `NULL` (`Alt+N`), `Empty` (`Alt+E`), `NOW` (`Alt+T`), `Date` (`Alt+D`), `UUID` (`Alt+U`).
- Visual hotkey badges guide users without cluttering the interface.
- Clicking a preset immediately populates the field and stages the edit.

### 4. Type-Aware Cell Editors
The grid detects column data types and swaps the default text input for specialized controls:
- **ENUM**: Native `<select>` dropdown populated directly from introspected enum values.
- **BOOLEAN**: Segmented toggle (`TRUE` / `FALSE` / `NULL`).
- **Foreign Keys**: Hyperlinked cells with preview popover and reference selector.

### 5. Resilient RFC4122 v4 UUID Generator
To guarantee UUID generation across all HTTP, HTTPS, and LAN environments without third-party libraries:
- Checks for `window.crypto.randomUUID()`.
- If unavailable (non-secure context), uses `window.crypto.getRandomValues(new Uint8Array(16))` with RFC4122 v4 bit masking (`0x40` on byte 6, `0x80` on byte 8).
- Fallback to standard `Math.random` if `crypto.getRandomValues` is absent.

## Consequences
- Enables seamless editing on any table structure, including legacy PK-less schemas.
- Drastically speeds up test data creation and repetitive manual corrections.
- Zero runtime crashes or security exceptions on mobile/tablet LAN connections.
- Adheres to Ponytail principles: pure browser APIs, zero external npm dependencies, and strict backend safety checks.
