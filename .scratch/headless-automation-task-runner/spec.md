# Specification: Scheduled Headless Automation & Task Runner (Milestone 25)

## 1. Overview & Objective
Provide automated, zero-daemon scheduled database maintenance (routine backups, SQL dumps, cross-connection transfers, and health checks) executed directly via headless CLI cron jobs.

## 2. Core Components

### 2.1 Subcommand Dispatcher Integration (`cmd/sqlfox/main.go` -> `internal/cli`)
- Adds `job` subcommand to `main.go`:
  - `sqlfox job run --file <path>`: Executes tasks sequentially with fail-fast semantics.
  - `sqlfox job check --file <path>`: Validates job spec, credentials, target connectivity, and output directory permissions without mutating data (pre-flight check).
  - `sqlfox job --help`: Shows job subcommand help.

### 2.2 Declarative Job Specification (`sqlfox-job.json`)
- JSON specification loaded via stdlib `encoding/json` + `os.ExpandEnv` for environment variable interpolation (`${VAR}` or `$VAR`).
- Top-level schema:
  - `name`: Human-readable job name.
  - `tasks`: Ordered array of task definitions.
- Task Types:
  1. `health-check`:
     - `target`: Connection name or DSN.
     - `timeout`: Optional timeout duration (e.g. `5s`).
  2. `dump`:
     - `target`: Connection name or DSN.
     - `type`: `standard` (default) or `smart-seed` (FK transitive closure).
     - `output`: Destination file path (supports timestamp tokens `{DATE}`, `{DATETIME}`, `{TIMESTAMP}`).
     - `sample_rows`: Integer sample row limit for smart-seed (default 20).
     - `rotation`: Optional retention policy:
       - `keep_last`: Integer count of newest backup files to keep.
       - `max_age`: Duration string (e.g. `14d`, `48h`, `7d`) to prune older files.
  3. `transfer`:
     - `from`: Source connection name or DSN.
     - `to`: Target connection name or DSN.
     - `tables`: Array of table names or `all`.
     - `create_missing`: Boolean flag.
     - `truncate`: Boolean flag.
  4. `query`:
     - `target`: Connection name or DSN.
     - `sql`: Inline SQL query or script.
     - `file`: Path to external SQL script file.
     - `output`: Optional output file path.
     - `format`: Output format (`table`, `json`, `ndjson`, `csv`).
     - `force`: Bypass safety guard for destructive statements.

### 2.3 Task Runner Engine (`internal/cli/job.go`)
- Executes tasks sequentially.
- If any task fails, halts execution immediately with non-zero exit code (1 for general error, 2 for safety guard).
- Diagnostics, task status, and timestamps logged to `stderr`.

### 2.4 Automated Dump Rotation Engine
- Inspects destination folder for files matching the output filename prefix pattern.
- Sorts by modification time.
- Enforces `keep_last` and `max_age` constraints by removing expired files.
- Non-matching files are never modified.

### 2.5 Pre-Flight Job Check (`sqlfox job check`)
- Verifies JSON structure.
- Resolves all targets without executing mutations.
- Confirms output directories exist and are writable.
- Exits 0 on all checks passing.

## 3. Constraints & Guidelines
- Zero new external dependencies (`encoding/json`, `os`, `time`, `path/filepath`).
- Pure Go / Cgo-free (`CGO_ENABLED=0`).
