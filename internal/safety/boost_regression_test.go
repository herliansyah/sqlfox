package safety

import (
	"testing"

	"github.com/herliansyah/sqlfox/internal/driver"
)

func TestBoostP07_SafetyGuardBypasses(t *testing.T) {
	t.Run("Writable CTE named with SQL keyword INSERT", func(t *testing.T) {
		q := "WITH insert AS (DELETE FROM users) SELECT * FROM insert"
		res := Inspect(q, "Production")
		if res.RiskLevel != RiskCritical || !res.IsDestructive || !res.RequiresChallenge {
			t.Fatalf("expected CRITICAL/destructive/challenge for writable CTE %q, got %+v", q, res)
		}
	})

	t.Run("Writable CTE with RECURSIVE and column list", func(t *testing.T) {
		q := "WITH RECURSIVE delete(id) AS MATERIALIZED (DELETE FROM orders RETURNING id) SELECT * FROM delete"
		res := Inspect(q, "Production")
		if res.RiskLevel != RiskCritical || !res.IsDestructive || !res.RequiresChallenge {
			t.Fatalf("expected CRITICAL/destructive/challenge for writable CTE %q, got %+v", q, res)
		}
	})

	t.Run("MariaDB executable comment DROP TABLE", func(t *testing.T) {
		cases := []string{
			"/*M! DROP TABLE users */",
			"/*M!100500 DROP TABLE users */",
			"SELECT 1 /*M! ; DROP TABLE users */",
		}
		for _, q := range cases {
			res := Inspect(q, "Production")
			if res.RiskLevel != RiskCritical || !res.IsDestructive || !res.RequiresChallenge {
				t.Fatalf("expected CRITICAL/destructive/challenge for MariaDB exec comment %q, got %+v", q, res)
			}
			if driver.IsQueryStatement(q) {
				t.Fatalf("driver.IsQueryStatement(%q) = true; want false", q)
			}
		}
	})

	t.Run("PostgreSQL E-string and JSONB #>> operator desync", func(t *testing.T) {
		q := `SELECT '{}'::jsonb #>> '{a}', E'x\' '; DROP TABLE users; SELECT '1'`
		res := Inspect(q, "Production")
		if res.RiskLevel != RiskCritical || !res.IsDestructive || !res.RequiresChallenge {
			t.Fatalf("expected CRITICAL/destructive/challenge for PG E-string + #>> %q, got %+v", q, res)
		}
	})
}
