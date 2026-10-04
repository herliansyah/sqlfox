# Spec: In-App Self-Update Engine, Changelog Viewer & Workspace Author Attribution

Status: resolved
Driver: herliansyah

## Context & Objectives
Provide users with an effortless, transparent, single-click update mechanism directly within the SqlFox embedded web application. Allow users to review release notes and changelogs fetched from GitHub Releases, stream platform-specific binary archives, perform atomic in-place replacement, and trigger a graceful restart with automatic client reconnection. In addition, cleanly credit the author and link the GitHub repository in the status bar footer.

## Core Deliverables
1. **Backend Update Engine (`internal/server/update.go`)**:
   - `GET /api/system/update/check?force=...`: Fetches latest release metadata from GitHub, performs semver comparison, caches response for 24h, and maps runtime OS/Arch to target assets (`.tar.gz` / `.zip`).
   - `POST /api/system/update/apply`: Streams release archive, extracts binary using Go standard library (`archive/tar`, `compress/gzip`, `archive/zip`), validates permissions, executes atomic in-place replacement with Windows `.old` rename fallback, and triggers background graceful re-exec.
2. **Frontend UI & Changelog Modal (`web/dist/index.html`)**:
   - Status bar footer: Displays "Created by herliansyah" with links to author GitHub profile and official repository.
   - Dynamic update badge: Highlights available updates in the footer and Command Palette.
   - Native Update Dialog (`#update-dialog`): Renders full Markdown changelog, version transition, and live download/restart progress states with 3-second auto-reconnection countdown.
3. **Verification**:
   - 100% test coverage in `internal/server/update_test.go`.
