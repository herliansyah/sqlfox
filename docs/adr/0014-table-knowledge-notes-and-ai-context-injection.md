# 0014: Table Knowledge Notes and AI Context Injection

To preserve institutional business logic, query runbooks, and domain constraints without modifying production database schemas or relying on server-side table comments, SqlFox introduces local Vault-persisted Table Knowledge Notes with automatic AI Copilot prompt context injection.

## Problem

1. Production database schemas often lack documentation, or restricted permissions prevent developers from executing `ALTER TABLE ... COMMENT`.
2. Critical business rules (e.g. "always filter `is_deleted = 0`", "tenant `test_corp` is deprecated", "amounts in cents, not dollars") live in scattered external wikis, leading to faulty ad-hoc queries.
3. AI Copilots generate syntactically valid but logically flawed queries because they only see raw column names and types without domain business rules.

## Solution

1. **Vault Persistence per Target Table**:
   - Stored in a new `table_notes` table in the local SQLite `config.db`.
   - Keyed by `(connection_id, database_name, table_name)`.
   - Safe and isolated: zero server-side schema modification or permission requirements.
   - Included in portable encrypted Vault Archives (`.sqlfox-vault`) and downloadable as standalone `.md` files.

2. **Table Designer Sub-Tab (`[📝 Notes & Runbook]`)**:
   - Added as a 6th sub-tab alongside `[Data]`, `[Structure & PK]`, `[Indexes]`, `[Foreign Keys]`, and `[CREATE Code]`.
   - Provides a Markdown editor with live preview toggle (`[Edit]` / `[Preview]`), shortcut save (`Ctrl+S`), and export to `.md`.

3. **Just-In-Time AI Copilot Context Injection**:
   - When users query or request AI assistance on a table, active Table Knowledge Notes are retrieved and injected into the prompt's Schema Context Window under a structured `[BUSINESS RULES & TABLE NOTES]` section.
   - Zero row-data leakage: only markdown guidelines and schema metadata reach the AI model.

## Consequences

- Institutional database knowledge stays close to where queries are written and executed.
- AI Copilot SQL generation aligns with specific team business rules.
- Runbooks and notes travel seamlessly with developer connection vaults.
