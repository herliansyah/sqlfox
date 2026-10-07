package mysql

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAdaptiveCorrelatedQueriesFiltering(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	schema := `
	CREATE TABLE PROCESSLIST (
		ID INTEGER,
		USER TEXT,
		HOST TEXT,
		DB TEXT,
		COMMAND TEXT,
		TIME INTEGER,
		STATE TEXT,
		INFO TEXT
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Insert fixtures:
	// 1. Long-running system user daemon (replication/internal worker)
	// 2. Event scheduler daemon
	// 3. Binlog dump thread
	// 4. Idle connection in sleep state
	// 5. Normal user query running
	// 6. Slow user query running
	// 7. Active thread with blank/whitespace query
	fixtures := `
	INSERT INTO PROCESSLIST (ID, USER, HOST, DB, COMMAND, TIME, STATE, INFO) VALUES
	(1, 'system user', '', NULL, 'Connect', 98765, 'Waiting for master', NULL),
	(2, 'event_scheduler', 'localhost', NULL, 'Daemon', 98765, 'Waiting on empty queue', ''),
	(3, 'repl', '192.168.1.10:1234', NULL, 'Binlog Dump', 54321, 'Master has sent all binlog', NULL),
	(4, 'app_user', '10.0.0.1:4001', 'shop', 'Sleep', 300, '', ''),
	(5, 'app_user', '10.0.0.1:4002', 'shop', 'Query', 2, 'Sending data', 'SELECT * FROM users WHERE id = 42'),
	(6, 'app_user', '10.0.0.1:4003', 'shop', 'Query', 15, 'Copying to tmp table', 'SELECT count(*) FROM orders GROUP BY user_id'),
	(7, 'app_user', '10.0.0.1:4004', 'shop', 'Query', 50, 'executing', '   ');
	`
	if _, err := db.Exec(fixtures); err != nil {
		t.Fatalf("failed to insert fixtures: %v", err)
	}

	// Run the exact filter query used in MySQLSession.SampleFlightMetrics
	query := `
		SELECT ID, USER, HOST, IFNULL(DB, ''), COMMAND, TIME, IFNULL(STATE, ''), IFNULL(INFO, '')
		FROM PROCESSLIST
		WHERE USER NOT IN ('system user', 'event_scheduler')
		  AND COMMAND NOT IN ('Sleep', 'Binlog Dump', 'Daemon')
		  AND INFO IS NOT NULL AND TRIM(INFO) != ''
		ORDER BY TIME DESC
		LIMIT 5
	`

	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	type resultRow struct {
		id   int64
		user string
		info string
		time int64
	}
	var results []resultRow
	for rows.Next() {
		var r resultRow
		var host, dbName, cmd, state string
		if err := rows.Scan(&r.id, &r.user, &host, &dbName, &cmd, &r.time, &state, &r.info); err != nil {
			t.Fatalf("scan error: %v", err)
		}
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 correlated queries, got %d", len(results))
	}

	// 1st row must be the slower query (ID 6, time 15), NOT system user
	if results[0].id != 6 || results[0].user != "app_user" || results[0].time != 15 {
		t.Errorf("expected row 0 to be query ID 6 (slowest active query), got %+v", results[0])
	}
	// 2nd row must be the faster query (ID 5, time 2)
	if results[1].id != 5 || results[1].user != "app_user" || results[1].time != 2 {
		t.Errorf("expected row 1 to be query ID 5, got %+v", results[1])
	}
}
