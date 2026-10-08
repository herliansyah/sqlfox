# 0043: Diagnostic Logger and Observability Suite

To equip SqlFox with enterprise-grade operational observability, configurable verbosity tiers, and safe persistent file logging across both interactive desktop sessions and headless automation runners, while strictly preserving the single-binary zero-dependency Ponytail standard.

## Context and Problem Statement

1. **Debugging & Silent Failures**:
   Previously, SqlFox only outputted static startup banners and fatal initialization errors directly to `os.Stdout`/`os.Stderr`. Operators diagnosing query execution timeouts, connection pooling stalls, or schema migration diffs had no mechanism to increase verbosity to inspect internal HTTP request methods, paths, response codes, latencies, or driver calls.
2. **Production Headless & Service Logging**:
   When running SqlFox in automated CI/CD runners, Docker containers, or background systemd/Windows services, operators need structured machine-readable logs (JSON) and the ability to isolate warnings or errors to a persistent log file without polluting stdout.
3. **Windows Double-Click Ergonomics**:
   Desktop operators on Windows commonly launch `sqlfox.exe` via GUI double-click without a command prompt, making CLI flag configuration inaccessible unless seamless configuration files (`sqlfox.env`) or OS shortcut arguments are natively recognized.
4. **Security & Credential Leakage at Trust Boundaries**:
   Unfiltered logging at `debug` level risks dumping Master PINs, database passwords, or vault authorization secrets into plaintext log sinks.

## Considered Options

- **Option 1: Heavy Logging Frameworks (Zap / Zerolog / Logrus)**:
  Import third-party logging engines with automatic file rotators (e.g. `lumberjack`).
  *Rejected*: Violates the Ponytail principle (unnecessary external dependencies when the Go standard library already provides `log/slog` in Go 1.21+).
- **Option 2: Status Quo (Ad-hoc fmt.Println)**:
  Rely on ad-hoc print statements.
  *Rejected*: Lacks log levels, structured JSON formatting, file sink targeting, and credential masking.
- **Option 3: Pure Go Stdlib Diagnostic Logger (`log/slog`) with Native OS Sinks (Selected)**:
  - Implement `internal/logger/logger.go` leveraging `log/slog` with zero external dependencies.
  - Support verbosity tiers: `debug` (`-v`), `info` (default), `warn`, `error`, `silent` (`-q`).
  - Dual format output: human-friendly `text` and machine-readable `json` (`-log-format`).
  - Log Sinks: console output, persistent atomic append file sink (`-log-file`), or concurrent dual-stream (Tee) with silent console override (`-silent`).
  - Automatic `sqlfox.env` / `.env` loader for Windows double-click configuration without command-line flags.
  - Deterministic Credential Masking at HTTP middleware boundaries (`[MASKED]`).

## Decision

Adopt **Option 3**.

## Consequences

- Full debug visibility into API latencies, query executions, and driver errors is available via a simple `-v` flag.
- Production environments can emit structured JSON logs directly into log collectors or persistent files.
- Windows desktop users can configure logging by dropping a simple `sqlfox.env` file next to `sqlfox.exe` or configuring shortcut properties.
- Zero credential leakage into log files.
- Preserves single-binary zero-dependency architecture and sub-second binary startup.
