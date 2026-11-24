package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/advisor"
	"github.com/herliansyah/sqlfox/internal/driver"
	"strconv"
	"strings"
)

func (s *MySQLSession) GetProcesslist(ctx context.Context) (*driver.ProcesslistResult, error) {
	// 1. Get current connection thread ID
	var currentThreadID int64
	_ = s.db.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&currentThreadID)

	// 2. Fetch processes: prefer performance_schema.threads (lock-free) to prevent LOCK_thread_count mutex contention
	var threads []driver.ProcessItem
	if s.pfsEnabled {
		pfsRows, pfsErr := s.db.QueryContext(ctx, `
			SELECT PROCESSLIST_ID, IFNULL(PROCESSLIST_USER, ''), IFNULL(PROCESSLIST_HOST, ''),
			       IFNULL(PROCESSLIST_DB, ''), IFNULL(PROCESSLIST_COMMAND, ''), IFNULL(PROCESSLIST_TIME, 0),
			       IFNULL(PROCESSLIST_STATE, ''), IFNULL(PROCESSLIST_INFO, '')
			FROM performance_schema.threads
			WHERE PROCESSLIST_ID IS NOT NULL
			ORDER BY PROCESSLIST_TIME DESC
		`)
		if pfsErr == nil {
			defer pfsRows.Close()
			for pfsRows.Next() {
				var p driver.ProcessItem
				if err := pfsRows.Scan(&p.ID, &p.User, &p.Host, &p.DB, &p.Command, &p.Time, &p.State, &p.Info); err == nil {
					threads = append(threads, p)
				}
			}
		}
	}

	// Fallback to information_schema.PROCESSLIST if performance_schema is disabled or empty
	if len(threads) == 0 {
		rows, err := s.db.QueryContext(ctx, `
			SELECT IFNULL(ID, 0), IFNULL(USER, ''), IFNULL(HOST, ''), IFNULL(DB, ''), IFNULL(COMMAND, ''), IFNULL(TIME, 0), IFNULL(STATE, ''), IFNULL(INFO, '')
			FROM information_schema.PROCESSLIST
			ORDER BY TIME DESC
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p driver.ProcessItem
				if err := rows.Scan(&p.ID, &p.User, &p.Host, &p.DB, &p.Command, &p.Time, &p.State, &p.Info); err == nil {
					threads = append(threads, p)
				}
			}
		}
	}

	// Fallback to SHOW FULL PROCESSLIST if still empty or restricted
	if len(threads) == 0 {
		fallbackRows, fErr := s.db.QueryContext(ctx, "SHOW FULL PROCESSLIST")
		if fErr != nil {
			return nil, fmt.Errorf("failed to fetch processlist: %w", fErr)
		}
		defer fallbackRows.Close()
		for fallbackRows.Next() {
			var id sql.NullInt64
			var user, host, command sql.NullString
			var db, state, info sql.NullString
			var timeVal sql.NullInt64
			if err := fallbackRows.Scan(&id, &user, &host, &db, &command, &timeVal, &state, &info); err == nil {
				threads = append(threads, driver.ProcessItem{
					ID:      id.Int64,
					User:    user.String,
					Host:    host.String,
					DB:      db.String,
					Command: command.String,
					Time:    timeVal.Int64,
					State:   state.String,
					Info:    info.String,
				})
			}
		}
	}

	if threads == nil {
		threads = []driver.ProcessItem{}
	}

	// 3. Inspect Lock Waits (Tier 1: sys.innodb_lock_waits, fallback: information_schema.innodb_lock_waits for MariaDB / MySQL 5.7)
	var locks []driver.LockWait
	lockRows, lErr := s.db.QueryContext(ctx, `
		SELECT
			IFNULL(waiting_pid, 0),
			IFNULL(waiting_query, ''),
			'',
			IFNULL(wait_age_secs, 0),
			IFNULL(blocking_pid, 0),
			IFNULL(blocking_query, ''),
			'',
			IFNULL(locked_table, ''),
			IFNULL(waiting_lock_mode, '')
		FROM sys.innodb_lock_waits
	`)
	if lErr != nil {
		// ponytail: MariaDB / MySQL 5.7 without sys schema fallback to information_schema
		lockRows, lErr = s.db.QueryContext(ctx, `
			SELECT
				IFNULL(r.trx_mysql_thread_id, 0),
				IFNULL(r.trx_query, ''),
				'',
				IFNULL(TIMESTAMPDIFF(SECOND, r.trx_wait_started, NOW()), 0),
				IFNULL(b.trx_mysql_thread_id, 0),
				IFNULL(b.trx_query, ''),
				'',
				IFNULL(rl.lock_table, ''),
				IFNULL(rl.lock_mode, '')
			FROM information_schema.innodb_lock_waits w
			JOIN information_schema.innodb_trx r ON r.trx_id = w.requesting_trx_id
			JOIN information_schema.innodb_trx b ON b.trx_id = w.blocking_trx_id
			LEFT JOIN information_schema.innodb_locks rl ON rl.lock_id = w.requested_lock_id
		`)
	}
	if lErr == nil {
		defer lockRows.Close()
		waitingPIDs := make(map[int64]bool)
		for lockRows.Next() {
			var lw driver.LockWait
			if err := lockRows.Scan(
				&lw.WaitingThreadID,
				&lw.WaitingQuery,
				&lw.WaitingUser,
				&lw.WaitingSeconds,
				&lw.BlockingThreadID,
				&lw.BlockingQuery,
				&lw.BlockingUser,
				&lw.LockedResource,
				&lw.LockMode,
			); err == nil {
				waitingPIDs[lw.WaitingThreadID] = true
				locks = append(locks, lw)
			}
		}

		// Identify root blockers: a blocking PID that is NOT currently waiting on any lock
		for i := range locks {
			if !waitingPIDs[locks[i].BlockingThreadID] {
				locks[i].IsRootBlocker = true
			}
		}
	} else {
		// Tier 2 Fallback: heuristic scan of threads with lock states
		for _, t := range threads {
			st := strings.ToLower(t.State)
			if strings.Contains(st, "lock") || strings.Contains(st, "waiting for table") {
				locks = append(locks, driver.LockWait{
					WaitingThreadID: t.ID,
					WaitingQuery:    t.Info,
					WaitingUser:     t.User,
					WaitingSeconds:  t.Time,
					LockedResource:  t.DB,
					LockMode:        t.State,
					IsRootBlocker:   false,
				})
			}
		}
	}

	if locks == nil {
		locks = []driver.LockWait{}
	}

	var questions int64
	var qName string
	_ = s.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Questions'").Scan(&qName, &questions)

	return &driver.ProcesslistResult{
		CurrentThreadID: currentThreadID,
		Threads:         threads,
		Locks:           locks,
		Questions:       questions,
	}, nil
}

func (s *MySQLSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	if threadID <= 0 {
		return errors.New("invalid thread id")
	}

	var cmd string
	if killConnection {
		cmd = fmt.Sprintf("KILL %d", threadID)
	} else {
		cmd = fmt.Sprintf("KILL QUERY %d", threadID)
	}

	// ponytail: Dial independent single-shot control connection to bypass saturated s.db pool
	if s.dsn != "" {
		controlDB, dErr := sql.Open("mysql", s.dsn)
		if dErr == nil {
			defer controlDB.Close()
			if _, kErr := controlDB.ExecContext(ctx, cmd); kErr == nil {
				return nil
			}
		}
	}

	if s.db != nil {
		_, err := s.db.ExecContext(ctx, cmd)
		return err
	}
	return errors.New("database connection not available")
}

// GetDatabaseDiagnostics returns physical storage and metadata for all databases on the MySQL server.
func (s *MySQLSession) GetDatabaseDiagnostics(ctx context.Context) ([]driver.DatabaseDiagnostics, error) {
	query := `
		SELECT 
			s.SCHEMA_NAME, 
			IFNULL(s.DEFAULT_CHARACTER_SET_NAME, ''), 
			IFNULL(s.DEFAULT_COLLATION_NAME, ''), 
			COUNT(t.TABLE_NAME), 
			IFNULL(SUM(t.DATA_LENGTH), 0), 
			IFNULL(SUM(t.INDEX_LENGTH), 0), 
			IFNULL(SUM(t.DATA_FREE), 0)
		FROM information_schema.SCHEMATA s
		LEFT JOIN information_schema.TABLES t ON s.SCHEMA_NAME = t.TABLE_SCHEMA
		GROUP BY s.SCHEMA_NAME, s.DEFAULT_CHARACTER_SET_NAME, s.DEFAULT_COLLATION_NAME
		ORDER BY 
			CASE WHEN s.SCHEMA_NAME IN ('information_schema', 'mysql', 'performance_schema', 'sys') THEN 1 ELSE 0 END,
			(IFNULL(SUM(t.DATA_LENGTH), 0) + IFNULL(SUM(t.INDEX_LENGTH), 0)) DESC,
			s.SCHEMA_NAME ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query database diagnostics: %w", err)
	}
	defer rows.Close()

	var result []driver.DatabaseDiagnostics
	for rows.Next() {
		var dd driver.DatabaseDiagnostics
		if err := rows.Scan(
			&dd.Name,
			&dd.Charset,
			&dd.Collation,
			&dd.TableCount,
			&dd.DataSize,
			&dd.IndexSize,
			&dd.OverheadSize,
		); err == nil {
			dd.TotalSize = dd.DataSize + dd.IndexSize
			lowerName := strings.ToLower(dd.Name)
			dd.IsSystem = lowerName == "information_schema" || lowerName == "mysql" || lowerName == "performance_schema" || lowerName == "sys"
			result = append(result, dd)
		}
	}
	if result == nil {
		result = []driver.DatabaseDiagnostics{}
	}
	return result, nil
}

func (s *MySQLSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	if database == "" {
		database = s.activeDB()
	}
	query := `
		SELECT 
			t.TABLE_NAME, 
			IFNULL(t.ENGINE, ''), 
			IFNULL(t.TABLE_ROWS, 0), 
			IFNULL(t.DATA_LENGTH, 0), 
			IFNULL(t.INDEX_LENGTH, 0), 
			IFNULL(t.DATA_FREE, 0), 
			IFNULL(t.ROW_FORMAT, ''), 
			IFNULL(t.TABLE_COLLATION, ''), 
			IFNULL(DATE_FORMAT(t.CREATE_TIME, '%Y-%m-%d %H:%i:%s'), ''), 
			IFNULL(DATE_FORMAT(t.UPDATE_TIME, '%Y-%m-%d %H:%i:%s'), ''),
			CASE WHEN pk.TABLE_NAME IS NOT NULL THEN 1 ELSE 0 END
		FROM information_schema.TABLES t
		LEFT JOIN (
			SELECT DISTINCT TABLE_NAME
			FROM information_schema.STATISTICS
			WHERE TABLE_SCHEMA = ? AND INDEX_NAME = 'PRIMARY'
		) pk ON t.TABLE_NAME = pk.TABLE_NAME
		WHERE t.TABLE_SCHEMA = ?
	`
	args := []any{database, database}
	if table != "" {
		query += " AND t.TABLE_NAME = ?"
		args = append(args, table)
	}
	query += " ORDER BY (t.DATA_LENGTH + t.INDEX_LENGTH) DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query table diagnostics: %w", err)
	}
	defer rows.Close()

	var result []driver.TableDiagnostics
	for rows.Next() {
		var td driver.TableDiagnostics
		var hasPk int
		if err := rows.Scan(
			&td.Name,
			&td.Engine,
			&td.Rows,
			&td.DataSize,
			&td.IndexSize,
			&td.OverheadSize,
			&td.RowFormat,
			&td.Collation,
			&td.CreatedAt,
			&td.UpdatedAt,
			&hasPk,
		); err == nil {
			td.HasPrimaryKey = hasPk == 1
			result = append(result, td)
		}
	}
	if result == nil {
		result = []driver.TableDiagnostics{}
	}
	return result, nil
}

func (s *MySQLSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	if database == "" {
		database = s.activeDB()
	}
	upperAction := strings.ToUpper(strings.TrimSpace(action))
	if upperAction != "OPTIMIZE" && upperAction != "ANALYZE" && upperAction != "CHECK" {
		return nil, fmt.Errorf("unsupported maintenance action: %s", action)
	}

	var results []driver.MaintenanceResult
	for _, tbl := range tables {
		sqlCmd := fmt.Sprintf("%s TABLE %s.%s", upperAction, driver.QuoteIdentifier(database, "mysql"), driver.QuoteIdentifier(tbl, "mysql"))
		rows, err := s.db.QueryContext(ctx, sqlCmd)
		if err != nil {
			results = append(results, driver.MaintenanceResult{
				Table:   tbl,
				Op:      upperAction,
				Status:  "Error",
				Message: err.Error(),
			})
			continue
		}
		for rows.Next() {
			var tblName, op, msgType, msgText string
			if err := rows.Scan(&tblName, &op, &msgType, &msgText); err == nil {
				results = append(results, driver.MaintenanceResult{
					Table:   tbl,
					Op:      op,
					Status:  msgType,
					Message: msgText,
				})
			}
		}
		rows.Close()
	}
	if results == nil {
		results = []driver.MaintenanceResult{}
	}
	return results, nil
}

func (s *MySQLSession) GetRedundantIndexes(ctx context.Context, database, table string) ([]driver.RedundantIndex, error) {
	if database == "" {
		database = s.activeDB()
	}
	schema, err := s.InspectSchema(ctx, database)
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
	return driver.DetectRedundantIndexes("mysql", database, tables), nil
}

func (s *MySQLSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	rows, err := s.db.QueryContext(ctx, "SHOW GLOBAL VARIABLES")
	if err != nil {
		return nil, fmt.Errorf("failed to introspect global variables: %w", err)
	}
	defer rows.Close()

	allVars := make(map[string]string)
	type kv struct {
		k, v string
	}
	var pairs []kv

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			allVars[k] = v
			pairs = append(pairs, kv{k: k, v: v})
		}
	}

	var version string
	_ = s.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)

	var vars []driver.ServerVariable
	curatedCount := 0
	for _, p := range pairs {
		sv := advisor.EvaluateMySQLVariable(p.k, p.v, allVars)
		if sv.IsCurated {
			curatedCount++
		}
		vars = append(vars, sv)
	}

	return &driver.ServerVariablesReport{
		Engine:       "mysql",
		Version:      version,
		TotalCount:   len(vars),
		CuratedCount: curatedCount,
		Variables:    vars,
	}, nil
}

// InspectDeadlock parses SHOW ENGINE INNODB STATUS to extract the latest detected deadlock.
func (s *MySQLSession) InspectDeadlock(ctx context.Context) (*driver.DeadlockReport, error) {
	var statusType, statusName, engineStatus string
	row := s.db.QueryRowContext(ctx, "SHOW ENGINE INNODB STATUS")
	if err := row.Scan(&statusType, &statusName, &engineStatus); err != nil {
		return nil, fmt.Errorf("failed to execute SHOW ENGINE INNODB STATUS: %w", err)
	}

	return parseDeadlockStatus(engineStatus), nil
}

// parseDeadlockStatus extracts and formats deadlock information from engine status output.
func parseDeadlockStatus(raw string) *driver.DeadlockReport {
	deadlockHeader := "------------------------\nLATEST DETECTED DEADLOCK\n------------------------"
	deadlockEnd := "------------\nTRANSACTIONS\n------------"

	startIdx := strings.Index(raw, deadlockHeader)
	if startIdx == -1 {
		// ponytail: friendly empty state when no deadlock has occurred since server boot
		return &driver.DeadlockReport{
			HasDeadlock: false,
			Summary:     "No deadlocks detected since MySQL server startup.",
		}
	}

	content := raw[startIdx+len(deadlockHeader):]
	endIdx := strings.Index(content, deadlockEnd)
	var deadlockSection string
	if endIdx != -1 {
		deadlockSection = content[:endIdx]
	} else {
		// If end marker not found, take up to next major section boundary
		nextSec := strings.Index(content, "\n---")
		if nextSec != -1 {
			deadlockSection = content[:nextSec]
		} else {
			deadlockSection = content
		}
	}
	deadlockSection = strings.TrimSpace(deadlockSection)

	rep := &driver.DeadlockReport{
		HasDeadlock:  true,
		RawSection:   deadlockSection,
		Transactions: make([]driver.DeadlockTransaction, 0, 2),
	}

	// Extract timestamp e.g. "2026-10-08 04:15:30 0x7f8a3c007700"
	lines := strings.Split(deadlockSection, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		parts := strings.Fields(firstLine)
		if len(parts) >= 2 {
			rep.DetectedAt = parts[0] + " " + parts[1]
		}
	}

	// Extract WE ROLL BACK TRANSACTION (1) or (2)
	rollbackVictim := 0
	if idx := strings.Index(deadlockSection, "WE ROLL BACK TRANSACTION ("); idx != -1 {
		sub := deadlockSection[idx+len("WE ROLL BACK TRANSACTION ("):]
		if closeParen := strings.Index(sub, ")"); closeParen != -1 {
			numStr := sub[:closeParen]
			if n, err := strconv.Atoi(numStr); err == nil {
				rollbackVictim = n
			}
		}
	}

	// Split by "*** (1) TRANSACTION:" and "*** (2) TRANSACTION:"
	t1Idx := strings.Index(deadlockSection, "*** (1) TRANSACTION:")
	t2Idx := strings.Index(deadlockSection, "*** (2) TRANSACTION:")

	if t1Idx != -1 && t2Idx != -1 && t2Idx > t1Idx {
		t1Chunk := deadlockSection[t1Idx:t2Idx]
		t2Chunk := deadlockSection[t2Idx:]
		if rbIdx := strings.Index(t2Chunk, "*** WE ROLL BACK TRANSACTION"); rbIdx != -1 {
			t2Chunk = t2Chunk[:rbIdx]
		}

		tx1 := parseDeadlockTransactionChunk(1, t1Chunk, rollbackVictim == 1)
		tx2 := parseDeadlockTransactionChunk(2, t2Chunk, rollbackVictim == 2)

		rep.Transactions = append(rep.Transactions, tx1, tx2)
		if tx1.IsVictim {
			rep.VictimTxID = tx1.TxID
			rep.VictimThreadID = tx1.ThreadID
			rep.Summary = fmt.Sprintf("Deadlock resolved: Transaction 1 (Thread %d) was rolled back to unblock Transaction 2.", tx1.ThreadID)
		} else if tx2.IsVictim {
			rep.VictimTxID = tx2.TxID
			rep.VictimThreadID = tx2.ThreadID
			rep.Summary = fmt.Sprintf("Deadlock resolved: Transaction 2 (Thread %d) was rolled back to unblock Transaction 1.", tx2.ThreadID)
		} else {
			rep.Summary = "Deadlock detected between Transaction 1 and Transaction 2."
		}
	} else {
		rep.Summary = "Deadlock detected. Review raw InnoDB status details."
	}

	return rep
}

// parseDeadlockTransactionChunk parses one transaction segment from a deadlock dump.
func parseDeadlockTransactionChunk(idx int, chunk string, isVictim bool) driver.DeadlockTransaction {
	tx := driver.DeadlockTransaction{
		Index:    idx,
		IsVictim: isVictim,
	}

	lines := strings.Split(chunk, "\n")
	readingQuery := false
	lockSection := ""
	var queryLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// TRANSACTION 123456, ACTIVE 5 sec
		if strings.HasPrefix(trimmed, "TRANSACTION ") {
			parts := strings.Split(trimmed, ",")
			if len(parts) > 0 {
				txPart := strings.TrimPrefix(parts[0], "TRANSACTION ")
				tx.TxID = strings.TrimSpace(txPart)
			}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(p, "ACTIVE ") {
					secStr := strings.TrimSuffix(strings.TrimPrefix(p, "ACTIVE "), " sec")
					if s, err := strconv.ParseInt(strings.TrimSpace(secStr), 10, 64); err == nil {
						tx.ActiveSeconds = s
					}
				}
			}
		}

		// MySQL thread id 42, OS thread handle ...
		if strings.Contains(trimmed, "MySQL thread id ") {
			if idIdx := strings.Index(trimmed, "MySQL thread id "); idIdx != -1 {
				sub := trimmed[idIdx+len("MySQL thread id "):]
				if comma := strings.Index(sub, ","); comma != -1 {
					thStr := strings.TrimSpace(sub[:comma])
					if thID, err := strconv.ParseInt(thStr, 10, 64); err == nil {
						tx.ThreadID = thID
					}
				}
			}
			if userIdx := strings.Index(trimmed, "query id "); userIdx != -1 {
				sub := trimmed[userIdx:]
				parts := strings.Fields(sub)
				if len(parts) >= 4 {
					tx.Host = parts[2]
					tx.AppUser = parts[3]
				}
			}
		}

		// undo log entries 2
		if strings.Contains(trimmed, "undo log entries ") {
			if uIdx := strings.Index(trimmed, "undo log entries "); uIdx != -1 {
				sub := trimmed[uIdx+len("undo log entries "):]
				fields := strings.Fields(sub)
				if len(fields) > 0 {
					if u, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
						tx.UndoLogEntries = u
					}
				}
			}
		}

		// Lock mode and wait. The header line only names the section; the lock
		// descriptor on the following line is what the reader needs.
		if strings.HasPrefix(trimmed, "*** (") && strings.Contains(trimmed, "WAITING FOR THIS LOCK TO BE GRANTED:") {
			lockSection = "waited"
			continue
		} else if strings.Contains(trimmed, "HOLDS THE LOCK(S):") {
			lockSection = "held"
			continue
		} else if strings.HasPrefix(trimmed, "RECORD LOCKS") || strings.HasPrefix(trimmed, "TABLE LOCK") {
			if tx.LockMode == "" {
				tx.LockMode = trimmed
			}
			switch lockSection {
			case "held":
				if tx.LockHeld == "" {
					tx.LockHeld = trimmed
					lockSection = ""
				}
			case "waited":
				if tx.LockWaited == "" {
					tx.LockWaited = trimmed
					lockSection = ""
				}
			}
		} else if lockSection != "" && !strings.HasPrefix(trimmed, "***") && trimmed != "" {
			// Any other line ends the lock descriptor block.
			lockSection = ""
		}

		// Query statement capture
		if trimmed == "*** (1) HOLDS THE LOCK(S):" || trimmed == "*** (2) HOLDS THE LOCK(S):" ||
			trimmed == "*** (1) WAITING FOR THIS LOCK TO BE GRANTED:" || trimmed == "*** (2) WAITING FOR THIS LOCK TO BE GRANTED:" {
			readingQuery = false
		}

		if readingQuery {
			if !strings.HasPrefix(trimmed, "***") {
				queryLines = append(queryLines, trimmed)
			}
		}

		if strings.HasPrefix(trimmed, "MySQL thread id") && strings.Contains(trimmed, "query id") {
			readingQuery = true
		}
	}

	if len(queryLines) > 0 {
		tx.Query = strings.TrimSpace(strings.Join(queryLines, " "))
	}

	return tx
}

// GetStatementDigest retrieves aggregated top SQL query statistics from sys/performance_schema.
func (s *MySQLSession) GetStatementDigest(ctx context.Context, limit int) ([]driver.StatementDigestEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	// performance_schema is optional (MariaDB may ship it disabled, sandboxes often
	// have it OFF). Probe first so we can explain the failure instead of showing a
	// raw "table doesn't exist" error inside the UI.
	var psEnabled string
	_ = s.db.QueryRowContext(ctx,
		"SELECT IFNULL(@@performance_schema, '0')").Scan(&psEnabled)
	psAvailable := false
	if psEnabled != "" && psEnabled != "0" {
		var probe int
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM performance_schema.events_statements_summary_by_digest").Scan(&probe); err == nil {
			psAvailable = true
		}
	}
	if !psAvailable {
		return nil, fmt.Errorf("performance_schema is disabled on this server (or the account lacks SELECT on it). Enable it with performance_schema=ON in my.cnf, then restart MySQL/MariaDB, to collect statement digests")
	}

	// 1. Try querying sys.statement_analysis (default in MySQL 5.7+ & 8.0)
	sysQuery := fmt.Sprintf(`
		SELECT 
			IFNULL(query, '') AS query_text,
			IFNULL(exec_count, 0) AS exec_count,
			IFNULL(ROUND(total_latency / 1000000, 2), 0) AS total_latency_ms,
			IFNULL(ROUND(avg_latency / 1000000, 2), 0) AS avg_latency_ms,
			IFNULL(ROUND(max_latency / 1000000, 2), 0) AS max_latency_ms,
			IFNULL(ROUND(lock_latency / 1000000, 2), 0) AS lock_time_ms,
			IFNULL(rows_sent, 0) AS rows_sent,
			IFNULL(rows_examined, 0) AS rows_examined,
			IFNULL(rows_affected, 0) AS rows_affected,
			IFNULL(full_scan, '') AS full_scan,
			IFNULL(no_index_used_count, 0) AS no_index_used,
			IFNULL(DATE_FORMAT(first_seen, '%%Y-%%m-%%d %%H:%%i:%%s'), '') AS first_seen,
			IFNULL(DATE_FORMAT(last_seen, '%%Y-%%m-%%d %%H:%%i:%%s'), '') AS last_seen,
			IFNULL(digest, '') AS digest
		FROM sys.statement_analysis
		WHERE query IS NOT NULL AND query != ''
		ORDER BY total_latency DESC
		LIMIT %d;
	`, limit)

	rows, err := s.db.QueryContext(ctx, sysQuery)
	if err == nil {
		defer rows.Close()
		var list []driver.StatementDigestEntry
		for rows.Next() {
			var e driver.StatementDigestEntry
			var fullScanStr string
			if err := rows.Scan(
				&e.Query, &e.ExecCount, &e.TotalLatencyMs, &e.AvgLatencyMs, &e.MaxLatencyMs,
				&e.LockTimeMs, &e.RowsSent, &e.RowsExamined, &e.RowsAffected,
				&fullScanStr, &e.NoIndexUsed, &e.FirstSeen, &e.LastSeen, &e.Digest,
			); err == nil {
				e.FullScan = strings.Contains(strings.ToLower(fullScanStr), "yes") || strings.Contains(strings.ToLower(fullScanStr), "1") || strings.Contains(strings.ToLower(fullScanStr), "*")
				list = append(list, e)
			}
		}
		if len(list) > 0 {
			return list, nil
		}
	}

	// 2. Fallback directly to performance_schema.events_statements_summary_by_digest
	psQuery := fmt.Sprintf(`
		SELECT 
			IFNULL(DIGEST_TEXT, '') AS query_text,
			IFNULL(COUNT_STAR, 0) AS exec_count,
			IFNULL(ROUND(SUM_TIMER_WAIT / 1000000000, 2), 0) AS total_latency_ms,
			IFNULL(ROUND(AVG_TIMER_WAIT / 1000000000, 2), 0) AS avg_latency_ms,
			IFNULL(ROUND(MAX_TIMER_WAIT / 1000000000, 2), 0) AS max_latency_ms,
			IFNULL(ROUND(SUM_LOCK_TIME / 1000000000, 2), 0) AS lock_time_ms,
			IFNULL(SUM_ROWS_SENT, 0) AS rows_sent,
			IFNULL(SUM_ROWS_EXAMINED, 0) AS rows_examined,
			IFNULL(SUM_ROWS_AFFECTED, 0) AS rows_affected,
			IFNULL(SUM_NO_INDEX_USED, 0) AS no_index_used,
			IFNULL(DATE_FORMAT(FIRST_SEEN, '%%Y-%%m-%%d %%H:%%i:%%s'), '') AS first_seen,
			IFNULL(DATE_FORMAT(LAST_SEEN, '%%Y-%%m-%%d %%H:%%i:%%s'), '') AS last_seen,
			IFNULL(DIGEST, '') AS digest
		FROM performance_schema.events_statements_summary_by_digest
		WHERE DIGEST_TEXT IS NOT NULL AND DIGEST_TEXT != ''
		ORDER BY SUM_TIMER_WAIT DESC
		LIMIT %d;
	`, limit)

	psRows, psErr := s.db.QueryContext(ctx, psQuery)
	if psErr != nil {
		return nil, fmt.Errorf("failed to read performance_schema statement digests: %w", psErr)
	}
	defer psRows.Close()

	var list []driver.StatementDigestEntry
	for psRows.Next() {
		var e driver.StatementDigestEntry
		if err := psRows.Scan(
			&e.Query, &e.ExecCount, &e.TotalLatencyMs, &e.AvgLatencyMs, &e.MaxLatencyMs,
			&e.LockTimeMs, &e.RowsSent, &e.RowsExamined, &e.RowsAffected,
			&e.NoIndexUsed, &e.FirstSeen, &e.LastSeen, &e.Digest,
		); err == nil {
			e.FullScan = e.NoIndexUsed > 0
			list = append(list, e)
		}
	}

	return list, nil
}

// ResetStatementDigest clears performance_schema statement digest metrics.
func (s *MySQLSession) ResetStatementDigest(ctx context.Context) error {
	// Try sys helper first
	_, err := s.db.ExecContext(ctx, "CALL sys.ps_truncate_all_tables(false)")
	if err == nil {
		return nil
	}

	// Fallback to truncating summary table directly
	_, tErr := s.db.ExecContext(ctx, "TRUNCATE TABLE performance_schema.events_statements_summary_by_digest")
	if tErr != nil {
		return fmt.Errorf("failed to reset statement digest: %w", tErr)
	}
	return nil
}

// GetRoutineDDL returns the raw CREATE statement for a Stored Procedure or Function.
