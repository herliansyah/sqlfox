# 0047: Single-Instance Guard and Lock Management

## Status
Accepted (Amends startup behavior in ADR-0007)

## Context
ADR-0007 established automatic linear port hunting (probing ports sequentially from 8080 to 8130) to prevent port collisions from crashing the application upon startup when another service occupies port 8080.

However, in desktop and local developer environments:
1. Re-running `sqlfox` (or double-clicking the executable while SqlFox is already running) previously triggered port hunting, binding a second or third instance to ports 8081, 8082, etc.
2. Multiple instances running concurrently against the same local configuration vault (`~/.sqlfox/config.db`) produced redundant memory overhead and confusing browser tab proliferation.
3. Users expect single-instance desktop application behavior: launching SqlFox when an instance is already active should simply surface/open the running instance in the browser and terminate the new launch process immediately without spawning duplicate servers on other ports.

## Decisions

### 1. State File & Loopback Health Verification
- On successful server startup, SqlFox writes an instance state lock to `~/.sqlfox/sqlfox.lock` (or `$CONFIG_DIR/sqlfox.lock`), containing the active PID, bound port, local URL, and start timestamp.
- When `sqlfox` starts, before opening the vault or hunting ports, it checks `sqlfox.lock`.
- If the lock exists, it issues a fast loopback HTTP probe (`GET http://127.0.0.1:<port>/api/auth/status` with a 350ms timeout) to verify that the recorded instance is actively running and responding as SqlFox.
- If verified active:
  - SqlFox logs: `ℹ️  SqlFox is already running at <url> (PID <pid>)`.
  - Unless `-no-browser` is specified, it opens the running instance's URL in the system browser.
  - The new process immediately exits with code 0 without opening the database or hunting ports.
- If stale (the previous process terminated abruptly or was killed without clean shutdown):
  - The stale lock file is automatically purged and normal startup proceeds.

### 2. Direct Default Port Probe Fallback
- If `sqlfox.lock` is missing (e.g. manually deleted), SqlFox also probes the target port directly before port hunting. If an existing SqlFox instance responds on that port, it activates the single-instance handover instead of jumping to the next port.

### 3. Graceful Lock Cleanup
- On process shutdown (`SIGINT`, `SIGTERM`), the lock file is cleanly removed.

### 4. Opt-Out Multi-Instance Flag
- For automated testing or advanced workflows requiring concurrent instances, operators can provide the `-multi` flag (or `SQLFOX_MULTI_INSTANCE=1` environment variable) to bypass the single-instance guard.

## Consequences
- Launching SqlFox multiple times on the same machine safely opens the active dashboard without opening redundant ports.
- Zero extra dependencies; implemented entirely using Go standard library primitives.
- Stale locks from hard crashes or power failures are automatically resolved with zero user intervention.
