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

## 9. Directory & Package Structure

```
sqlfox/
├── cmd/
│   └── sqlfox/
│       ├── main.go                 # Entry point: flags, localhost listener, port hunting, browser launcher
│       └── main_test.go
├── internal/
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
│   │   ├── mysql/                  # MySQL 5.7/8.x, PXC, MariaDB with Galera cluster inspection & fail-fast DDL
│   │   │   ├── mysql.go
│   │   │   └── cluster_test.go     # Tri-metric cluster bottleneck heuristics test
│   │   └── sqlite/                 # modernc.org/sqlite CGo-free pure-Go driver with FK inspection
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
│       ├── flight_recorder.go      # Flight recorder background controller & auto-stop lifecycle
│       ├── handlers_flight_recorder.go # Flight recorder REST API endpoints
│       ├── handlers_connections.go # Connection CRUD & live testing
│       ├── handlers_query.go       # SQL execution & cancellation
│       ├── handlers_schema.go      # Schema tree, DDL, preflight DDL, pushdown filtering
│       ├── handlers_dba.go         # Processlist, cluster health, diagnostics, privilege matrix
│       ├── handlers_snippets.go    # Snippet management
│       ├── handlers_import.go      # CSV upload & import endpoints
│       └── handlers_ai.go          # AI chat & assistance streaming
├── web/                            # Single-file zero-build SPA embedded into binary
│   ├── dist/                       # index.html + vendor assets (CodeMirror, Tailwind)
│   │   ├── vendor/
│   │   └── index.html
│   └── embed.go                    # //go:embed dist/*
├── docs/
│   ├── adr/                        # Architectural Decision Records (0001 - 0026)
│   ├── playbooks/                  # Production operational playbooks (large-scale database management)
│   └── agents/                     # Agent workflow documentation
├── CONTEXT.md                      # Ubiquitous domain language
├── ARCHITECTURE.md                 # System architecture (this document)
├── SECURITY.md                     # Security & privacy specification
├── UX_DESIGN.md                    # Interface wireframes & design tokens
├── MILESTONES.md                   # Delivery roadmap & milestones (1 - 19)
└── go.mod
```
