<div align="center">

  <a href="https://github.com/herliansyah/sqlfox">
    <img src="docs/assets/logo.svg" alt="SqlFox Logo" width="120" height="120" />
  </a>

  <h1>SqlFox</h1>

  <p><b>Lightweight Database Manager &amp; Headless CLI for PostgreSQL, MySQL, MariaDB, and SQLite</b></p>

[![GitHub Release](https://img.shields.io/github/v/release/herliansyah/sqlfox?style=flat-square&color=FF6B35&logo=github)](https://github.com/herliansyah/sqlfox/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/herliansyah/sqlfox?style=flat-square&color=00ADD8&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-F59E0B?style=flat-square)](https://opensource.org/licenses/MIT)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-4B5563?style=flat-square&logo=linux)](https://github.com/herliansyah/sqlfox/releases)
[![Arch](https://img.shields.io/badge/arch-amd64%20%7C%20arm64-6366F1?style=flat-square)](https://github.com/herliansyah/sqlfox/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/herliansyah/sqlfox/ci.yml?branch=master&label=CI&style=flat-square&logo=githubactions)](https://github.com/herliansyah/sqlfox/actions)

<br />

[ 🇬🇧 English ](README.md) &nbsp;•&nbsp; [ 🇮🇩 Bahasa Indonesia ](README_ID.md) &nbsp;•&nbsp; [ User Guide (EN) ](docs/USER_GUIDE.md) &nbsp;•&nbsp; [ Panduan Pengguna (ID) ](docs/USER_GUIDE_ID.md)

</div>

---

**SqlFox** is a fast, standalone database client and automation runner distributed as a single static binary. It embeds a responsive local web workspace and includes a headless CLI designed for development, site reliability, and database operations.

SqlFox operates with zero runtime dependencies (no Electron, no JVM, zero CGO), maintains a strict memory budget (**< 100MB RAM**), launches in under one second, and enforces deterministic safety guardrails before destructive operations can touch production databases.

> Detailed operational guides and architectural specifications are available in the **[User Guide & DBA Playbook](docs/USER_GUIDE.md)** and **[Architecture Documentation](ARCHITECTURE.md)**.

### Core Capabilities

#### Architecture & Performance
- **Standalone Binary**: Zero installation dependencies. Compiles to static executables via pure-Go drivers (`CGO_ENABLED=0`) across Linux, macOS, and Windows.
- **Strict Memory Ceiling**: Operates within a `< 100MB RAM` budget under enterprise workloads (1,000+ tables, 50,000+ columns).
- **Virtual Data Grid**: Fixed-height DOM row windowing (32px uniform height) maintaining `< 500` active DOM nodes across result sets of 100,000+ records, eliminating browser tab freezes.
- **Low-Latency Streaming**: Query results stream over NDJSON (`/api/session/{id}/execute-stream`) with an immediate 20-row initial flush for Time to First Row (TTFR) under 50ms, cancellable out-of-band via browser `AbortController`.
- **Bottom Pagination Bar**: Unified sticky navigation bar supporting direct page jumps, configurable page sizes (25, 50, 100, 200), on-demand accurate count probes, and automatic `%...%` wildcard formatting for filter operators.

#### Safety Guardrails & Governance
- **Deterministic AST Inspection**: Evaluates SQL queries against target database catalogs before execution, detecting unqualified `UPDATE`/`DELETE`, `DROP`, and `TRUNCATE` operations.
- **Pre-Execution Mutation Estimation**: Rewrites `UPDATE` and `DELETE` statements into bounded `SELECT COUNT(*)` probes (500ms timeout) to inspect affected row counts prior to user confirmation.
- **Tiered Affirmation Pipeline**: Adaptive execution gating offering silent execution for single-row primary-key mutations in Development, a 1-click affirmation modal for 1–100 rows, and mandatory typed Challenge Modals for large mutations, unqualified queries, and all operations in Staging/Production.
- **Comment Unwrapping**: Unwraps MySQL executable comments (`/*! ... */`) to prevent syntax-cloaked destructive query evasion.
- **Pre-DDL Tiered Backups**: Automatically snapshots schema DDL and table rows (<50MB) to local disk before executing structural changes.

#### DBA Operations & Incident Triage
- **Real-Time Processlist**: Live server thread inspection with auto-refresh (1s, 2s, 5s), state filtering, and out-of-band termination (`KILL QUERY`, `KILL CONNECTION`).
- **Batch Termination Guardrails**: Terminate runaway queries or idle connections in bulk with fail-closed checks and immune shields for system workers and caller sessions.
- **Root Blocker Triage**: Isolates blocking transactions in lock storms using `sys.innodb_lock_waits` and `pg_locks`, showing full scrollable SQL and dedicated kill triggers.
- **Heuristic Index Advisor**: Rule-based index recommendations based on the ESR rule (Equality, Sort, Range) with index bloat warnings (>5 secondary indexes) and non-blocking DDL syntax (`ALGORITHM=INPLACE` on MySQL, `CONCURRENTLY` on PostgreSQL).
- **Performance Flight Recorder**: On-demand 1-second telemetry recorder capturing QPS, active thread counts, row lock waits, and buffer pool waits, correlated directly with concurrent query text.
- **Replication Diagnostics**: Dual-syntax telemetry for modern MySQL 8.4 LTS / 9.x (`SHOW REPLICA STATUS`), legacy MySQL 5.7 / 8.0, MariaDB Galera Flow Control pauses, and PostgreSQL standby sync states (`pg_stat_replication`).

#### Schema & Data Synchronization
- **Structure Sync**: AST-based DDL comparison across databases and engines, generating color-coded diffs and safe, ordered migration scripts.
- **Cross-Connection Data Transfer**: Direct, memory-bounded (<50MB buffer) data streaming between connections with transaction rollback protection.
- **Row-Level Data Sync**: Primary-key keyset reconciliation calculating row-by-row deltas (inserts, updates, deletes) with atomic apply scripts.
- **Visual Table Designer**: In-place DOM state management preserving keyboard focus, with interactive ENUM/SET value builders, composite index builders, and foreign key compatibility guards.
- **Visual ERD Canvas**: Drag-and-drop table relationship visualizer with dynamic SVG Bézier curves, auto-layout, and viewport scaling.

#### Security & Access Control
- **Connection Vault**: Local credentials and client SSL private keys (`SSLKey`) encrypted at rest with **AES-256-GCM** and **Argon2id** key derivation.
- **Strict Ingress Filtering**: Loopback-default binding (`127.0.0.1`), DNS rebinding protection, and byte-for-byte Same-Origin checks blocking cross-port CSRF.
- **Remote LAN Mode**: Controlled non-loopback access (`--lan`) guarded by a Master PIN with rate-limited brute-force lockout.
- **BYOK AI Copilot**: Cloud (OpenAI, Claude, Gemini, DeepSeek) or local (Ollama) AI integration with a zero-data-leakage guarantee—only sanitized DDL schema metadata is passed, never row data.

---

### Quick Start

#### Standalone Binary

Pre-built static binaries for Linux, macOS, and Windows are available from [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

| Operating System | Architecture | Archive Format |
| :--- | :--- | :--- |
| **Linux** | `x86_64` (amd64), `ARM64` | `.tar.gz` |
| **macOS** | Apple Silicon (`arm64`), Intel (`amd64`) | `.tar.gz` |
| **Windows** | `x86_64` (amd64), `ARM64` | `.zip` |

**Linux (amd64):**
```bash
curl -sL https://github.com/herliansyah/sqlfox/releases/download/v0.10.13/sqlfox-v0.10.13-linux-amd64.tar.gz | tar -xz
./sqlfox
```

**macOS (Apple Silicon):**
```bash
curl -sL https://github.com/herliansyah/sqlfox/releases/download/v0.10.13/sqlfox-v0.10.13-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

#### Docker Container

```bash
docker run -d \
  --name sqlfox \
  -p 8080:8080 \
  -v sqlfox_data:/data \
  --add-host=host.docker.internal:host-gateway \
  ghcr.io/herliansyah/sqlfox:latest
```

Or deploy with Docker Compose:
```bash
docker compose up -d
```

#### Go Install / Source Build

```bash
# Install via Go toolchain
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest

# Build from source
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
```

---

### Command-Line Interface

SqlFox provides both desktop daemon mode and headless automation commands:

```text
Usage:
  sqlfox [command] [flags]
  sqlfox [server-flags]    (launches desktop web UI)

Commands:
  query     Execute SQL queries or scripts with streaming output (table, json, ndjson, csv)
  transfer  Stream cross-connection data transfer between databases
  job       Run declarative database automation pipelines
  advise    Analyze query AST and generate ESR index recommendations
  help      Display command help

Server Options:
  -port int          Port to listen on (default 8080, with automatic port hunting)
  -host string       Interface address to bind (default "127.0.0.1")
  -lan               Enable remote LAN and tablet access (binds 0.0.0.0 and prints Master PIN)
  -pass string       Master password for vault encryption (or uses ~/.sqlfox/master.key)
  -db string         Path to config vault database (default: ~/.sqlfox/config.db)
  -no-browser        Disable automatic browser launch on startup
  -strict-port       Exit immediately if specified port is occupied
  -log-level string  Log verbosity: debug, info, warn, error, silent (default "info")
  -log-file string   File path for persistent log sink
  -log-format string Log output format: text, json (default "text")
  -silent, -q        Suppress terminal output while retaining file logs
```

#### CLI Examples

```bash
# Launch web workspace on port 9000 without opening browser
sqlfox -port 9000 -no-browser

# Execute headless query and output clean JSON
sqlfox query --dsn "root:secret@tcp(127.0.0.1:3306)/app" --sql "SELECT id, email FROM users" --format json

# Pipe SQL through STDIN in CI/CD pipelines
echo "SELECT count(*) FROM orders WHERE status = 'pending';" | sqlfox query --dsn "$DATABASE_URL" --format csv

# Run heuristic index advice on a slow query
sqlfox advise --dsn "$DATABASE_URL" --sql "SELECT * FROM orders WHERE user_id = 42 ORDER BY created_at DESC"
```

---

### High-Level Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│ User Browser / Tablet / LAN Client                                     │
│        ▲                                                               │
│        │ Local HTTP REST & SSE (Protected by Master PIN & Origin Guard)│
│        ▼                                                               │
│ Go Backend Process (< 100MB RAM, Single Static Binary)                 │
│   ├── Safety Guard: Deterministic AST Analyzer & Row Impact Probes     │
│   ├── Session Manager: Singleflight Deduplication & Control Pools      │
│   ├── Virtual Data Grid: Fixed-Height DOM Windowing & Micro-Chunk NDJSON│
│   ├── AI Firewall: Schema-Only Context Sanitizer (BYOK)                │
│   └── Connection Vault: AES-256-GCM & Argon2id at Rest                 │
└────────┬─────────────────────────────────────────────┬─────────────────┘
         │                                             │
         │ TLS / Local Socket (MySQL, PG, SQLite)      │ HTTPS (DDL Metadata Only)
         ▼                                             ▼
[ Target Database Instances ]                 [ AI Provider API ]
(MySQL, MariaDB, PostgreSQL, SQLite)          (Claude, OpenAI, Gemini, Ollama)
```

---

### License

Distributed under the [MIT](LICENSE) open-source license.
