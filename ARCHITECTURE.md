# SqlFox Architecture Specification

System architecture, component boundaries, and execution flows for SqlFox.

## 1. High-Level System Architecture

SqlFox is designed as a **single-binary local application** with an embedded reactive web application. It operates under a strict memory ceiling (<100MB RAM) with sub-second startup time (<1s).

```mermaid
flowchart TD
    subgraph UI ["Embedded Frontend (CodeMirror 6 + Vue 3 / Svelte)"]
        UI_Editor["Query Editor (CodeMirror 6)"]
        UI_Grid["Virtual Data Grid (@tanstack/virtual)"]
        UI_Tree["Schema Explorer Tree"]
        UI_Guard["Safety Challenge Modal"]
        UI_AI["AI Assistant Panel"]
    end

    subgraph GoCore ["Go Core Backend (Go 1.22+)"]
        API["HTTP / WebSocket API (Chi Router)"]
        Vault["Connection Vault (Argon2id + AES-GCM)"]
        Safety["Safety Guard Engine (Deterministic AST)"]
        SessionMgr["Session & Out-of-Band Pool"]
        AIProxy["AI Context Firewall (BYOK / Ollama)"]
    end

    subgraph TargetDB ["Target Databases"]
        MySQL["MySQL 5.7/8.x / MariaDB (Direct / SSH)"]
        Postgres["PostgreSQL 9.6-16+ (pgx/v5 Pure Go)"]
        SQLite["Local SQLite (Pure Go)"]
    end

    subgraph Storage ["Local Machine Storage"]
        ConfigDB[("Local config.db (SQLite)")]
        DumpDir["Local Backups (.scratch/backups/)"]
    end

    UI <-->|HTTP REST & WS Stream| API
    API --> Vault
    API --> Safety
    API --> SessionMgr
    API --> AIProxy
    Vault <--> ConfigDB
    Safety --> DumpDir
    SessionMgr <--> TargetDB
```

---

## 2. End-to-End Query Execution Lifecycle

The following sequence details how arbitrary SQL queries flow from the editor through Safety Guard to execution and streaming:

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Editor as CodeMirror 6
    participant Server as Go API Server
    participant Guard as Safety Guard (AST)
    participant Driver as DB Driver (MySQL/SQLite)
    participant Target as Target Database
    participant Grid as Virtual Data Grid

    User->>Editor: Type query & press Ctrl+Enter
    Editor->>Server: POST /api/session/{id}/execute {sql}
    Server->>Guard: InspectQuery(sql, environment)
    
    alt Destructive Query Detected (e.g. DELETE without WHERE)
        Guard-->>Server: Blocked (DestructiveReason)
        Server-->>Editor: 422 Unprocessable (Requires Challenge Confirmation)
        Editor->>User: Open Challenge Modal (Type table name)
        User->>Editor: Confirms Challenge
        Editor->>Server: POST /api/session/{id}/execute {sql, confirmed: true}
    else Safe or Pre-Approved Query
        Guard-->>Server: Approved
    end

    Server->>Driver: QueryStream(ctx, sql, implicitLimit=1000)
    Driver->>Target: Execute SQL statement
    Target-->>Driver: MySQL Result Buffer / Wire stream

    Server-->>Grid: WS Frame: Columns Metadata
    loop Chunked Row Streaming (Batch 100 rows)
        Driver->>Target: rows.Next()
        Driver-->>Server: Chunk [100 records]
        Server-->>Grid: WS Frame: Data Chunk
        Grid->>Grid: Append to virtual scroll buffer
    end
    Server-->>Grid: WS Frame: Stream Complete (duration, row_count)
```

---

## 3. Out-of-Band Query Cancellation Flow

To prevent socket tearing and orphaned background execution on the database server, SqlFox executes cancellations out-of-band:

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Frontend as Frontend UI
    participant Server as Go Backend
    participant WorkerConn as Worker Connection (Thread 101)
    participant ControlConn as Control Connection (Pool)
    participant MySQL as MySQL Server

    User->>Frontend: Click "Cancel Query"
    Frontend->>Server: POST /api/session/{id}/cancel {query_id}
    Server->>Server: Lookup active thread: Query 42 -> Thread 101
    Server->>ControlConn: Acquire secondary connection from pool
    ControlConn->>MySQL: KILL QUERY 101;
    MySQL-->>WorkerConn: Error 1317: Query execution was interrupted
    WorkerConn-->>Server: Query Cancelled Error
    Server-->>Frontend: WS Frame: Query Cancelled by User
    Server->>ControlConn: Release secondary connection to pool
```

---

## 4. Driver Interface & Extensibility Contract

To allow clean additions of PostgreSQL, ClickHouse, or MongoDB in future releases without modifying core routing or safety logic:

```go
package driver

import (
    "context"
    "io"
)

// Driver manages connection lifecycle and dialers
type Driver interface {
    Connect(ctx context.Context, cfg ConnectionConfig) (Session, error)
}

// Session represents an active connection context
type Session interface {
    // Stream-based query for arbitrary SELECTs
    QueryStream(ctx context.Context, sql string, limit int) (RowStream, error)
    
    // Mutation execution for DML/DDL
    Exec(ctx context.Context, sql string) (ExecResult, error)
    
    // Schema introspection
    InspectSchema(ctx context.Context, schemaName string) (*SchemaMetadata, error)
    GetTableDDL(ctx context.Context, tableName string) (string, error)
    GetTableEstimate(ctx context.Context, tableName string) (TableEstimate, error)
    
    // Lock inspection & Safe DDL execution
    InspectTableLocks(ctx context.Context, database, table string) ([]TableLockInfo, error)
    ExecSafeDDL(ctx context.Context, sql string, lockTimeoutSec int) (*ExecResult, error)
    
    // Cluster & Replication diagnostics
    InspectClusterHealth(ctx context.Context) (*ClusterHealthReport, error)

    // Query plan & optimization
    Explain(ctx context.Context, sql string) (*ExplainPlan, error)
    
    // Interruption
    CancelQuery(ctx context.Context, queryID string) error
    
    io.Closer
}

// RowStream yields chunked data without loading entire resultsets into RAM
type RowStream interface {
    Columns() []ColumnDefinition
    NextChunk(batchSize int) ([][]any, error)
    TotalRows() int64
    io.Closer
}
```

---

## 5. Table Viewer Inline Filtering & Pushdown Flow

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Grid as Virtual Data Grid & Filter Bar
    participant Server as Go API Server
    participant Validator as Filter Guard (Token & Verb Check)
    participant Driver as DB Driver (MySQL/SQLite)
    participant Target as Target Database

    User->>Grid: Type filter (e.g. `status = 'active'`) & press Enter
    Grid->>Server: GET /api/session/{id}/table/{name}/rows?limit=50&offset=0&filter=status+%3D+%27active%27
    Server->>Validator: validateTableFilter(filter)
    alt Semicolon or Destructive Keyword Detected
        Validator-->>Server: Error (invalid filter expression)
        Server-->>Grid: 400 Bad Request
        Grid->>User: Display error message in table body
    else Safe Filter Expression
        Validator-->>Server: OK
        Server->>Driver: QueryStream(ctx, "SELECT * FROM `table` WHERE (status = 'active') LIMIT 50")
        Driver->>Target: Execute indexed SELECT with WHERE
        Target-->>Driver: Result rows
        Driver-->>Server: Row stream
        Server-->>Grid: JSON { columns, rows, pk_col, filter }
        Grid->>Grid: Render filtered rows & update "Filtered" indicator
    end
```

---

## 6. Safe DDL & Lock Contention Prevention Pipeline

To eliminate Metadata Lock (MDL) queues, application pool exhaustion, and deadlock cascades on massive datasets (e.g. 500GB tables and small master tables referenced by high-traffic transactional child tables):

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Editor as DDL Designer / SQL Tab
    participant Server as Safe Execution Pipeline
    participant Guard as Safety Guard & Lock Inspector
    participant DB as Target Database (MySQL/SQLite)

    User->>Editor: Execute ALTER TABLE master_status ADD COLUMN code TEXT
    Editor->>Server: POST /api/session/{id}/execute {sql}
    Server->>Guard: Inspect(sql, environment="Production")
    
    par Pre-Flight Inspections
        Guard->>DB: InspectTableLocks("master_status") (innodb_trx / metadata_locks)
        DB-->>Guard: Active blocking transactions (if any)
        Guard->>DB: InspectSchema & InspectTableRelationships("master_status")
        DB-->>Guard: Inbound FK from `orders` (150GB) detected!
    end

    Guard-->>Server: Requires Challenge (Attach locks, FK relationships)
    Server-->>Editor: 422 Blocked {requires_challenge: true, relationships, table_locks}
    Editor->>User: Display Challenge Modal with Active Lock and FK Relationship Warning
    User->>Editor: Type "master_status" & Confirm
    Editor->>Server: POST /api/session/{id}/execute {sql, confirmed: true, lock_timeout_sec: 3}
    
    Server->>DB: Dedicated *sql.Conn -> SET SESSION lock_wait_timeout = 3
    Server->>DB: Dedicated *sql.Conn -> ALTER TABLE master_status ...
    
    alt Lock Acquired within 3s
        DB-->>Server: DDL Succeeded
        Server->>DB: Close Dedicated Connection (Clean Pool)
        Server-->>Editor: 200 OK (Schema Updated)
    else Lock Wait Timeout Exceeded (>3s)
        DB-->>Server: Error 1205: Lock wait timeout exceeded
        Server->>DB: Close Dedicated Connection (Zero Pool Pollution)
        Server-->>Editor: Fail-Fast Aborted (Server remained 100% responsive)
    end
```

---

## 7. Cluster & Replication Inspector Pipeline

The Cluster & Replication Inspector provides zero-daemon, dynamic clustering and replication telemetry for Percona XtraDB Cluster (PXC 5.7/8.0), Galera Cluster, MariaDB, and MySQL Source-Replica setups.

```mermaid
sequenceDiagram
    autonumber
    actor DBA as Database Administrator
    participant UI as Dedicated Tab (🌐 Cluster)
    participant Server as Go API (/diagnostics/cluster)
    participant Driver as MySQL / SQLite Driver
    participant LocalDB as Local Database Node
    participant Peers as Remote Peer Nodes (Auto-Probed)

    DBA->>UI: Open "🌐 Cluster" tab (or 1s/3s/5s ticker)
    UI->>Server: GET /api/session/{id}/diagnostics/cluster
    Server->>Driver: InspectClusterHealth(ctx)
    Driver->>LocalDB: SHOW GLOBAL STATUS (wsrep_ready, wsrep_cluster_*, etc.)
    
    alt Galera / PXC Active (wsrep_ready = ON)
        Driver->>LocalDB: Read wsrep_incoming_addresses
        par Parallel Peer Probing (1.5s timeout)
            Driver->>LocalDB: Read local wsrep_local_recv_queue, FC sent
        and
            Driver->>Peers: Probe peer node 2 (Queue, FC sent)
        and
            Driver->>Peers: Probe peer node 3 (Queue, FC sent)
        end
        Driver->>Driver: Tri-Metric Correlation (EVS Latency + InnoDB waits + Slave Threads)
        Driver->>Driver: Identify Slowest/Culprit Node triggering Flow Control
    else Standard Replication (SHOW SLAVE STATUS)
        Driver->>LocalDB: Read Seconds_Behind_Master & IO/SQL thread states
    else Standalone Instance
        Driver->>LocalDB: Read InnoDB storage wait telemetry (Buffer pool & fsyncs)
    end

    Driver-->>Server: Return ClusterHealthReport
    Server-->>UI: 200 OK (JSON)
    UI-->>DBA: Render Diagnostic Summary Badge + Tri-Metric Cards + Node Roster
```

---

## 8. Performance Flight Recorder & Correlated Query Pipeline

The Performance Flight Recorder provides on-demand, time-bounded telemetry and thread snapshotting to isolate transient latency spikes and root-cause queries without running heavy 24/7 APM daemons.

```mermaid
sequenceDiagram
    autonumber
    actor DBA as Developer / DBA
    participant UI as Web UI (📈 Flight Recorder & Navbar)
    participant Server as Go API (/flight-recorder)
    participant Mgr as FlightRecorderManager
    participant Driver as MySQL / SQLite Driver
    participant Vault as SQLite Vault (flight_recorder_*)

    DBA->>UI: Click "● Start Recording" (1s interval)
    UI->>Server: POST /api/session/{id}/flight-recorder/start
    Server->>Mgr: StartRecording(sessionId, connId, interval)
    Mgr->>Vault: CreateFlightRecordingSession(...)
    Mgr->>Mgr: Launch background loop with 10m auto-stop timer
    Server-->>UI: 200 OK (recording_id)
    UI->>UI: Navbar displays pulsing badge (● REC 00:00)

    loop Every 1s Ticker (until user stops or 10m ceiling)
        Mgr->>Driver: SampleFlightMetrics(ctx)
        Driver->>Driver: SHOW GLOBAL STATUS (Questions, Threads, Locks, Buffer)
        Driver->>Driver: Query non-idle threads from PROCESSLIST (Top 5)
        Driver-->>Mgr: Return FlightMetricSample
        Mgr->>Mgr: Compute differential QPS
        Mgr->>Vault: AppendFlightRecordingTick(...)
        UI->>Server: GET /api/session/{id}/flight-recorder/status
        Server-->>UI: Return recent ticks (last 120s)
        UI->>UI: Update Canvas chart & telemetry cards
    end

    DBA->>UI: Click "⏹ Stop Recording"
    UI->>Server: POST /api/session/{id}/flight-recorder/stop
    Server->>Mgr: StopRecording(sessionId)
    Mgr->>Vault: FinishFlightRecordingSession(...)
    Server-->>UI: Return completed session metadata
    DBA->>UI: Click on graph spike point -> Drawer reveals correlated Top-5 SQL queries with Explain & AI actions
```

---

## 9. Data Synchronization & Transfer Pipeline

SqlFox provides direct database-to-database schema and data synchronization without intermediary disk files, bounded to <50MB RAM:

```mermaid
sequenceDiagram
    autonumber
    actor User as Developer / DBA
    participant UI as Sync Dialogs (Structure / Transfer / Row Sync)
    participant Server as Go API (/api/sync/*)
    participant Src as Source Database Session
    participant Tgt as Target Database Session

    alt Structure Sync (Schema Comparison)
        User->>UI: Select Source & Target -> Compare
        UI->>Server: POST /api/sync/structure/compare
        Server->>Src: Fetch schema & DDL
        Server->>Tgt: Fetch schema & DDL
        Server->>Server: AST Diff (tables, columns, indexes)
        Server-->>UI: Return categorized diff & migration SQL
        User->>UI: Select items & click "Apply Migration"
        UI->>Server: POST /api/sync/structure/apply {statements}
        Server->>Tgt: Execute safe DDL migrations
    else Cross-Connection Data Transfer
        User->>UI: Select tables & options -> Start Transfer
        UI->>Server: POST /api/sync/data/transfer
        loop For each table (chunked 500 rows)
            Server->>Src: QueryStream("SELECT * FROM ...")
            Src-->>Server: Row chunks
            Server->>Tgt: Exec(multi-row parameterized INSERT)
            Server-->>UI: NDJSON stream progress event
        end
    else Row-Level Data Sync
        User->>UI: Compare Table Rows
        UI->>Server: POST /api/sync/data/compare {table}
        Server->>Src: Fetch rows with PK
        Server->>Tgt: Fetch rows with PK
        Server->>Server: Keyset delta reconciliation (Insert/Update/Delete)
        Server-->>UI: Return delta preview
        User->>UI: Apply Changes
        UI->>Server: POST /api/sync/data/apply {operations}
        Server->>Tgt: Atomic transaction execution
    end
```

---

## 10. Visual ERD Interactive Canvas Architecture

The Visual ERD Canvas delivers zero-dependency, pure-frontend database schema visualization:

```mermaid
flowchart TD
    subgraph Frontend ["Embedded Web UI (web/dist/index.html)"]
        Dialog["#erd-dialog (Native HTML5 Dialog)"]
        Wrapper["#erd-canvas-wrapper (Dot-grid background)"]
        Viewport["#erd-viewport (CSS transform: translate + scale)"]
        SVG["#erd-svg (Cubic Bézier FK Lines & Markers)"]
        Cards["#erd-cards (Draggable Table Cards & Column Badges)"]
        SessionStore[("sessionStorage (erd-pos-{sessionId}-{db})")]
    end

    subgraph Backend ["Go Server API"]
        SchemaAPI["GET /api/session/{id}/schema?db={db}"]
    end

    Dialog --> Wrapper
    Wrapper --> Viewport
    Viewport --> SVG
    Viewport --> Cards
    Cards -.->|Store / Restore Coordinates| SessionStore
    Dialog -->|Fetch Schema & FK Relations| SchemaAPI
```

1. **SVG & DOM Layer Separation**: Relationship connectors are rendered as SVG `<path>` elements using cubic Bézier curves (`M x1,y1 C cx1,y1 cx2,y2 x2,y2`) behind draggable HTML table cards.
2. **Auto-Layout & Viewport Transform**: Simple square-root grid positioning algorithm eliminates card overlaps; `erdFitAll()` calculates card bounding boxes and scales/pans `#erd-viewport` to fit all tables within the viewport.
3. **Session Cache**: Coordinates are preserved in `sessionStorage` per session and database, surviving modal toggles while discarding stale coordinates upon browser close.

---

## 11. Directory & Package Structure

```
sqlfox/
├── cmd/
│   └── sqlfox/
│       ├── main.go                 # Entry point: flags, localhost listener, port hunting, browser launcher
│       └── main_test.go
├── internal/
│   ├── cli/                        # Headless CLI suite for CI/CD automation & scheduled jobs
│   │   ├── runner.go               # Subcommand dispatcher, DSN parser, delimiter splitting
│   │   ├── query.go                # sqlfox query: table, json, ndjson, csv streaming & fail-closed safety
│   │   ├── transfer.go             # sqlfox transfer: cross-connection streaming with per-table atomicity
│   │   ├── job.go                  # sqlfox job: declarative task runner, dump generator & retention rotation
│   │   ├── job_test.go             # End-to-end task runner & rotation tests
│   │   └── cli_test.go             # End-to-end CLI integration tests
│   ├── vault/                      # SQLite config storage & AES-256-GCM credential encryption
│   │   ├── vault.go                # Connections, master password, audit log, flight recorder schema
│   │   ├── crypto.go               # Argon2id key derivation & AES-256-GCM
│   │   ├── snippets.go             # Parameterized SQL snippets
│   │   ├── notes.go                # Table documentation notes
│   │   ├── export.go               # .sqlfox-vault portable archive export/import
│   │   └── flight_recorder.go      # Performance flight recorder sessions & tick persistence with 24h purge
│   ├── driver/                     # Database driver abstraction
│   │   ├── driver.go               # Core interfaces (Driver, Session, RowStream, SafeDDL, OSC, Cluster, FlightRecorder)
│   │   ├── rowstream.go            # Canonical chunked row streaming
│   │   ├── redundant_index.go      # Shadowed / duplicate index detection
│   │   ├── benchmark_test.go       # Large schema benchmark suite (1,000 tables, 50k columns, 1M rows streaming)
│   │   ├── mysql/                  # MySQL 5.7/8.x, PXC, MariaDB with Galera cluster inspection & fail-fast DDL
│   │   │   ├── mysql.go
│   │   │   └── cluster_test.go     # Tri-metric cluster bottleneck heuristics test
│   │   ├── postgres/               # PostgreSQL 9.6-16+ pure-Go native driver (pgx/v5)
│   │   │   ├── postgres.go
│   │   │   └── postgres_test.go
│   │   └── sqlite/                 # modernc.org/sqlite CGo-free pure-Go driver with FK inspection
│   ├── mock/                       # FK-aware synthetic mock data generator
│   │   ├── generator.go            # Relational closure, topological sort, FK pool sampling
│   │   └── generator_test.go
│   ├── safety/                     # Safety Guard engine
│   │   ├── guard.go                # AST inspection & destructive operation detection with DDL guards
│   │   └── backup.go               # Smart tiered pre-destruction table snapshotting (pure function)
│   ├── ai/                         # AI Assistant integration
│   │   ├── client.go               # Pure stdlib client (OpenAI, Claude, Gemini, Ollama)
│   │   ├── firewall.go             # Zero-leakage schema pruner & data masking (pure function)
│   │   └── prompts.go              # Structured prompts for explain, optimize, NL-to-SQL
│   ├── dataimport/                 # CSV Ingestion Engine
│   │   └── importer.go             # Delimiter sniffing, column mapping, batched streaming
│   └── server/                     # HTTP transport & handlers
│       ├── server.go               # HTTP routing & SPA static file serving
│       ├── auth.go                 # Remote Access Guard & LAN tablet token auth
│       ├── session_manager.go      # Active database connection pool & session state with Safe DDL execution
│       ├── pprof.go                # Loopback-guarded net/http/pprof diagnostic endpoints (SQLFOX_PPROF=1)
│       ├── pprof_test.go           # pprof access & loopback security unit tests
│       ├── flight_recorder.go      # Flight recorder background controller & auto-stop lifecycle
│       ├── handlers_flight_recorder.go # Flight recorder REST API endpoints
│       ├── handlers_connections.go # Connection CRUD & live testing
│       ├── handlers_query.go       # SQL execution & cancellation with 3s fail-fast pool guard
│       ├── handlers_schema.go      # Schema tree, DDL, preflight DDL, pushdown filtering
│       ├── handlers_crud.go        # Table spreadsheet CRUD (insert draft, batch delete)
│       ├── handlers_dump.go        # Streaming SQL dump export, smart seed exporter, large script runner
│       ├── handlers_sync.go        # Structure sync, cross-connection data transfer, row data sync
│       ├── handlers_dba.go         # Processlist, cluster health, diagnostics, privilege matrix
│       ├── handlers_snippets.go    # Snippet management
│       ├── handlers_import.go      # CSV upload & import endpoints
│       └── handlers_ai.go          # AI chat & assistance streaming
├── web/                            # Single-file zero-build SPA embedded into binary
│   ├── dist/                       # index.html + vendor assets (CodeMirror, Tailwind)
│   │   ├── vendor/
│   │   └── index.html              # Content visibility virtualization & global network error boundary
│   └── embed.go                    # //go:embed dist/*
├── docs/
│   ├── adr/                        # Architectural Decision Records (0001 - 0034)
│   ├── playbooks/                  # Production operational playbooks (large-scale database management)
│   └── agents/                     # Agent workflow documentation
├── CONTEXT.md                      # Ubiquitous domain language
├── ARCHITECTURE.md                 # System architecture (this document)
├── SECURITY.md                     # Security & privacy specification
├── UX_DESIGN.md                    # Interface wireframes & design tokens
├── MILESTONES.md                   # Delivery roadmap & milestones (1 - 22)
└── go.mod
```

---

## 12. Headless CLI Suite & Automation Pipeline

The headless CLI suite (`cmd/sqlfox/main.go` -> `internal/cli`) provides scriptable database access without the embedded HTTP server:

1. **Subcommand Dispatch**: Inspects `os.Args[1]`. If `query`, `transfer`, or `job`, execution delegates directly to `cli.RunQuery()`, `cli.RunTransfer()`, or `cli.RunJob()`. If empty or containing server flags (`-port`, `-host`), the desktop HTTP daemon launches normally.
2. **Dual Target Resolution**: Resolves connection profiles either from the encrypted SQLite vault (`--conn`) or directly from URI/DSN connection strings (`--dsn`), auto-detecting drivers (MySQL/SQLite/PostgreSQL) for zero-config CI/CD runners.
3. **Stream Pipeline & Output Formats**: Emits structured data exclusively to `stdout` (`table`, `json`, `ndjson`, `csv`), with diagnostics and transfer progress routed to `stderr`. `ndjson` streams row-by-row with $O(1)$ memory consumption (<20MB RAM).
4. **Fail-Closed Safety Policy**: High-risk destructive statements (`DROP`, `TRUNCATE`, unqualified `DELETE`/`UPDATE`) trigger an immediate halt with exit code `2` unless explicitly bypassed with `--force`.
5. **Per-Table Atomicity**: `sqlfox transfer` partitions transactions per table on the target database, preventing lock contention cascades and transaction rollback deadlocks.
6. **Declarative Task Runner (`sqlfox job run`)**: Executes structured JSON automation pipelines (`sqlfox-job.json`) sequentially across `health-check`, `dump`, `transfer`, and `query` tasks with fail-fast error handling and environment variable expansion (`$VAR`/`${VAR}`).
7. **Automated Dump Rotation & Retention**: The `dump` task automatically prunes expired backup files based on `keep_last` count and `max_age` duration using prefix pattern matching, preventing disk exhaustion without running a daemon.
8. **Pre-Flight Job Check (`sqlfox job check`)**: Non-destructive dry-run inspection validating job syntax, target connectivity, script file existence, and output directory writability before cron deployment.

---

## 13. Large-Scale Hardening, Memory Ceiling & Resilience Engine

To ensure unwavering reliability when operating against massive production databases (1,000+ tables, 50,000+ columns, and multi-gigabyte datasets), SqlFox enforces strict systemic boundaries:

```mermaid
flowchart TD
    subgraph EnterpriseLoad ["Enterprise Scale Workload"]
        Tbls["1,000+ Tables / 50k Columns"]
        Rows["1,000,000+ Streaming Records"]
        Disconnects["Transient Network Drops / Restarts"]
        PoolSat["Target Connection Pool Saturation"]
    end

    subgraph Defenses ["SqlFox Hardening Subsystems"]
        VTree["Content Visibility Virtualization (CSS content-visibility: auto + 150ms Debounce)"]
        StreamBudget["Bounded Chunk Buffers (<100MB RAM RSS Enforcement)"]
        ErrBoundary["Global Error Boundary (Exponential Backoff + State Retention)"]
        FastCancel["Fail-Fast Control Channel (3s Timeout + Instant Client Stream Abort)"]
        PprofGuard["Loopback-Guarded pprof (SQLFOX_PPROF=1)"]
    end

    Tbls --> VTree
    Rows --> StreamBudget
    Disconnects --> ErrBoundary
    PoolSat --> FastCancel
    StreamBudget --> PprofGuard
```

1. **Synthetic Schema Benchmark Suite (`internal/driver/benchmark_test.go`)**:
   - Tiered regression harness testing 100 tables / 5,000 columns in standard unit tests (<200ms) and 1,000 tables / 50,000 columns with FK cycles in `BenchmarkLargeSchema` (~25ms/op, 2 allocs/op).
   - `BenchmarkMemoryCeilingStreaming` verifies that streaming 1,000,000 rows through chunked buffers strictly preserves the <100MB RAM budget.
2. **Browser-Native Content Visibility Virtualization (`web/dist/index.html`)**:
   - Table and view elements utilize native CSS `content-visibility: auto; contain-intrinsic-size: 0 28px;` to skip layout and paint phases for off-screen objects.
   - Schema search input (`filterSchemaTree`) is wrapped in a 150ms debounce timer to prevent DOM churn on rapid typing.
3. **Loopback-Guarded pprof Profiling (`internal/server/pprof.go`)**:
   - Diagnostic endpoints (`/debug/pprof/*`) are mounted dynamically only when `SQLFOX_PPROF=1` is exported.
   - All profiling requests are gated by `isLoopback()`, returning `403 Forbidden` to non-loopback clients.
4. **Fail-Fast Control Channel (`internal/driver/`, `internal/server/handlers_query.go`)**:
   - Out-of-band cancellation (`KILL QUERY` / `pg_cancel_backend`) executes with a strict 3-second context timeout.
   - If target connection slots are saturated, client-side RowStream reading aborts immediately, and the API returns `504 Gateway Timeout` with clear pool exhaustion diagnostics.
5. **Global Web UI Error Boundary (`web/dist/index.html`)**:
   - Captures network and server drop events during API fetches and displays a non-blocking floating banner (`#global-reconnect-banner`).
   - Automatically polls `/api/auth/status` using exponential backoff (1s, 2s, 4s, 8s) and recovers seamlessly while preserving active editor tabs, unsaved queries, and staged grid edits.
6. **Ponytail Zero-Regression Pruning**:
   - Single release binary size verified at **18MB** (stripped with `-ldflags="-s -w"`), well below the 25MB ceiling.
   - Startup time benchmarks at **0.008s (8ms)**, fulfilling the sub-second launch promise.


