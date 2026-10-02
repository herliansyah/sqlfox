# Issue 01: PostgreSQL Driver Implementation and Catalog Introspection

Status: resolved
Type: task
Feature: postgresql-driver

## Goal
Implement `internal/driver/postgres` using `jackc/pgx/v5/stdlib`, register it in driver registry, support schema introspection via `pg_catalog`, out-of-band cancellation via `pg_cancel_backend`, and verify with unit tests.

## Requirements
- `internal/driver/postgres/postgres.go` implementing `driver.Driver` and `driver.Session`.
- Connection string builder handling SSL mode, search path, host, port, credentials.
- Schema introspection using `pg_catalog` for schemas, tables, columns, PKs, FKs, and indexes.
- Reconstructed `CREATE TABLE` DDL generation.
- Out-of-band cancellation tracking `pg_backend_pid()` and executing `pg_cancel_backend`.
- `GetProcesslist` and `KillProcess` via `pg_stat_activity` and `pg_blocking_pids`.
- Unit tests covering driver registration, DSN formatting, and SQL dialect handling.

## Answer
Implemented in `internal/driver/postgres/postgres.go` and verified in `internal/driver/postgres/postgres_test.go` with 100% test pass rate:
- Implemented `driver.Driver` and `driver.Session` using pure-Go `github.com/jackc/pgx/v5/stdlib`.
- High-speed schema introspection querying `pg_catalog` (`pg_class`, `pg_attribute`, `pg_constraint`, `pg_index`, `pg_trigger`).
- Reconstructed `CREATE TABLE` DDL generation in `generatePostgresCreateDDL`.
- `GetProcesslist` with automatic `LockWait` extraction via `pg_blocking_pids(pid)`.
- Out-of-band cancellation via `pg_cancel_backend` and termination via `pg_terminate_backend`.
- Web UI driver selection support in `web/dist/index.html` with default port `5432`.
- Headless CLI DSN support (`postgres://` & `postgresql://`) in `internal/cli/runner.go`.
- Safety Guard fail-closed handling for Postgres dialect in `internal/safety/guard.go`.
- Formally documented in `docs/adr/0031-postgresql-native-driver-engine-and-catalog-introspection.md`.
