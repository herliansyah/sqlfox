# 0036: In-App Self-Update Engine, Changelog Viewer & Author Attribution

## Status
Accepted

## Context
SqlFox is distributed as a lightweight, single-binary application without external package managers or auto-update background daemons. As users run SqlFox across diverse environments (Linux, macOS, Windows) and architectures (amd64, arm64), updating the application previously required manually downloading release tarballs/zips from GitHub Releases and replacing the binary via terminal.
Additionally:
1. **Discoverability of New Releases**: Users lacked visibility into new features, performance improvements, and critical safety patches unless they actively monitored the GitHub repository.
2. **Changelog Transparency**: Users need to review detailed release notes/changelogs before deciding to update, ensuring breaking changes or new behaviors are understood.
3. **Zero-Bloat In-App Replacement**: Updating must be executable directly from the web UI with a single click, operating purely on Go standard libraries (`archive/tar`, `compress/gzip`, `archive/zip`) without introducing external third-party dependencies.
4. **Platform Safety & Windows Executable Locking**: In Windows, a running `.exe` cannot be overwritten in place; it must be renamed before writing the replacement binary. On Linux/macOS, atomic replacement via `os.Rename` with proper executable permissions (`0755`) is required.
5. **Rate-Limiting Protection**: GitHub's unauthenticated API limit (60 requests/hour per IP) requires 24-hour client-side/server-side caching for automatic checks while preserving an on-demand manual check trigger.
6. **Workspace Identity & Author Attribution**: The status bar footer should cleanly credit the creator (`herliansyah`) with links to GitHub and the official repository without disrupting existing connection latency and memory telemetry.

## Decisions

### 1. Zero-Dependency Update Engine
A dedicated package/module in `internal/server/update.go` handles GitHub Releases interaction:
- **API Check (`GET /api/system/update/check`)**: Fetches `https://api.github.com/repos/herliansyah/sqlfox/releases/latest`. Compares semver against `main.Version`. Resolves target archive based on `runtime.GOOS` and `runtime.GOARCH` following the convention `sqlfox-{TAG}-{GOOS}-{GOARCH}.(tar.gz|zip)`.
- **Cache Guard**: Responses are cached in memory for 24 hours to prevent GitHub API rate-limit exhaustion during routine browser usage. A `?force=true` query parameter bypasses the cache on manual request.
- **In-App Binary Replacement (`POST /api/system/update/apply`)**: Streams the target release archive directly, extracts the `sqlfox` / `sqlfox.exe` binary to a sibling temporary file, validates permissions, and performs atomic replacement:
  - **Linux / macOS**: Atomic `os.Rename(tempBinary, currentBinary)` followed by `os.Chmod(currentBinary, 0755)`.
  - **Windows**: Renames `sqlfox.exe` to `sqlfox.exe.old`, writes the new `sqlfox.exe`, and schedules removal of the old executable.
- **Graceful Self-Restart & Test Seam**: Dispatches background re-execution of the newly updated binary using `os.StartProcess` with original arguments, followed by graceful shutdown of the current server process. For testing environments, an injectable `restartFn` seam dispatches immediately in a goroutine, synchronized deterministically via channel primitives in `update_test.go` rather than fragile `time.Sleep` windows to prevent timing flakes on heavily loaded Windows CI runners.

### 2. Native HTML5 Update Modal with Rich Markdown Changelog
- An `#update-dialog` modal presents the release name, published timestamp, version transition (`vCurrent` -> `vLatest`), and full Markdown-rendered release notes using SqlFox's built-in client-side markdown formatter.
- Displays animated progress states (`Downloading...`, `Extracting...`, `Restarting...`) with error handling for permission issues.
- The web UI conducts a countdown reload after restart confirmation to automatically reconnect to the upgraded binary.

### 3. Clean Two-Region Status Bar, Author Attribution & Update Indicator
- **Footer Left**: Houses complete database runtime telemetry (engine flavor & version, connection target, server uptime, and interactive ping latency) with dynamic space allocation and truncation guards.
- **Footer Right**: Houses the unified utility cluster: heap memory telemetry, keyboard shortcuts cheatsheet launcher, dynamic update action badge (`🚀 Update`), persistent application version label (`SqlFox v...`), and terminates with author attribution (`Created by herliansyah • GitHub • ☕ Traktir Kopi`) at the far-right edge. When a new release is detected, the update badge highlights next to the version, launching the Update Modal upon click.
- **Command Palette & Shortcuts**: "Check for Updates" is integrated into the Command Palette (`Ctrl+K` / `Cmd+K`) and keyboard shortcuts modal for quick access.

## Consequences
- Users can update SqlFox seamlessly with 1 click directly inside the web UI without leaving their browser or executing terminal commands.
- Transparent changelogs allow users to verify release notes before applying updates.
- Adheres strictly to Ponytail principles: uses Go standard libraries exclusively, adds zero external dependencies, and maintains <100MB RAM footprint.
- Test suites run sub-millisecond and deterministically across all operating systems (Linux, macOS, and Windows) without timing race conditions.
