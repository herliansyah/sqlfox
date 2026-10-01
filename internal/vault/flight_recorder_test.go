package vault

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFlightRecorderVault(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "vault.db")

	v, err := OpenVault(dbPath, "test-master-pass")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}

	sessionID := "sess-123"
	connID := "conn-456"

	rec := &FlightRecordingSession{
		SessionID:    sessionID,
		ConnectionID: connID,
		Name:         "Incident Investigation #1",
		StartedAt:    time.Now().Add(-10 * time.Minute),
	}

	if err := v.CreateFlightRecordingSession(rec); err != nil {
		t.Fatalf("CreateFlightRecordingSession failed: %v", err)
	}
	if rec.ID == "" {
		t.Fatalf("expected generated ID, got empty")
	}

	// Append 3 ticks
	for i := 0; i < 3; i++ {
		tick := &FlightRecordingTick{
			RecordingID:        rec.ID,
			TickIndex:          i,
			SampledAt:          time.Now().Add(time.Duration(i) * time.Second),
			QPS:                int64(150 + i*50),
			ThreadsRunning:     int64(2 + i),
			ThreadsConnected:   25,
			RowLockWaits:       int64(i),
			BufferPoolWaitFree: 0,
			TopQueriesJSON:     `[{"thread_id": 101, "info": "SELECT * FROM orders WHERE status = 'pending'"}]`,
		}
		if err := v.AppendFlightRecordingTick(tick); err != nil {
			t.Fatalf("AppendFlightRecordingTick %d failed: %v", i, err)
		}
	}

	if err := v.FinishFlightRecordingSession(rec.ID, 3, 3); err != nil {
		t.Fatalf("FinishFlightRecordingSession failed: %v", err)
	}

	// List sessions
	list, err := v.ListFlightRecordingSessions(sessionID)
	if err != nil {
		t.Fatalf("ListFlightRecordingSessions failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list))
	}
	if list[0].DurationSeconds != 3 || list[0].SampleCount != 3 {
		t.Errorf("unexpected session stats: %+v", list[0])
	}

	// Get detail with ticks
	detail, err := v.GetFlightRecordingSessionWithTicks(rec.ID)
	if err != nil {
		t.Fatalf("GetFlightRecordingSessionWithTicks failed: %v", err)
	}
	if len(detail.Ticks) != 3 {
		t.Fatalf("expected 3 ticks, got %d", len(detail.Ticks))
	}
	if detail.Ticks[1].QPS != 200 {
		t.Errorf("expected tick 1 QPS 200, got %d", detail.Ticks[1].QPS)
	}

	// Test purge
	purged, err := v.PurgeOldFlightRecordings(1 * time.Hour)
	if err != nil {
		t.Fatalf("PurgeOldFlightRecordings failed: %v", err)
	}
	if purged != 0 {
		t.Errorf("expected 0 purged, got %d", purged)
	}

	// Test delete
	if err := v.DeleteFlightRecordingSession(rec.ID); err != nil {
		t.Fatalf("DeleteFlightRecordingSession failed: %v", err)
	}

	listAfter, err := v.ListFlightRecordingSessions(sessionID)
	if err != nil {
		t.Fatalf("ListFlightRecordingSessions failed: %v", err)
	}
	if len(listAfter) != 0 {
		t.Fatalf("expected 0 sessions after delete, got %d", len(listAfter))
	}
}
