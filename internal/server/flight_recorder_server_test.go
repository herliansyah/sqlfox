package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sqlfox/internal/vault"
)

func TestFlightRecorderAPIE2E(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-flight-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "vault.db")
	v, err := vault.OpenVault(dbPath, "test-pw")
	if err != nil {
		t.Fatalf("failed to open vault: %v", err)
	}
	defer v.Close()

	sqliteTarget := filepath.Join(tmpDir, "target.db")
	connRec := &vault.ConnectionRecord{
		Name:         "Target DB",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: sqliteTarget,
	}
	if err := v.SaveConnection(connRec); err != nil {
		t.Fatalf("failed to save connection: %v", err)
	}

	backupDir := filepath.Join(tmpDir, "backups")
	srv := NewServer(v, nil, backupDir, "test-pw")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// Connect session
	connectResp, err := client.Post(ts.URL+"/api/session/connect/"+connRec.ID, "application/json", nil)
	if err != nil || connectResp.StatusCode != http.StatusOK {
		t.Fatalf("connect failed: %v", err)
	}
	connectResp.Body.Close()

	// 1. Check initial status (should be idle)
	statusResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/flight-recorder/status")
	if err != nil || statusResp.StatusCode != http.StatusOK {
		t.Fatalf("get status failed: %v", err)
	}
	var statusMap map[string]any
	_ = json.NewDecoder(statusResp.Body).Decode(&statusMap)
	statusResp.Body.Close()
	if statusMap["is_recording"] != false {
		t.Fatalf("expected is_recording false, got %v", statusMap["is_recording"])
	}

	// 2. Start recording
	startPayload := []byte(`{"name": "E2E Test Run", "interval_ms": 1000}`)
	startResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/flight-recorder/start", "application/json", bytes.NewReader(startPayload))
	if err != nil || startResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(startResp.Body)
		t.Fatalf("start recording failed (%d): %s", startResp.StatusCode, string(body))
	}
	var startMap map[string]any
	_ = json.NewDecoder(startResp.Body).Decode(&startMap)
	startResp.Body.Close()
	recID, ok := startMap["recording_id"].(string)
	if !ok || recID == "" {
		t.Fatalf("missing recording_id in response: %+v", startMap)
	}

	// Wait 1.5 seconds for at least one tick to fire
	time.Sleep(1500 * time.Millisecond)

	// 3. Check status during recording
	statusResp2, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/flight-recorder/status")
	if err != nil || statusResp2.StatusCode != http.StatusOK {
		t.Fatalf("get status 2 failed: %v", err)
	}
	var statusMap2 map[string]any
	_ = json.NewDecoder(statusResp2.Body).Decode(&statusMap2)
	statusResp2.Body.Close()
	if statusMap2["is_recording"] != true {
		t.Fatalf("expected is_recording true, got %v", statusMap2["is_recording"])
	}

	// 4. Stop recording
	stopResp, err := client.Post(ts.URL+"/api/session/"+connRec.ID+"/flight-recorder/stop", "application/json", nil)
	if err != nil || stopResp.StatusCode != http.StatusOK {
		t.Fatalf("stop recording failed: %v", err)
	}
	stopResp.Body.Close()

	// 5. List sessions
	listResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/flight-recorder/sessions")
	if err != nil || listResp.StatusCode != http.StatusOK {
		t.Fatalf("list sessions failed: %v", err)
	}
	var listSessions []*vault.FlightRecordingSession
	_ = json.NewDecoder(listResp.Body).Decode(&listSessions)
	listResp.Body.Close()
	if len(listSessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(listSessions))
	}
	if listSessions[0].ID != recID {
		t.Errorf("expected session ID %s, got %s", recID, listSessions[0].ID)
	}

	// 6. Get session detail with ticks
	detailResp, err := client.Get(ts.URL + "/api/session/" + connRec.ID + "/flight-recorder/sessions/" + recID)
	if err != nil || detailResp.StatusCode != http.StatusOK {
		t.Fatalf("get session detail failed: %v", err)
	}
	var detail vault.FlightRecordingDetail
	_ = json.NewDecoder(detailResp.Body).Decode(&detail)
	detailResp.Body.Close()
	if detail.Session.ID != recID {
		t.Errorf("expected detail session ID %s, got %s", recID, detail.Session.ID)
	}

	// 7. Delete session
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/session/"+connRec.ID+"/flight-recorder/sessions/"+recID, nil)
	delResp, err := client.Do(delReq)
	if err != nil || delResp.StatusCode != http.StatusOK {
		t.Fatalf("delete session failed: %v", err)
	}
	delResp.Body.Close()

	// Verify deleted
	listRespAfter, _ := client.Get(ts.URL + "/api/session/" + connRec.ID + "/flight-recorder/sessions")
	var listSessionsAfter []*vault.FlightRecordingSession
	_ = json.NewDecoder(listRespAfter.Body).Decode(&listSessionsAfter)
	listRespAfter.Body.Close()
	if len(listSessionsAfter) != 0 {
		t.Fatalf("expected 0 sessions after deletion, got %d", len(listSessionsAfter))
	}
}
