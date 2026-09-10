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

## 5. Directory & Package Structure

```
sqlfox/
├── cmd/
│   └── sqlfox/
│       └── main.go                 # Entry point: flags, localhost listener, browser launcher
├── internal/
│   ├── app/                        # Application container & dependency wiring
│   ├── vault/                      # SQLite config storage & AES-256-GCM credential encryption
│   │   ├── vault.go
│   │   ├── crypto.go               # Argon2id key derivation & AES-GCM
│   │   └── export.go               # .sqlfox-vault portable archive export/import
│   ├── driver/                     # Database driver abstraction
│   │   ├── driver.go               # Core interfaces (Driver, Session, RowStream)
│   │   ├── mysql/                  # go-sql-driver/mysql implementation + SSH tunnel dialer
│   │   └── sqlite/                 # modernc.org/sqlite pure-Go implementation
│   ├── safety/                     # Safety Guard engine
│   │   ├── guard.go                # Safety policy orchestrator
│   │   ├── parser.go               # TiDB SQL AST parser integration
│   │   ├── rules.go                # Destructive pattern evaluators
│   │   └── backup.go               # Smart tiered pre-DDL backup manager
│   ├── ai/                         # AI Assistant integration
│   │   ├── client.go               # Generic client (OpenAI, Claude, Gemini, Ollama)
│   │   ├── firewall.go             # Schema filter, data masking & prompt builder
│   │   └── prompts.go              # Structured prompts for explain, plan, optimize
│   └── server/                     # HTTP and WebSocket transport
│       ├── server.go               # Chi router setup & static file embedding
│       ├── handlers_connection.go  # Connection CRUD & test
│       ├── handlers_query.go       # Query execution & WS streaming
│       ├── handlers_schema.go      # Schema tree & DDL retrieval
│       ├── handlers_ai.go          # AI chat & explanation endpoints
│       └── ws.go                   # WebSocket streaming hub
├── web/                            # Frontend source (Svelte / Vue 3 + Tailwind)
│   ├── src/
│   │   ├── components/
│   │   │   ├── editor/             # CodeMirror 6 SQL editor & autocompleter
│   │   │   ├── grid/               # Virtualized data grid with inline edit
│   │   │   ├── schema/             # Database tree viewer
│   │   │   ├── modal/              # Challenge modals & impact previews
│   │   │   └── ai/                 # AI chat sidebar
│   │   ├── stores/                 # State management (tabs, active connection)
│   │   └── App.vue / App.svelte
│   ├── dist/                       # Compiled assets embedded via //go:embed
│   ├── embed.go                    # //go:embed dist/*
│   └── package.json
├── docs/
│   ├── adr/                        # Architectural Decision Records
│   └── agents/                     # Agent workflow documentation
├── CONTEXT.md                      # Ubiquitous domain language
├── ARCHITECTURE.md                 # System architecture (this document)
├── SECURITY.md                     # Security & privacy specification
├── UX_DESIGN.md                    # Interface wireframes & design tokens
├── MILESTONES.md                   # Delivery roadmap & MVPs
└── go.mod
```
