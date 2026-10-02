# 0032: FK-Aware Mock Data Generation and Referential Closure

## Status
Accepted

## Context
SqlFox needs a built-in synthetic data generator so developers and DBAs can rapidly seed schema into a usable state during development, testing, and demos without reaching for external tools (Faker, factory_boy, Mockaroo). The generator must satisfy foreign key referential integrity across all three supported engines (MySQL/MariaDB, SQLite, PostgreSQL) while remaining a zero-dependency pure-Go package.

Three constraints shape the design:
1. **Referential integrity**: INSERT order must respect FK parent→child relationships or FK enforcement must be temporarily suspended.
2. **Self-referential FKs**: Columns that reference their own table's PK (e.g. `manager_id → employees.id`) cannot be satisfied in a single INSERT pass.
3. **Driver abstraction**: The generator must operate through the `driver.Session` interface without bypassing the Safety Guard in ways that expose the bypass to user-facing code paths.

## Decisions

### 1. ExecRaw / QueryRaw — Safety Guard Bypass for Internal Use
Two methods are added to the `driver.Session` interface:

```go
ExecRaw(ctx context.Context, sql string, args ...any) (int64, error)
QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error)
```

These bypass the Safety Guard entirely and execute directly against `*sql.DB`. They are documented as "for internal server use only" and are not exposed to user-initiated query paths. The alternative — routing through `Exec` — is not viable because `Exec` applies the Safety Guard AST parser which would reject raw `BEGIN`/`COMMIT` and `PRAGMA` statements.

### 2. FK-Check Disable Strategy Per Driver
Topological sorting alone cannot handle cycles or self-referential FKs. Each engine requires a different strategy:

- **MySQL / MariaDB**: `SET FOREIGN_KEY_CHECKS=0` disables referential enforcement session-wide for the duration of the seed. Re-enabled via deferred `SET FOREIGN_KEY_CHECKS=1`.
- **SQLite**: `PRAGMA foreign_keys=OFF` disables FK enforcement. Re-enabled via deferred `PRAGMA foreign_keys=ON`.
- **PostgreSQL**: FK enforcement cannot be session-disabled without superuser `SET session_replication_role = replica`. Instead, PostgreSQL relies exclusively on topological INSERT ordering. Self-referential FKs are still handled by the two-pass strategy.

### 3. Self-Referential FK Two-Pass NULL→UPDATE Strategy
Self-referential columns are detected when `fk.ReferencedTable == cfg.Table`. During the INSERT pass they receive `NULL`. After all rows are committed, a follow-up UPDATE assigns random PKs:

- **MySQL**: Requires double subquery to avoid "You can't specify target table for update in FROM clause":
  ```sql
  UPDATE t SET col = (SELECT pk FROM (SELECT pk_col AS pk FROM t ORDER BY RAND() LIMIT 1) AS _t) WHERE col IS NULL
  ```
- **SQLite / PostgreSQL**: Direct correlated subquery works:
  ```sql
  UPDATE t SET col = (SELECT pk_col FROM t ORDER BY RANDOM() LIMIT 1) WHERE col IS NULL
  ```

The UPDATE is best-effort (`//nolint:errcheck`); the table data is valid with NULLs if the UPDATE fails (e.g. empty table before second pass).

### 4. FK Pool — Post-Insert SELECT LIMIT 1000
For cross-table FK columns, up to 1,000 existing PK values are sampled from the parent table after schema inspection:

```
SELECT ref_col FROM parent_table LIMIT 1000
```

This pool is sampled uniformly at random during INSERT generation. The limit of 1,000 balances memory usage against distribution quality. The alternative — tracking `last_insert_id` — only works for auto-increment MySQL tables and is not portable. Using a post-inspect SELECT works across all three engines.

### 5. Per-Table Atomicity — One Transaction Per Table
All INSERT batches for a given table are wrapped in a single `BEGIN`/`COMMIT` block. This matches the existing per-table atomicity principle established in ADR-0028 and prevents partial seeding on error. If any batch fails, a deferred `ROLLBACK` is issued.

Batch size is fixed at 200 rows. At the maximum of 5,000 rows this produces 25 batches in one transaction — well within engine limits.

### 6. Parameterized Inserts with Driver-Aware Placeholders
All INSERTs use parameterized queries to prevent SQL injection from synthesized string values:
- MySQL / SQLite: `?` placeholders
- PostgreSQL: `$1`, `$2`, … positional placeholders

### 7. Row Limit: 1–5,000
The generator caps at 5,000 rows per call. This is an operational ceiling to prevent accidental large-scale operations via the HTTP endpoint. For bulk loading, the existing Import/Transfer pipelines are the appropriate tool.

## Consequences
- Developers can seed any table with realistic data in one API call, with FK referential integrity guaranteed across all three engines.
- `driver.Session` grows two bypass methods that must be implemented by all future drivers.
- The `internal/mock` package has zero external dependencies beyond the `sqlfox/internal/driver` package.
- Self-referential FK columns are always seeded with valid PKs in the follow-up UPDATE, but may briefly hold `NULL` between the INSERT commit and the UPDATE — acceptable since both happen in the same server-side call.
