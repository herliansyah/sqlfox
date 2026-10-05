package advisor

import (
	"testing"
)

func TestEvaluateMySQLVariable_Curated(t *testing.T) {
	allVars := map[string]string{
		"innodb_buffer_pool_size": "8589934592", // 8GB
		"max_heap_table_size":     "67108864",
	}

	// 1. Critical lock mode
	v1 := EvaluateMySQLVariable("innodb_autoinc_lock_mode", "1", allVars)
	if !v1.IsCurated {
		t.Fatalf("expected innodb_autoinc_lock_mode to be curated")
	}
	if v1.Severity != "critical" {
		t.Fatalf("expected severity critical for autoinc lock mode 1, got %s", v1.Severity)
	}
	if v1.Category != "Cluster & WSREP" {
		t.Fatalf("expected category 'Cluster & WSREP', got %s", v1.Category)
	}

	// 2. Optimal autoinc mode
	v2 := EvaluateMySQLVariable("innodb_autoinc_lock_mode", "2", allVars)
	if v2.Severity != "optimal" {
		t.Fatalf("expected severity optimal for autoinc lock mode 2, got %s", v2.Severity)
	}

	// 3. Buffer pool instances check
	v3 := EvaluateMySQLVariable("innodb_buffer_pool_instances", "1", allVars)
	if v3.Severity != "warning" {
		t.Fatalf("expected warning for 1 instance on 8GB pool, got %s", v3.Severity)
	}

	// 4. Slow query log OFF
	v4 := EvaluateMySQLVariable("slow_query_log", "OFF", allVars)
	if v4.Severity != "warning" {
		t.Fatalf("expected warning for slow_query_log OFF, got %s", v4.Severity)
	}
}

func TestEvaluateMySQLVariable_Uncurated(t *testing.T) {
	v := EvaluateMySQLVariable("custom_unknown_variable", "42", nil)
	if v.IsCurated {
		t.Fatalf("expected unknown variable to be uncurated")
	}
	if v.Severity != "info" {
		t.Fatalf("expected severity info, got %s", v.Severity)
	}
	if v.Category != "General" {
		t.Fatalf("expected category General, got %s", v.Category)
	}
}

func TestEvaluateClusterTuning(t *testing.T) {
	sysVars := map[string]string{
		"innodb_autoinc_lock_mode": "1",
		"binlog_format":            "STATEMENT",
		"wsrep_slave_threads":      "1",
		"wsrep_sync_wait":          "0",
		"wsrep_retry_autocommit":   "0",
	}

	checks := EvaluateClusterTuning(sysVars, nil)
	if len(checks) < 4 {
		t.Fatalf("expected at least 4 cluster checks, got %d", len(checks))
	}

	hasCritical := false
	for _, c := range checks {
		if c.Status == "critical" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Fatalf("expected critical check for lock mode 1 and statement binlog")
	}
}
