# 0006: Smart Tiered Backup for Destructive DDL

To avoid multi-hour freezes or disk saturation when dropping large tables, SqlFox enforces automated DDL backups and bounds full-data backups to tables under 50MB.

## Considered Options

- **Unbounded Auto-Dump**: Freezes the client and saturates local disk/remote network when dropping large tables (>50GB).
- **No Automatic Backup**: Leaves users completely vulnerable to accidental loss.
- **Smart Tiered Backup**: Automatically backs up schema DDL (`SHOW CREATE TABLE`) regardless of size; performs data dump only if table size is under 50MB (<100,000 rows). For tables exceeding 50MB, data dump is skipped with an explicit warning requiring a manual name-typing challenge.

## Consequences

Fast, zero-hang safety guarantees for smaller tables while preventing catastrophic client-side freezing on large tables.
