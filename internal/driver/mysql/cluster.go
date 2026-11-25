package mysql

import (
	"context"
	"database/sql"
	"fmt"
	gomysql "github.com/go-sql-driver/mysql"
	"github.com/herliansyah/sqlfox/internal/advisor"
	"github.com/herliansyah/sqlfox/internal/driver"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (s *MySQLSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	// 1. Check if Galera / WSREP is active
	var wsrepReadyVal string
	_ = s.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'wsrep_ready'").Scan(new(string), &wsrepReadyVal)

	if strings.EqualFold(wsrepReadyVal, "ON") {
		return s.inspectGaleraCluster(ctx)
	}

	// 2. Fallback: inspect standard replication or standalone
	return s.inspectStandardReplicationOrStandalone(ctx)
}

func (s *MySQLSession) inspectGaleraCluster(ctx context.Context) (*driver.ClusterHealthReport, error) {
	statusVars := make(map[string]string)
	rows, err := s.db.QueryContext(ctx, `SHOW GLOBAL STATUS WHERE Variable_name IN (
		'wsrep_cluster_size', 'wsrep_cluster_status', 'wsrep_cluster_name',
		'wsrep_node_name', 'wsrep_local_state_comment', 'wsrep_flow_control_paused',
		'wsrep_flow_control_sent', 'wsrep_flow_control_recv', 'wsrep_local_recv_queue',
		'wsrep_local_send_queue', 'wsrep_evs_repl_latency', 'wsrep_incoming_addresses',
		'wsrep_local_cert_failures', 'wsrep_local_bf_aborts',
		'Innodb_buffer_pool_wait_free', 'Innodb_log_waits', 'Innodb_os_log_fsyncs'
	)`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				statusVars[k] = v
			}
		}
	}

	sysVars := make(map[string]string)
	vRows, err := s.db.QueryContext(ctx, `SHOW VARIABLES WHERE Variable_name IN (
		'wsrep_slave_threads', 'wsrep_node_address', 'innodb_autoinc_lock_mode',
		'binlog_format', 'wsrep_sync_wait', 'wsrep_retry_autocommit'
	)`)
	if err == nil {
		defer vRows.Close()
		for vRows.Next() {
			var k, v string
			if err := vRows.Scan(&k, &v); err == nil {
				sysVars[k] = v
			}
		}
	}

	clusterSize, _ := strconv.Atoi(statusVars["wsrep_cluster_size"])
	clusterStatus := statusVars["wsrep_cluster_status"]
	if clusterStatus == "" {
		clusterStatus = "Primary"
	}
	clusterName := statusVars["wsrep_cluster_name"]
	localNodeName := statusVars["wsrep_node_name"]
	localNodeAddress := sysVars["wsrep_node_address"]
	if localNodeAddress == "" {
		localNodeAddress = fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	}
	localNodeState := statusVars["wsrep_local_state_comment"]
	if localNodeState == "" {
		localNodeState = "Synced"
	}

	fcPaused, _ := strconv.ParseFloat(statusVars["wsrep_flow_control_paused"], 64)
	fcSent, _ := strconv.ParseInt(statusVars["wsrep_flow_control_sent"], 10, 64)
	fcRecv, _ := strconv.ParseInt(statusVars["wsrep_flow_control_recv"], 10, 64)
	localRecvQueue, _ := strconv.ParseInt(statusVars["wsrep_local_recv_queue"], 10, 64)
	localSendQueue, _ := strconv.ParseInt(statusVars["wsrep_local_send_queue"], 10, 64)
	certFailures, _ := strconv.ParseInt(statusVars["wsrep_local_cert_failures"], 10, 64)
	bfAborts, _ := strconv.ParseInt(statusVars["wsrep_local_bf_aborts"], 10, 64)

	bufWaitFree, _ := strconv.ParseInt(statusVars["Innodb_buffer_pool_wait_free"], 10, 64)
	logWaits, _ := strconv.ParseInt(statusVars["Innodb_log_waits"], 10, 64)
	osLogFsyncs, _ := strconv.ParseInt(statusVars["Innodb_os_log_fsyncs"], 10, 64)

	slaveThreads, _ := strconv.Atoi(sysVars["wsrep_slave_threads"])
	if slaveThreads <= 0 {
		slaveThreads = 1
	}

	// Parse wsrep_evs_repl_latency: min/avg/max/stddev in seconds
	var latMin, latAvg, latMax float64
	evsLatStr := statusVars["wsrep_evs_repl_latency"]
	if evsLatStr != "" {
		parts := strings.Split(evsLatStr, "/")
		if len(parts) >= 3 {
			p0, _ := strconv.ParseFloat(parts[0], 64)
			p1, _ := strconv.ParseFloat(parts[1], 64)
			p2, _ := strconv.ParseFloat(parts[2], 64)
			latMin = p0 * 1000.0 // seconds to milliseconds
			latAvg = p1 * 1000.0
			latMax = p2 * 1000.0
		}
	}

	// 3. Peer Node Auto-Discovery via wsrep_incoming_addresses
	incomingAddrs := statusVars["wsrep_incoming_addresses"]
	addrList := strings.Split(incomingAddrs, ",")
	cleanAddrs := make([]string, 0, len(addrList))
	for _, a := range addrList {
		trimmed := strings.TrimSpace(a)
		if trimmed != "" {
			cleanAddrs = append(cleanAddrs, trimmed)
		}
	}

	if len(cleanAddrs) == 0 {
		cleanAddrs = []string{fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)}
	}

	nodeResults := make([]driver.ClusterNodeInfo, len(cleanAddrs))
	var wg sync.WaitGroup

	for i, addr := range cleanAddrs {
		idx := i
		nodeAddr := addr
		hostPort := nodeAddr
		if !strings.Contains(hostPort, ":") {
			hostPort = fmt.Sprintf("%s:3306", hostPort)
		}

		isLocal := strings.HasPrefix(nodeAddr, s.cfg.Host) ||
			strings.HasPrefix(nodeAddr, "127.0.0.1") ||
			nodeAddr == localNodeAddress ||
			len(cleanAddrs) == 1

		if isLocal {
			nodeResults[idx] = driver.ClusterNodeInfo{
				Name:            localNodeName,
				Address:         nodeAddr,
				State:           localNodeState,
				ReceiveQueue:    localRecvQueue,
				SendQueue:       localSendQueue,
				FlowControlSent: fcSent,
				IsLocal:         true,
				Status:          "online",
			}
		} else if os.Getenv("SQLFOX_GALERA_PEER_PROBE") == "1" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				nodeResults[idx] = s.probePeerNode(ctx, hostPort, nodeAddr)
			}()
		} else {
			// Do not automatically transmit DB credentials to unverified peer addresses from wsrep_incoming_addresses.
			nodeResults[idx] = driver.ClusterNodeInfo{
				Name:    nodeAddr,
				Address: nodeAddr,
				State:   "Discovered (peer probe disabled)",
				Status:  "discovered",
			}
		}
	}
	wg.Wait()

	// 4. Identify Culprit Node (Slowest Node triggering Flow Control / Queue backlog)
	var culpritNode *driver.ClusterNodeInfo
	var highestQueue int64 = -1

	for i := range nodeResults {
		n := &nodeResults[i]
		if n.Status == "online" {
			// A node is a culprit if it sent Flow Control or has an elevated receive queue
			if n.FlowControlSent > 0 && n.ReceiveQueue > highestQueue {
				highestQueue = n.ReceiveQueue
				culpritNode = n
			} else if culpritNode == nil && n.ReceiveQueue > 20 && n.ReceiveQueue > highestQueue {
				highestQueue = n.ReceiveQueue
				culpritNode = n
			}
		}
	}

	slowestNodeAddr := ""
	if culpritNode != nil {
		culpritNode.IsCulprit = true
		slowestNodeAddr = culpritNode.Address
		if culpritNode.Name != "" {
			slowestNodeAddr = fmt.Sprintf("%s (%s)", culpritNode.Name, culpritNode.Address)
		}
	}

	// 5. Tri-Metric Bottleneck Detective Evaluation
	diagnosis, severity := s.diagnoseClusterBottleneck(
		fcPaused, fcSent, fcRecv, culpritNode, localNodeName,
		latAvg, latMax, bufWaitFree, logWaits, localRecvQueue,
		slaveThreads, certFailures, bfAborts, clusterStatus,
	)

	return &driver.ClusterHealthReport{
		TopologyType:        "galera",
		ClusterName:         clusterName,
		ClusterStatus:       clusterStatus,
		ClusterSize:         clusterSize,
		LocalNodeName:       localNodeName,
		LocalNodeAddress:    localNodeAddress,
		LocalNodeState:      localNodeState,
		FlowControlPaused:   fcPaused,
		FlowControlSent:     fcSent,
		FlowControlRecv:     fcRecv,
		BottleneckDiagnosis: diagnosis,
		BottleneckSeverity:  severity,
		SlowestNode:         slowestNodeAddr,
		Nodes:               nodeResults,
		TriMetric: driver.TriMetricStats{
			NetworkLatencyMinMs: latMin,
			NetworkLatencyAvgMs: latAvg,
			NetworkLatencyMaxMs: latMax,
			BufferPoolWaitFree:  bufWaitFree,
			LogWaits:            logWaits,
			OSLogFsyncs:         osLogFsyncs,
			SlaveThreads:        slaveThreads,
			CertFailures:        certFailures,
			BFAborts:            bfAborts,
		},
		TuningChecks: advisor.EvaluateClusterTuning(sysVars, statusVars),
	}, nil
}

// probePeerNode connects to a cluster peer node with a 1500ms timeout to read queue and FC sent.
// ponytail: Quick single-shot query with fail-fast timeout prevents hanging on firewalled peer nodes.
func (s *MySQLSession) probePeerNode(parentCtx context.Context, hostPort, origAddr string) driver.ClusterNodeInfo {
	ctx, cancel := context.WithTimeout(parentCtx, 1500*time.Millisecond)
	defer cancel()

	mc := gomysql.NewConfig()
	mc.User = s.cfg.User
	mc.Passwd = s.cfg.Password
	mc.Net = "tcp"
	mc.Addr = hostPort
	mc.DBName = s.activeDB()
	mc.Timeout = 1200 * time.Millisecond
	mc.ReadTimeout = 1200 * time.Millisecond
	mc.Params = map[string]string{"charset": "utf8mb4"}
	if s.cfg.UseSSL {
		if s.tlsKey != "" {
			mc.TLSConfig = s.tlsKey
		} else {
			mc.TLSConfig = "true"
		}
	} else {
		mc.TLSConfig = "false"
	}
	peerDB, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return driver.ClusterNodeInfo{
			Address: origAddr,
			Status:  "unreachable",
			Message: "Failed to initialize dialer",
		}
	}
	defer peerDB.Close()

	if err := peerDB.PingContext(ctx); err != nil {
		return driver.ClusterNodeInfo{
			Address: origAddr,
			Status:  "unreachable",
			Message: "🔒 Unreachable directly from client (VPC/Firewall restricted)",
		}
	}

	var nodeName, nodeState string
	var recvQueue, fcSent int64

	rows, err := peerDB.QueryContext(ctx, `SHOW GLOBAL STATUS WHERE Variable_name IN ('wsrep_node_name', 'wsrep_local_state_comment', 'wsrep_local_recv_queue', 'wsrep_flow_control_sent')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				switch k {
				case "wsrep_node_name":
					nodeName = v
				case "wsrep_local_state_comment":
					nodeState = v
				case "wsrep_local_recv_queue":
					recvQueue, _ = strconv.ParseInt(v, 10, 64)
				case "wsrep_flow_control_sent":
					fcSent, _ = strconv.ParseInt(v, 10, 64)
				}
			}
		}
	}

	return driver.ClusterNodeInfo{
		Name:            nodeName,
		Address:         origAddr,
		State:           nodeState,
		ReceiveQueue:    recvQueue,
		FlowControlSent: fcSent,
		Status:          "online",
	}
}

func (s *MySQLSession) diagnoseClusterBottleneck(
	fcPaused float64, fcSent, fcRecv int64, culprit *driver.ClusterNodeInfo, localName string,
	latAvg, latMax float64, bufWaitFree, logWaits, localQueue int64,
	slaveThreads int, certFailures, bfAborts int64, clusterStatus string,
) (string, string) {
	if !strings.EqualFold(clusterStatus, "Primary") {
		return fmt.Sprintf("Cluster quorum lost: current cluster status is '%s' (split-brain danger)", clusterStatus), "critical"
	}

	// 1. Flow Control Active
	if fcPaused > 0.05 {
		if culprit != nil && culprit.IsLocal {
			return fmt.Sprintf("Local node '%s' is bottlenecking the cluster: Flow Control active (paused %.1f%% of time, queue: %d)", localName, fcPaused*100, localQueue), "critical"
		} else if culprit != nil {
			return fmt.Sprintf("Cluster write freeze: Remote node '%s' (%s) triggered Flow Control (Receive Queue: %d, FC Sent: %d)", culprit.Name, culprit.Address, culprit.ReceiveQueue, culprit.FlowControlSent), "critical"
		}
		return fmt.Sprintf("Cluster write freeze: Flow Control is paused %.1f%% of time across cluster", fcPaused*100), "critical"
	} else if fcPaused > 0.005 {
		if culprit != nil {
			return fmt.Sprintf("Intermittent Flow Control throttling: node '%s' is applying writesets slowly (Queue: %d)", culprit.Address, culprit.ReceiveQueue), "warning"
		}
		return fmt.Sprintf("Intermittent Flow Control throttling (paused %.2f%%)", fcPaused*100), "warning"
	}

	// 2. Storage / Disk I/O Bottleneck
	if bufWaitFree > 0 || logWaits > 0 {
		return fmt.Sprintf("InnoDB Storage / Disk I/O stall detected: %d buffer pool wait free events, %d log flush waits", bufWaitFree, logWaits), "warning"
	}

	// 3. Network Latency Spike
	if latAvg > 5.0 || latMax > 50.0 {
		return fmt.Sprintf("High inter-node network replication latency: avg %.2fms (peak %.2fms)", latAvg, latMax), "warning"
	}

	// 4. Worker Concurrency Saturation
	if localQueue > int64(slaveThreads*15) && slaveThreads <= 4 {
		return fmt.Sprintf("Apply queue backlog (%d items): wsrep_slave_threads (%d) is under-provisioned for current write rate", localQueue, slaveThreads), "warning"
	}

	// 5. Certification Aborts (Concurrent write contention)
	if certFailures > 50 || bfAborts > 50 {
		return fmt.Sprintf("Optimistic certification conflicts detected: %d cert failures, %d brute-force aborts", certFailures, bfAborts), "warning"
	}

	return "Cluster is healthy: quorum primary, all nodes synchronized without flow control pressure", "ok"
}

func (s *MySQLSession) inspectStandardReplicationOrStandalone(ctx context.Context) (*driver.ClusterHealthReport, error) {
	// Query InnoDB storage waits for all MySQL engines
	var bufWaitFree, logWaits, osLogFsyncs int64
	rows, err := s.db.QueryContext(ctx, `SHOW GLOBAL STATUS WHERE Variable_name IN ('Innodb_buffer_pool_wait_free', 'Innodb_log_waits', 'Innodb_os_log_fsyncs')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				switch k {
				case "Innodb_buffer_pool_wait_free":
					bufWaitFree, _ = strconv.ParseInt(v, 10, 64)
				case "Innodb_log_waits":
					logWaits, _ = strconv.ParseInt(v, 10, 64)
				case "Innodb_os_log_fsyncs":
					osLogFsyncs, _ = strconv.ParseInt(v, 10, 64)
				}
			}
		}
	}

	triMetric := driver.TriMetricStats{
		BufferPoolWaitFree: bufWaitFree,
		LogWaits:           logWaits,
		OSLogFsyncs:        osLogFsyncs,
	}

	// ponytail: Universal Replication Normalizer: probe SHOW REPLICA STATUS (MySQL 8.4+/9.x), fallback to SHOW SLAVE STATUS (MySQL 5.7/8.0/MariaDB)
	slaveRows, err := s.db.QueryContext(ctx, "SHOW REPLICA STATUS")
	if err != nil {
		slaveRows, err = s.db.QueryContext(ctx, "SHOW SLAVE STATUS")
	}
	if err == nil {
		defer slaveRows.Close()
		cols, err := slaveRows.Columns()
		if err == nil && slaveRows.Next() {
			colVals := make([]any, len(cols))
			colPtrs := make([]any, len(cols))
			for i := range colVals {
				colPtrs[i] = &colVals[i]
			}
			if err := slaveRows.Scan(colPtrs...); err == nil {
				colMap := make(map[string]string)
				for i, c := range cols {
					if colVals[i] != nil {
						switch v := colVals[i].(type) {
						case []byte:
							colMap[strings.ToLower(c)] = string(v)
						case string:
							colMap[strings.ToLower(c)] = v
						default:
							colMap[strings.ToLower(c)] = fmt.Sprintf("%v", v)
						}
					}
				}

				// Normalize column names across MySQL 8.4+ and legacy MySQL 5.7/8.0
				ioRunning := strings.EqualFold(colMap["replica_io_running"], "yes") || strings.EqualFold(colMap["slave_io_running"], "yes")
				sqlRunning := strings.EqualFold(colMap["replica_sql_running"], "yes") || strings.EqualFold(colMap["slave_sql_running"], "yes")

				sbm := colMap["seconds_behind_source"]
				if sbm == "" {
					sbm = colMap["seconds_behind_master"]
				}
				var secBehind *int64
				if sbm != "" && sbm != "NULL" {
					val, _ := strconv.ParseInt(sbm, 10, 64)
					secBehind = &val
				}

				lastErr := colMap["last_error"]
				if lastErr == "" {
					lastErr = colMap["last_sql_error"]
				}

				masterHost := colMap["source_host"]
				if masterHost == "" {
					masterHost = colMap["master_host"]
				}

				diagnosis := "Replication synchronized and operating normally"
				severity := "ok"
				if !ioRunning || !sqlRunning {
					diagnosis = fmt.Sprintf("Replication broken (IO: %t, SQL: %t): %s", ioRunning, sqlRunning, lastErr)
					severity = "critical"
				} else if secBehind != nil && *secBehind > 10 {
					diagnosis = fmt.Sprintf("Replication lag behind source: %d seconds", *secBehind)
					severity = "warning"
				} else if bufWaitFree > 0 || logWaits > 0 {
					diagnosis = fmt.Sprintf("InnoDB storage write stall detected (wait_free: %d, log_waits: %d)", bufWaitFree, logWaits)
					severity = "warning"
				}

				return &driver.ClusterHealthReport{
					TopologyType:        "replication",
					ClusterName:         "mysql-replication",
					ClusterStatus:       "Replica",
					ClusterSize:         2,
					LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
					LocalNodeState:      "Replica",
					BottleneckDiagnosis: diagnosis,
					BottleneckSeverity:  severity,
					Replication: &driver.ReplicationInfo{
						Role:                "Replica",
						SecondsBehindMaster: secBehind,
						IORunning:           ioRunning,
						SQLRunning:          sqlRunning,
						MasterHost:          masterHost,
						LastError:           lastErr,
					},
					Nodes: []driver.ClusterNodeInfo{
						{
							Name:    "replica-local",
							Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
							State:   "Replica",
							IsLocal: true,
							Status:  "online",
						},
						{
							Name:    "source-master",
							Address: masterHost,
							State:   "Source",
							Status:  "online",
						},
					},
					TriMetric: triMetric,
				}, nil
			}
		}
	}

	// Check if this instance is a Source / Master with binary logging enabled.
	// Scan dynamically by column count so MySQL 8.4+ (5 cols), MySQL 5.7/8.0 (5 cols),
	// and MariaDB (4 cols) all parse cleanly without silently failing Scan.
	var binlogFile string
	var binlogPos int64
	scanBinlogStatus := func(q string) bool {
		rows, err := s.db.QueryContext(ctx, q)
		if err != nil {
			return false
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil || len(cols) < 2 || !rows.Next() {
			return false
		}
		vals := make([]any, len(cols))
		vals[0] = &binlogFile
		vals[1] = &binlogPos
		for i := 2; i < len(cols); i++ {
			vals[i] = new(sql.RawBytes)
		}
		return rows.Scan(vals...) == nil && binlogFile != ""
	}
	if !scanBinlogStatus("SHOW BINARY LOG STATUS") {
		_ = scanBinlogStatus("SHOW MASTER STATUS")
	}
	if binlogFile != "" {
		diagnosis := "Replication Source: binary logging active"
		severity := "ok"
		if bufWaitFree > 0 || logWaits > 0 {
			diagnosis = fmt.Sprintf("InnoDB storage write stall on Source (wait_free: %d, log_waits: %d)", bufWaitFree, logWaits)
			severity = "warning"
		}
		return &driver.ClusterHealthReport{
			TopologyType:        "replication",
			ClusterName:         "mysql-source",
			ClusterStatus:       "Source (Master)",
			ClusterSize:         1,
			LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
			LocalNodeState:      "Source",
			BottleneckDiagnosis: diagnosis,
			BottleneckSeverity:  severity,
			Replication: &driver.ReplicationInfo{
				Role:       "Source (Master)",
				IORunning:  true,
				SQLRunning: true,
			},
			Nodes: []driver.ClusterNodeInfo{
				{
					Name:    "source-master",
					Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
					State:   "Source",
					IsLocal: true,
					Status:  "online",
				},
			},
			TriMetric: triMetric,
		}, nil
	}

	// Standalone instance fallback
	diagnosis := "Standalone MySQL / MariaDB instance"
	severity := "ok"
	if bufWaitFree > 0 || logWaits > 0 {
		diagnosis = fmt.Sprintf("InnoDB storage write stall detected (wait_free: %d, log_waits: %d)", bufWaitFree, logWaits)
		severity = "warning"
	}

	return &driver.ClusterHealthReport{
		TopologyType:        "standalone",
		ClusterName:         "standalone-mysql",
		ClusterStatus:       "Standalone",
		ClusterSize:         1,
		LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
		LocalNodeState:      "Standalone",
		BottleneckDiagnosis: diagnosis,
		BottleneckSeverity:  severity,
		Nodes: []driver.ClusterNodeInfo{
			{
				Name:    "standalone-node",
				Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
				State:   "Standalone",
				IsLocal: true,
				Status:  "online",
			},
		},
		TriMetric: triMetric,
	}, nil
}

// SampleFlightMetrics captures an on-demand performance telemetry slice with correlated active queries.
func (s *MySQLSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	sample := &driver.FlightMetricSample{
		SampledAt:     time.Now(),
		ActiveQueries: []driver.FlightQueryInfo{},
	}

	// 1. Fetch Global Status variables
	rows, err := s.db.QueryContext(ctx, `
		SHOW GLOBAL STATUS WHERE Variable_name IN (
			'Questions',
			'Threads_running',
			'Threads_connected',
			'Innodb_row_lock_waits',
			'Innodb_row_lock_current_waits',
			'Innodb_buffer_pool_wait_free'
		)
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, val string
			if err := rows.Scan(&name, &val); err == nil {
				intVal, _ := strconv.ParseInt(val, 10, 64)
				switch name {
				case "Questions":
					sample.Questions = intVal
				case "Threads_running":
					sample.ThreadsRunning = intVal
				case "Threads_connected":
					sample.ThreadsConnected = intVal
				case "Innodb_row_lock_current_waits":
					sample.RowLockWaits = intVal
				case "Innodb_buffer_pool_wait_free":
					sample.BufferPoolWaitFree = intVal
				}
			}
		}
	}

	// 2. Adaptive Correlated Queries: fetch active non-idle queries with non-empty SQL (Top 5)
	// ponytail: Prefer performance_schema.threads (lock-free) to avoid LOCK_thread_count mutex contention.
	var qRows *sql.Rows
	var qErr error
	if s.pfsEnabled {
		qRows, qErr = s.db.QueryContext(ctx, `
			SELECT PROCESSLIST_ID, IFNULL(PROCESSLIST_USER, ''), IFNULL(PROCESSLIST_HOST, ''),
			       IFNULL(PROCESSLIST_DB, ''), IFNULL(PROCESSLIST_COMMAND, ''), IFNULL(PROCESSLIST_TIME, 0),
			       IFNULL(PROCESSLIST_STATE, ''), IFNULL(PROCESSLIST_INFO, '')
			FROM performance_schema.threads
			WHERE PROCESSLIST_ID IS NOT NULL
			  AND PROCESSLIST_USER NOT IN ('system user', 'event_scheduler')
			  AND PROCESSLIST_COMMAND NOT IN ('Sleep', 'Binlog Dump', 'Daemon')
			  AND PROCESSLIST_INFO IS NOT NULL AND TRIM(PROCESSLIST_INFO) != ''
			ORDER BY PROCESSLIST_TIME DESC
			LIMIT 5
		`)
	}
	if !s.pfsEnabled || qErr != nil {
		// Fallback to information_schema.PROCESSLIST if performance_schema is disabled
		qRows, qErr = s.db.QueryContext(ctx, `
			SELECT ID, USER, HOST, IFNULL(DB, ''), COMMAND, TIME, IFNULL(STATE, ''), IFNULL(INFO, '')
			FROM information_schema.PROCESSLIST
			WHERE USER NOT IN ('system user', 'event_scheduler')
			  AND COMMAND NOT IN ('Sleep', 'Binlog Dump', 'Daemon')
			  AND INFO IS NOT NULL AND TRIM(INFO) != ''
			ORDER BY TIME DESC
			LIMIT 5
		`)
	}
	if qErr == nil {
		defer qRows.Close()
		for qRows.Next() {
			var q driver.FlightQueryInfo
			if err := qRows.Scan(&q.ThreadID, &q.User, &q.Host, &q.DB, &q.Command, &q.Time, &q.State, &q.Info); err == nil {
				sample.ActiveQueries = append(sample.ActiveQueries, q)
			}
		}
	}

	return sample, nil
}
