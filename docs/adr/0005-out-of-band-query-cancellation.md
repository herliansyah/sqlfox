# 0005: Out-of-Band Query Cancellation

To prevent socket destruction and orphaned background server churn during user cancellation, SqlFox cancels active queries via out-of-band protocol commands rather than standard `context.WithCancel` socket termination.

## Considered Options

- **Standard `context.WithCancel`**: Closes the underlying client socket. In MySQL, the server often continues executing the query in the background, wasting server resources, and client session variables are lost.
- **Out-of-Band Thread Cancellation**: Tracks `(SessionID, QueryID) -> ThreadID` mapping. When cancellation is triggered, a secondary pool connection issues `KILL QUERY <ThreadID>` directly to MySQL, or invokes `sqlite3_interrupt` for SQLite.

## Consequences

Target database servers immediately halt intensive queries without losing the primary client connection or tearing down the session context.
