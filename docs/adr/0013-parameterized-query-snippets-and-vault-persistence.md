# 0013: Parameterized Query Snippets and Vault Persistence

To enhance daily query productivity and enable team collaboration without query errors, SqlFox introduces local Vault-persisted Query Snippets supporting dynamic substitution variables (`{{name:default}}`), native modal parameter prompting, and CodeMirror autocompletion integration.

## Problem

1. Developers frequently rewrite repetitive diagnostic and analytical queries (e.g. searching duplicate records, checking table sizes, filtering tenant data).
2. Existing workarounds (copy-pasting from external notes or scratchpads) introduce syntax errors, incomplete placeholder replacements, and accidental execution of untargeted statements.
3. Users switching machines or sharing configurations lacked a clean, portable way to carry parameterized query templates alongside connection vaults.

## Solution

1. **Vault Persistence & Hybrid Scoping**:
   - Stored in a new `snippets` table inside local SQLite `config.db`.
   - Each snippet contains `id`, `title`, `sql_content`, `tags` (optional connection/engine scoping, e.g. `mysql`, `sqlite`, or global), `created_at`, and `updated_at`.
   - Seeded on initial vault setup with common productivity snippets (Find Duplicates, Active Queries, Table Sizes).
   - Included automatically in encrypted Vault Archive export/import (`.sqlfox-vault`).

2. **Dynamic Variable Syntax (`{{name:default}}`)**:
   - Lightweight regex token parser (`/\{\{([a-zA-Z0-9_]+)(?::([^}]*))?\}\}/g`).
   - Supports simple tokens (`{{tenant_id}}`) and tokens with default values (`{{limit:50}}`, `{{status:'active'}}`).

3. **Snippet Parameter Modal & Live Preview**:
   - When a snippet is invoked, a native `<dialog>` modal opens if dynamic variables are detected.
   - Dynamically renders clean inputs with pre-populated default values and live SQL preview.
   - Directly injects the final substituted SQL into the active CodeMirror Query Editor upon confirmation.

4. **Dual Trigger Autocompletion & Snippets Drawer**:
   - Integrated into the CodeMirror autocompletion provider with badge `[SNP]`.
   - Accessible via general title search or dedicated `snip:` prefix.
   - Includes a dedicated "Snippets" button in the Query Editor toolbar for creating, editing, and managing snippets.

## Consequences

- Zero-dependency client-side token parsing and live preview prevent malformed SQL execution.
- Safety Guard continues to deterministically evaluate substituted SQL prior to execution.
- Snippets persist locally and travel safely across machines via encrypted Vault Archives.
