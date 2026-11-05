# 0060: Supply-Chain Signing, Script-Runner Safety Parity, and Vault/Ingress Crypto Hardening

## Status
Accepted

## Context
A pre-release enterprise security audit (following ADR 0059) surfaced seven findings across the self-update supply chain, the Safe Execution Pipeline, network ingress, at-rest crypto, and test-only code paths that had leaked into production builds:

1. **Safe Execution Pipeline Bypass in the Large SQL Runner**: `handleRunSQLScript` executed each parsed statement directly via `sess.Session.Exec`, never invoking the AST Safety Guard, Challenge Modal, or pre-DDL Smart Tiered Backup. An uploaded/pasted script containing `DROP DATABASE`, `TRUNCATE`, or an unqualified `DELETE`/`UPDATE` ran with no confirmation and no backup — contradicting the product's "Safe Execution Pipeline across all client transports" guarantee.
2. **Unsigned Checksum Manifest (Supply Chain)**: The self-updater fetched `checksums.txt` from the same GitHub release origin as the binary, with no cryptographic signature. The integrity model was purely "same TLS origin serves both artifacts," which defends against network tampering but not against a compromised or malicious GitHub release (an attacker who can publish both a malicious binary and a matching checksum defeats it).
3. **Test-Only `updateAPIURL` Host Allowance in `isAllowedAssetURL`**: A fallback clause accepted any asset whose host matched `s.updateAPIURL`. Although never set in production builds, it was a test-only backdoor pattern in the supply-chain allowlist that could be re-enabled by accident.
4. **`testing` Package Linked into Production**: `internal/server/auth.go` imported `testing` and called `testing.Testing()` to permit the httptest default Host (`example.com`) through the DNS Rebinding guard. Though `testing.Testing()` is false in release binaries, linking the testing package into production is an audit red flag.
5. **Loose DNS Rebinding Guard in LAN Mode**: When `--lan` was enabled, `isHostAllowed` accepted *any* IP-literal `Host` header, including routable public IPs — weakening rebinding protection beyond the intended LAN scope.
6. **Keyset Pagination Value Formatted, Not Bound**: The `last_key` seek marker was spliced into the `WHERE` clause via `EscapeSQLString` rather than a bind parameter. Dialect-correct escaping made it non-exploitable, but it was the one grid value path relying on escaping, and non-MySQL escaping does not neutralize NUL bytes.
7. **AES-256-GCM Without AAD in the Vault**: Connection passwords and SSL private keys were sealed with nil additional-authenticated-data, so an actor with raw vault write access could relocate one field's ciphertext into another field/connection and have it still decrypt.

## Decisions

### 1. Script-Runner Safety Guard Parity
- In `internal/server/handlers_dump.go`, `handleRunSQLScript` runs a Safety Guard pre-pass (`ActiveSession.InspectScriptStatements`) over every statement. Any statement that `RequiresChallenge` (unqualified `DELETE`/`UPDATE`, `DROP`, `TRUNCATE`, `DROP DATABASE`, CREATE/ALTER in Production, or a schema-scope mismatch) blocks the whole run with `HTTP 409 Conflict`, reporting the offending statements. The operator proceeds only via an explicit `?confirm=true`.
- Before each confirmed `DROP`/`TRUNCATE TABLE`, `ActiveSession.BackupBeforeDestructive` triggers the same pre-DDL Smart Tiered Backup as the editor; a qualified target outside the active database is skipped (not snapshotted against the wrong database) with a logged warning.

### 2. Ed25519 Signed Checksum Manifest (Supply-Chain Trust Anchor)
- In `internal/server/update.go`, `verifyAssetChecksum` calls `verifyManifestSignature` before trusting any checksum. When a build embeds a public key (`manifestSigningPublicKeyB64`, injected via `-ldflags`), it fetches a detached Ed25519 signature (`checksums.txt.sig`, then `SHA256SUMS.sig`) and requires it to verify against the exact manifest bytes. Verification is fail-closed; a missing/malformed/non-verifying signature aborts the update.
- Unprovisioned builds (empty embedded key) fall back to the legacy same-origin checksum behavior, so development builds keep working.
- Releases are signed by a CGO-free, stdlib-only helper (`scripts/sign-manifest.go`, build-tagged `ignore`). The GitHub Actions release workflow embeds the public key at build time and signs the manifest with a private key held only in CI secrets. Operational provisioning is documented in `docs/playbooks/release-signing-key-provisioning.md`.

### 3. Test-Only Flags Removed from Production Logic
- `isAllowedAssetURL`'s `updateAPIURL` host-match clause is gated behind the package-level `allowUpdateAPIHostAssets` flag (false in production, set true only by the test harness's `init`). A production build can never accept a non-`github.com` asset host.
- `auth.go` no longer imports `testing`; the `example.com` DNS-rebinding bypass is gated behind the package-level `allowDefaultTestHost` flag, set true only by the server test binary's `init`.

### 4. LAN-Scoped DNS Rebinding Guard
- In `internal/server/auth.go`, `isHostAllowed` delegates IP-literal hosts in LAN mode to `isAllowedLANHostIP`, which accepts only the detected LAN IP (`s.lanIP`) plus loopback, RFC 1918 private, RFC 4193 unique-local, and link-local ranges. A public IP in the `Host` header is treated as a rebinding signal and rejected.

### 5. Keyset Marker Trust-Boundary Validation
- In `internal/server/handlers_schema.go`, `last_key` is rejected at the trust boundary if it contains a NUL byte or exceeds 4096 bytes, in addition to the existing dialect-correct `EscapeSQLString`. The `QueryStream` driver interface intentionally remains unchanged (no bind-parameter channel) to avoid a broad, higher-risk refactor across all drivers; the NUL guard closes the only residual concern on an already non-exploitable path.

### 6. Per-Field AAD Binding for Vault Secrets
- In `internal/vault/crypto.go`, `EncryptWithAAD`/`DecryptWithAAD` accept additional authenticated data; `Encrypt`/`Decrypt` remain as nil-AAD wrappers. `DecryptWithAAD` falls back to nil-AAD on authentication failure for backward compatibility with legacy ciphertext.
- In `internal/vault/vault.go`, connection passwords and SSL keys are sealed with AAD bound to `(connection ID, field)` via `connFieldAAD` (`sqlfox:connection:<id>:password` / `:ssl_key`). Relocating a ciphertext to another field/connection fails authentication. Values predating AAD are re-sealed on their next save.

## Consequences

### Positive
- **No Safe Execution Pipeline Bypass**: Destructive scripts are gated and backed up identically to editor execution.
- **Real Supply-Chain Integrity**: A signed manifest binds updates to an offline key, defending against a compromised/tampered GitHub release, not merely network MITM.
- **Clean Production Surface**: No `testing` package or test-only backdoor clauses linked into release binaries.
- **Tighter Ingress in LAN Mode**: Public-IP Host headers are rejected, narrowing the DNS-rebinding surface.
- **Ciphertext-Swap Resistance**: AAD binding ties each secret to its field identity with zero migration friction.

### Negative & Trade-Offs
- The Large SQL Runner now requires `confirm=true` for destructive scripts, adding one explicit step for power users running intentional destructive batches.
- Signed updates require maintainers to provision and safeguard an Ed25519 keypair (runbook provided). Unprovisioned builds silently retain legacy checksum-only behavior until keys are configured.
- Keyset pagination still formats `last_key` as an escaped literal rather than a bind parameter; a full placeholder refactor was deliberately deferred to keep the driver interface stable.

## Verification
- New tests: `TestRunSQLScript_SafetyGuardGate`, `TestVerifyAssetChecksum_Ed25519ManifestSignature`, `TestIsAllowedAssetURL_ProductionIgnoresUpdateAPIHost`, `TestDNSRebindingGuardLANHostScoping`, keyset + NUL cases in `TestHandleGetTableRowsWithFilter`, `TestCrypto_AADBindingPreventsFieldSwap`, `TestCrypto_LegacyNoAADFallback`, `TestVault_PasswordCiphertextSwapRejected`.
- Full `go build ./...`, `go vet ./...`, and `go test ./...` pass.
