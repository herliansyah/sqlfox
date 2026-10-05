package advisor

import (
	"fmt"
	"strconv"
	"strings"

	"sqlfox/internal/driver"
)

// VariableRule contains curated metadata and evaluation heuristics for a database configuration parameter.
type VariableRule struct {
	Name             string
	Category         string
	Scope            string
	IsDynamic        bool
	Description      string
	ImpactLow        string
	ImpactHigh       string
	Recommendation   string
	DefaultSuggested string
	Evaluate         func(val string, allVars map[string]string) (severity, suggested, summary string)
}

// MySQLCuratedRules defines expert tuning rules for critical MySQL/MariaDB and Galera cluster parameters.
var MySQLCuratedRules = map[string]VariableRule{
	// --- Cluster & WSREP ---
	"innodb_autoinc_lock_mode": {
		Name:             "innodb_autoinc_lock_mode",
		Category:         "Cluster & WSREP",
		Scope:            "Global",
		IsDynamic:        false,
		Description:      "Controls the lock mode used for generating auto-increment values. Critical for Galera/PXC multi-master clusters.",
		ImpactLow:        "Mode 0 (traditional) or 1 (consecutive) locks table auto-inc mutex across transactions, creating deadlocks and severe stalls in multi-master Galera replication.",
		ImpactHigh:       "Mode 2 (interleaved) does not guarantee gapless auto-inc IDs for multi-row INSERTs, but provides maximum concurrency.",
		Recommendation:   "Must be set to 2 (interleaved) for Galera Cluster. Non-cluster environments typically use 1 or 2.",
		DefaultSuggested: "2",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			if val == "2" {
				return "optimal", "2", "Interleaved lock mode is optimal for Galera clusters and high-concurrency writes."
			}
			return "critical", "2", "innodb_autoinc_lock_mode is not 2. In Galera clusters, modes 0 and 1 cause severe deadlocks and certification stalls."
		},
	},
	"binlog_format": {
		Name:             "binlog_format",
		Category:         "Cluster & WSREP",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Format of the binary log. Determines whether SQL statements or row change images are recorded.",
		ImpactLow:        "STATEMENT format records SQL text which can cause non-deterministic replica drift (e.g. NOW(), UUID(), LIMIT without ORDER BY). Unusable in Galera.",
		ImpactHigh:       "ROW format increases binlog disk space and network bandwidth, but guarantees exact row byte matching across all replicas.",
		Recommendation:   "Must be ROW for Galera/PXC and modern GTID replication.",
		DefaultSuggested: "ROW",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			if strings.EqualFold(val, "ROW") {
				return "optimal", "ROW", "ROW format guarantees deterministic replica state and is required for Galera."
			}
			return "critical", "ROW", fmt.Sprintf("binlog_format is %s. Galera cluster requires ROW format; STATEMENT or MIXED will cause desynchronization.", val)
		},
	},
	"wsrep_slave_threads": {
		Name:             "wsrep_slave_threads",
		Category:         "Cluster & WSREP",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Number of parallel applier worker threads processing incoming cluster write sets.",
		ImpactLow:        "Too low (e.g. 1) creates single-threaded applier bottlenecks, causing write set backlog and triggering Flow Control pause.",
		ImpactHigh:       "Too high can cause CPU context switching overhead, lock aborts, and certification collisions between parallel appliers.",
		Recommendation:   "Baseline recommendation is 2x to 4x CPU cores (typically between 4 and 16).",
		DefaultSuggested: "4",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n <= 1 {
				return "warning", "4", "wsrep_slave_threads is 1. Single-threaded apply can cause write backlog and trigger cluster Flow Control."
			}
			if n > 64 {
				return "warning", "16", "wsrep_slave_threads is very high (>64). May cause worker CPU churn and local certification aborts."
			}
			return "optimal", val, fmt.Sprintf("%d parallel applier threads configured.", n)
		},
	},
	"wsrep_sync_wait": {
		Name:             "wsrep_sync_wait",
		Category:         "Cluster & WSREP",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Bitmask for causal read consistency. Forces read queries to wait until all pending write sets are applied.",
		ImpactLow:        "0 provides fastest reads with no wait overhead, but read queries might see stale data if the local node is slightly lagging.",
		ImpactHigh:       "Non-zero (e.g. 1 for READs, 7 for all) ensures zero stale reads, but adds latency to queries if the node is processing write sets.",
		Recommendation:   "Set to 0 for general high-throughput reads; set to 1 on read-your-own-writes sensitive sessions.",
		DefaultSuggested: "0",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n == 0 {
				return "optimal", "0", "Causal read wait disabled (fastest read latency; minor replica lag possible)."
			}
			return "info", val, fmt.Sprintf("Causal sync wait enabled (mask %d). Ensures strict fresh reads at cost of query latency.", n)
		},
	},
	"wsrep_max_ws_rows": {
		Name:             "wsrep_max_ws_rows",
		Category:         "Cluster & WSREP",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Maximum number of rows permitted in a single cluster replication write set.",
		ImpactLow:        "Too low (e.g. <10,000) causes legitimate batch operations and bulk migrations to abort with cluster error.",
		ImpactHigh:       "0 (unlimited) allows massive transactions to overwhelm memory and cause cluster-wide multi-second stall.",
		Recommendation:   "Typically set between 64,000 and 256,000 to prevent runaway batch queries from freezing peer nodes.",
		DefaultSuggested: "131072",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n == 0 {
				return "warning", "131072", "No row ceiling configured (0). Unbounded transactions risk exhausting cluster memory."
			}
			return "optimal", val, fmt.Sprintf("Bounded to %d rows per write set.", n)
		},
	},
	"wsrep_max_ws_size": {
		Name:             "wsrep_max_ws_size",
		Category:         "Cluster & WSREP",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Maximum total payload size in bytes permitted for a replication write set.",
		ImpactLow:        "Too small causes large row inserts or updates to fail.",
		ImpactHigh:       "Values > 2GB (or unbounded) risk OOM crashes on nodes when replicating huge transactions.",
		Recommendation:   "Recommended 1,073,741,824 (1GB) or 2,147,483,647 (2GB).",
		DefaultSuggested: "1073741824",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.ParseInt(val, 10, 64)
			if n > 2147483648 {
				return "warning", "1073741824", "Payload limit exceeds 2GB. May cause memory pressure during large transaction replication."
			}
			return "optimal", val, fmt.Sprintf("Maximum write set size bounded to %d bytes.", n)
		},
	},
	"wsrep_retry_autocommit": {
		Name:             "wsrep_retry_autocommit",
		Category:         "Cluster & WSREP",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Number of times to automatically retry autocommit queries upon Galera certification conflict.",
		ImpactLow:        "0 immediately bubbles deadlock error (1213) to client on transient multi-master collision.",
		ImpactHigh:       "High values (>10) can hide application contention and delay error propagation.",
		Recommendation:   "Recommended 1 to 3 to transparently absorb transient certification race conditions.",
		DefaultSuggested: "3",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n == 0 {
				return "warning", "3", "Auto-retry is 0. Certification conflicts immediately throw deadlock errors to clients."
			}
			return "optimal", val, fmt.Sprintf("Retries %d times on transient certification failure.", n)
		},
	},

	// --- Memory & Buffer ---
	"innodb_buffer_pool_size": {
		Name:             "innodb_buffer_pool_size",
		Category:         "Memory & Buffer",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Size of the memory buffer InnoDB uses to cache table and index data.",
		ImpactLow:        "Too low causes heavy disk read I/O, slow query execution, and high IOPS wait states.",
		ImpactHigh:       "Too high (>80% of host RAM) causes OS page swapping and fatal out-of-memory (OOM) killer terminations.",
		Recommendation:   "50% to 70% of total host RAM on dedicated database servers; 25% to 40% on shared application hosts.",
		DefaultSuggested: "4294967296",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			bytes, _ := strconv.ParseInt(val, 10, 64)
			mb := bytes / (1024 * 1024)
			if mb < 128 {
				return "warning", "1073741824", fmt.Sprintf("Buffer pool is only %dMB. Suitable only for micro-instances; heavy workloads will disk-thrash.", mb)
			}
			return "optimal", val, fmt.Sprintf("Buffer pool configured to %dMB (%d bytes).", mb, bytes)
		},
	},
	"innodb_buffer_pool_instances": {
		Name:             "innodb_buffer_pool_instances",
		Category:         "Memory & Buffer",
		Scope:            "Global",
		IsDynamic:        false,
		Description:      "Number of partitioned regions for the InnoDB buffer pool to minimize thread concurrency mutex contention.",
		ImpactLow:        "1 instance on multi-gigabyte pools causes thread wait contention on buffer pool mutexes.",
		ImpactHigh:       "Each instance must be at least 1GB. Too many on small pools wastes overhead.",
		Recommendation:   "Set to 8 on pools >= 8GB; set to 1 if buffer pool < 1GB.",
		DefaultSuggested: "8",
		Evaluate: func(val string, allVars map[string]string) (string, string, string) {
			inst, _ := strconv.Atoi(val)
			poolBytes, _ := strconv.ParseInt(allVars["innodb_buffer_pool_size"], 10, 64)
			poolGB := poolBytes / (1024 * 1024 * 1024)
			if poolGB >= 4 && inst < 4 {
				return "warning", "8", fmt.Sprintf("Buffer pool is %dGB but instances is only %d. Increase to 4-8 to reduce mutex contention.", poolGB, inst)
			}
			return "optimal", val, fmt.Sprintf("%d buffer pool instances.", inst)
		},
	},
	"key_buffer_size": {
		Name:             "key_buffer_size",
		Category:         "Memory & Buffer",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Size of the buffer used for MyISAM index blocks and internal temporary tables.",
		ImpactLow:        "Slows down internal temporary table creation and MyISAM operations.",
		ImpactHigh:       "Wastes memory if all application tables use InnoDB engine.",
		Recommendation:   "16M to 64M is sufficient for modern InnoDB-centric applications.",
		DefaultSuggested: "33554432",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			b, _ := strconv.ParseInt(val, 10, 64)
			mb := b / (1024 * 1024)
			if mb > 512 {
				return "warning", "67108864", fmt.Sprintf("key_buffer_size is %dMB. If using pure InnoDB, this RAM is mostly wasted.", mb)
			}
			return "optimal", val, fmt.Sprintf("%dMB key buffer.", mb)
		},
	},
	"tmp_table_size": {
		Name:             "tmp_table_size",
		Category:         "Memory & Buffer",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Maximum size of in-memory temporary tables created by complex queries before spilling to on-disk tables.",
		ImpactLow:        "Spills to on-disk temporary tables (`Created_tmp_disk_tables`), causing disk I/O latency.",
		ImpactHigh:       "Allocated dynamically per active query session; excessive values can exhaust server memory under high concurrency.",
		Recommendation:   "32M to 64M. Should be kept identical to max_heap_table_size.",
		DefaultSuggested: "67108864",
		Evaluate: func(val string, allVars map[string]string) (string, string, string) {
			heapVal := allVars["max_heap_table_size"]
			if heapVal != "" && heapVal != val {
				return "warning", val, "tmp_table_size and max_heap_table_size differ. The engine uses the minimum of the two."
			}
			return "optimal", val, "Temporary table memory threshold configured."
		},
	},
	"max_heap_table_size": {
		Name:             "max_heap_table_size",
		Category:         "Memory & Buffer",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Maximum size to which user-created MEMORY tables and internal temp tables can grow.",
		ImpactLow:        "Forces MEMORY tables and internal temp tables to disk early.",
		ImpactHigh:       "High concurrent queries using temp tables can cause RAM spikes.",
		Recommendation:   "32M to 64M. Keep equal to tmp_table_size.",
		DefaultSuggested: "67108864",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			return "optimal", val, "Max heap table size configured."
		},
	},

	// --- Concurrency & Connections ---
	"max_connections": {
		Name:             "max_connections",
		Category:         "Concurrency & Lock",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Maximum permitted number of simultaneous client connections.",
		ImpactLow:        "Clients receive 'Too many connections' (error 1040) during traffic spikes.",
		ImpactHigh:       "Each connection allocates thread buffers (sort, join, read, net). 1,000+ idle connections can consume gigabytes of RAM.",
		Recommendation:   "150 to 500 with a connection pooler (ProxySQL, application pools) rather than 5,000 unpooled connections.",
		DefaultSuggested: "300",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n > 2000 {
				return "warning", "500", fmt.Sprintf("max_connections is %d. Very high limits without connection pooling risk OOM under spike load.", n)
			}
			if n < 50 {
				return "warning", "150", fmt.Sprintf("max_connections is only %d. May cause 'Too many connections' errors under modest load.", n)
			}
			return "optimal", val, fmt.Sprintf("%d max connections permitted.", n)
		},
	},
	"thread_cache_size": {
		Name:             "thread_cache_size",
		Category:         "Concurrency & Lock",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Number of connection threads cached for reuse when clients disconnect.",
		ImpactLow:        "Too low causes frequent thread creation overhead (observe `Threads_created` status variable).",
		ImpactHigh:       "Excessive values hold idle memory with diminishing performance returns.",
		Recommendation:   "Formula: 8 + (max_connections / 100), typically between 16 and 64.",
		DefaultSuggested: "32",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n == 0 {
				return "warning", "16", "thread_cache_size is 0. New client connections will incur thread creation overhead every time."
			}
			return "optimal", val, fmt.Sprintf("%d cached threads retained for reuse.", n)
		},
	},
	"wait_timeout": {
		Name:             "wait_timeout",
		Category:         "Concurrency & Lock",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Number of seconds the server waits for activity on an inactive non-interactive connection before terminating it.",
		ImpactLow:        "Prematurely closes idle pooled connections if pool validation query is infrequent.",
		ImpactHigh:       "Default (28800s / 8 hours) leaves abandoned connections lingering in Sleep state holding locks and memory.",
		Recommendation:   "300 to 600 seconds (5 to 10 minutes) for web services; 60 seconds on high-concurrency microservices.",
		DefaultSuggested: "300",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			s, _ := strconv.Atoi(val)
			if s > 3600 {
				return "warning", "300", fmt.Sprintf("wait_timeout is %ds (>1 hour). Abandoned client connections will occupy slots for hours.", s)
			}
			return "optimal", val, fmt.Sprintf("%ds timeout for idle connections.", s)
		},
	},
	"interactive_timeout": {
		Name:             "interactive_timeout",
		Category:         "Concurrency & Lock",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Number of seconds the server waits for activity on an interactive connection (e.g. mysql CLI) before closing it.",
		ImpactLow:        "Disconnects interactive DBA sessions while writing queries.",
		ImpactHigh:       "Keeps forgotten admin sessions open indefinitely.",
		Recommendation:   "600 to 1800 seconds.",
		DefaultSuggested: "600",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			return "optimal", val, "Interactive timeout configured."
		},
	},
	"table_open_cache": {
		Name:             "table_open_cache",
		Category:         "Concurrency & Lock",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Number of open table handles cached across all concurrent client threads.",
		ImpactLow:        "Frequent table reopen overhead (check `Opened_tables` status rate).",
		ImpactHigh:       "Consumes OS file descriptors; ensure `open_files_limit` is set high enough.",
		Recommendation:   "2000 to 4000 for standard databases with multiple schemas.",
		DefaultSuggested: "2000",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n < 400 {
				return "warning", "2000", fmt.Sprintf("table_open_cache is only %d. May cause excessive table reopening.", n)
			}
			return "optimal", val, fmt.Sprintf("%d table descriptors cached.", n)
		},
	},

	// --- IO & Durability ---
	"innodb_flush_log_at_trx_commit": {
		Name:             "innodb_flush_log_at_trx_commit",
		Category:         "IO & Durability",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Controls strictness of ACID write durability and redo log fsync timing on transaction commit.",
		ImpactLow:        "Mode 0 or 2 writes without immediate fsync. Extremely fast, but up to 1 second of transactions can be lost upon power outage or OS crash.",
		ImpactHigh:       "Mode 1 performs fsync to disk on EVERY transaction commit. Strictly ACID compliant, but heavy disk I/O on mechanical disks.",
		Recommendation:   "1 for financial/mission-critical data. 2 is acceptable on battery-backed RAID or Galera multi-master (since peer nodes hold data).",
		DefaultSuggested: "1",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			switch val {
			case "1":
				return "optimal", "1", "Strict ACID durability: fsync on every commit."
			case "2":
				return "info", "1", "Relaxed durability: flush to OS cache per commit, fsync every 1s. Maximum 1s loss on OS crash."
			case "0":
				return "warning", "1", "Vulnerable durability: flushed and synced only once per second. 1s loss on mysqld crash."
			default:
				return "warning", "1", "Unrecognized durability setting."
			}
		},
	},
	"sync_binlog": {
		Name:             "sync_binlog",
		Category:         "IO & Durability",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Controls how often the binary log is synchronized (fsync) to disk.",
		ImpactLow:        "0 allows OS to handle flushing. Fast, but crashes can corrupt or truncate binlog, breaking replica consistency.",
		ImpactHigh:       "1 fsyncs after every transaction. Safest for replication failover, higher disk write IOPS.",
		Recommendation:   "1 for high-durability and automated replica failover (Orchestrator).",
		DefaultSuggested: "1",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			if val == "1" {
				return "optimal", "1", "Strict binlog synchronization: fsync after every transaction."
			}
			return "warning", "1", fmt.Sprintf("sync_binlog is %s. During sudden crash, un-synced binlog events can desync replicas.", val)
		},
	},
	"innodb_flush_method": {
		Name:             "innodb_flush_method",
		Category:         "IO & Durability",
		Scope:            "Global",
		IsDynamic:        false,
		Description:      "Method InnoDB uses to flush data files and log files to physical storage.",
		ImpactLow:        "`fsync` on Linux can cause double buffering (caching in both InnoDB buffer pool and OS page cache).",
		ImpactHigh:       "Certain virtualized filesystems do not support O_DIRECT.",
		Recommendation:   "`O_DIRECT` or `O_DIRECT_NO_FSYNC` on modern Linux servers to bypass double caching.",
		DefaultSuggested: "O_DIRECT",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			if strings.Contains(strings.ToUpper(val), "O_DIRECT") {
				return "optimal", val, "Direct I/O bypassing OS page cache."
			}
			return "info", "O_DIRECT", fmt.Sprintf("Flush method is %s.", val)
		},
	},
	"innodb_io_capacity": {
		Name:             "innodb_io_capacity",
		Category:         "IO & Durability",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Number of I/O operations per second (IOPS) available for InnoDB background flush tasks.",
		ImpactLow:        "Default (200) was sized for old HDDs. Causes dirty page backlog and stall spikes during write bursts on SSDs.",
		ImpactHigh:       "Setting higher than storage capability will saturate disk controller queue and spike query latency.",
		Recommendation:   "200 on HDD; 2,000 to 10,000 on SATA/NVMe SSD storage.",
		DefaultSuggested: "2000",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			n, _ := strconv.Atoi(val)
			if n <= 200 {
				return "warning", "2000", "innodb_io_capacity is 200 (HDD baseline). If running on SSD/NVMe, increase to 2000+ for smooth background flushing."
			}
			return "optimal", val, fmt.Sprintf("%d background IOPS capacity configured.", n)
		},
	},

	// --- Optimizer & Performance ---
	"slow_query_log": {
		Name:             "slow_query_log",
		Category:         "Optimizer & Performance",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Enables logging of queries whose execution duration exceeds long_query_time.",
		ImpactLow:        "OFF leaves DBAs blind to slow, unindexed, or resource-hogging queries in production.",
		ImpactHigh:       "Negligible overhead in modern MySQL versions.",
		Recommendation:   "Must be ON in production to capture query optimization opportunities.",
		DefaultSuggested: "ON",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			if strings.EqualFold(val, "ON") || val == "1" {
				return "optimal", "ON", "Slow query logging is active."
			}
			return "warning", "ON", "slow_query_log is OFF. Slow queries are not being recorded for performance tuning."
		},
	},
	"long_query_time": {
		Name:             "long_query_time",
		Category:         "Optimizer & Performance",
		Scope:            "Both",
		IsDynamic:        true,
		Description:      "Threshold in seconds (supports fractional seconds) above which queries are logged to slow query log.",
		ImpactLow:        "0 logs every single query (high log volume).",
		ImpactHigh:       "Default (10s) misses queries that take 1-3 seconds and degrade API response times.",
		Recommendation:   "1.0 or 2.0 seconds in standard web environments; 0.5s for low-latency APIs.",
		DefaultSuggested: "1.0",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			f, _ := strconv.ParseFloat(val, 64)
			if f >= 5.0 {
				return "warning", "1.0", fmt.Sprintf("long_query_time is %s seconds. Queries taking 1-4s will escape the slow log.", val)
			}
			return "optimal", val, fmt.Sprintf("Queries taking >%ss are logged.", val)
		},
	},
	"max_allowed_packet": {
		Name:             "max_allowed_packet",
		Category:         "Optimizer & Performance",
		Scope:            "Global",
		IsDynamic:        true,
		Description:      "Maximum size of a network packet or generated intermediate string.",
		ImpactLow:        "Clients encounter 'Packet too large' (error 1153) during multi-row batch INSERTs or BLOB uploads.",
		ImpactHigh:       "Memory is only allocated as needed by communication packets; large values carry minimal overhead.",
		Recommendation:   "64M to 128M.",
		DefaultSuggested: "67108864",
		Evaluate: func(val string, _ map[string]string) (string, string, string) {
			b, _ := strconv.ParseInt(val, 10, 64)
			mb := b / (1024 * 1024)
			if mb < 16 {
				return "warning", "67108864", fmt.Sprintf("max_allowed_packet is only %dMB. Large batch queries or migrations may fail.", mb)
			}
			return "optimal", val, fmt.Sprintf("%dMB packet size ceiling.", mb)
		},
	},
}

// EvaluateMySQLVariable annotates a raw MySQL variable with curated knowledge and trade-off heuristics.
func EvaluateMySQLVariable(name, val string, allVars map[string]string) driver.ServerVariable {
	rule, found := MySQLCuratedRules[strings.ToLower(name)]
	if !found {
		// Generic uncurated variable
		return driver.ServerVariable{
			Name:             name,
			Value:            val,
			Category:         categorizeGenericVariable(name),
			Severity:         "info",
			IsCurated:        false,
			SetGlobalSnippet: fmt.Sprintf("SET GLOBAL %s = '%s';", name, val),
			MyCnfSnippet:     fmt.Sprintf("%s = %s", name, val),
		}
	}

	severity, suggested, _ := rule.Evaluate(val, allVars)
	if suggested == "" {
		suggested = rule.DefaultSuggested
	}

	return driver.ServerVariable{
		Name:             rule.Name,
		Value:            val,
		Scope:            rule.Scope,
		IsDynamic:        rule.IsDynamic,
		Category:         rule.Category,
		Description:      rule.Description,
		ImpactLow:        rule.ImpactLow,
		ImpactHigh:       rule.ImpactHigh,
		Recommendation:   rule.Recommendation,
		SuggestedValue:   suggested,
		Severity:         severity,
		IsCurated:        true,
		MyCnfSnippet:     fmt.Sprintf("%s = %s", rule.Name, suggested),
		SetGlobalSnippet: generateSetGlobalSnippet(rule.Name, suggested, rule.IsDynamic),
	}
}

func categorizeGenericVariable(name string) string {
	low := strings.ToLower(name)
	switch {
	case strings.HasPrefix(low, "wsrep_") || strings.Contains(low, "cluster") || strings.Contains(low, "replica") || strings.Contains(low, "slave"):
		return "Cluster & WSREP"
	case strings.Contains(low, "buffer") || strings.Contains(low, "cache") || strings.Contains(low, "memory") || strings.Contains(low, "heap"):
		return "Memory & Buffer"
	case strings.Contains(low, "connect") || strings.Contains(low, "thread") || strings.Contains(low, "timeout") || strings.Contains(low, "lock"):
		return "Concurrency & Lock"
	case strings.Contains(low, "flush") || strings.Contains(low, "sync") || strings.Contains(low, "log") || strings.Contains(low, "binlog"):
		return "IO & Durability"
	case strings.Contains(low, "query") || strings.Contains(low, "optimizer") || strings.Contains(low, "join") || strings.Contains(low, "sort"):
		return "Optimizer & Performance"
	default:
		return "General"
	}
}

func generateSetGlobalSnippet(name, val string, isDynamic bool) string {
	if !isDynamic {
		return fmt.Sprintf("# Note: %s requires server restart. Add to my.cnf:\n%s = %s", name, name, val)
	}
	// Check if numeric or boolean
	if _, err := strconv.ParseInt(val, 10, 64); err == nil {
		return fmt.Sprintf("SET GLOBAL %s = %s;", name, val)
	}
	if strings.EqualFold(val, "ON") || strings.EqualFold(val, "OFF") {
		return fmt.Sprintf("SET GLOBAL %s = %s;", name, strings.ToUpper(val))
	}
	return fmt.Sprintf("SET GLOBAL %s = '%s';", name, val)
}

// EvaluateClusterTuning evaluates critical Galera/Replication guardrail parameters.
func EvaluateClusterTuning(sysVars map[string]string, statusVars map[string]string) []driver.ClusterTuningCheck {
	var checks []driver.ClusterTuningCheck

	// 1. innodb_autoinc_lock_mode
	autoInc := sysVars["innodb_autoinc_lock_mode"]
	if autoInc == "" {
		autoInc = "1"
	}
	if autoInc == "2" {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "innodb_autoinc_lock_mode",
			CurrentValue: autoInc,
			Recommended:  "2",
			Status:       "optimal",
			Summary:      "Interleaved lock mode is active (optimal for multi-master Galera).",
			TradeOff:     "Eliminates auto-inc table locks between peer nodes during concurrent INSERTs.",
			Remediation:  "my.cnf: innodb_autoinc_lock_mode = 2",
		})
	} else {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "innodb_autoinc_lock_mode",
			CurrentValue: autoInc,
			Recommended:  "2",
			Status:       "critical",
			Summary:      "Lock mode is not interleaved (2). High risk of certification deadlocks.",
			TradeOff:     "Traditional (0) or consecutive (1) mode causes table-level locks across nodes during bulk inserts.",
			Remediation:  "Set innodb_autoinc_lock_mode = 2 in my.cnf and restart mysqld.",
		})
	}

	// 2. binlog_format
	binlog := sysVars["binlog_format"]
	if strings.EqualFold(binlog, "ROW") {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "binlog_format",
			CurrentValue: binlog,
			Recommended:  "ROW",
			Status:       "optimal",
			Summary:      "ROW-based binlog format guarantees deterministic cluster replication.",
			TradeOff:     "Slightly larger binlog size for guaranteed zero replica drift.",
			Remediation:  "SET GLOBAL binlog_format = 'ROW';",
		})
	} else {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "binlog_format",
			CurrentValue: binlog,
			Recommended:  "ROW",
			Status:       "critical",
			Summary:      fmt.Sprintf("binlog_format is %s. Galera requires ROW format.", binlog),
			TradeOff:     "STATEMENT or MIXED format causes certification crashes and desynchronization.",
			Remediation:  "SET GLOBAL binlog_format = 'ROW'; (also update my.cnf)",
		})
	}

	// 3. wsrep_slave_threads
	slaveThreadsStr := sysVars["wsrep_slave_threads"]
	slaveThreads, _ := strconv.Atoi(slaveThreadsStr)
	if slaveThreads <= 1 {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "wsrep_slave_threads",
			CurrentValue: slaveThreadsStr,
			Recommended:  "4 - 16",
			Status:       "warning",
			Summary:      "Single-threaded apply worker is prone to queue backlog and Flow Control stalls.",
			TradeOff:     "Increasing threads enables parallel write set application across independent schemas/tables.",
			Remediation:  "SET GLOBAL wsrep_slave_threads = 4; (recommend 2x-4x CPU cores)",
		})
	} else {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "wsrep_slave_threads",
			CurrentValue: slaveThreadsStr,
			Recommended:  "4 - 16",
			Status:       "optimal",
			Summary:      fmt.Sprintf("%d parallel apply threads configured.", slaveThreads),
			TradeOff:     "Balances apply throughput without CPU thrashing.",
			Remediation:  fmt.Sprintf("SET GLOBAL wsrep_slave_threads = %d;", slaveThreads),
		})
	}

	// 4. wsrep_sync_wait
	syncWait := sysVars["wsrep_sync_wait"]
	if syncWait == "" || syncWait == "0" {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "wsrep_sync_wait",
			CurrentValue: syncWait,
			Recommended:  "0 (default) / 1 (strict)",
			Status:       "optimal",
			Summary:      "Causal read wait disabled (fastest read latency).",
			TradeOff:     "Reads execute with zero wait overhead; minor replica lag possible if node is catching up.",
			Remediation:  "SET GLOBAL wsrep_sync_wait = 0;",
		})
	} else {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "wsrep_sync_wait",
			CurrentValue: syncWait,
			Recommended:  "0 (default) / 1 (strict)",
			Status:       "optimal",
			Summary:      fmt.Sprintf("Causal sync wait active (mask %s). Guarantees zero stale reads.", syncWait),
			TradeOff:     "Ensures strict causal consistency across all nodes at cost of minor read query latency.",
			Remediation:  fmt.Sprintf("SET GLOBAL wsrep_sync_wait = %s;", syncWait),
		})
	}

	// 5. wsrep_retry_autocommit
	retry := sysVars["wsrep_retry_autocommit"]
	retryCount, _ := strconv.Atoi(retry)
	if retryCount == 0 {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "wsrep_retry_autocommit",
			CurrentValue: retry,
			Recommended:  "1 - 3",
			Status:       "warning",
			Summary:      "Retry autocommit is disabled (0). Transient certification conflicts error immediately.",
			TradeOff:     "Enabling 1-3 retries transparently absorbs multi-master concurrent write conflicts.",
			Remediation:  "SET GLOBAL wsrep_retry_autocommit = 3;",
		})
	} else {
		checks = append(checks, driver.ClusterTuningCheck{
			Variable:     "wsrep_retry_autocommit",
			CurrentValue: retry,
			Recommended:  "1 - 3",
			Status:       "optimal",
			Summary:      fmt.Sprintf("Auto-retries deadlocked autocommit transactions %d times.", retryCount),
			TradeOff:     "Reduces client deadlock exceptions without performance degradation.",
			Remediation:  fmt.Sprintf("SET GLOBAL wsrep_retry_autocommit = %d;", retryCount),
		})
	}

	return checks
}
