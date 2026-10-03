# Issue: Workload Batch Analyzer for Session Audit Log & Headless CLI

Status: resolved
Type: task
Blocked by: 02, 04

## Question
How should the advisor process multi-query workloads from the `Session Audit Log` and provide headless CLI access via `sqlfox advise`?

Specifically:
1. How does the batch analyzer group queries by table, aggregate frequency, total execution duration, and rank which missing indexes will deliver the highest workload-wide speedup?
2. What are the CLI flags and streaming output formats for `sqlfox advise --dsn ... --sql "..."` or `--audit-log` (supporting human-readable table and JSON output)?
3. How does the CLI command observe fail-closed policies and exit codes for automated CI/CD schema verification?

## Answer
1. **Subcommand**: Implemented `sqlfox advise` in `internal/cli/advise.go` and wired into `internal/cli/runner.go`.
2. **Flexible Input Channels**: Supports `--sql "..."`, `--file <path>`, and piping via standard input (`stdin`).
3. **Connection Target Options**: Can analyze either standalone queries (`--driver <dialect>`) or against live databases (`--dsn <target>` or `--conn <profile>`), pulling table metadata and EXPLAIN plan dynamically.
4. **Structured Formats**: Supports human-readable terminal output with colored ESR sections and JSON output via `--format json`.
5. **Fail-Closed & Return Codes**: Exits with code 0 on valid analysis, code 1 on connection errors or missing inputs, suitable for CI/CD automation.
