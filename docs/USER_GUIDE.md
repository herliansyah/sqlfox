# SqlFox Complete User Guide & DBA Playbook

<div align="center">

**[ 🇬🇧 English Manual ](USER_GUIDE.md)** &nbsp;•&nbsp; **[ 🇮🇩 Panduan Bahasa Indonesia ](USER_GUIDE_ID.md)**

---

<p>The authoritative, production-grade manual for developers, data engineers, and senior database administrators running SqlFox.</p>

</div>

---

## 📑 Table of Contents

1. [Overview & Architectural Philosophy](#1-overview--architectural-philosophy)
   - [Single-Binary Zero-CGO Model](#single-binary-zero-cgo-model)
   - [Strict Memory Ceiling (< 100MB RAM)](#strict-memory-ceiling--100mb-ram)
   - [Supported Engines & Drivers](#supported-engines--drivers)
2. [Installation & Launch Modes](#2-installation--launch-modes)
   - [Standalone Binaries (Linux, macOS, Windows)](#standalone-binaries-linux-macos-windows)
   - [Install via Go](#install-via-go)
   - [Build from Source](#build-from-source)
   - [CLI Runtime Flags & Port Hunting](#cli-runtime-flags--port-hunting)
   - [Remote Access Guard & Master PIN](#remote-access-guard--master-pin)
3. [Connection Vault & Security Architecture](#3-connection-vault--security-architecture)
   - [AES-256-GCM & Argon2id Vault](#aes-256-gcm--argon2id-vault)
   - [Native SSH Tunneling (Bastion / Jump Hosts)](#native-ssh-tunneling-bastion--jump-hosts)
   - [Multi-Engine Configuration (MySQL, PostgreSQL, SQLite)](#multi-engine-configuration-mysql-postgresql-sqlite)
   - [Environment Tiers (Production, Staging, Development)](#environment-tiers-production-staging-development)
   - [Read-Only Mode Enforcement](#read-only-mode-enforcement)
4. [Developer & Data Operator Manual (User Level)](#4-developer--data-operator-manual-user-level)
   - [Table Explorer & Search Path Navigation](#table-explorer--search-path-navigation)
   - [Keyset Pagination & Quick Search](#keyset-pagination--quick-search)
   - [Spreadsheet CRUD & Inline Cell Editing](#spreadsheet-crud--inline-cell-editing)
   - [Quick Value Presets & PK-Less Row Mutations](#quick-value-presets--pk-less-row-mutations)
   - [Zero-Dependency CSV Import & Column Mapping](#zero-dependency-csv-import--column-mapping)
   - [Interactive Visual ERD Canvas](#interactive-visual-erd-canvas)
   - [BYOK AI Copilot & Automated Error Healer](#byok-ai-copilot--automated-error-healer)
   - [Unified Operational Notes Inbox & Follow-up Lifecycle](#unified-operational-notes-inbox--follow-up-lifecycle)
   - [Workspace Ergonomics, Tab Context Menu & Fluid Splitter](#workspace-ergonomics-tab-context-menu--fluid-splitter)
5. [Advanced DBA & Operations Playbook (DBA Level)](#5-advanced-dba--operations-playbook-dba-level)
   - [Deterministic AST Safety Guard & Challenge Modal](#deterministic-ast-safety-guard--challenge-modal)
   - [Pre-DDL Smart Tiered Backups](#pre-ddl-smart-tiered-backups)
   - [Pre-Flight Lock Contention & Root Blocker Tree](#pre-flight-lock-contention--root-blocker-tree)
   - [Real-Time Processlist & Out-of-Band Cancellation](#real-time-processlist--out-of-band-cancellation)
   - [Process Activity Trend Panel](#process-activity-trend-panel)
   - [Cluster & Replication Inspector (PXC / Galera / Replica)](#cluster--replication-inspector-pxc--galera--replica)
   - [Performance Flight Recorder (Quad-Metric Telemetry)](#performance-flight-recorder-quad-metric-telemetry)
   - [Server Variables Inspector & Tuning Advisor](#server-variables-inspector--tuning-advisor)
   - [Heuristic Index Advisor & Query Tuner (ESR Rule)](#heuristic-index-advisor--query-tuner-esr-rule)
   - [Structure Sync (AST-Powered Schema Diff & Migration)](#structure-sync-ast-powered-schema-diff--migration)
   - [Cross-Connection Data Transfer & Keyset Row Sync](#cross-connection-data-transfer--keyset-row-sync)
   - [Headless CLI Suite (`run`, `transfer`, `advise`)](#headless-cli-suite-run-transfer-advise)
   - [Zero-Daemon Scheduled Task Runner](#zero-daemon-scheduled-task-runner)
   - [Memory Ceiling Diagnostics & `pprof` Profiling](#memory-ceiling-diagnostics--pprof-profiling)
6. [Keyboard Shortcuts Cheatsheet](#6-keyboard-shortcuts-cheatsheet)
7. [Troubleshooting & FAQ](#7-troubleshooting--faq)

---

## 1. Overview & Architectural Philosophy

SqlFox is engineered from the ground up for teams who demand instant responsiveness, ironclad database safety, and deep diagnostic observability without the overhead of heavy Electron wrappers or JVM runtimes.

### Single-Binary Zero-CGO Model
- SqlFox compiles into a **single standalone executable** with an embedded high-performance web frontend.
- Zero external runtime dependencies: no Node.js, no Chromium, no Java, no shared C libraries.
- Pure-Go database drivers provide complete cross-compilation capability (`amd64`, `arm64`) across Linux, macOS, and Windows.

### Strict Memory Ceiling (< 100MB RAM)
- Even when connected to massive enterprise catalogs (>1,000 tables, >50,000 columns) or streaming multi-gigabyte result sets, SqlFox enforces a **< 100MB resident set size (RSS)** ceiling.
- Achieved via:
  - Streaming chunked transports over WebSockets/SSE.
  - Keyset pagination without unbounded in-memory row buffering.
  - Native CSS `content-visibility: auto` virtualization for large sidebar schema trees.
  - 150ms debounced search filters and 3-second fail-fast timeout queues.

### Supported Engines & Drivers
| Engine | Supported Versions | Driver Details |
| :--- | :--- | :--- |
| **MySQL** | 5.7, 8.0, 8.4+ | Pure-Go `go-sql-driver/mysql` with full SSL/TLS and TLS 1.3 support |
| **MariaDB** | 10.3 to 11.x+ | Pure-Go MySQL protocol driver with Galera cluster inspection |
| **PostgreSQL** | 9.6 through 16+ | Native `pgx/v5` protocol driver with out-of-band `pg_cancel_backend` |
| **SQLite** | 3.x | Pure-Go CGO-free `modernc.org/sqlite` engine (local and over SSH) |

---

## 2. Installation & Launch Modes

### Standalone Binaries (Linux, macOS, Windows)
Download the latest archive from [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

```bash
# Linux amd64
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.2.0-linux-amd64.tar.gz | tar -xz
./sqlfox

# macOS Apple Silicon (ARM64)
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.2.0-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

On Windows, download the `.zip` archive, extract `sqlfox.exe`, and double-click to launch.

### Install via Go
```bash
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest
sqlfox
```

### Build from Source
```bash
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
./sqlfox
```

### CLI Runtime Flags & Port Hunting
SqlFox supports intuitive command-line options:

```bash
# Custom port and host binding
./sqlfox -port 8080 -host 127.0.0.1

# Headless server mode (suppress automatic browser launch)
./sqlfox -no-browser

# Enable verbose / debug logging to terminal
./sqlfox -v
./sqlfox -log-level debug

# Output logs in structured JSON format
./sqlfox -log-format json

# Persist diagnostic logs to a file (dual-stream: console + file)
./sqlfox -log-file sqlfox.log -log-level debug

# Silent mode (suppress stdout/stderr banner and request logs)
./sqlfox -silent   # or -q

# Enable loopback-guarded performance profiling
SQLFOX_PPROF=1 ./sqlfox -port 9090
```

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-port <num>` | `8080` | HTTP listening port (with automatic sequential port hunting) |
| `-host <ip>` | `127.0.0.1` | Network interface to bind (`0.0.0.0` for LAN with Master PIN guard) |
| `-log-level <lvl>`| `info` | Minimum log verbosity level: `debug`, `info`, `warn`, `error` |
| `-v`, `-verbose` | `false` | Convenience shorthand for `-log-level debug` |
| `-log-format <fmt>`| `text` | Structured log formatting: `text` (human-readable) or `json` |
| `-log-file <path>`| `""` | File path to append logs into (writes to both console and file) |
| `-silent`, `-q` | `false` | Suppresses console logs completely (file logging remains active if configured) |
| `-no-browser` | `false` | Run headless without spawning the default OS web browser |

> [!NOTE]
> **Automatic Port Hunting**: If the chosen port is already in use by another service (`EADDRINUSE`), SqlFox automatically hunts for and claims the next available sequential port (e.g. 8081, 8082), printing the active URL to stdout.

### Windows Double-Click Logging Configuration
When you launch `sqlfox.exe` by double-clicking it on Windows, no terminal is attached, meaning command-line flags cannot be passed directly. SqlFox provides four seamless methods to configure logging and debug verbosity:

1. **Local Configuration File (`sqlfox.env` or `.env`)**:
   Place a plain text file named `sqlfox.env` or `.env` in the same folder as `sqlfox.exe` (or in `~/.sqlfox/sqlfox.env`). SqlFox automatically parses and applies it upon startup:
   ```ini
   # sqlfox.env
   SQLFOX_LOG_LEVEL=debug
   SQLFOX_LOG_FILE=sqlfox-debug.log
   SQLFOX_LOG_FORMAT=text
   # SQLFOX_SILENT=true
   ```
2. **Windows Desktop Shortcut**:
   - Right-click `sqlfox.exe` &rarr; **Create shortcut**.
   - Right-click the newly created shortcut &rarr; **Properties**.
   - In the **Target** field, append the flags at the end:
     ```
     C:\path\to\sqlfox.exe -log-level debug -log-file C:\path\to\sqlfox.log
     ```
   - Double-clicking the shortcut will launch SqlFox with the specified flags.
3. **Batch Launcher Script (`run-sqlfox.bat`)**:
   Create a small batch file in the same directory:
   ```bat
   @echo off
   start "" "%~dp0sqlfox.exe" -v -log-file "%~dp0sqlfox-debug.log"
   ```
4. **Windows System / User Environment Variables**:
   Set `SQLFOX_LOG_LEVEL=debug` or `SQLFOX_LOG_FILE=C:\logs\sqlfox.log` in Windows Environment Variables (`sysdm.cpl` &rarr; *Environment Variables*). SqlFox respects these across all launch methods.

### Remote Access Guard & Master PIN
When you bind SqlFox to a local network interface or access it over LAN (`http://<your-lan-ip>:8080`):
1. **Remote Access Guard** automatically triggers a full-screen lock modal on non-loopback devices.
2. The user must authenticate using the **Master Password** or the 6-digit **Master PIN** generated in the host server terminal.
3. This guarantees secure tablet, mobile, or team access across local WiFi without exposing raw credentials.
4. **Shoulder-Surfing Privacy Guard**: In the desktop interface header, the Master PIN badge masks sensitive digits by default (`fox-••••`). Click the **👁️** toggle icon to temporarily reveal the PIN when pairing secondary devices, keeping credentials private in open office or screen-sharing environments.

---

## 3. Connection Vault & Security Architecture

### AES-256-GCM & Argon2id Vault
- All connection endpoints, usernames, passwords, and private SSH keys are stored in an encrypted SQLite database (`~/.sqlfox/vault.db`).
- Vault records are encrypted using **AES-256-GCM** authenticated cipher blocks with unique 96-bit initialization vectors.
- Encryption keys are derived using **Argon2id** key derivation function (KDF) parameterized to defend against GPU/ASIC brute-force attacks.
- Portable Vault Archives (`.sqlfox-vault`) can be exported and imported with password protection for seamless team onboarding or machine migration.

### Native SSH Tunneling (Bastion / Jump Hosts)
SqlFox embeds an in-process, pure-Go SSH client:
- Connect securely to private RDS/Aurora clusters, internal VPC databases, or on-premise servers behind bastion hosts.
- Supports password authentication and encrypted **RSA / Ed25519** private SSH keys with passphrases.
- Zero external `ssh` commands or background agent dependencies.

### Multi-Engine Configuration (MySQL, PostgreSQL, SQLite)
1. Click **+ New Connection** in the sidebar.
2. Select your dialect:
   - **MySQL / MariaDB**: Host, Port (default `3306`), Username, Password, Database, SSL Mode (`disable`, `prefer`, `require`).
   - **PostgreSQL**: Host, Port (default `5432`), Username, Password, Database, SSL Mode, and default **Search Path** (`public` or custom schema).
   - **SQLite**: Local absolute path (e.g. `/var/data/app.db`) or remote path over an active SSH bastion connection.
3. Click **Test Connection** for an instant pre-flight ping before saving.

### Environment Tiers (Production, Staging, Development)
Assign an operational tier to every connection:
- `Production 🔴`: Maximum Safety Guard enforcement. All destructive operations require manual typing challenges and automatic pre-DDL backup triggers.
- `Staging 🟡`: Guarded AST evaluation with explicit confirmation alerts.
- `Development 🟢`: Streamlined confirmation for rapid prototyping.

The **Environment Badge** in the top session switcher displays the active tier prominently at all times.

### Read-Only Mode Enforcement
Click the **🔒 Read-Only** toggle in the top header at any time:
- Transforms the entire session into a non-mutating replica.
- Intercepts and rejects `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, and `TRUNCATE` at the driver layer before queries reach the database wire.
- Essential for safe auditing, incident triage, and production log investigations.

---

## 4. Developer & Data Operator Manual (User Level)

### Table Explorer & Search Path Navigation
- **Filter Bar**: Type in the sidebar to filter schemas and tables instantly with 150ms debounce.
- **PostgreSQL Schema Switcher**: Easily switch between `public`, `tenant_1`, and private schemas without reconnecting.
- **Table Context Menu**: Right-click any table to quickly Open Table, View DDL, Inspect Footprint, or Copy Table Name.
- **Quick Jump (Ctrl+P)**: Open the Schema Search modal to instantly find any table or column across the entire database.

### Keyset Pagination & Quick Search
Traditional `OFFSET 1000000` pagination forces database engines to scan and discard millions of rows. SqlFox employs **Keyset Pagination**:
- Traverses records using primary keys (`WHERE id > :last_seen_id ORDER BY id LIMIT 50`).
- Delivers instantaneous page jumps even on tables containing 100,000,000+ rows.
- Use the **Inline Filter Bar** above the grid for instant column search without writing manual `WHERE` clauses.

### Spreadsheet CRUD & Inline Cell Editing
- **Double-click any cell** to edit values directly inside the table grid.
- **Cell Highlighting**: Staged cell changes are highlighted in emerald with a change counter pill.
- **Review & Apply**: Click **Review Changes** to inspect the generated `UPDATE` statements before submitting.
- **+ Add Row**: Draft new rows at the bottom of the table, populate columns, and submit in a single transactional batch.
- **Multi-Row Delete**: Select multiple rows using checkboxes and click Delete; destructive deletion is intercepted by the Safety Guard.

### Quick Value Presets & PK-Less Row Mutations
- **Value Presets**: Right-click any cell or press floating hotkeys to insert:
  - `Alt+N` &rarr; `NULL`
  - `Alt+E` &rarr; `""` (Empty string)
  - `Alt+T` &rarr; `NOW()` / Current timestamp
  - `Alt+D` &rarr; Current date (`YYYY-MM-DD`)
  - `Alt+U` &rarr; Generated RFC4122 `UUID v4`
- **Tables Without Primary Keys (PK-Less)**: SqlFox generates safe full-row criteria (`WHERE col1 = ? AND col2 = ? AND col3 = ? LIMIT 1`) ensuring reliable updates and deletions without unintended cascade damage.

### Zero-Dependency CSV Import & Column Mapping
1. Click **Import CSV** in any open table view.
2. Select your `.csv` file. SqlFox inspects headers and previews the first 10 rows automatically.
3. Map CSV headers to target table columns using smart auto-matching.
4. Choose Batch Size (default 500 rows) and Conflict Strategy (`Skip on Error`, `Stop on Error`).
5. Watch the streaming progress bar with real-time throughput metrics.

### Interactive Visual ERD Canvas
Navigate to **📦 DB Tools &rarr; 🗺️ Visual ERD Canvas**:
- **Automatic Relationship Mapping**: Renders all Foreign Key relationships with smooth cubic Bézier SVG lines.
- **Interactive Drag & Drop**: Freely reposition table cards on the infinite grid. Table positions are cached per session.
- **Auto Layout**: Automatically arranges tables using topological hierarchy.
- **Fit Viewport**: Scales and centers the entire schema diagram in a single click.
- **Card Inspector**: Displays column names, data types, Primary Key (`🔑`), and Foreign Key (`🔗`) badges.

### BYOK AI Copilot & Automated Error Healer
SqlFox features a **Bring-Your-Own-Key (BYOK)** AI assistant:
- Supported Providers: **OpenAI**, **Anthropic Claude**, **Google Gemini**, **DeepSeek**, and local **Ollama** (`http://localhost:11434`).
- **Context Injection**: Automatically injects schema information, active table definitions, and engine dialect into prompt context.
- **1-Click SQL Execution**: SQL suggestions rendered in chat include `Insert & Run` and `Replace in Editor` buttons.
- **Chat Session Forking**: Fork an active AI discussion at any point into a new branch to explore alternative query strategies without losing previous context.
- **Automated Error Healer**: When a query fails in the editor, click **🩺 Heal Error**; the AI diagnoses the syntax/logical error, explains the root cause, and provides the corrected SQL.

### Unified Operational Notes Inbox & Follow-up Lifecycle
SqlFox integrates a centralized operational notebook and follow-up engine directly into the application, bridging raw database telemetry with developer runbooks:

- **Central Notes Inbox**:
  - Click the **📝 Notes** button in the top navigation header at any time to open the slide-over Notes Inbox drawer (`#notes-inbox-drawer`).
  - An overdue badge displays the count of actionable reminders requiring immediate attention.
  - Filter notes seamlessly across status tabs (`All`, `Inbox`, `Resolved`, `Archived`), search titles and content in real-time, or toggle pinned runbooks (`📌`).
- **Comprehensive Note Composer**:
  - Click **+ New Note** or capture notes directly from query results, Processlist threads, or Table Views.
  - Every note captures:
    - **Metadata**: Title, lifecycle status (`inbox`, `resolved`, `archived`), and pin status.
    - **Object Tags**: Soft-link URI tags (`db://{connection}/{database}/{table}`) tying notes directly to database objects.
    - **User Tags**: Custom categorization tags (e.g. `#incident`, `#migration`, `#runbook`).
    - **Developer Action Notes**: Rich markdown area for human analysis, follow-up steps, and resolution details.
    - **Raw Snapshot**: Read-only capture of original SQL statements, AI explanations, or diagnostic payloads with syntax highlighting and live preview.
- **Follow-up Reminders & Alarm Chime**:
  - Schedule reminders with quick presets (`10m`, `1h`, `Tomorrow 09:00`, `Next Monday`) or a custom date-time picker.
  - A background evaluation engine checks active reminders every 30 seconds.
  - When a reminder matures, SqlFox triggers an audio chime via the native **Web Audio API** (zero external sound file dependencies) and renders a non-blocking interactive toast alert.
  - Operators can take action directly from the toast: **Open Note**, **Snooze 10m**, or mark as **Done**.
- **Table Viewer Runbook Feed**:
  - Inside any table view, open the **📝 Notes & Runbook** sub-tab.
  - Automatically displays a scoped feed of all operational notes tagged to the active table (`db://...`), keeping schema documentation, known data quirks, and maintenance runbooks pinned right where operators work.
- **AI Copilot `@` Mention Autocomplete**:
  - In the Copilot prompt input, type `@` to summon the interactive note picker dropdown.
  - Selecting a note inserts an `[@note:<title>]` reference token.
  - When submitted, SqlFox automatically extracts referenced notes and injects the raw snapshot and developer action notes directly into the AI system prompt context, empowering the AI to diagnose historical incidents and analyze past query issues accurately.

### Workspace Ergonomics, Tab Context Menu & Fluid Splitter
SqlFox provides a modern, distraction-free desktop-grade workspace optimized for long development and debugging sessions:

- **Workspace Pane Splitter**:
  - Drag the horizontal divider between the Query Editor and Virtual Data Grid to flexibly reallocate screen real-estate based on your current task (more lines of SQL vs more result rows).
  - Enforces strict boundary guards (editor minimum 80px, grid minimum 120px) to prevent toolbar and pagination controls from being clipped.
  - **Double-Click Reset**: Double-click anywhere on the splitter bar to instantly snap the workspace back to a balanced **50:50** layout.
  - **Global Workspace Persistence**: Custom pane heights are automatically remembered in `localStorage` across all query tabs and browser reloads.
  - **Dynamic Tab Visibility**: The splitter cleanly hides when switching to single-surface views like Table Viewer, Processlist, or Cluster Inspector.
- **Workspace Tab Context Menu & Lifecycle Suite**:
  - Right-click any workspace tab to reveal comprehensive lifecycle actions: `Close` (`Ctrl+W`), `Close Others`, `Close Tabs to the Right`, `Close All`, `Duplicate Tab` (cloning draft SQL and filters), `Rename Tab...`, and `Save as Operational Note...`.
  - **Tab Close Confirmation Guard**: Intercepts tab closures with a native challenge confirmation if dirty, unexecuted SQL drafts or unsaved mutations exist, preventing accidental data loss. Clean tabs close instantaneously without alert fatigue.
  - **Empty Workspace Graceful Reset**: Closing all tabs automatically resets the workspace into a fresh, clean `Query 1.sql` tab.

---

## 5. Advanced DBA & Operations Playbook (DBA Level)

### Deterministic AST Safety Guard & Challenge Modal
SqlFox parses SQL statements into an Abstract Syntax Tree (AST) before sending them to the database engine:
- **Intercepted Queries**:
  - `DROP TABLE`, `DROP DATABASE`, `DROP SCHEMA`
  - `TRUNCATE TABLE`
  - Unqualified `DELETE FROM tbl` (without `WHERE` clause)
  - Unqualified `UPDATE tbl SET ...` (without `WHERE` clause)
  - Mass mutations affecting unexpected row thresholds
- **Challenge Modal**: Displays the intercepted SQL, explains the risk, and requires the DBA to explicitly type the target table name before execution can proceed.

### Pre-DDL Smart Tiered Backups
Before running dangerous `ALTER TABLE` or `DROP TABLE` statements:
1. SqlFox triggers a **Smart Tiered Backup**.
2. **Schema DDL**: Captured and archived automatically.
3. **Table Data**: If the table footprint is under 50MB, data is streamed into a local temporary snapshot (`~/.sqlfox/backups/`).
4. If a migration goes awry, the DBA can restore the exact table state immediately.

### Pre-Flight Lock Contention & Root Blocker Tree
Access via **⚡ Monitor &rarr; 🔒 Lock Contention & Blockers**:
- Analyzes active locks on MySQL (`sys.innodb_lock_waits`, `performance_schema.data_locks`) and PostgreSQL (`pg_locks`, `pg_blocking_pids`).
- **Root Blocker Tree**: Visualizes the exact hierarchical dependency graph:
  ```
  Root Blocker (PID: 4120, User: batch_worker, Time: 182s, Query: UPDATE orders ...)
  └── Waiting Query (PID: 4891, Time: 45s, Query: ALTER TABLE orders ADD COLUMN ...)
      └── Blocked Query (PID: 5102, Time: 12s, Query: SELECT * FROM orders WHERE ...)
  ```
- **Mitigation Action**: Terminate the root blocking session safely with out-of-band cancellation directly from the tree view.

### Real-Time Processlist & Out-of-Band Cancellation
Access via **⚡ Monitor &rarr; ⚡ Processlist**:
- Real-time tabular monitoring of all server threads, execution times, client hosts, and active query states.
- Filter by non-idle threads to eliminate connection pool noise.
- **Out-of-Band Cancellation**: Clicking **Kill Query** sends `KILL QUERY <id>` (MySQL) or `SELECT pg_cancel_backend(<pid>)` (PostgreSQL) through a dedicated control connection without terminating the primary database session.
- **Process Detail Modal**: Inspect full query text, user, host, database, state, execution time, and lock dependencies with 1-click clipboard copy (featuring robust fallback support for plain HTTP non-SSL environments).

### Process Activity Trend Panel
Access via **⚡ Monitor &rarr; ⚡ Processlist** (Click **📈 Activity Trend** button):
- **Live Concurrency Telemetry**: Renders an ephemeral rolling 60-point time-series chart directly on native HTML5 Canvas without backend polling overhead.
- **Dual Metric Visualizer**:
  - Sky blue solid line with soft gradient fill representing active executing query concurrency (`Command != 'Sleep'`).
  - Slate dashed secondary line representing total connected client threads.
- **Real-Time KPIs**: Dynamic metric pills detailing `Running Now`, `Total Threads`, `Peak Concurrency`, and `60s Avg Running`.
- **Flight Recorder Escalation**: 1-click **Escalate to Flight Recorder** shortcut to immediately trigger deep 1-second interval forensic recording when query concurrency spikes.
- **Collapsible Ergonomics**: Toggle button with state preserved in `localStorage` (`sqlfox_pl_trend_open`).

### Cluster & Replication Inspector (PXC / Galera / MySQL 8.4+ / PostgreSQL)
Access via **⚡ Monitor &rarr; 🌐 Cluster**:
- Designed for **Percona XtraDB Cluster (PXC)**, **MariaDB Galera**, modern **MySQL 8.4 LTS / 9.x**, and **PostgreSQL Primary/Standby** topologies.
- **Universal Replication Normalizer (MySQL & MariaDB)**:
  - Automatically executes modern `SHOW REPLICA STATUS` and `SHOW BINARY LOG STATUS`, gracefully falling back to legacy `SHOW SLAVE STATUS` and `SHOW MASTER STATUS` on MySQL 5.7 / 8.0.
  - Normalizes metric keys (`replica_io_running` vs `slave_io_running`, `seconds_behind_source` vs `seconds_behind_master`) across all engine generations.
  - On MariaDB, seamlessly probes `information_schema.innodb_lock_waits` when `sys.innodb_lock_waits` is not installed.
- **PostgreSQL Replication & Standby Telemetry**:
  - Automatically probes `pg_stat_replication` on primary nodes (streaming clients, write/flush/replay LSNs, sync states) and `pg_stat_wal_receiver` on standbys.
- **Galera / PXC Flow Control**: Real-time tracking of `wsrep_flow_control_paused` (evaluates pauses above 0.05).
- **Slowest Node Pinpointer**: Automatically probes all cluster peer nodes to identify the specific node causing replication backpressure.
- **Tri-Metric Matrix**: Evaluates EVS network latency, InnoDB disk stalls, and applier worker queue capacity.

### Historical Statement Digest & Post-Mortem Deadlocks
Access via **⚡ Monitor &rarr; ⚡ Processlist** (Segmented Pill Switcher):
- **Live Threads**: View and cancel active queries out-of-band.
- **Statement Digest**: Historical latency percentiles and slowest query patterns.
  - MySQL/MariaDB: Queries `sys.statement_analysis` with fallback to `performance_schema.events_statements_summary_by_digest`.
  - PostgreSQL: Probes `pg_stat_statements` with adaptive telemetry and 1-click extension enablement guidance when not loaded.
- **Deadlock Inspector**: Extracts and parses the `LATEST DETECTED DEADLOCK` section from `SHOW ENGINE INNODB STATUS` with visual Transaction 1 vs Transaction 2 conflict matrix.

### Programmability & User Privilege Matrix (MySQL & PostgreSQL)
- **Routines Explorer**: Inspect Stored Procedures and Functions in the sidebar tree:
  - MySQL: Introspects `information_schema.routines` with 1-click `CALL sp(...)` query tab templates.
  - PostgreSQL: Introspects `pg_proc` + `pg_namespace` with overload-safe signatures (`func_name(type1, type2)`) and extracts DDL via `pg_get_functiondef`.
- **User & Role Management**: Access via **⚡ Monitor &rarr; 🛡️ Privilege Matrix**:
  - MySQL: Introspects `mysql.user` grant columns with SSL and password expiry states.
  - PostgreSQL: Dialect-aware role matrix introspecting `pg_roles` attributes (`SUPERUSER`, `CREATEDB`, `CREATEROLE`, `REPLICATION`, `BYPASSRLS`, `LOGIN`) and role membership (`MEMBER OF`).

### Performance Flight Recorder (Quad-Metric Telemetry)
Access via **⚡ Monitor &rarr; 📈 Recorder**:
- High-resolution, on-demand diagnostic flight recorder capturing 1-second interval telemetry:
  1. **Differential QPS** (Queries Per Second)
  2. **`Threads_running`** (Active thread concurrency)
  3. **Row Lock Waits** (`Innodb_row_lock_waits`)
  4. **Buffer Pool Waits** (`Innodb_buffer_pool_wait_free`)
- **Correlated Query Inspector**: Click any spike on the timeline scrubber to inspect the exact SQL queries running during that second.
- **10-Minute Auto-Stop Safety Guard**: Prevents runaway recording sessions from filling disk space or impacting production servers.

### Server Variables Inspector & Tuning Advisor
Access via **⚡ Monitor &rarr; ⚙️ Server Variables**:
- Inspects hundreds of engine system parameters with instant search and categorical filtering.
- **Tuning Advisor**: Evaluates critical memory, connection, and I/O variables against best practices:
  - MySQL: `innodb_buffer_pool_size` vs system RAM, `max_connections`, `innodb_flush_log_at_trx_commit`.
  - PostgreSQL: `shared_buffers`, `work_mem`, `maintenance_work_mem`, `effective_cache_size`, `wal_buffers`, `checkpoint_completion_target`, and `autovacuum`.

### Heuristic Index Advisor & Query Tuner (ESR Rule)
1. Write or paste any `SELECT` query in the Query Editor.
2. Click **Explain** or open the **💡 Advisor** tab.
3. SqlFox evaluates the AST according to the **ESR Rule** (Equality $\to$ Sort $\to$ Range):
   - Proposes optimal composite index definitions.
   - Detects **Index Bloat** (>5 secondary indexes on a single table) and recommends index consolidation paths.
   - Generates non-blocking DDL syntax:
     - MySQL: `ALTER TABLE tbl ADD INDEX idx_name (...) ALGORITHM=INPLACE, LOCK=NONE;`
     - PostgreSQL: `CREATE INDEX CONCURRENTLY idx_name ON tbl (...);`

### Structure Sync (AST-Powered Schema Diff & Migration)
Access via **📦 DB Tools &rarr; 🔄 Structure Sync**:
1. Select **Source** session and **Target** session.
2. SqlFox introspects both schemas, parses definitions into normalized ASTs, and calculates structural differences:
   - Missing tables
   - Added / removed / modified columns (with dialect-specific DDL: MySQL `MODIFY COLUMN` vs PostgreSQL atomic `ALTER TABLE tbl ALTER COLUMN col TYPE ...; ALTER COLUMN col SET/DROP NOT NULL;`)
   - Index deltas and primary key changes
   - Foreign key constraint differences
3. Generates a safe, order-aware DDL migration script with execution preview.

### Cross-Connection Data Transfer & Keyset Row Sync
- **Data Transfer**: Stream entire tables directly from one database to another across different hosts, cloud providers, or engines with a strictly bounded `<50MB` memory buffer.
- **Row-Level Data Sync**: Reconciles table data row-by-row using keyset delta hashing. Detects `INSERT`, `UPDATE`, and `DELETE` deltas and applies synchronization safely without full-table locks.

### Headless CLI Suite (`run`, `transfer`, `advise`)
SqlFox includes powerful headless commands for automation scripts, cron jobs, and CI/CD pipelines:

```bash
# Execute SQL and output clean tabular or JSON data
./sqlfox run -conn "my-prod-db" -query "SELECT id, status FROM orders WHERE status = 'pending'" -format json

# Pipe SQL directly via STDIN with raw DSN (zero vault dependencies in CI)
echo "SELECT count(*) FROM users;" | ./sqlfox run -dsn "user:pass@tcp(10.0.0.5:3306)/app" -format csv

# Run Cross-Connection Data Transfer headlessly
./sqlfox transfer --from "source-db" --to "backup-db" --tables "orders,order_items" --batch 1000

# Run Heuristic Index Advisor on a slow query
./sqlfox advise -conn "my-prod-db" -query "SELECT * FROM logs WHERE tenant_id = 4 AND created_at > NOW() - INTERVAL 1 DAY ORDER BY id DESC"
```

### Zero-Daemon Scheduled Task Runner
Schedule recurring database maintenance, telemetry dumps, and health checks without running heavy external daemons or Celery workers:
- Configure tasks via declarative YAML / JSON definitions.
- Automatic retry on network glitches with exponential backoff.
- Direct output streaming to local storage or monitoring endpoints.

### Memory Ceiling Diagnostics & `pprof` Profiling
To verify the `<100MB RAM` guarantee under enterprise workloads:
1. Launch SqlFox with profiling enabled:
   ```bash
   SQLFOX_PPROF=1 ./sqlfox -port 8080
   ```
2. The runtime opens a loopback-guarded `pprof` endpoint (`http://127.0.0.1:8080/debug/pprof/`).
3. Capture heap allocation profiles:
   ```bash
   go tool pprof http://127.0.0.1:8080/debug/pprof/heap
   ```

---

## 6. Keyboard Shortcuts Cheatsheet

| Shortcut | Scope | Description |
| :--- | :--- | :--- |
| `Ctrl + Enter` | Query Editor | Execute current query or selected block |
| `Ctrl + Shift + Enter` | Query Editor | Execute `EXPLAIN` and view Visual Plan Tree |
| `Ctrl + T` | Workspace | Open new Query Tab |
| `Ctrl + W` | Workspace | Close current active tab |
| `Ctrl + K` | Universal | Open Command Palette |
| `Ctrl + P` | Universal | Open Schema Search (Quick Table / Column Jump) |
| `Ctrl + B` | Universal | Toggle Left Sidebar (Maximize workspace) |
| `Ctrl + L` | Universal | Toggle AI Copilot Sidebar |
| `Alt + N` | Table Grid | Insert `NULL` preset into active cell |
| `Alt + E` | Table Grid | Insert Empty string `""` preset |
| `Alt + T` | Table Grid | Insert `NOW()` / current timestamp preset |
| `Alt + D` | Table Grid | Insert current date (`YYYY-MM-DD`) preset |
| `Alt + U` | Table Grid | Insert generated `UUID v4` preset |
| `F1` or `?` | Universal | Open Keyboard Shortcuts Modal & Online Docs link |
| `Esc` | Dialogs / Modals | Dismiss any active modal or context menu |

---

## 7. Troubleshooting & FAQ

#### Q: How do I resolve `port 8080 already in use`?
> **Answer**: You don't have to! SqlFox features built-in **Port Hunting**. If port 8080 is occupied, it automatically probes and binds to the next free port (8081, 8082, etc.) and prints the active URL to your terminal.

#### Q: The browser asks for a Master PIN when accessing from my tablet. Where do I find it?
> **Answer**: Check the terminal where you launched `sqlfox`. A 6-digit Master PIN is printed upon startup. Alternatively, on your desktop session, click the **🔑 PIN** badge in the top header to reveal and copy your current PIN.

#### Q: What happens if my network drops during a query?
> **Answer**: SqlFox contains a **Global Network Error Boundary**. It retains your query drafts, unsaved editor changes, and staged grid edits in memory while automatically attempting connection recovery with backoff.

#### Q: How can I update SqlFox to the latest release?
> **Answer**: SqlFox includes an **In-App Self-Update Engine**. Whenever a new release is published on GitHub, an **🚀 Update** badge appears in the bottom status bar. Click it to review the release notes and upgrade the standalone binary in-place with a single click.

---

<div align="center">

**SqlFox** • Designed with care for developers and DBAs who value safety and speed.  
Need help or want to suggest a feature? Visit the [GitHub Repository](https://github.com/herliansyah/sqlfox).

</div>
