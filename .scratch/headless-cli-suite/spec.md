# Specification: Headless CLI Suite (Milestone 21)

## 1. Overview & Objective
Provide non-interactive, terminal-driven CLI execution of database queries (`sqlfox query`) and cross-connection data migrations (`sqlfox transfer`) for CI/CD pipelines, shell scripts, and developer terminal workflows without launching the embedded HTTP daemon or browser interface.

## 2. Core Components

### 2.1 Subcommand Dispatcher (`cmd/sqlfox/main.go` -> `internal/cli`)
- Inspects `os.Args[1]`:
  - `query`: Routes to `cli.RunQuery(args)`
  - `transfer`: Routes to `cli.RunTransfer(args)`
  - Empty or starts with `-` (e.g. `-port`, `-host`, `-db`): Runs existing Web UI HTTP server daemon (100% backward compatibility).
  - `help` / `--help` / `-h`: Displays CLI help menu listing subcommands and web server options.

### 2.2 Connection & DSN Resolver
- `--conn <name>`: Opens local encrypted connection vault (`~/.sqlfox/config.db`), decrypts credentials, and retrieves connection profile.
- `--dsn <connection_string>`: Zero-config direct connection (e.g. `mysql://user:pass@host:3306/db`, `sqlite:///path/to/db.sqlite`, `./test.db`, or `user:pass@tcp(host:3306)/db`).
  - Auto-detects driver from scheme, file extension, or `@tcp(` pattern.
  - Optional `--driver mysql|sqlite` fallback.

### 2.3 Query Execution Engine (`sqlfox query`)
- Inputs:
  - Positional argument: `sqlfox query --dsn "$URL" "SELECT * FROM users"`
  - Flag `--file <path>`: `sqlfox query --dsn "$URL" --file ./migration.sql`
  - STDIN pipe: `cat migration.sql | sqlfox query --dsn "$URL"`
- Multi-statement delimiter splitting:
  - Breaks multiple statements on `;` while respecting string literals (`'...'`, `"..."`, `` `...` ``) and comments.
- Transaction control:
  - `--tx`: Wraps script in `BEGIN ... COMMIT`, with automatic rollback on error.
- Output formatting & stream separation:
  - `--format=table`: ASCII grid for human terminal inspection (default limit 100).
  - `--format=json`: JSON array for programmatic consumption.
  - `--format=ndjson`: Newline-delimited JSON (streamed row-by-row, default limit 0 / unlimited).
  - `--format=csv`: Standard RFC 4180 CSV with header.
  - Data emitted to `stdout`. Diagnostics, elapsed time, and errors emitted to `stderr`.
- Safety Guard Policy (Fail-Closed):
  - Destructive queries (`DROP`, `TRUNCATE`, `DELETE`/`UPDATE` without WHERE) are blocked with exit code `2` unless `--force` / `--dangerously-skip-safety` is passed.
- Exit codes:
  - `0`: Success.
  - `1`: Query syntax error or connection error.
  - `2`: Blocked by Safety Guard.

### 2.4 Cross-Connection Transfer Engine (`sqlfox transfer`)
- Direct streaming transfer between `--from` and `--to` (using vault connection name or DSN).
- Scope: `--tables "tbl1,tbl2"` or `--all-tables`.
- Options: `--create-missing` (auto DDL create), `--truncate` (truncate before insert), `--batch-size` (default 100).
- Per-table atomicity: Target modifications for each table execute in an isolated transaction.
- Progress reporting: Live streaming progress emitted to `stderr`.

## 3. Constraints & Guidelines
- Zero new Go dependencies (pure standard library `flag`, `encoding/json`, `encoding/csv`, `text/tabwriter`).
- Pure Go / Cgo-free (`CGO_ENABLED=0`).
