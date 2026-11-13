# 0061: Modular Frontend Pipeline, Ingress TLS Listener, and Safety Guard Hardening

## Status
Accepted

## Context
Following an in-depth architectural and code critique of SqlFox, four systemic areas required decisive engineering hardening while maintaining the zero-CGO, single-binary distribution invariant:

1. **Extreme Frontend Monolithic Technical Debt**:
   The embedded user interface had grown to over 30,000 lines in a single file (`web/dist/index.html`), mixing HTML skeletons, dozens of modal dialogs, and over 24,000 lines of state, grid, query, and visualization logic. This created merge conflicts, made code navigation cumbersome, and hindered domain separation.
2. **Client-Side CSS Compilation Overhead**:
   The application relied on Tailwind CSS Play CDN (`tailwind.min.js`, 398 KB) at runtime. The browser had to download, parse, and execute the CSS compiler on every page load, triggering client-side layout shifts (CLS) and consuming unnecessary CPU/RAM.
3. **Plaintext Ingress Vulnerability in LAN Mode**:
   When `--lan` was enabled to permit tablet and secondary workstation access, HTTP traffic (including database credentials entered into new connection forms, query results, and Master PIN tokens) was transmitted in cleartext across local networks unless placed behind an external reverse proxy.
4. **Safety Guard Multi-Statement Bypass & Heavy Probe Scans**:
   Operators chaining multiple SQL statements separated by semicolons (e.g., `SELECT 1; DROP TABLE users;`) could bypass the lexical Safety Guard if the leading statement was benign. Additionally, mutation impact estimation on unqualified queries performed a heavy `SELECT COUNT(*)` on un-indexed tables, creating risk of production pool starvation.
5. **AI Privacy Firewall Default Posture**:
   Identifier masking for AI Copilot schema context defaulted to opt-in rather than opt-out, risking accidental schema exposure if an operator did not explicitly activate masking.

## Decisions

### 1. Modular Frontend Source Tree & Validation Pipeline
> **Superseded (Svelte 5 migration)**: `web/src/` has been retired. The frontend now lives in
> `web/frontend/` (Svelte 5 runes + TypeScript) built via `scripts/build_web.js` → `npm run build:svelte`
> (Vite + Tailwind). See the migration notes in `MIGRATION_CHECKLIST.md`.

- Decompose `web/dist/index.html` into a structured source tree under `web/src/`:
  - `web/src/index.html`: Clean HTML structure, semantic layout, and dialog shells with an injection anchor `<!-- @INJECT:SCRIPTS -->`.
  - `web/src/js/`: 15 domain JavaScript modules (`01_core_state.js` through `15_system_lifecycle.js`).
- Introduce `scripts/build_web.js` and `npm run build:web` / `npm run build`:
  - Automatically compiles static CSS via Tailwind CLI.
  - Concatenates the 15 JavaScript domain modules.
  - Verifies combined JavaScript syntax in-memory via Node's `vm.Script` before writing to disk.
  - Replaces `<!-- @INJECT:SCRIPTS -->` using a replacer function to avoid `$pattern` expansion bugs with SQL dollar quotes (`$'`).
  - Outputs the self-contained bundle directly to `web/dist/index.html`, preserving the single-binary `go:embed` asset distribution.
- Synchronize `scripts/release.sh` to update versions across `web/src/index.html`, `package.json`, and rebuild `web/dist/index.html`.

### 2. Pre-Compiled Tailwind CSS
- Replace runtime Tailwind Play CDN (`tailwind.min.js`, 398 KB) with a pre-compiled, minified stylesheet (`web/dist/vendor/tailwind/tailwind.css`, 85 KB).
- Configure `scripts/tailwind.config.js` to scan both `web/src/index.html` and `web/src/js/**/*.js` for class token purge.
- Eliminates browser-side runtime compilation, saves >78% in asset transfer, and eliminates layout shifts.

### 3. Ingress TLS/HTTPS Listener
- Add `internal/server/tls.go` providing an in-memory self-signed ECDSA P-256 certificate generator with Subject Alternative Names (SANs) covering `localhost`, `127.0.0.1`, `::1`, and the detected LAN IP.
- Add CLI flags `-tls`, `-tls-cert`, `-tls-key`, and environment variable `SQLFOX_TLS=1`.
- Update `internal/cli/banner.go` to display HTTPS protocol URLs and notify operators of auto-generated certificates.
- Support both plain HTTP and HTTPS modes, enabling zero-config encryption for tablet and remote LAN access without requiring CGO or external reverse proxies.

### 4. Safety Guard Multi-Statement AST Split & Row Count Cache
- In `internal/safety/guard.go`, implement `inspectMultiStatements` to pre-split semicolon-delimited SQL scripts prior to AST and lexical inspection, ensuring every sub-statement is evaluated against destructive operation rules.
- Support bracket-quoted identifiers (`[...]`) in SQL sanitization and token masking.
- In `internal/server/session_manager.go`, query mutation risk estimation checks cached schema row counts before issuing expensive `SELECT COUNT(*)` probes, preventing table locks and CPU spikes on high-cardinality production tables.

### 5. AI Privacy Firewall Secure-by-Default
- Update `internal/server/handlers_ai.go` to treat `MaskIdentifiers` as a pointer `*bool` defaulting to `true` switch.
- Ensure identifier masking is active out-of-the-box unless explicitly unchecked by the user.

## Consequences

### Positive
- **Maintainability**: Frontend code is now cleanly organized into focused, manageable modules with automated syntax checking at build time.
- **Performance**: Zero runtime CSS compilation; client load time and memory usage are substantially reduced.
- **LAN Security**: Remote database management can be fully encrypted over HTTPS with zero external dependencies.
- **Safety Parity**: Chained statements and bracket identifiers can no longer circumvent the Safety Guard challenge.
- **Zero-CGO Invariant Intact**: All Go code remains 100% pure Go (`CGO_ENABLED=0`), cross-compiling seamlessly for Linux, macOS, and Windows.

### Negative & Trade-Offs
- Developers editing the frontend UI must run `npm run build` or `npm run build:web` before building the Go binary or running Go tests.
- Browsers accessing an auto-generated self-signed TLS certificate will display an untrusted certificate warning on first connection until accepted or configured with custom certificates via `--tls-cert` and `--tls-key`.

## Verification
- Unit test suite: `go test ./...` passes across all packages (`cmd/sqlfox`, `internal/cli`, `internal/safety`, `internal/server`, `web`).
- Embedded frontend test harness: All 69 tests in `web/embed_test.go` pass against the generated `web/dist/index.html`.
- Self-signed TLS SAN validation and TLS handshake tested live under `internal/server/tls_test.go`.
