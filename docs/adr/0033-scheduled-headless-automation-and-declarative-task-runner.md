# 0033: Scheduled Headless Automation and Declarative Task Runner

## Status
Accepted

## Context
Following the delivery of the headless query and transfer CLI suite (ADR-0030), teams running SqlFox in production and staging environments require hands-off, scheduled database maintenance: routine SQL dumps, cross-connection transfers, recurring analytical queries, health checks, and automatic pruning of expired backup files.

Running these operations via custom ad-hoc bash scripts often leads to fragmentation, unhandled error codes, lack of connection credential reuse from the vault, and risk of disk exhaustion when backups accumulate indefinitely without retention enforcement.

At the same time, SqlFox strictly adheres to a zero-daemon desktop and single-binary runtime philosophy. Running an internal 24/7 background scheduler or cron daemon inside SqlFox would violate this architecture, increase memory footprint, and duplicate platform-native scheduling facilities already provided by Linux (cron, systemd timers), macOS (launchd), and Windows (Task Scheduler).

## Decisions

### 1. Zero-Daemon Platform Orchestration
SqlFox does not run an internal background scheduling daemon. Instead, it provides an idempotent, declarative task execution CLI command:
- `sqlfox job run --file <config.json>`
- `sqlfox job check --file <config.json>`

Scheduled execution is delegated entirely to native OS facilities (`cron`, `systemd.timer`, `launchd`, `schtasks`), ensuring zero idle resource consumption.

### 2. Standard-Library JSON Declarative Job Specification
In accordance with Ponytail guidelines (no new dependencies when stdlib suffices), job definitions use standard JSON format (`sqlfox-job.json`), augmented with standard `os.ExpandEnv` support for `${VAR}` and `$VAR` environment variable resolution. This avoids introducing third-party YAML dependencies to `go.mod` while maintaining native compatibility with CI/CD runners and jq/jsonnet toolchains.

### 3. Sequential Task List with Fail-Fast Execution
A Job Specification contains an ordered array of `tasks`. Tasks are executed sequentially:
- `health-check`: Verifies connection latency, server version, and basic read ping.
- `dump`: Streams DDL and chunked data inserts (standard dump or FK-aware smart seed) to a destination `.sql` file.
- `transfer`: Performs direct cross-connection table data streaming.
- `query`: Executes SQL statements or scripts, with optional file output and format options (`table`, `json`, `ndjson`, `csv`).

If any task encounters an unhandled error, the Task Runner halts immediately (fail-fast) with exit code `1`, preventing subsequent dependent tasks from executing against an unstable state.

### 4. Automated Dump Rotation (Retention Policy)
To prevent disk exhaustion on automated backup targets, the `dump` task supports an optional `rotation` block:
- `keep_last`: Retains only the most recent N matching files, deleting older ones.
- `max_age`: Prunes files older than a duration string (e.g. `14d`, `48h`, `7d`).
- **Safety Boundary**: Pruning is restricted to files matching the output filename's prefix pattern in the destination directory, preventing accidental deletion of unrelated files.

### 5. Pre-Flight Job Check (`sqlfox job check`)
The `job check` command performs non-destructive dry-run validation:
- Validates job JSON structure and task schemas.
- Resolves vault connection profiles or validates DSN strings.
- Tests network connectivity and read authentication to all specified targets.
- Verifies that destination output directories exist and are writable.
- Exits with `0` on validation success or `1` with clear diagnostics on failure, without executing any data mutations or schema changes.

## Consequences
- Automated routine maintenance can be scheduled reliably via standard OS cron/systemd with zero background memory footprint.
- Retains zero new external dependencies in `go.mod`.
- Fully composable with existing vault profiles (`--conn`) or ephemeral container credentials (`--dsn`).
