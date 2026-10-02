# Issue 02: Dump Rotation and Pre-Flight Job Check

Status: resolved
Type: task
Feature: headless-automation-task-runner

## Goal
Implement automated dump rotation (`keep_last` and `max_age`) with prefix pattern matching, and non-destructive pre-flight job check (`sqlfox job check --file <path>`).

## Requirements
- Support timestamp token interpolation in dump output path: `{DATE}`, `{DATETIME}`, `{TIMESTAMP}`.
- Prune matching backup files based on `keep_last` count and `max_age` duration without touching unrelated files.
- Implement `sqlfox job check --file <path>` verifying syntax, target connectivity, and output directory writability.
- Add unit tests validating rotation algorithms and check command behavior.

## Answer
Implemented in `internal/cli/job.go` and verified in `internal/cli/job_test.go`:
- Built `InterpolatePathTokens` supporting dynamic `{DATE}`, `{DATETIME}`, and `{TIMESTAMP}` tokens.
- Implemented `ApplyDumpRotation` sorting candidate backups by mod time and enforcing `keep_last` and `max_age` pruning.
- Isolated deletion strictly to matching filename prefixes to protect unrelated files in target backup directories.
- Implemented `RunJobCheck` (`sqlfox job check`) validating spec syntax, target connections, and destination folder write permissions.
- Verified in `TestApplyDumpRotation` and `TestJob_RunAndCheckEndToEnd`.
