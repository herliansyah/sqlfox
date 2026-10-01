# 0023: Dynamic Cluster & Replication Inspector with Peer Node Auto-Discovery and Tri-Metric Bottleneck Detective

To address real-time operational troubleshooting in clustered environments (Percona XtraDB Cluster 5.7/8.0, Galera Cluster, MariaDB Galera) and replicated MySQL architectures without vendor lock-in or heavyweight monitoring daemons, SqlFox introduces a dynamic, engine-extensible Cluster & Replication Inspector.

## Context and Problem Statement

1. **Slowest-Link Vulnerability in Galera/PXC**: In multi-master synchronous replication architectures (Galera / PXC), cluster write throughput is strictly bounded by the slowest node. When a single node encounters write stall or queue saturation, it triggers Flow Control (`wsrep_flow_control_paused`), which temporarily freezes writes across all nodes in the cluster.
2. **Ambiguity of Slowness ("Lambat Kenapa")**: When a DBA experiences cluster stalls, identifying the root cause manually requires correlating multiple obscure status variables (`wsrep_flow_control_paused`, `wsrep_local_recv_queue`, `wsrep_local_cert_failures`, `wsrep_evs_repl_latency`, and InnoDB storage wait states). DBAs cannot easily determine whether slowness originates from disk I/O, network latency, worker thread exhaustion, or optimistic lock collisions.
3. **Peer Node Blame Attribution**: While a connected node knows flow control is active (`wsrep_flow_control_recv > 0`), it cannot pinpoint from local status alone which peer node is the bottleneck holding back the cluster.
4. **Generalization Requirement**: SqlFox must avoid hardcoding Percona-only logic; standalone MySQL/MariaDB instances and standard Source-Replica setups must benefit from the same telemetry interface with graceful degradation.

## Considered Options

- **Option 1: Static Raw Status Explorer**: Provide a filtered list of `SHOW GLOBAL STATUS LIKE 'wsrep%'`. Rejected: places the entire cognitive burden on the DBA and fails to answer *why* the cluster is slow or which node is the culprit.
- **Option 2: Heavyweight Daemon Metric Exporter**: Run an internal background time-series collector storing continuous metrics in SQLite. Rejected: violates Ponytail rules (unnecessary complexity, exceeds < 100MB RAM ceiling, causes continuous database background queries).
- **Option 3: Dynamic Cluster & Replication Inspector with Peer Node Auto-Discovery and Tri-Metric Bottleneck Detective**:
  - **Dynamic Topology Detection**: Automatically identifies Galera/PXC clustering, standard MySQL replication, or standalone instances.
  - **Peer Node Auto-Discovery**: Extracts active cluster IPs from `wsrep_incoming_addresses` and conducts bounded parallel health probes (1.5s timeout) to pinpoint the slowest peer node.
  - **Tri-Metric Bottleneck Matrix**: Analyzes network latency (`wsrep_evs_repl_latency`), disk I/O wait stalls (InnoDB buffer pool waits and fsync latencies), and apply worker saturation (`wsrep_slave_threads`).
  - **Actionable Remediation**: Provides one-click shortcuts to inspect processlists on the bottleneck node, switch active sessions, or request tuning guidance from AI Copilot.
  - **Graceful Degradation**: Seamlessly adapts to standard replication lag counters (`Seconds_Behind_Master`) or standalone engine health when Galera is absent.

## Decision

Adopt **Option 3**.

### Architecture & Driver Seam

1. **Driver Seam**:
   - `InspectClusterHealth(ctx context.Context) (*ClusterHealthReport, error)` added to `driver.Session`.
2. **Telemetry Structure**:
   - Cluster metadata: Cluster Name, Size, Status (Primary/Non-Primary), Node State (Synced/Donor/Joiner).
   - Flow Control: Paused percentage, sent vs received pause events.
   - Bottleneck diagnosis: Rule-based evaluation classifying the primary bottleneck (Storage I/O, Network Latency, Apply Thread Contention, or Certification Conflicts).
   - Node Roster: Comparative list of active nodes with receive queues and culprit highlighting.
3. **UI Integration**:
   - Dedicated tab (`🌐 Cluster`) in the main workspace with active auto-refresh ticker (1s/3s/5s) and tab-visibility pause.

## Consequences

- DBAs can immediately pinpoint the culprit node and hardware bottleneck during cluster stalls.
- Zero external monitoring agent required; operates entirely within single-binary SqlFox.
- Full compatibility preserved across Galera/PXC, standard MySQL/MariaDB replication, and standalone servers.
