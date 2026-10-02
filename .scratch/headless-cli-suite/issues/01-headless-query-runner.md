# Issue 01: Headless Query Runner with Multi-Format Output and Safety Guard

Status: resolved
Type: task
Feature: headless-cli-suite

## Goal
Implement `sqlfox query` subcommand supporting `--conn` and `--dsn` targeting, multi-statement SQL parsing, stdin piping, multi-format output (`table`, `json`, `ndjson`, `csv`), and fail-closed Safety Guard protection.

## Requirements
- Subcommand router in `cmd/sqlfox/main.go` delegating `query` to `internal/cli`.
- Support `--dsn` with auto-detected driver (MySQL/SQLite) and optional `--conn` using vault profile.
- Support query via positional argument, `--file`, or `stdin`.
- Format outputs:
  - `table`: via `text/tabwriter`.
  - `json`: `[{"col": "val"}]`.
  - `ndjson`: 1 row per line.
  - `csv`: via `encoding/csv`.
- Safety Guard fail-closed with exit code `2` unless `--force` is specified.
- Optional `--tx` transaction wrapper.

## Answer
Implemented in `internal/cli/runner.go` and `internal/cli/query.go` with subcommand router in `cmd/sqlfox/main.go`. Verified with unit tests in `internal/cli/cli_test.go` (`TestCLI_QueryDirectDSN_Formats`, `TestCLI_Query_StdinPipe`, `TestCLI_Query_SafetyGuard_FailClosed`, `TestCLI_Query_VaultProfile`). Destructive queries trigger fail-closed exit code 2 unless `--force` is supplied.

