# 0001: Pure-Go SQLite and Standard MySQL Drivers

For cross-platform single-binary distribution without C compiler toolchains, we use `modernc.org/sqlite` and `go-sql-driver/mysql` with `CGO_ENABLED=0`.

## Considered Options

- `mattn/go-sqlite3`: CGO-based, faster bulk writes, but breaks easy cross-compilation across Linux/macOS/Windows.
- `vitess.io/vitess/go/mysql`: High performance for Vitess clusters, but brings an enormous dependency tree unnecessary for a client GUI.
- `modernc.org/sqlite` + `go-sql-driver/mysql`: Pure Go, zero C dependencies, stable streaming, and compiles seamlessly on any target architecture.

## Consequences

Build pipelines remain pure Go (`go build`), binaries run out-of-the-box on developer workstations, and query memory remains predictable using standard `database/sql` streaming.
