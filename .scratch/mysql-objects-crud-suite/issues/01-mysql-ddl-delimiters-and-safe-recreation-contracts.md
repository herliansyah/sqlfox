# Issue: MySQL DDL Delimiters, Definers, and Safe Object Recreation Contracts

Status: resolved
Type: research

## Question

What are the exact MySQL and MariaDB DDL syntax requirements, delimiter handling, `DEFINER` clauses, catalog queries, and safe recreation patterns (`DROP ... IF EXISTS` followed by `CREATE ...`) across MySQL 5.7, 8.0, 8.4, and MariaDB 10+ for:
1. **Views**: `CREATE OR REPLACE VIEW`, view definition extraction without host-specific `DEFINER` lockouts, and updatable view constraints.
2. **Triggers**: Safe replacement pattern (since MySQL does not support `CREATE OR REPLACE TRIGGER`), `DEFINER` sanitization, and execution timing (`BEFORE`/`AFTER` `INSERT`/`UPDATE`/`DELETE`).
3. **Stored Procedures & Functions**: Parameter extraction (`IN`, `OUT`, `INOUT`), deterministic declarations (`DETERMINISTIC`, `READS SQL DATA`), multi-statement `BEGIN ... END;` delimiter boundary requirements, and return types for functions.
4. **Scheduled Events**: `CREATE EVENT` / `ALTER EVENT` schedule syntax (`EVERY n MINUTE/HOUR/DAY` vs `AT timestamp`), `ON COMPLETION PRESERVE`, and event comment/definition extraction.

## Answer

1. **Delimiter Handling on Wire Protocol**:
   - `DELIMITER $$` is strictly a client-side command (mysql CLI / mysqldump). It is **not** valid SQL grammar on the MySQL wire protocol and causes `ERROR 1064 (42000)` if sent over `database/sql`.
   - Single-object creation endpoints (Designer modals) must send the complete assembled `CREATE ...` string as one individual statement to `sess.Session.Exec()`.
2. **DEFINER Sanitization & Host Lockouts**:
   - Explicit `DEFINER = user@host` causes `ERROR 1449` if the user is missing on the target server, or `ERROR 1227` (requires `SUPER` or `SET_USER_ID`).
   - Sanitization rule: Strip explicit `DEFINER` clauses upon extraction and generation. Omitting `DEFINER` defaults to `CURRENT_USER`, avoiding all permission lockouts.
3. **Safe Recreation Patterns Across MySQL/MariaDB**:
   - **Views**: Universal `CREATE OR REPLACE VIEW` is atomic and preserves user grants. Do **not** use `DROP VIEW IF EXISTS`.
   - **Triggers**: MySQL (5.7, 8.0, 8.4) does **not** support `CREATE OR REPLACE TRIGGER` (only MariaDB 10.1.4+ does). Requires sequential `DROP TRIGGER IF EXISTS \`name\`;\nCREATE TRIGGER \`name\` ...`.
   - **Procedures & Functions**: Neither MySQL nor MariaDB (in default sql_mode) supports `CREATE OR REPLACE PROCEDURE/FUNCTION`. Requires sequential `DROP {PROCEDURE|FUNCTION} IF EXISTS \`name\`;\nCREATE {PROCEDURE|FUNCTION} \`name\` ...`.
     - In functions, parameters are strictly input-only and must **not** include the `IN` keyword. Functions must declare `RETURNS <type>` and include `DETERMINISTIC` (or `READS SQL DATA`) when binary logging is active.
   - **Scheduled Events**: Neither engine supports `CREATE OR REPLACE EVENT`. In-place updates can use `ALTER EVENT`; full replacement requires `DROP EVENT IF EXISTS \`name\`;\nCREATE EVENT \`name\` ...`. Always specify `ON COMPLETION PRESERVE` so expired events remain in the catalog in a `DISABLED` state rather than being auto-deleted by MySQL.
4. **Safety Guard Compatibility**:
   - Sequential batches like `DROP TRIGGER IF EXISTS ...; CREATE TRIGGER ...;` have only one destructive statement (`DROP`), passing `inspectAllStatements()` multi-destructive validation. In Production, they safely trigger the typed challenge or Smart Tiered Backup; in Development, they pass with 1-click affirmation.
