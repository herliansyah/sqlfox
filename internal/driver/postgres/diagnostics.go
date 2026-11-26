package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"strconv"
	"strings"
	"time"
)

func (s *PostgresSession) GetProcesslist(ctx context.Context) (*driver.ProcesslistResult, error) {
	var currentPID int64
	_ = s.db.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&currentPID)

	rows, err := s.db.QueryContext(ctx, `
		SELECT pid,
		       COALESCE(usename, '') as usename,
		       COALESCE(client_addr::text, 'local') as host,
		       COALESCE(datname, '') as db,
		       COALESCE(state, 'idle') as state,
		       COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - query_start))::bigint, 0) as time_sec,
		       COALESCE(state, '') as command,
		       COALESCE(query, '') as query,
		       COALESCE(pg_blocking_pids(pid), '{}') as blocking_pids
		FROM pg_stat_activity
		WHERE datname = current_database()
		ORDER BY pid;
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PostgreSQL processlist: %w", err)
	}
	defer rows.Close()

	var threads []driver.ProcessItem
	var locks []driver.LockWait

	for rows.Next() {
		var pid, timeSec int64
		var user, host, db, state, command, query string
		var rawBlockers any

		if err := rows.Scan(&pid, &user, &host, &db, &state, &timeSec, &command, &query, &rawBlockers); err != nil {
			continue
		}

		blockingPids := parseBlockingPids(rawBlockers)

		threads = append(threads, driver.ProcessItem{
			ID:      pid,
			User:    user,
			Host:    host,
			DB:      db,
			Command: command,
			Time:    timeSec,
			State:   state,
			Info:    query,
		})

		for _, blocker := range blockingPids {
			locks = append(locks, driver.LockWait{
				WaitingThreadID:  pid,
				WaitingQuery:     query,
				WaitingUser:      user,
				WaitingSeconds:   timeSec,
				BlockingThreadID: blocker,
				LockMode:         "ROW_EXCLUSIVE",
			})
		}
	}

	var questions int64
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(sum(xact_commit + xact_rollback), 0) FROM pg_stat_database WHERE datname = current_database()").Scan(&questions)

	return &driver.ProcesslistResult{
		CurrentThreadID: currentPID,
		Threads:         threads,
		Locks:           locks,
		Questions:       questions,
	}, nil
}

func parseBlockingPids(rawBlockers any) []int64 {
	var blockingPids []int64
	switch v := rawBlockers.(type) {
	case []int64:
		blockingPids = v
	case []int32:
		for _, b := range v {
			blockingPids = append(blockingPids, int64(b))
		}
	case []any:
		for _, item := range v {
			switch num := item.(type) {
			case int64:
				blockingPids = append(blockingPids, num)
			case int32:
				blockingPids = append(blockingPids, int64(num))
			}
		}
	case string:
		trimmed := strings.Trim(v, "{}")
		if trimmed != "" {
			for _, part := range strings.Split(trimmed, ",") {
				if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
					blockingPids = append(blockingPids, n)
				}
			}
		}
	}
	return blockingPids
}

func (s *PostgresSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	if threadID <= 0 {
		return errors.New("invalid thread id")
	}

	var query string
	if killConnection {
		query = "SELECT pg_terminate_backend($1)"
	} else {
		query = "SELECT pg_cancel_backend($1)"
	}

	executeKill := func(db *sql.DB) (bool, error) {
		var ok bool
		err := db.QueryRowContext(ctx, query, threadID).Scan(&ok)
		return ok, err
	}

	// ponytail: Dial independent single-shot control connection to bypass saturated s.db pool
	if s.dsn != "" {
		controlDB, dErr := sql.Open("pgx", s.dsn)
		if dErr == nil {
			defer controlDB.Close()
			ok, kErr := executeKill(controlDB)
			if kErr == nil {
				if !ok {
					action := "cancel query"
					if killConnection {
						action = "terminate connection"
					}
					return fmt.Errorf("failed to %s for backend PID %d (process may not exist or insufficient privilege)", action, threadID)
				}
				return nil
			}
		}
	}

	if s.db != nil {
		ok, err := executeKill(s.db)
		if err != nil {
			return err
		}
		if !ok {
			action := "cancel query"
			if killConnection {
				action = "terminate connection"
			}
			return fmt.Errorf("failed to %s for backend PID %d (process may not exist or insufficient privilege)", action, threadID)
		}
		return nil
	}
	return errors.New("database connection not available")
}

// GetDatabaseDiagnostics returns physical storage and metadata for all databases on PostgreSQL.
func (s *PostgresSession) GetDatabaseDiagnostics(ctx context.Context) ([]driver.DatabaseDiagnostics, error) {
	query := `
		SELECT 
			d.datname,
			COALESCE(pg_database_size(d.datname), 0),
			COALESCE(d.datcollate, ''),
			COALESCE(pg_encoding_to_char(d.encoding), ''),
			d.datistemplate
		FROM pg_database d
		ORDER BY 
			CASE WHEN d.datname IN ('postgres', 'template0', 'template1') THEN 1 ELSE 0 END,
			pg_database_size(d.datname) DESC,
			d.datname ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query postgres database diagnostics: %w", err)
	}
	defer rows.Close()

	var result []driver.DatabaseDiagnostics
	for rows.Next() {
		var dd driver.DatabaseDiagnostics
		var isTemplate bool
		if err := rows.Scan(
			&dd.Name,
			&dd.TotalSize,
			&dd.Collation,
			&dd.Charset,
			&isTemplate,
		); err == nil {
			dd.DataSize = dd.TotalSize
			lowerName := strings.ToLower(dd.Name)
			dd.IsSystem = isTemplate || lowerName == "postgres" || strings.HasPrefix(lowerName, "template")
			result = append(result, dd)
		}
	}
	if result == nil {
		result = []driver.DatabaseDiagnostics{}
	}
	return result, nil
}

func (s *PostgresSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	if database == "" {
		database = s.activeSchema()
	}
	query := `
		SELECT 
			c.relname,
			COALESCE(c.reltuples::bigint, 0),
			COALESCE(pg_relation_size(c.oid), 0),
			COALESCE(pg_indexes_size(c.oid), 0),
			CASE WHEN pk.indrelid IS NOT NULL THEN 1 ELSE 0 END
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN (
			SELECT DISTINCT indrelid
			FROM pg_index
			WHERE indisprimary = true
		) pk ON c.oid = pk.indrelid
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'p')
	`
	args := []any{database}
	if table != "" {
		query += " AND c.relname = $2"
		args = append(args, table)
	}
	query += " ORDER BY (pg_relation_size(c.oid) + pg_indexes_size(c.oid)) DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get table diagnostics: %w", err)
	}
	defer rows.Close()

	var results []driver.TableDiagnostics
	for rows.Next() {
		var td driver.TableDiagnostics
		var hasPk int
		if err := rows.Scan(
			&td.Name,
			&td.Rows,
			&td.DataSize,
			&td.IndexSize,
			&hasPk,
		); err == nil {
			td.Engine = "PostgreSQL Heap"
			td.RowFormat = "Dynamic"
			td.Collation = "default"
			td.HasPrimaryKey = hasPk == 1
			results = append(results, td)
		}
	}
	if results == nil {
		results = []driver.TableDiagnostics{}
	}
	return results, nil
}

func (s *PostgresSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	var results []driver.MaintenanceResult
	for _, tbl := range tables {
		regClass := fmt.Sprintf("%s.%s", quoteIdentifier(s.activeSchema()), quoteIdentifier(tbl))
		var cmd string
		switch strings.ToUpper(action) {
		case "OPTIMIZE":
			cmd = fmt.Sprintf("VACUUM FULL %s", regClass)
		case "ANALYZE":
			cmd = fmt.Sprintf("ANALYZE %s", regClass)
		case "REINDEX":
			cmd = fmt.Sprintf("REINDEX TABLE %s", regClass)
		default:
			cmd = fmt.Sprintf("VACUUM %s", regClass)
		}

		start := time.Now()
		_, err := s.db.ExecContext(ctx, cmd)
		dur := time.Since(start).Milliseconds()

		status := "OK"
		msg := fmt.Sprintf("Executed %s in %dms", cmd, dur)
		if err != nil {
			status = "Error"
			msg = err.Error()
		}

		results = append(results, driver.MaintenanceResult{
			Table:   tbl,
			Op:      action,
			Status:  status,
			Message: msg,
		})
	}
	return results, nil
}

func (s *PostgresSession) GetRedundantIndexes(ctx context.Context, database, table string) ([]driver.RedundantIndex, error) {
	schema, err := s.InspectSchema(ctx, s.activeSchema())
	if err != nil {
		return nil, err
	}
	var tables []driver.TableMetadata
	if table != "" {
		for _, tbl := range schema.Tables {
			if strings.EqualFold(tbl.Name, table) {
				tables = append(tables, tbl)
			}
		}
	} else {
		tables = schema.Tables
	}
	return driver.DetectRedundantIndexes("postgres", s.activeSchema(), tables), nil
}

func (s *PostgresSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	var isRecovery bool
	_ = s.db.QueryRowContext(ctx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)

	role := "Primary (Source)"
	if isRecovery {
		role = "Standby (Replica)"
	}

	nodes := []driver.ClusterNodeInfo{
		{
			Name:    fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
			Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
			State:   role,
			IsLocal: true,
			Status:  "online",
		},
	}

	diagnosis := "PostgreSQL instance operating normally"
	severity := "ok"
	var repInfo *driver.ReplicationInfo

	if !isRecovery {
		// Primary: probe pg_stat_replication for active standbys
		rows, err := s.db.QueryContext(ctx, `
			SELECT 
				COALESCE(client_addr::text, 'unknown'),
				COALESCE(application_name, 'walreceiver'),
				COALESCE(state, 'streaming'),
				COALESCE(sync_state, 'async'),
				COALESCE(EXTRACT(EPOCH FROM write_lag)::bigint, 0)
			FROM pg_stat_replication
		`)
		if err == nil {
			defer rows.Close()
			standbyCount := 0
			for rows.Next() {
				var addr, app, st, syncSt string
				var lagSec int64
				if err := rows.Scan(&addr, &app, &st, &syncSt, &lagSec); err == nil {
					standbyCount++
					nodes = append(nodes, driver.ClusterNodeInfo{
						Name:    fmt.Sprintf("%s (%s)", app, syncSt),
						Address: addr,
						State:   "Standby",
						Status:  st,
					})
					if lagSec > 10 {
						diagnosis = fmt.Sprintf("Replication lag on standby %s: %d seconds", app, lagSec)
						severity = "warning"
					}
				}
			}
			if standbyCount > 0 {
				repInfo = &driver.ReplicationInfo{
					Role:       "Primary (Source)",
					IORunning:  true,
					SQLRunning: true,
				}
			}
		}
	} else {
		// Standby: probe pg_stat_wal_receiver
		var senderHost, status string
		var lastMsgReceipt time.Time
		err := s.db.QueryRowContext(ctx, `
			SELECT COALESCE(sender_host, ''), COALESCE(status, ''), COALESCE(last_msg_receipt_time, now())
			FROM pg_stat_wal_receiver
		`).Scan(&senderHost, &status, &lastMsgReceipt)
		if err == nil && senderHost != "" {
			repInfo = &driver.ReplicationInfo{
				Role:       "Standby (Replica)",
				MasterHost: senderHost,
				IORunning:  strings.EqualFold(status, "streaming"),
				SQLRunning: true,
			}
			nodes = append(nodes, driver.ClusterNodeInfo{
				Name:    "primary-source",
				Address: senderHost,
				State:   "Primary",
				Status:  "online",
			})
		}
	}

	return &driver.ClusterHealthReport{
		TopologyType:        "replication",
		ClusterName:         s.cfg.DatabaseName,
		ClusterStatus:       role,
		ClusterSize:         len(nodes),
		LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
		LocalNodeState:      role,
		BottleneckDiagnosis: diagnosis,
		BottleneckSeverity:  severity,
		Replication:         repInfo,
		Nodes:               nodes,
	}, nil
}

func (s *PostgresSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	var activeConns, activeThreads, questions, rowLockWaits int64
	_ = s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()),
			(SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'active'),
			(SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'),
			(SELECT COALESCE(sum(xact_commit + xact_rollback), 0) FROM pg_stat_database WHERE datname = current_database())
	`).Scan(&activeConns, &activeThreads, &rowLockWaits, &questions)

	// Fetch active queries
	// ponytail: Filter out empty queries/daemon tasks and order by duration descending.
	rows, err := s.db.QueryContext(ctx, `
		SELECT pid, COALESCE(usename, '') as u, COALESCE(client_addr::text, 'local') as h,
		       COALESCE(datname, '') as db, COALESCE(state, '') as st,
		       COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - query_start))::bigint, 0) as dur,
		       COALESCE(query, '') as q
		FROM pg_stat_activity
		WHERE datname = current_database() AND state = 'active' AND pid != pg_backend_pid()
		  AND query IS NOT NULL AND TRIM(query) != ''
		ORDER BY dur DESC
		LIMIT 5;
	`)

	var queries []driver.FlightQueryInfo
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pid, dur int64
			var u, h, db, st, q string
			if err := rows.Scan(&pid, &u, &h, &db, &st, &dur, &q); err == nil {
				queries = append(queries, driver.FlightQueryInfo{
					ThreadID: pid,
					User:     u,
					Host:     h,
					DB:       db,
					Command:  "active",
					Time:     dur,
					State:    st,
					Info:     q,
				})
			}
		}
	}

	return &driver.FlightMetricSample{
		SampledAt:          time.Now(),
		Questions:          questions,
		ThreadsRunning:     activeThreads,
		ThreadsConnected:   activeConns,
		RowLockWaits:       rowLockWaits,
		BufferPoolWaitFree: 0,
		ActiveQueries:      queries,
	}, nil
}

type pgCuratedRule struct {
	Category       string
	Recommendation string
	SuggestedValue string
	ImpactLow      string
	ImpactHigh     string
	Evaluate       func(setting, unit string) string
}

func parsePGMemoryBytes(setting, unit string) (int64, bool) {
	s := strings.TrimSpace(setting)
	u := strings.TrimSpace(unit)
	if s == "" {
		return 0, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 {
			return n, true
		}
		switch u {
		case "8kB":
			return n * 8 * 1024, true
		case "kB":
			return n * 1024, true
		case "MB":
			return n * 1024 * 1024, true
		case "GB":
			return n * 1024 * 1024 * 1024, true
		case "TB":
			return n * 1024 * 1024 * 1024 * 1024, true
		case "B", "":
			return n, true
		default:
			// Handle arbitrary "<N>kB" / "<N>MB" units reported by pg_settings (e.g. "16MB" wal_segment_size)
			if mul, ok := parsePGUnitMultiplier(u); ok {
				return n * mul, true
			}
			return n, true
		}
	}
	// Handle human-formatted strings such as "128MB", "4kB", "2GB"
	lower := strings.ToLower(s)
	suffixes := []struct {
		sfx string
		mul int64
	}{
		{"tb", 1024 * 1024 * 1024 * 1024},
		{"gb", 1024 * 1024 * 1024},
		{"mb", 1024 * 1024},
		{"kb", 1024},
		{"b", 1},
	}
	for _, sf := range suffixes {
		if strings.HasSuffix(lower, sf.sfx) {
			numPart := strings.TrimSpace(lower[:len(lower)-len(sf.sfx)])
			if f, err := strconv.ParseFloat(numPart, 64); err == nil {
				return int64(f * float64(sf.mul)), true
			}
		}
	}
	return 0, false
}

func parsePGUnitMultiplier(u string) (int64, bool) {
	lower := strings.ToLower(strings.TrimSpace(u))
	suffixes := []struct {
		sfx string
		mul int64
	}{
		{"tb", 1024 * 1024 * 1024 * 1024},
		{"gb", 1024 * 1024 * 1024},
		{"mb", 1024 * 1024},
		{"kb", 1024},
		{"b", 1},
	}
	for _, sf := range suffixes {
		if strings.HasSuffix(lower, sf.sfx) {
			numPart := strings.TrimSpace(lower[:len(lower)-len(sf.sfx)])
			if numPart == "" {
				return sf.mul, true
			}
			if n, err := strconv.ParseInt(numPart, 10, 64); err == nil && n > 0 {
				return n * sf.mul, true
			}
		}
	}
	return 0, false
}

var pgCuratedRules = map[string]pgCuratedRule{
	"shared_buffers": {
		Category:       "Memory & Buffer",
		Recommendation: "Typically set to 25% of total system RAM (e.g. 1GB - 8GB on modern servers)",
		SuggestedValue: "25% of RAM",
		ImpactLow:      "Frequent disk reads and cache thrashing",
		ImpactHigh:     "Diminishing returns, double-buffering with OS page cache",
		Evaluate: func(setting, unit string) string {
			b, ok := parsePGMemoryBytes(setting, unit)
			if !ok {
				return "info"
			}
			const mb = 1024 * 1024
			if b < 128*mb {
				return "warning"
			}
			return "optimal"
		},
	},
	"work_mem": {
		Category:       "Memory & Buffer",
		Recommendation: "Allocate based on available RAM divided by max_connections (e.g. 16MB - 64MB)",
		SuggestedValue: "16MB - 64MB",
		ImpactLow:      "Complex sorts and hash joins spill to temporary disk files",
		ImpactHigh:     "OOM risk when multiple concurrent backends run multi-node queries",
		Evaluate: func(setting, unit string) string {
			b, ok := parsePGMemoryBytes(setting, unit)
			if !ok {
				return "info"
			}
			const mb = 1024 * 1024
			if b < 4*mb || b > 256*mb {
				return "warning"
			}
			return "optimal"
		},
	},
	"maintenance_work_mem": {
		Category:       "Memory & Buffer",
		Recommendation: "10% of system RAM up to 1GB or 2GB for VACUUM, CREATE INDEX, and ALTER TABLE",
		SuggestedValue: "256MB - 1GB",
		ImpactLow:      "Slow VACUUM, autovacuum, and index build operations",
		ImpactHigh:     "Starves other processes during bulk maintenance tasks",
		Evaluate: func(setting, unit string) string {
			b, ok := parsePGMemoryBytes(setting, unit)
			if !ok {
				return "info"
			}
			const mb = 1024 * 1024
			if b < 64*mb {
				return "warning"
			}
			if b > 2048*mb {
				return "info"
			}
			return "optimal"
		},
	},
	"effective_cache_size": {
		Category:       "Optimizer & Performance",
		Recommendation: "Set to 50% - 75% of total system RAM to help planner estimate OS disk cache",
		SuggestedValue: "50% - 75% of RAM",
		ImpactLow:      "Planner underestimates memory cache and favors sequential scans over index scans",
		ImpactHigh:     "Planner may overly favor index scans when pages aren't cached",
		Evaluate: func(setting, unit string) string {
			b, ok := parsePGMemoryBytes(setting, unit)
			if !ok {
				return "info"
			}
			const mb = 1024 * 1024
			if b < 512*mb {
				return "warning"
			}
			return "optimal"
		},
	},
	"wal_buffers": {
		Category:       "IO & Durability",
		Recommendation: "Typically 16MB (or automatic -1) for high-write transaction workloads",
		SuggestedValue: "16MB",
		ImpactLow:      "Frequent WAL flushes before transaction commit",
		ImpactHigh:     "Wasted shared memory",
		Evaluate: func(setting, unit string) string {
			b, ok := parsePGMemoryBytes(setting, unit)
			if !ok {
				return "info"
			}
			if b == -1 {
				return "optimal"
			}
			const mb = 1024 * 1024
			if b < 16*mb {
				return "warning"
			}
			return "optimal"
		},
	},
	"checkpoint_completion_target": {
		Category:       "IO & Durability",
		Recommendation: "Set to 0.9 to spread checkpoint writes across 90% of checkpoint interval",
		SuggestedValue: "0.9",
		ImpactLow:      "Spiky I/O bursts causing query latency jitter during checkpoints",
		ImpactHigh:     "Slightly longer crash recovery time",
		Evaluate: func(setting, _ string) string {
			f, err := strconv.ParseFloat(strings.TrimSpace(setting), 64)
			if err != nil {
				return "info"
			}
			if f < 0.7 {
				return "warning"
			}
			if f > 0.95 {
				return "info"
			}
			return "optimal"
		},
	},
	"max_connections": {
		Category:       "Concurrency & Lock",
		Recommendation: "Keep between 100 - 300; use connection pooler (PgBouncer) for higher concurrency",
		SuggestedValue: "100 - 200",
		ImpactLow:      "Connection exhaustion ('too many clients already')",
		ImpactHigh:     "Severe memory overhead and CPU context-switching contention",
		Evaluate: func(setting, _ string) string {
			n, err := strconv.Atoi(strings.TrimSpace(setting))
			if err != nil {
				return "info"
			}
			if n < 20 || n > 500 {
				return "warning"
			}
			return "optimal"
		},
	},
	"autovacuum": {
		Category:       "IO & Durability",
		Recommendation: "Keep 'on' to prevent table bloat and transaction ID (XID) wraparound failure",
		SuggestedValue: "on",
		ImpactLow:      "Critical table bloat, slow queries, and database shutdown on XID wraparound",
		ImpactHigh:     "Excessive background I/O if cost limits are not properly tuned",
		Evaluate: func(setting, _ string) string {
			if !strings.EqualFold(strings.TrimSpace(setting), "on") {
				return "critical"
			}
			return "optimal"
		},
	},
}

// EvaluatePostgresVariable evaluates a single PostgreSQL pg_settings row against curated DBA rules.
func EvaluatePostgresVariable(name, setting, unit, category, shortDesc, contextStr string) driver.ServerVariable {
	isDynamic := (contextStr == "user" || contextStr == "superuser")
	scope := "Global"
	if contextStr == "user" {
		scope = "Both"
	}
	isCurated := false
	severity := "info"
	recommendation := ""
	suggestedVal := ""
	impactLow := ""
	impactHigh := ""

	if rule, ok := pgCuratedRules[name]; ok {
		isCurated = true
		category = rule.Category
		recommendation = rule.Recommendation
		suggestedVal = rule.SuggestedValue
		impactLow = rule.ImpactLow
		impactHigh = rule.ImpactHigh
		if rule.Evaluate != nil {
			severity = rule.Evaluate(setting, unit)
		} else {
			severity = "optimal"
		}
	}

	displayVal := setting
	if unit != "" && setting != "-1" {
		displayVal = setting + " (" + unit + ")"
	}
	escapedSetting := strings.ReplaceAll(setting, "'", "''")

	return driver.ServerVariable{
		Name:             name,
		Value:            displayVal,
		Scope:            scope,
		IsDynamic:        isDynamic,
		Category:         category,
		Description:      shortDesc,
		ImpactLow:        impactLow,
		ImpactHigh:       impactHigh,
		Recommendation:   recommendation,
		SuggestedValue:   suggestedVal,
		Severity:         severity,
		IsCurated:        isCurated,
		MyCnfSnippet:     fmt.Sprintf("%s = '%s'", name, escapedSetting),
		SetGlobalSnippet: fmt.Sprintf("ALTER SYSTEM SET %s = '%s';", name, escapedSetting),
	}
}

func (s *PostgresSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name, setting, COALESCE(unit, ''), category, short_desc, context FROM pg_settings ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to introspect pg_settings: %w", err)
	}
	defer rows.Close()

	var vars []driver.ServerVariable
	curatedCount := 0
	for rows.Next() {
		var name, setting, unit, category, shortDesc, contextStr string
		if err := rows.Scan(&name, &setting, &unit, &category, &shortDesc, &contextStr); err == nil {
			sv := EvaluatePostgresVariable(name, setting, unit, category, shortDesc, contextStr)
			if sv.IsCurated {
				curatedCount++
			}
			vars = append(vars, sv)
		}
	}

	var version string
	_ = s.db.QueryRowContext(ctx, "SELECT version()").Scan(&version)

	return &driver.ServerVariablesReport{
		Engine:       "postgres",
		Version:      version,
		TotalCount:   len(vars),
		CuratedCount: curatedCount,
		Variables:    vars,
	}, nil
}

func (s *PostgresSession) InspectDeadlock(ctx context.Context) (*driver.DeadlockReport, error) {
	return &driver.DeadlockReport{
		HasDeadlock: false,
		Summary:     "PostgreSQL deadlock post-mortem is captured in pg_stat_activity / server logs.",
	}, nil
}

func (s *PostgresSession) GetStatementDigest(ctx context.Context, limit int) ([]driver.StatementDigestEntry, error) {
	if limit <= 0 {
		limit = 25
	}
	// ponytail: Adaptive Telemetry Probe: check pg_stat_statements (PG13+ uses total_exec_time, PG<13 uses total_time)
	query := fmt.Sprintf(`
		SELECT 
			query,
			calls,
			ROUND(total_exec_time::numeric, 2) as total_latency,
			ROUND(mean_exec_time::numeric, 2) as avg_latency,
			ROUND(max_exec_time::numeric, 2) as max_latency,
			0 as lock_time,
			rows,
			rows as rows_examined,
			0 as rows_affected,
			0 as no_index_used,
			'' as first_seen,
			'' as last_seen,
			queryid::text as digest
		FROM pg_stat_statements
		WHERE query IS NOT NULL AND TRIM(query) != ''
		ORDER BY total_exec_time DESC
		LIMIT %d;
	`, limit)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		fallbackQuery := fmt.Sprintf(`
			SELECT 
				query,
				calls,
				ROUND(total_time::numeric, 2) as total_latency,
				ROUND(mean_time::numeric, 2) as avg_latency,
				0 as max_latency,
				0 as lock_time,
				rows,
				rows as rows_examined,
				0 as rows_affected,
				0 as no_index_used,
				'' as first_seen,
				'' as last_seen,
				queryid::text as digest
			FROM pg_stat_statements
			WHERE query IS NOT NULL AND TRIM(query) != ''
			ORDER BY total_time DESC
			LIMIT %d;
		`, limit)
		rows, err = s.db.QueryContext(ctx, fallbackQuery)
	}
	if err != nil {
		return []driver.StatementDigestEntry{
			{
				Query:  "-- pg_stat_statements extension is not installed or enabled in shared_preload_libraries. Run: CREATE EXTENSION IF NOT EXISTS pg_stat_statements;",
				Digest: "uninstalled",
			},
		}, nil
	}
	defer rows.Close()

	var list []driver.StatementDigestEntry
	for rows.Next() {
		var e driver.StatementDigestEntry
		if err := rows.Scan(
			&e.Query, &e.ExecCount, &e.TotalLatencyMs, &e.AvgLatencyMs, &e.MaxLatencyMs,
			&e.LockTimeMs, &e.RowsSent, &e.RowsExamined, &e.RowsAffected,
			&e.NoIndexUsed, &e.FirstSeen, &e.LastSeen, &e.Digest,
		); err == nil {
			list = append(list, e)
		}
	}
	if list == nil {
		list = []driver.StatementDigestEntry{}
	}
	return list, nil
}

func (s *PostgresSession) ResetStatementDigest(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "SELECT pg_stat_statements_reset()")
	return err
}
