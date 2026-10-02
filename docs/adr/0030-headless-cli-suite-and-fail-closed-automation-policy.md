# 0030: Headless CLI Suite Architecture and Fail-Closed Automation Policy

## Status
Accepted

## Context
SqlFox provides a desktop web UI for database management, query execution, and data synchronization. However, DevOps automation, CI/CD deployment pipelines (such as GitHub Actions or GitLab CI), and terminal power users require non-interactive, headless database operations—specifically running migration queries and streaming data migrations between environments—without starting a web server daemon or loading a browser.

Furthermore, non-interactive execution poses distinct safety challenges: in an automated script, an unconfirmed destructive statement (`DROP TABLE`, `TRUNCATE`, unqualified `DELETE`/`UPDATE`) cannot trigger an interactive challenge modal. If allowed blindly, it risks irreversible data loss; if blocked by hanging for TTY confirmation, it freezes CI/CD runners indefinitely.

## Decisions

### 1. In-Process Subcommand Dispatcher
We introduce CLI subcommands (`sqlfox query` and `sqlfox transfer`) directly within the main executable. When invoked without subcommands (or with server flags such as `-port`), SqlFox executes the existing embedded Web UI server daemon with 100% backward compatibility.

### 2. Dual Connection Targeting (`--conn` vs `--dsn`)
- `--conn <name>`: Resolves connection profiles from the local encrypted SQLite vault (`~/.sqlfox/config.db`).
- `--dsn <string>`: Accepts direct connection strings (e.g. `mysql://user:pass@host:3306/db`, `sqlite:///path/to/db`, `./file.db`, or `user:pass@tcp(host:3306)/db`). The driver is auto-detected from the URI scheme, file extension, or TCP pattern, with an optional `--driver` fallback. This provides zero-configuration usage in ephemeral CI/CD containers without needing to initialize a vault.

### 3. Stream Output & UNIX Pipeline Composability
- `stdout` is reserved exclusively for query output data (`table`, `json`, `ndjson`, `csv`).
- `stderr` is used for telemetry, transfer progress, and error logging.
- `ndjson` streams row-by-row with unbounded default limit (`--limit 0`), ensuring constant memory usage (<20MB) during massive data exports.

### 4. Fail-Closed Safety Policy
The AST Safety Guard inspects statements prior to execution. In headless mode:
- Destructive statements are rejected immediately with exit code `2` and an explanatory error message on `stderr`.
- Execution proceeds only if the caller explicitly supplies `--force` (or `--dangerously-skip-safety`).

### 5. Per-Table Atomicity for Headless Transfers
`sqlfox transfer` isolates transactions per table on the target database, avoiding monolithic transaction locks across entire databases while ensuring each individual table is either completely transferred or cleanly rolled back.

## Consequences
- Single binary now serves both as a desktop GUI app and a powerful, composable CLI tool for CI/CD.
- Zero new external dependencies (implemented purely using standard library `flag`, `encoding/json`, `encoding/csv`, `text/tabwriter`, and internal driver packages).
- Builds remain pure Go and Cgo-free (`CGO_ENABLED=0`).
