## Agent skills

### Issue tracker

Local markdown files in `.scratch/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Five canonical triage roles (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context (`CONTEXT.md` + `docs/adr/`). See `docs/agents/domain.md`.

### Release routine

Deterministic release workflow when user commands a release:
1. Scan git commits since the last tag (`git log $(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")..HEAD --oneline`).
2. Curate and organize changes into `CHANGELOG.md` under `## [Unreleased]` following Keep a Changelog taxonomy (`Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`).
3. Synchronize `CHANGELOG.md` to `web/dist/CHANGELOG.md`.
4. Confirm target version with human operator (e.g. `patch`, `minor`, `major`, or `vX.Y.Z`).
5. Execute `./scripts/release.sh <version> --push` to run pre-flight unit tests, commit `chore(release): vX.Y.Z`, create annotated tag, and push to GitHub to trigger multi-OS binary builds.

One-time prerequisite (update signing): to enable the self-update supply-chain trust anchor, the Ed25519 manifest signing keypair must be provisioned once (GitHub secret `SQLFOX_MANIFEST_SIGNING_KEY` + repo variable `SQLFOX_MANIFEST_SIGNING_PUBKEY`). See `docs/playbooks/release-signing-key-provisioning.md`. Until provisioned, releases build and ship with the legacy checksum-only update behavior.

