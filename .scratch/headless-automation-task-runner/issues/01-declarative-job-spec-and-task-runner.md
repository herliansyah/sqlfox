# Issue 01: Declarative Job Specification and CLI Task Runner

Status: resolved
Type: task
Feature: headless-automation-task-runner

## Goal
Implement the declarative JSON job parser with environment variable expansion, the CLI subcommand dispatcher for `sqlfox job run` and `sqlfox job check`, and sequential task execution for `query`, `transfer`, `dump`, and `health-check`.

## Requirements
- Define Job Specification structs supporting `tasks` (`query`, `transfer`, `dump`, `health-check`) in `internal/cli/job.go`.
- Parse JSON with `os.ExpandEnv` to allow `$VAR` / `${VAR}` credentials.
- Integrate `job` subcommand into `cmd/sqlfox/main.go` and `internal/cli/runner.go`.
- Implement `health-check` task testing ping and latency.
- Implement `dump` task connecting to `Session` and generating streaming SQL dump or smart seed to destination file.
- Implement `transfer` task reusing `RunTransfer` logic.
- Implement `query` task reusing `RunQuery` logic with output redirection.
- Add comprehensive unit tests in `internal/cli/job_test.go`.

## Answer
Implemented in `internal/cli/job.go` and verified in `internal/cli/job_test.go`:
- Defined `JobSpec`, `JobTask`, and `JobRotation` data models with JSON serialization.
- Built `LoadJobSpec` supporting `os.ExpandEnv` for dynamic `$VAR` / `${VAR}` credentials without new dependencies.
- Added `sqlfox job` subcommand router in `cmd/sqlfox/main.go` and `internal/cli/runner.go`.
- Implemented sequential pipeline runner with fail-fast abort on task error.
- Implemented `health-check` with ping latency measurement, `dump` with standard and smart-seed streaming, `query` with format selection and output redirection, and `transfer` cross-connection data mirroring.
- Verified 100% pass across unit tests in `internal/cli/job_test.go`.
