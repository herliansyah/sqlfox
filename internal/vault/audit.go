package vault

import (
	"database/sql"
	"fmt"
	"time"
)

// AuditLogRecord tracks an executed SQL statement within a session.
type AuditLogRecord struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	ConnectionID string    `json:"connection_id"`
	QuerySQL     string    `json:"query_sql"`
	DurationMs   int64     `json:"duration_ms"`
	RowsAffected int64     `json:"rows_affected"`
	Error        string    `json:"error"`
	ExecutedAt   time.Time `json:"executed_at"`
}

// RecordAuditLog writes an execution record to the audit ledger in the vault.
func (v *Vault) RecordAuditLog(log *AuditLogRecord) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if log.ID == "" {
		log.ID = newUUID()
	}
	if log.ExecutedAt.IsZero() {
		log.ExecutedAt = time.Now()
	}

	query := `
	INSERT INTO audit_logs (id, session_id, connection_id, query_sql, duration_ms, rows_affected, error, executed_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := v.db.Exec(query,
		log.ID,
		log.SessionID,
		log.ConnectionID,
		log.QuerySQL,
		log.DurationMs,
		log.RowsAffected,
		log.Error,
		log.ExecutedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record audit log: %w", err)
	}
	return nil
}

// ListAuditLogs returns paginated audit records for a session, optionally filtered by search text.
func (v *Vault) ListAuditLogs(sessionID string, limit, offset int, search string) ([]*AuditLogRecord, int, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM audit_logs WHERE (session_id = ? OR connection_id = ?)"
	countArgs := []any{sessionID, sessionID}

	if search != "" {
		countQuery += " AND query_sql LIKE ?"
		countArgs = append(countArgs, "%"+search+"%")
	}

	if err := v.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	selectQuery := `
	SELECT id, session_id, connection_id, query_sql, duration_ms, rows_affected, error, executed_at
	FROM audit_logs
	WHERE (session_id = ? OR connection_id = ?)
	`
	selectArgs := []any{sessionID, sessionID}

	if search != "" {
		selectQuery += " AND query_sql LIKE ?"
		selectArgs = append(selectArgs, "%"+search+"%")
	}

	selectQuery += " ORDER BY executed_at DESC LIMIT ? OFFSET ?"
	selectArgs = append(selectArgs, limit, offset)

	rows, err := v.db.Query(selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*AuditLogRecord
	for rows.Next() {
		var l AuditLogRecord
		var errStr sql.NullString
		if err := rows.Scan(
			&l.ID,
			&l.SessionID,
			&l.ConnectionID,
			&l.QuerySQL,
			&l.DurationMs,
			&l.RowsAffected,
			&errStr,
			&l.ExecutedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit log: %w", err)
		}
		l.Error = errStr.String
		logs = append(logs, &l)
	}

	if logs == nil {
		logs = []*AuditLogRecord{}
	}

	return logs, total, rows.Err()
}

// ClearAuditLogs removes all audit records for a given session.
func (v *Vault) ClearAuditLogs(sessionID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	_, err := v.db.Exec("DELETE FROM audit_logs WHERE session_id = ? OR connection_id = ?", sessionID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to clear audit logs: %w", err)
	}
	return nil
}
