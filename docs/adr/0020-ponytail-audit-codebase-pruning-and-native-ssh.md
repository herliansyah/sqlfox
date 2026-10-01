# 0020: Ponytail Audit, Codebase Pruning, and Native Platform Tunneling

To maintain extreme simplicity, minimize attack surface, and prevent code bloat, SqlFox executes an exhaustive Lazy Senior Developer (Ponytail) audit to remove speculative features, unreferenced error sentinels, and dead ad-hoc scripts.

## Context & Problem

Over multiple development iterations, exploratory and speculative code accumulated:
1. **Speculative In-Process SSH Tunneling**: An in-process SSH tunnel client (`internal/driver/mysql/ssh_dialer.go`) and associated encrypted vault storage columns (`use_ssh`, `ssh_host`, `ssh_port`, `ssh_user`, `encrypted_ssh_password`, `encrypted_ssh_key`) were implemented in the backend, but never exposed in the UI or requested by users. This added redundant encryption/decryption layers, complex dialer registrations, and pulled in cryptographic dependencies (`golang.org/x/crypto/ssh`).
2. **Ad-Hoc Throwaway Test Scripts**: 15 standalone Node.js scripts in `scripts/` (~1,030 lines) were created during past manual DOM validation sessions. None were wired into package.json, npm scripts, or CI pipelines, cluttering the repository.
3. **Unused Sentinels and Manifests**: `ErrConnectionFailed` and `ErrQueryCancelled` remained unreferenced in `internal/driver/driver.go`. An empty `package-lock.json` remained despite `package.json` having zero dependencies.

## Decision

1. **Delete Dead Ad-Hoc Scripts**:
   - Completely remove the `scripts/` directory (15 files, -1,033 lines).
   - Reaffirm standard Go test suite (`go test -v ./...`) as the canonical automated test suite.
2. **Eliminate Speculative In-Process SSH Tunneling (`native`)**:
   - Remove `internal/driver/mysql/ssh_dialer.go`.
   - Remove SSH tunnel branching from `internal/driver/mysql/mysql.go` and restore direct `tcp` dialing.
   - Remove SSH configuration fields from `driver.Config`.
   - Remove SSH columns, encryption, and decryption from `internal/vault/vault.go`.
   - Rely on native platform capabilities (e.g. `ssh -N -L 3306:remote-db:3306 user@bastion` or platform VPN) for bastion access, which is standard practice for developer desktop tooling.
3. **Delete Unused Error Sentinels & Empty Lockfile**:
   - Remove `ErrConnectionFailed` and `ErrQueryCancelled` from `internal/driver/driver.go`.
   - Remove `package-lock.json`.
4. **Prune Dependencies**:
   - Run `go mod tidy` to clean module graph.

## Consequences

- Net reduction of over 1,400 lines of dead and speculative code.
- Significantly simpler vault database schema and zero SSH-tunnel maintenance overhead.
- Cleaner repository root and test structure.
- 100% of existing functionality, security guards, and unit tests continue to pass without regression.
