# SqlFox Domain Glossary

Core ubiquitous language for the SqlFox database management tool.

## Connections & Environment

**Connection**:
A configured target database endpoint containing host, network, protocol (direct or SSH tunnel), and authentication credentials.
_Avoid_: Profile, host config, server alias

**Environment**:
The operational trust tier assigned to a Connection (`Production`, `Staging`, `Development`) that dictates the enforcement level of the Safety Guard.
_Avoid_: Stage, label, group

**Connection Vault**:
The encrypted local SQLite storage holding connection metadata and credentials.
_Avoid_: Credential manager, config file

**Vault Archive**:
An encrypted, portable export file (`.sqlfox-vault`) protected by an export passphrase for cross-machine migration.
_Avoid_: Backup zip, cred dump

**Remote Access Guard**:
A security gate intercepting HTTP and WebSocket requests from non-loopback IP addresses, requiring Master Password authentication before granting access to the vault and sessions.
_Avoid_: Firewall, IP filter

**Port Hunting**:
An automated sequential port probing mechanism that resolves `EADDRINUSE` port collision errors by scanning subsequent free ports.
_Avoid_: Port hopping, auto-port

**Network Endpoint**:
The non-loopback IPv4 network address of the host machine accessible from other devices on the local area network.
_Avoid_: Public URL, remote host

## Drivers & Sessions

**Driver**:
A protocol-specific database connector (e.g. MySQL, SQLite) implementing lifecycle and session creation.
_Avoid_: Adapter, DB client

**Session**:
An active client interaction context bound to a Connection, managing open tabs, temporary schema cache, and transaction state.
_Avoid_: Workspace, window, console

**RowStream**:
A memory-bounded record sequence streamed from database drivers to the frontend via WebSocket/SSE chunks.
_Avoid_: Result buffer, fetch array

**Out-of-Band Cancellation**:
A query interruption mechanism dispatched via a secondary control connection issuing protocol termination (`KILL QUERY`) without destroying the primary connection.
_Avoid_: Context cancel, socket kill

## Safety & Governance

**Safety Guard**:
The deterministic AST analysis gate that intercepts and evaluates SQL statements prior to execution against the target database.
_Avoid_: Linter, query validator, firewall

**Destructive Query**:
A SQL statement capable of permanently altering or removing schema objects or mass row data without explicit targeting (e.g., unqualified DELETE/UPDATE, DROP, TRUNCATE).
_Avoid_: Bad query, invalid SQL, dangerous command

**Challenge Modal**:
A mandatory confirmation dialog requiring explicit typing of target identifiers before executing high-risk operations.
_Avoid_: Alert box, confirm prompt

**Implicit Safe Limit**:
A bounded row restriction automatically applied to arbitrary user queries in the editor to prevent client-side and server-side memory exhaustion.
_Avoid_: Pagination cap, auto-limit

**Keyset Pagination**:
A stateful table viewing traversal mechanism that seeks records using indexed sequential keys rather than ordinal offsets.
_Avoid_: Cursor scan, row offset

**Impact Preview**:
A non-intrusive metadata and execution plan evaluation (`EXPLAIN`) displaying estimated affected rows and scan types prior to execution.
_Avoid_: Dry run, query check

**Smart Tiered Backup**:
An automated pre-DDL backup policy that captures DDL always and table data conditionally based on a 50MB size threshold.
_Avoid_: Auto-dump, full backup

## Artificial Intelligence

**BYOK (Bring Your Own Key)**:
Direct client-side configuration and local storage of user-provided AI provider credentials, bypassing any intermediary server.
_Avoid_: API proxy, SaaS key

**Schema Context Window**:
The curated, just-in-time subset of database DDL metadata supplied to AI prompts based on active queries or explicit user references.
_Avoid_: Full schema dump, DB prompt
