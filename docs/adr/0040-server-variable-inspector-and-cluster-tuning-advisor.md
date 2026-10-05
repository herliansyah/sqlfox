# 0040: Server Variable Inspector and Cluster Tuning Advisor

To address the opacity and complexity of database configuration tuning, SqlFox introduces a unified, offline-first Server Variable Inspector alongside a specialized Cluster Parameter Advisor embedded in the Cluster & Replication Inspector.

## Context and Problem Statement

1. **Cognitive Burden of Engine Parameters**: MySQL exposes 600+ global system variables (`SHOW GLOBAL VARIABLES`), while PostgreSQL exposes hundreds of `pg_settings`. Navigating these raw key-value lists leaves developers and DBAs without context regarding:
   - What the variable actually does.
   - What happens when set too low vs too high (trade-offs and operational risks).
   - What baseline formulas or values are recommended for production stability.
2. **Cluster & Replication Fragility**: In Galera (PXC/MariaDB) or replica topologies, misconfiguring specific variables (e.g. `innodb_autoinc_lock_mode != 2`, `binlog_format != ROW`, or under-provisioning `wsrep_slave_threads`) directly triggers node desynchronization, transaction certification deadlocks, or cluster-wide write stalls. DBAs need immediate visibility into whether their cluster parameters adhere to best practices without digging through manual documentation.
3. **Safety and Mutation Risks**: Executing runtime mutations (`SET GLOBAL`) directly from a GUI without restart awareness or caution poses extreme risks to production stability (such as sudden OOM or buffer thrashing). Safe operational workflows require actionable guidance with copy-pasteable snippets rather than uncontrolled automatic mutations.

## Considered Options

- **Option 1: Raw Status & Variables Table Only**: Display an uncurated key-value grid for `SHOW GLOBAL VARIABLES`. Rejected: offers zero guidance, context, or tuning value to the user.
- **Option 2: Mandatory AI Chatbot Lookup for Every Variable**: Query an external LLM for every inspected variable. Rejected: incurs network latency, requires active API credentials, fails when offline, and is non-deterministic.
- **Option 3: Curated Offline Heuristic Engine with AI Copilot Bridge and Cluster Guardrails**:
  - **Server Variable Inspector**: A dedicated monitor tab querying all live global variables with search, subsystem category filtering (`Memory`, `Concurrency`, `Cluster & WSREP`, `Performance & IO`), and curated cards for 60+ critical variables.
  - **Tuning Advisor Cards**: Explanations of variable purpose, low vs high setting trade-offs, baseline formulas, and 1-click `my.cnf` / `SET GLOBAL` snippet copying.
  - **Cluster Parameter Advisor**: A dedicated guardrails card inside the Cluster & Replication Inspector that automatically checks cluster-critical variables against best practices and displays traffic-light status badges (🟢 Optimal, 🟡 Suboptimal, 🔴 Critical).
  - **Copilot Handoff**: A 1-click "Ask Copilot" button to seamlessly transfer structured queries to the internal SqlFox Copilot drawer for deeper workload-specific recommendations.

## Decision

Adopt **Option 3**.

### Architecture & Driver Seam

1. **Driver Seam**:
   - `InspectServerVariables(ctx context.Context) (*ServerVariablesReport, error)` in `driver.Session`.
   - MySQL driver introspects `SHOW GLOBAL VARIABLES` and pairs them with curated rule definitions and live statuses.
   - Cluster inspector extends existing `InspectClusterHealth` to evaluate cluster guardrail parameters.
2. **Heuristic Catalog**:
   - An embedded, zero-dependency catalog covering memory, concurrency, cluster/Galera, binary logs, and I/O durability.
3. **User Experience**:
   - Monitor menu entry: `⚙️ Server Variables`.
   - Cluster view extension: `🛡️ Cluster Tuning Guardrails` card.

## Consequences

- DBAs and engineers can quickly audit server configuration and understand exact trade-offs before modifying parameters.
- Cluster and replication misconfigurations are detected immediately before causing cluster outages.
- Operates 100% offline with zero external dependencies, with an optional AI Copilot bridge for complex workloads.
