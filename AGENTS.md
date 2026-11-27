## Agent skills

### Issue tracker

Local markdown files in `.scratch/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Five canonical triage roles (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context (`CONTEXT.md` + `docs/adr/`). See `docs/agents/domain.md`.

### Build & run

`web/embed.go` embeds `web/dist/`, and Vite outputs directly to `web/dist/`. Whenever `web/frontend/` changes, always run `npm run build:svelte` (or `npm run build:all`) from the repo root before running or rebuilding `./sqlfox`.

### Release routine

Deterministic release workflow when user commands a release:
1. Scan git commits since the last tag (`git log $(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")..HEAD --oneline`).
2. Curate and organize changes into `CHANGELOG.md` under `## [Unreleased]` following Keep a Changelog taxonomy (`Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`).
3. Synchronize `CHANGELOG.md` to `web/dist/CHANGELOG.md`.
4. Confirm target version with human operator (e.g. `patch`, `minor`, `major`, or `vX.Y.Z`).
5. Execute `./scripts/release.sh <version> --push` to run pre-flight unit tests, commit `chore(release): vX.Y.Z`, create annotated tag, and push to GitHub to trigger multi-OS binary builds.

Release signing (self-update trust anchor): GitHub secret `SQLFOX_MANIFEST_SIGNING_KEY` and repo variable `SQLFOX_MANIFEST_SIGNING_PUBKEY` are provisioned. Releases embed the public key, sign `checksums.txt`, and fail if either setting is missing; unsigned checksum-only fallback applies only to local/dev builds without the key. Rotation: `docs/playbooks/release-signing-key-provisioning.md`.

