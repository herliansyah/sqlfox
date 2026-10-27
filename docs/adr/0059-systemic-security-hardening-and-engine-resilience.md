# 0059: Systemic Security Hardening and Multi-Engine Resilience

## Status
Accepted

## Context
As SqlFox expands across enterprise workloads and diverse database environments, auditing revealed critical edge cases across data storage, network ingress, SQL AST parsing, driver concurrency, and schema dump pipelines:

1. **Client SSL Private Key Storage at Rest**: While database user passwords were encrypted in the Connection Vault, client SSL private keys (`SSLKey`) were stored in plaintext in SQLite, exposing sensitive cryptographic material on disk.
2. **Cross-Port Ingress CSRF Vulnerability**: The `IngressGuardMiddleware` permitted any `Origin` header whose host matched `isHostAllowed`. Consequently, another application running on a different localhost port (e.g. a browser app running on `http://localhost:3000`) could issue authenticated state-changing requests to SqlFox on `http://localhost:8080` riding the session cookie.
3. **Safety Guard Bypass via MySQL Executable Comments**: MySQL treats conditional comments of the form `/*! ... */` and `/*!NNNNN ... */` as executable SQL statements. The SQL comment stripper (`StripLeadingComments`) discarded them as comments, creating a blind spot where destructive statements wrapped inside executable comments could bypass the AST Safety Guard without triggering confirmation challenges.
4. **Data Races on Active Database/Schema in Drivers**: During concurrent query execution, `s.cfg.DatabaseName` (MySQL) and `s.currentSchema` (PostgreSQL) were read without mutex locks while `SelectDatabase` mutated them concurrently.
5. **Silent TLS Downgrade in MySQL Driver**: When registering a custom TLS configuration failed in `Connect`, the driver silently fell back to `skip-verify`, exposing connections to potential man-in-the-middle attacks when the user explicitly configured TLS.
6. **View DDL Generation & SQL Dump Restore Order**:
   - In MySQL, `SHOW CREATE TABLE` on a SQL View returns four columns (`View`, `Create View`, `character_set_client`, `collation_connection`). Scanning only two columns triggered a runtime scan error.
   - In PostgreSQL, `GetTableDDL` on a view failed because it attempted to synthesize table columns.
   - In SQL Dumps and Smart Seed exports, views were dumped before base tables using `DROP TABLE IF EXISTS`, causing foreign dependency and relation type restore errors.
7. **PostgreSQL Composite Foreign Keys**: Schema introspection only inspected the first key index (`con.conkey[1]`), losing constituent columns in composite foreign key constraints and omitting `ON DELETE` / `ON UPDATE` actions.
8. **Unscoped Mock Data Generation FK Checks**: MySQL's `FOREIGN_KEY_CHECKS` is connection-scoped. Disabling it on the session pool before acquiring a transaction connection resulted in transactions executing on a connection where checks might still be enabled.
9. **Stateful Mutation Row Estimation Side Effect**: Pre-execution mutation estimation (`runSingleCountEstimate`) invoked `SelectDatabase(targetDB)` on the underlying session, inadvertently mutating the active database before the operator affirmed or executed the query.
10. **Fail-Open Processlist Batch Kill**: If retrieving the processlist failed during a batch kill operation, the server proceeded without knowing immune threads, risking killing internal processes or the operator's own active connection.
11. **Frontend Total Count & Datalist Route Mismatch**: The grid total count feature hardcoded backticks regardless of dialect and posted to `/api/session/{id}/query` instead of `/api/session/{id}/execute`, and foreign key datalist lookups targeted an obsolete route.

## Decisions

### 1. Vault Client SSL Key Encryption at Rest
- In `internal/vault/vault.go`, client SSL private keys are encrypted using AES-256-GCM via `Encrypt()` and prefixed with a versioned envelope identifier `enc:v1:`.
- `decryptConnection()` conditionally unpacks records with the `enc:v1:` prefix, preserving backward compatibility with existing plaintext rows while preventing double-encryption when re-saving listed connections.

### 2. Strict Same-Origin Ingress Enforcement
- In `internal/server/auth.go`, the `IngressGuardMiddleware` mandates that the `Origin` header host matches the request `Host` exactly (`strings.EqualFold(u.Host, r.Host)`).
- Cross-port origins (e.g. `localhost:3000` targeting `localhost:8080`) are rejected immediately with `403 Forbidden` (`Unauthorized Origin (CSRF protection)`).

### 3. MySQL Executable Comment Unwrapping in AST Safety Guard
- In `internal/driver/driver.go`, `StripLeadingComments` detects `/*!` prefixes and unwraps the executable SQL body (stripping any MySQL version numbers) instead of discarding the block.
- This feeds executable statements directly into `safety.Inspect`, guaranteeing that destructive queries (e.g. `/*! DROP TABLE users */`) trigger mandatory Safety Guard challenges.

### 4. Thread-Safe Active Database and Schema Scoping
- In `internal/driver/mysql/mysql.go` and `internal/driver/postgres/postgres.go`, access to the active database/schema is encapsulated in `s.activeDB()` and `s.activeSchema()`, protected by session read/write mutexes to eliminate data races during concurrent queries and database switching.

### 5. Fail-Closed TLS Configuration Registration
- In `internal/driver/mysql/mysql.go`, failure to register a custom TLS configuration returns an explicit connection error rather than silently downgrading to `skip-verify`.

### 6. View DDL Introspection and View-Last Dump Sorting
- In `internal/driver/postgres/postgres.go`, `GetTableDDL` delegates views to `s.viewDDL()`, reconstructing `CREATE VIEW` and `CREATE MATERIALIZED VIEW` definitions from `pg_get_viewdef`.
- In `internal/driver/mysql/mysql.go`, `GetTableDDL` dynamically scans variable column counts from `SHOW CREATE TABLE`.
- In `internal/server/handlers_dump.go`, tables to dump are sorted using `viewsLast()`, ensuring views are placed after base tables, and emitted with appropriate `DROP VIEW IF EXISTS` or `DROP MATERIALIZED VIEW IF EXISTS` statements.

### 7. PostgreSQL Composite Foreign Key and Referential Action Extraction
- In `internal/driver/postgres/postgres.go`, foreign key introspection employs `CROSS JOIN LATERAL unnest(con.conkey, con.confkey) WITH ORDINALITY` to extract every column pair of composite foreign keys, while mapping `confdeltype` and `confupdtype` to standard ANSI SQL actions (`CASCADE`, `SET NULL`, `SET DEFAULT`, `RESTRICT`, `NO ACTION`).

### 8. Transaction-Pinned Foreign Key Checks in Mock Data Generation
- In `internal/mock/mock.go`, MySQL's `SET FOREIGN_KEY_CHECKS=0` and its restoration (`SET FOREIGN_KEY_CHECKS=1`) are executed strictly within the pinned transaction connection (`tx.ExecRaw`), guaranteeing that bulk batch insertions bypass foreign key constraints without connection pool leakage.

### 9. Stateless Mutation Row Estimation
- In `internal/server/session_manager.go`, `runSingleCountEstimate` ceases invoking `SelectDatabase`. Qualified statements produce qualified count queries, and unqualified queries run within the request's database context, ensuring pre-flight estimation cannot mutate active session state.

### 10. Fail-Closed Processlist Batch Kill
- In `internal/server/handlers_dba.go`, if `sess.GetProcesslist(ctx)` returns an error or empty result, the batch kill handler fails closed with `503 Service Unavailable`, preventing unverified thread termination.

### 11. Dialect-Aware Quoting and Endpoint Alignment
- In `web/dist/index.html`, total count queries dynamically wrap table names in dialect-specific quotes (backticks for MySQL/MariaDB, double quotes for PostgreSQL/SQLite) and invoke `/api/session/{id}/execute`.
- Foreign key datalists query `/api/session/{id}/table/{table}/rows?db={db}&limit=100` and verify `res.ok`.

## Consequences

### Positive
- **Guaranteed At-Rest Confidentiality**: Both database passwords and client private keys are encrypted at rest with AES-256-GCM.
- **Robust Ingress Security**: Eliminates cross-port CSRF attack vectors from other local browser-facing applications.
- **Zero AST Safety Blind Spots**: Destructive statements in MySQL executable comments are caught deterministically.
- **Flawless Dump Interoperability**: Views and tables dump in dependency-compatible sequence with engine-accurate DROP commands.
- **Thread Safety**: Mutex-guarded session state prevents data races during active database switches.

### Negative & Trade-Offs
- Strict same-origin origin validation disallows multi-port web interfaces unless operators configure a reverse proxy.
- Views referencing other views maintain stable schema order; circular view definitions require manual ordering.
