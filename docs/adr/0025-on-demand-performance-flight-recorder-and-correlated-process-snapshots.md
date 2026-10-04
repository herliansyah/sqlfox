# 0025: On-Demand Performance Flight Recorder and Correlated Process Snapshots

To empower developers and DBAs to isolate database performance degradation, transient latency spikes, and root-cause queries without running heavy 24/7 APM daemons, SqlFox introduces the On-Demand Performance Flight Recorder.

## Context and Problem Statement

1. **Transient Bottleneck Blindspot ("Kenapa Tiba-Tiba Lambat?")**: Developers often encounter transient performance regressions or sudden query stalls. Standard static status screens (`SHOW GLOBAL STATUS` or `Processlist`) show only a single point in time, failing to capture the dynamic progression of a spike.
2. **Heavyweight APM Dilemma**: Traditional monitoring suites (Prometheus, Datadog, Percona PMM) require persistent background agent daemons, external metric collectors, and large disk footprints. Running continuous background telemetry violates SqlFox's core ethos (< 100MB RAM ceiling, zero external bloat, single-binary portable architecture).
3. **Missing Telemetry $\leftrightarrow$ Query Linkage**: Existing monitoring tools frequently show disconnected time-series charts (e.g. QPS drop, lock wait spike) without linking the specific point on the graph to the exact SQL queries running at that precise second.

## Considered Options

- **Option 1: Continuous Background Telemetry Daemon**: Continuously poll database status and store time-series metrics in SQLite 24/7. Rejected: consumes ongoing background CPU/network resources on target databases, inflates local SQLite database size, and violates Ponytail zero-bloat principles.
- **Option 2: Browser In-Memory Sliding Buffer**: Sample metrics only while the browser tab is open, holding data purely in JavaScript memory. Rejected: data is lost upon tab reload or accidental browser navigation, and cannot be exported or reviewed across investigation sessions.
- **Option 3: On-Demand Performance Flight Recorder with Adaptive Correlated Process Snapshots**:
  - **Explicit Lifecycle**: User manually starts (`● Record`) and stops the recorder. A global navbar indicator (`● REC 00:02:15`) keeps state visible across all tabs.
  - **Auto-Stop Safety Guard**: Automatically halts recording after 10 minutes to protect database servers from indefinite polling.
  - **Quad-Metric Telemetry**: Samples 4 critical diagnostic vectors per tick (Throughput/QPS, Concurrency/Threads Running, Contention/Row Lock Waits, Storage/Buffer Pool Waits).
  - **Adaptive Correlated Process Snapshots**: Captures non-idle active threads (`Command != 'Sleep'` with `Time >= 1s` or `Threads_running > 1`, bounded to Top-5 queries per second) directly correlated to the timeline timestamp.
  - **Local Vault Storage & Auto-Purge**: Persisted in local SQLite tables (`flight_recorder_sessions`, `flight_recorder_ticks`) with an automatic 24-hour retention cleanup.
  - **Zero-Dependency Native HTML5 Canvas Visualization**: Interactive time-series line chart with crosshair scrub and click-to-inspect drawer featuring standard SqlFox query actions (`⚡ Explain`, `📝 To Tab`, `🤖 AI Copilot Draft`, `📋 Copy SQL`).

## Decision

Adopt **Option 3**.

### Architecture & Driver Seam

1. **Driver Seam**:
   - `SampleFlightMetrics(ctx context.Context) (*FlightMetricSample, error)` added to `driver.Session`.
   - MySQL evaluates `SHOW GLOBAL STATUS WHERE Variable_name IN (...)` and queries active non-idle threads from `information_schema.PROCESSLIST` (Top 5).
   - SQLite provides clean standalone fallback telemetry.
2. **Storage Architecture (`internal/vault/flight_recorder.go`)**:
   - `flight_recorder_sessions`: Tracks recording lifecycle, metadata, connection ID, duration, and tick counts.
   - `flight_recorder_ticks`: Stores per-second telemetry records alongside serialized Top-5 query snapshots.
   - Auto-purge routine pruning sessions older than 24 hours.
3. **Backend Service (`internal/server/handlers_flight_recorder.go`)**:
   - REST endpoints:
     - `POST /api/session/{id}/flight-recorder/start`
     - `POST /api/session/{id}/flight-recorder/stop`
     - `GET /api/session/{id}/flight-recorder/status`
     - `GET /api/session/{id}/flight-recorder/sessions`
     - `GET /api/session/{id}/flight-recorder/sessions/{recId}`
     - `DELETE /api/session/{id}/flight-recorder/sessions/{recId}`
4. **Frontend Architecture (`web/dist/index.html`)**:
   - Global navbar toggle button and active pulsing badge.
   - Dedicated workspace tab (`📈 Flight Recorder`).
   - Native HTML5 Canvas chart renderer supporting multi-series display, synchronous crosshair tracking, and point selection.
   - Correlated query drill-down drawer with seamless query handoff actions.

## Consequences

- Zero-daemon operation: SqlFox creates zero background noise on database servers when not recording.
- Instant incident diagnosis: DBAs and developers can click any spike on the chart and immediately see the culpable SQL statements that caused the regression.
- Local storage safety: Fixed 10-minute auto-stop ceiling and 24-hour auto-purge ensure disk usage remains bounded under a few megabytes.
