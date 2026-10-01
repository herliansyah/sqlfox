package mysql

import (
	"testing"

	"sqlfox/internal/driver"
)

func TestDiagnoseClusterBottleneck(t *testing.T) {
	s := &MySQLSession{}

	// Case 1: Healthy cluster
	diag, sev := s.diagnoseClusterBottleneck(
		0.0, 0, 0, nil, "node-1",
		0.2, 1.1, 0, 0, 0,
		4, 0, 0, "Primary",
	)
	if sev != "ok" {
		t.Errorf("expected sev ok, got %s (diag: %s)", sev, diag)
	}

	// Case 2: Quorum lost / split-brain
	diag, sev = s.diagnoseClusterBottleneck(
		0.0, 0, 0, nil, "node-1",
		0.2, 1.1, 0, 0, 0,
		4, 0, 0, "Non-Primary",
	)
	if sev != "critical" {
		t.Errorf("expected critical for Non-Primary, got %s", sev)
	}

	// Case 3: Flow Control Active with Culprit Node
	culprit := &driver.ClusterNodeInfo{
		Name:            "node-3",
		Address:         "192.168.1.13:3306",
		ReceiveQueue:    142,
		FlowControlSent: 15,
		IsLocal:         false,
	}
	diag, sev = s.diagnoseClusterBottleneck(
		0.12, 0, 15, culprit, "node-1",
		0.4, 2.0, 0, 0, 5,
		4, 0, 0, "Primary",
	)
	if sev != "critical" {
		t.Errorf("expected critical for Flow Control pause, got %s", sev)
	}
	if diag == "" || culprit.Name == "" {
		t.Errorf("expected diagnosis mentioning culprit node, got %s", diag)
	}

	// Case 4: Storage I/O Stall (InnoDB buffer pool waits)
	diag, sev = s.diagnoseClusterBottleneck(
		0.0, 0, 0, nil, "node-1",
		0.3, 1.5, 45, 12, 0,
		4, 0, 0, "Primary",
	)
	if sev != "warning" {
		t.Errorf("expected warning for disk IO stall, got %s", sev)
	}

	// Case 5: Network latency spike
	diag, sev = s.diagnoseClusterBottleneck(
		0.0, 0, 0, nil, "node-1",
		12.5, 85.0, 0, 0, 0,
		4, 0, 0, "Primary",
	)
	if sev != "warning" {
		t.Errorf("expected warning for network latency, got %s", sev)
	}

	// Case 6: High certification conflicts
	diag, sev = s.diagnoseClusterBottleneck(
		0.0, 0, 0, nil, "node-1",
		0.2, 1.0, 0, 0, 0,
		4, 250, 180, "Primary",
	)
	if sev != "warning" {
		t.Errorf("expected warning for certification conflicts, got %s", sev)
	}
}
