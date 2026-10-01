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

## 6. Directory & Package Structure

```
sqlfox/
├── cmd/
│   └── sqlfox/
│       ├── main.go                 # Entry point: flags, localhost listener, port hunting, browser launcher
│       └── main_test.go
├── internal/
│   ├── vault/                      # SQLite config storage & AES-256-GCM credential encryption
│   │   ├── vault.go                # Connections, master password, audit log
│   │   ├── crypto.go               # Argon2id key derivation & AES-256-GCM
│   │   ├── snippets.go             # Parameterized SQL snippets
│   │   ├── notes.go                # Table documentation notes
│   │   └── export.go               # .sqlfox-vault portable archive export/import
│   ├── driver/                     # Database driver abstraction
│   │   ├── driver.go               # Core interfaces (Driver, Session, RowStream)
│   │   ├── rowstream.go            # Canonical chunked row streaming
│   │   ├── redundant_index.go      # Shadowed / duplicate index detection
│   │   ├── mysql/                  # go-sql-driver/mysql pure-Go driver
│   │   └── sqlite/                 # modernc.org/sqlite CGo-free pure-Go driver
│   ├── safety/                     # Safety Guard engine
│   │   ├── guard.go                # AST inspection & destructive operation detection
│   │   └── backup.go               # Smart tiered pre-destruction table snapshot manager
│   ├── ai/                         # AI Assistant integration
│   │   ├── client.go               # Pure stdlib client (OpenAI, Claude, Gemini, Ollama)
│   │   ├── firewall.go             # Zero-leakage schema pruner & data masking
│   │   └── prompts.go              # Structured prompts for explain, optimize, NL-to-SQL
│   ├── dataimport/                 # CSV Ingestion Engine
│   │   └── importer.go             # Delimiter sniffing, column mapping, batched streaming
│   └── server/                     # HTTP transport & handlers
│       ├── server.go               # HTTP routing & SPA static file serving
│       ├── auth.go                 # Remote Access Guard & LAN tablet token auth
│       ├── session_manager.go      # Active database connection pool & session state
│       ├── handlers_connections.go # Connection CRUD & live testing
│       ├── handlers_query.go       # SQL execution & cancellation
│       ├── handlers_schema.go      # Schema tree, DDL, table rows, pushdown filtering
│       ├── handlers_dba.go         # Processlist, kill query, diagnostics, privilege matrix
│       ├── handlers_snippets.go    # Snippet management
│       ├── handlers_import.go      # CSV upload & import endpoints
│       └── handlers_ai.go          # AI chat & assistance streaming
├── web/                            # Single-file zero-build SPA embedded into binary
│   ├── dist/                       # index.html + vendor assets (CodeMirror, Tailwind)
│   │   ├── vendor/
│   │   └── index.html
│   └── embed.go                    # //go:embed dist/*
├── docs/
│   ├── adr/                        # Architectural Decision Records (0001 - 0020)
│   └── agents/                     # Agent workflow documentation
├── CONTEXT.md                      # Ubiquitous domain language
├── ARCHITECTURE.md                 # System architecture (this document)
├── SECURITY.md                     # Security & privacy specification
├── UX_DESIGN.md                    # Interface wireframes & design tokens
├── MILESTONES.md                   # Delivery roadmap & milestones (1 - 14)
└── go.mod
```
