# Issue 02: Headless Cross-Connection Data Transfer and CLI Integration

Status: resolved
Type: task
Feature: headless-cli-suite

## Goal
Implement `sqlfox transfer` subcommand supporting cross-connection streaming data transfer between `--from` and `--to` endpoints, with per-table atomicity, live progress reporting on stderr, and standard UNIX exit codes.

## Requirements
- Endpoint resolution for `--from` and `--to` accepting DSN or vault connection names.
- Tables scope via `--tables "tbl1,tbl2"` or `--all-tables`.
- Execution options `--create-missing` and `--truncate`.
- Per-table atomicity on target database.
- Stream transfer progress to `stderr` with percentage/row counters.
- Integration tests in `internal/cli/cli_test.go` covering `query`, `transfer`, stdin piping, formats, and safety aborts.

## Answer
Implemented in `internal/cli/transfer.go` with chunked stream pipeline, DDL auto-creation, per-table transaction isolation, and live progress reporting on `stderr`. Verified with unit test `TestCLI_Transfer` in `internal/cli/cli_test.go`.

