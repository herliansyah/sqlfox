package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"sqlfox/internal/vault"
)

// ActiveRecording represents an in-flight recording session.
type ActiveRecording struct {
	ID           string
	SessionID    string
	ConnectionID string
	Name         string
	StartedAt    time.Time
	Interval     time.Duration
	cancel       context.CancelFunc
	sampleCount  int

	prevQuestions int64
	prevSampleAt  time.Time

	mu          sync.RWMutex
	recentTicks []*vault.FlightRecordingTick
}

// FlightRecorderManager oversees on-demand recording lifecycles.
type FlightRecorderManager struct {
	vault      *vault.Vault
	sessionMgr *SessionManager
	active     map[string]*ActiveRecording // key: sessionID/connID
	mu         sync.Mutex
}

// NewFlightRecorderManager creates a manager and triggers a background 24h purge.
func NewFlightRecorderManager(v *vault.Vault, sm *SessionManager) *FlightRecorderManager {
	mgr := &FlightRecorderManager{
		vault:      v,
		sessionMgr: sm,
		active:     make(map[string]*ActiveRecording),
	}

	// Purge stale recordings older than 24 hours on startup
	go func() {
		_, _ = v.PurgeOldFlightRecordings(24 * time.Hour)
	}()

	return mgr
}

// StartRecording initiates an on-demand telemetry capture with a 10-minute auto-stop safety limit.
func (m *FlightRecorderManager) StartRecording(sessionID, connID, name string, intervalMs int) (*ActiveRecording, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.active[sessionID]; exists {
		return nil, fmt.Errorf("flight recorder is already running for this session")
	}

	if intervalMs < 1000 {
		intervalMs = 1000
	} else if intervalMs > 5000 {
		intervalMs = 5000
	}
	interval := time.Duration(intervalMs) * time.Millisecond

	if name == "" {
		name = fmt.Sprintf("Recording %s", time.Now().Format("2006-01-02 15:04:05"))
	}

	recSession := &vault.FlightRecordingSession{
		SessionID:    sessionID,
		ConnectionID: connID,
		Name:         name,
		StartedAt:    time.Now(),
	}

	if err := m.vault.CreateFlightRecordingSession(recSession); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	rec := &ActiveRecording{
		ID:           recSession.ID,
		SessionID:    sessionID,
		ConnectionID: connID,
		Name:         name,
		StartedAt:    recSession.StartedAt,
		Interval:     interval,
		cancel:       cancel,
		recentTicks:  make([]*vault.FlightRecordingTick, 0, 100),
	}
	m.active[sessionID] = rec

	go m.runLoop(ctx, rec)
	return rec, nil
}

// StopRecording terminates an active recording and finalizes session metrics in the vault.
func (m *FlightRecorderManager) StopRecording(sessionID string) (*vault.FlightRecordingSession, error) {
	m.mu.Lock()
	rec, exists := m.active[sessionID]
	if !exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("no active flight recording found for this session")
	}
	delete(m.active, sessionID)
	m.mu.Unlock()

	rec.cancel()

	durationSec := int64(time.Since(rec.StartedAt).Seconds())
	_ = m.vault.FinishFlightRecordingSession(rec.ID, durationSec, rec.sampleCount)

	detail, err := m.vault.GetFlightRecordingSessionWithTicks(rec.ID)
	if err != nil {
		return nil, err
	}
	return detail.Session, nil
}

// GetStatus returns the current active recording state and recent ticks for live charts.
func (m *FlightRecorderManager) GetStatus(sessionID string) (map[string]any, error) {
	m.mu.Lock()
	rec, exists := m.active[sessionID]
	m.mu.Unlock()

	if !exists {
		return map[string]any{
			"is_recording": false,
		}, nil
	}

	rec.mu.RLock()
	defer rec.mu.RUnlock()

	durationSec := int64(time.Since(rec.StartedAt).Seconds())
	return map[string]any{
		"is_recording":     true,
		"recording_id":     rec.ID,
		"name":             rec.Name,
		"started_at":       rec.StartedAt,
		"duration_seconds": durationSec,
		"sample_count":     rec.sampleCount,
		"interval_ms":      rec.Interval.Milliseconds(),
		"recent_ticks":     rec.recentTicks,
	}, nil
}

func (m *FlightRecorderManager) runLoop(ctx context.Context, rec *ActiveRecording) {
	ticker := time.NewTicker(rec.Interval)
	defer ticker.Stop()

	defer func() {
		m.mu.Lock()
		if current, ok := m.active[rec.SessionID]; ok && current.ID == rec.ID {
			delete(m.active, rec.SessionID)
			durationSec := int64(time.Since(rec.StartedAt).Seconds())
			_ = m.vault.FinishFlightRecordingSession(rec.ID, durationSec, rec.sampleCount)
		}
		m.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			sess, err := m.sessionMgr.GetSession(rec.ConnectionID)
			if err != nil {
				return
			}

			sampleCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			sample, err := sess.SampleFlightMetrics(sampleCtx)
			cancel()
			if err != nil {
				continue
			}

			// Compute differential QPS
			var qps int64
			if rec.prevQuestions > 0 && !rec.prevSampleAt.IsZero() {
				deltaQ := sample.Questions - rec.prevQuestions
				deltaSec := now.Sub(rec.prevSampleAt).Seconds()
				if deltaSec > 0 && deltaQ >= 0 {
					qps = int64(float64(deltaQ) / deltaSec)
				}
			}
			rec.prevQuestions = sample.Questions
			rec.prevSampleAt = now

			queriesJSON, _ := json.Marshal(sample.ActiveQueries)

			tick := &vault.FlightRecordingTick{
				RecordingID:        rec.ID,
				TickIndex:          rec.sampleCount,
				SampledAt:          now,
				QPS:                qps,
				ThreadsRunning:     sample.ThreadsRunning,
				ThreadsConnected:   sample.ThreadsConnected,
				RowLockWaits:       sample.RowLockWaits,
				BufferPoolWaitFree: sample.BufferPoolWaitFree,
				TopQueriesJSON:     string(queriesJSON),
			}

			_ = m.vault.AppendFlightRecordingTick(tick)

			rec.mu.Lock()
			rec.sampleCount++
			if len(rec.recentTicks) >= 120 {
				rec.recentTicks = rec.recentTicks[1:]
			}
			rec.recentTicks = append(rec.recentTicks, tick)
			rec.mu.Unlock()
		}
	}
}
