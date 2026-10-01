package vault

import (
	"database/sql"
	"fmt"
	"time"
)

// FlightRecordingSession metadata for a performance flight recorder capture.
type FlightRecordingSession struct {
	ID              string     `json:"id"`
	SessionID       string     `json:"session_id"`
	ConnectionID    string     `json:"connection_id"`
	Name            string     `json:"name"`
	DurationSeconds int64      `json:"duration_seconds"`
	SampleCount     int        `json:"sample_count"`
	StartedAt       time.Time  `json:"started_at"`
	StoppedAt       *time.Time `json:"stopped_at,omitempty"`
}

// FlightRecordingTick represents a single sampled second of database telemetry.
type FlightRecordingTick struct {
	ID                 string    `json:"id"`
	RecordingID        string    `json:"recording_id"`
	TickIndex          int       `json:"tick_index"`
	SampledAt          time.Time `json:"sampled_at"`
	QPS                int64     `json:"qps"`
	ThreadsRunning     int64     `json:"threads_running"`
	ThreadsConnected   int64     `json:"threads_connected"`
	RowLockWaits       int64     `json:"row_lock_waits"`
	BufferPoolWaitFree int64     `json:"buffer_pool_wait_free"`
	TopQueriesJSON     string    `json:"top_queries_json"`
}

// FlightRecordingDetail combines session metadata with all chronological ticks.
type FlightRecordingDetail struct {
	Session *FlightRecordingSession `json:"session"`
	Ticks   []*FlightRecordingTick  `json:"ticks"`
}

// CreateFlightRecordingSession registers a new recording run in the vault.
func (v *Vault) CreateFlightRecordingSession(s *FlightRecordingSession) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if s.ID == "" {
		s.ID = newUUID()
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = time.Now()
	}

	query := `
	INSERT INTO flight_recorder_sessions (id, session_id, connection_id, name, duration_seconds, sample_count, started_at, stopped_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := v.db.Exec(query,
		s.ID,
		s.SessionID,
		s.ConnectionID,
		s.Name,
		s.DurationSeconds,
		s.SampleCount,
		s.StartedAt,
		s.StoppedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create flight recording session: %w", err)
	}
	return nil
}

// AppendFlightRecordingTick records a single point in the time-series.
func (v *Vault) AppendFlightRecordingTick(t *FlightRecordingTick) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if t.ID == "" {
		t.ID = newUUID()
	}
	if t.SampledAt.IsZero() {
		t.SampledAt = time.Now()
	}
	if t.TopQueriesJSON == "" {
		t.TopQueriesJSON = "[]"
	}

	query := `
	INSERT INTO flight_recorder_ticks (id, recording_id, tick_index, sampled_at, qps, threads_running, threads_connected, row_lock_waits, buffer_pool_wait_free, top_queries_json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := v.db.Exec(query,
		t.ID,
		t.RecordingID,
		t.TickIndex,
		t.SampledAt,
		t.QPS,
		t.ThreadsRunning,
		t.ThreadsConnected,
		t.RowLockWaits,
		t.BufferPoolWaitFree,
		t.TopQueriesJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to append flight recording tick: %w", err)
	}
	return nil
}

// FinishFlightRecordingSession marks a recording run as completed.
func (v *Vault) FinishFlightRecordingSession(recordingID string, durationSeconds int64, sampleCount int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	query := `
	UPDATE flight_recorder_sessions
	SET duration_seconds = ?, sample_count = ?, stopped_at = ?
	WHERE id = ?
	`
	_, err := v.db.Exec(query, durationSeconds, sampleCount, now, recordingID)
	if err != nil {
		return fmt.Errorf("failed to finish flight recording session: %w", err)
	}
	return nil
}

// ListFlightRecordingSessions returns all recording sessions for a given session or connection.
func (v *Vault) ListFlightRecordingSessions(sessionID string) ([]*FlightRecordingSession, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	query := `
	SELECT id, session_id, connection_id, name, duration_seconds, sample_count, started_at, stopped_at
	FROM flight_recorder_sessions
	WHERE session_id = ? OR connection_id = ?
	ORDER BY started_at DESC
	LIMIT 50
	`
	rows, err := v.db.Query(query, sessionID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list flight recording sessions: %w", err)
	}
	defer rows.Close()

	var result []*FlightRecordingSession
	for rows.Next() {
		var s FlightRecordingSession
		var stoppedAt sql.NullTime
		if err := rows.Scan(
			&s.ID,
			&s.SessionID,
			&s.ConnectionID,
			&s.Name,
			&s.DurationSeconds,
			&s.SampleCount,
			&s.StartedAt,
			&stoppedAt,
		); err != nil {
			return nil, err
		}
		if stoppedAt.Valid {
			s.StoppedAt = &stoppedAt.Time
		}
		result = append(result, &s)
	}
	if result == nil {
		result = []*FlightRecordingSession{}
	}
	return result, nil
}

// GetFlightRecordingSessionWithTicks retrieves session metadata and all recorded ticks.
func (v *Vault) GetFlightRecordingSessionWithTicks(recordingID string) (*FlightRecordingDetail, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var s FlightRecordingSession
	var stoppedAt sql.NullTime
	sessionQuery := `
	SELECT id, session_id, connection_id, name, duration_seconds, sample_count, started_at, stopped_at
	FROM flight_recorder_sessions
	WHERE id = ?
	`
	err := v.db.QueryRow(sessionQuery, recordingID).Scan(
		&s.ID,
		&s.SessionID,
		&s.ConnectionID,
		&s.Name,
		&s.DurationSeconds,
		&s.SampleCount,
		&s.StartedAt,
		&stoppedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("recording session not found")
		}
		return nil, fmt.Errorf("failed to get flight recording session: %w", err)
	}
	if stoppedAt.Valid {
		s.StoppedAt = &stoppedAt.Time
	}

	ticksQuery := `
	SELECT id, recording_id, tick_index, sampled_at, qps, threads_running, threads_connected, row_lock_waits, buffer_pool_wait_free, top_queries_json
	FROM flight_recorder_ticks
	WHERE recording_id = ?
	ORDER BY tick_index ASC
	`
	rows, err := v.db.Query(ticksQuery, recordingID)
	if err != nil {
		return nil, fmt.Errorf("failed to query flight recording ticks: %w", err)
	}
	defer rows.Close()

	var ticks []*FlightRecordingTick
	for rows.Next() {
		var t FlightRecordingTick
		if err := rows.Scan(
			&t.ID,
			&t.RecordingID,
			&t.TickIndex,
			&t.SampledAt,
			&t.QPS,
			&t.ThreadsRunning,
			&t.ThreadsConnected,
			&t.RowLockWaits,
			&t.BufferPoolWaitFree,
			&t.TopQueriesJSON,
		); err != nil {
			return nil, err
		}
		ticks = append(ticks, &t)
	}
	if ticks == nil {
		ticks = []*FlightRecordingTick{}
	}

	return &FlightRecordingDetail{
		Session: &s,
		Ticks:   ticks,
	}, nil
}

// DeleteFlightRecordingSession deletes a recording and its associated ticks.
func (v *Vault) DeleteFlightRecordingSession(recordingID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	_, _ = v.db.Exec("DELETE FROM flight_recorder_ticks WHERE recording_id = ?", recordingID)
	_, err := v.db.Exec("DELETE FROM flight_recorder_sessions WHERE id = ?", recordingID)
	if err != nil {
		return fmt.Errorf("failed to delete flight recording session: %w", err)
	}
	return nil
}

// PurgeOldFlightRecordings deletes all recordings older than the specified duration (e.g. 24 hours).
func (v *Vault) PurgeOldFlightRecordings(olderThan time.Duration) (int64, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	threshold := time.Now().Add(-olderThan)
	// Delete orphan ticks first
	_, _ = v.db.Exec(`
		DELETE FROM flight_recorder_ticks
		WHERE recording_id IN (
			SELECT id FROM flight_recorder_sessions WHERE started_at < ?
		)
	`, threshold)

	res, err := v.db.Exec("DELETE FROM flight_recorder_sessions WHERE started_at < ?", threshold)
	if err != nil {
		return 0, fmt.Errorf("failed to purge old flight recordings: %w", err)
	}
	return res.RowsAffected()
}
