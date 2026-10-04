# Issue 01: In-App Self-Update Engine, Changelog Viewer & Workspace Author Attribution

Status: resolved
Type: task
Assignee: herliansyah

## Question
How to enable zero-dependency in-app updates with transparent changelogs and author attribution without violating the single-binary architecture?

## Deliverables
- [x] Backend update endpoints (`/api/system/update/check` & `/api/system/update/apply`) with 24h rate-limit caching.
- [x] Pure Go standard library archive streaming and extraction (`tar.gz` and `zip`).
- [x] Atomic binary replacement with Windows `.exe.old` lock handling.
- [x] Native HTML5 Update Modal with rendered Markdown changelog and countdown reconnection.
- [x] Status bar footer with creator attribution (`Created by herliansyah`) and GitHub repo link.
- [x] Integration tests in `internal/server/update_test.go` and documentation in ADR-0036.
- [x] Hardened `TestHandleApplyUpdate` with channel synchronization (`select` / `time.After`) to eliminate Windows CI timing flakes and data races.

## Answer
Implemented via `internal/server/update.go` and `web/dist/index.html`. Uses Go stdlib exclusively with zero new dependencies. Verified via `go test -v ./internal/server -run TestUpdate` and all tests passing 100% across all OS platforms (Linux, macOS, and Windows) with zero flakiness.
