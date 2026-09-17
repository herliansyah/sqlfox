# 0010: Stateless Schema Introspection and Query Session Database Isolation

To prevent background metadata prefetching from corrupting the active database context for queries, explanations, and safety checks, schema introspection is decoupled from session database mutation, and query execution requests explicitly preserve target database context.

## Problem

When the frontend dynamically prefetched cross-database catalogs for autocomplete and Schema Search (`/api/session/{id}/schema?db={remote_db}`), the backend previously called `SelectDatabase(ctx, remote_db)`. This silently switched the shared database connection on the server behind the user's back. Consequently, subsequent `EXPLAIN` and `EXECUTE` commands executed against the wrong database (e.g. attempting to resolve `db_b.users` instead of `db_a.users`), resulting in missing table errors.

## Considered Options

- **One Connection Pool per Database**: Expensive in memory and socket resources, especially for servers with dozens of tenant databases.
- **Stateless Introspection + Target Database Propagation (Chosen)**:
  - Introspection (`InspectSchema`) queries `information_schema` directly with `WHERE TABLE_SCHEMA = ?` without issuing `USE \`dbname\`` or mutating `sess.Session`'s active database.
  - Active database switching is exclusively managed through `SelectDatabase` during explicit database selection.
  - `handleExplainQuery` and `handleExecuteQuery` accept an explicit `database` parameter from the client, ensuring the session selects the intended schema before executing any statement.
  - A structured GUI modal (`#database-creator-dialog`) provides visual `CREATE DATABASE` functionality with standard charset and collation defaults.

## Consequences

- Zero side-effect background prefetching: Cross-Database Catalog and Schema Search run completely isolated from active editor queries.
- `EXPLAIN` and execution plans reliably target the active table's database schema.
- Creating a database is accessible via the sidebar and Command Palette with automated DDL review.
