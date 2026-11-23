# 0064: Multi-Profile AI Copilot and GitHub Vault Synchronization

## Status
Accepted

## Context
Prior to this architectural enhancement, SqlFox provided in-context AI Copilot assistance (BYOK - Bring Your Own Key) for SQL query generation, plan tuning, and error healing. However, the configuration model had several architectural constraints:
1. **Single-Profile Ceiling**: Only a single provider and model could be configured at a time. Users frequently needed to switch between providers (e.g., Anthropic Claude 3.5 Sonnet for deep optimization, OpenAI GPT-4o Mini for fast auto-completes, Google Gemini for wide contexts, and local Ollama for zero-cost offline development). Switching required manually re-entering keys, model names, endpoints, and custom instructions.
2. **Ephemeral Browser-Local Storage**: Settings were persisted exclusively inside browser `localStorage` (`sqlfox_ai_config`). As a result, configuration did not exist on the Go daemon backend, could not be accessed across different browsers/profiles without retyping, and was completely isolated from SqlFox's encrypted SQLite database (`production_store.db`).
3. **Absence of Cloud Replication**: SqlFox includes a Git-backed GitHub Cloud Sync engine (`internal/vault/cloud_sync.go`) for operational notes, table documentation, and SQL snippets. However, AI Copilot configurations were excluded from sync, forcing users to reconfigure BYOK keys and prompts on every device or developer machine.

## Decisions

### 1. Backend SQLite Vault Persistence (`internal/vault/ai_profiles.go`)
- Created a dedicated `ai_profiles` table in the SQLite vault:
  ```sql
  CREATE TABLE IF NOT EXISTS ai_profiles (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      provider TEXT NOT NULL,
      encrypted_api_key TEXT DEFAULT '',
      base_url TEXT DEFAULT '',
      model_name TEXT NOT NULL,
      temperature REAL DEFAULT 0.3,
      language TEXT DEFAULT 'auto',
      response_style TEXT DEFAULT 'normal',
      custom_prompt TEXT DEFAULT '',
      include_schema INTEGER DEFAULT 1,
      mask_identifiers INTEGER DEFAULT 1,
      is_active INTEGER DEFAULT 0,
      created_at TIMESTAMP NOT NULL,
      updated_at TIMESTAMP NOT NULL
  );
  CREATE INDEX IF NOT EXISTS idx_ai_profiles_active ON ai_profiles(is_active DESC, updated_at DESC);
  ```
- **Seeded Default**: Bootstraps an initial `default-openai` profile if the table is empty.
- **Envelope Encryption**: API keys are encrypted at rest with AES-256-GCM using the master vault key and domain-separated AAD (`ai_profile:<id>:api_key`) adhering to ADR-0060.
- **Active State Atomicity**: Setting a profile as active updates the active flag atomically in a transaction.
- **Delete Safeguards**: Enforces a minimum of 1 profile; deleting the active profile automatically reassigns active status to the next available profile.

### 2. Full REST CRUD API Handlers (`internal/server/handlers_ai.go`)
- Exposed endpoints under `/api/ai/profiles`:
  - `GET /api/ai/profiles`: Lists all profiles with active indicators (`has_api_key` boolean; raw API keys masked).
  - `GET /api/ai/profiles/{id}`: Retrieves profile details with decrypted API key for editing.
  - `POST /api/ai/profiles`: Creates a new profile and triggers debounced GitHub Cloud Sync.
  - `PUT /api/ai/profiles/{id}`: Updates an existing profile.
  - `DELETE /api/ai/profiles/{id}`: Deletes a profile with last-profile protection.
  - `POST /api/ai/profiles/{id}/active`: Atomically activates a profile.
- **Transparent Chat Fallback**: In `handleAIChat`, if `api_key` or `provider` is not explicitly provided, the backend automatically resolves and decrypts credentials from the currently active vault profile (or specific `profile_id` if provided).

### 3. Hybrid GitHub Cloud Sync & Secret Protection (`internal/vault/bundle.go`, `cloud_sync.go`, `export.go`)
- **Declarative Git Export**: `ExportNotesFiles` exports AI profiles as individual JSON files under `ai-profiles/<id>-<slug>.json`. Field `api_key` is intentionally stripped/redacted to guarantee zero secret leakage into Git commit history.
- **Path Whitelisting**: Added `strings.HasPrefix(p, "ai-profiles/")` to `isSyncManagedPath` so Git sync synchronizes profile definitions.
- **Encrypted Vault Archive**: Added `AIProfiles []*AIProfile` to `ExportPayload` so the encrypted backup (`vault-backup.sqlfox-vault` / `.sqlfox-vault`) securely carries full credentials encrypted with the master passphrase.
- **Preserve Local Secrets**: On import/pull from non-encrypted Git payloads, incoming profiles with empty keys never overwrite existing valid local keys.

### 4. Dual-Entry UI & Quick-Switch Ergonomics (`AiDrawer.svelte`, `AiSettingsPanel.svelte`)
- **Copilot Header Quick Switcher**: Added an active profile select dropdown directly in the AI Copilot drawer header (`AiDrawer.svelte`), enabling one-click switching between models (e.g., Claude, GPT-4o, Ollama) while authoring queries.
- **Full CRUD Profile Manager**: `AiSettingsPanel.svelte` supports creating, editing, testing, deleting, and activating profiles.
- **Automatic Legacy Migration**: Detects pre-existing `localStorage` configurations (`sqlfox_ai_config`) on launch and transparently migrates them into the active vault profile before retiring the local storage key.

## Consequences

### Positive
- Users can maintain unlimited customized AI profiles (different models, prompts, temperature, and privacy settings) and switch instantly.
- Sensitive API keys are securely encrypted at rest inside SqlFox Vault rather than plaintext browser localStorage.
- AI profiles sync seamlessly to GitHub private repositories without exposing plain API keys in git history.
- Backward compatibility is 100% preserved through transparent auto-migration.
