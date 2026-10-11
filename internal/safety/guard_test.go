package safety

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/sqlite"
)

func TestSafetyGuard_Inspect(t *testing.T) {

	tests := []struct {
		name              string
		query             string
		env               string
		expectedRisk      RiskLevel
		expectedChallenge bool
		expectedTable     string
	}{
		{
			name:              "Unqualified DELETE in Dev",
			query:             "DELETE FROM users;",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "users",
		},
		{
			name:              "Qualified DELETE in Dev",
			query:             "DELETE FROM users WHERE id = 10;",
			env:               "Development",
			expectedRisk:      RiskNone,
			expectedChallenge: false,
		},
		{
			name:              "Qualified DELETE in Prod",
			query:             "DELETE FROM users WHERE id = 10;",
			env:               "Production",
			expectedRisk:      RiskWarning,
			expectedChallenge: false,
			expectedTable:     "users",
		},
		{
			name:              "Unqualified UPDATE",
			query:             "UPDATE accounts SET balance = 0;",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "accounts",
		},
		{
			name:              "DROP TABLE",
			query:             "DROP TABLE customers;",
			env:               "Production",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "customers",
		},
		{
			name:              "TRUNCATE TABLE",
			query:             "TRUNCATE TABLE audit_logs;",
			env:               "Staging",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "audit_logs",
		},
		{
			name:              "Safe SELECT",
			query:             "SELECT * FROM users WHERE active = 1;",
			env:               "Production",
			expectedRisk:      RiskNone,
			expectedChallenge: false,
		},
		{
			name:              "PostgreSQL Unqualified DELETE with RETURNING",
			query:             `DELETE FROM "public"."orders" RETURNING *;`,
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "orders",
		},
		{
			name:              "PostgreSQL Qualified DELETE with RETURNING in Dev",
			query:             `DELETE FROM "public"."orders" WHERE id = $1 RETURNING *;`,
			env:               "Development",
			expectedRisk:      RiskNone,
			expectedChallenge: false,
		},
		{
			name:              "PostgreSQL DROP TABLE with quotes",
			query:             `DROP TABLE IF EXISTS "analytics"."page_views";`,
			env:               "Production",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "page_views",
		},
		{
			name:              "PostgreSQL DROP TABLE with leading comments",
			query:             "-- Important migration\n/* block comment */\nDROP TABLE IF EXISTS \"analytics\".\"page_views\";",
			env:               "Production",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "page_views",
		},
		{
			name:              "Unqualified DELETE with leading comments",
			query:             "-- purge data\nDELETE FROM customers;",
			env:               "Development",
			expectedRisk:      RiskCritical,
			expectedChallenge: true,
			expectedTable:     "customers",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Inspect(tc.query, tc.env)
			if res.RiskLevel != tc.expectedRisk {
				t.Fatalf("expected risk %s, got %s (reason: %s)", tc.expectedRisk, res.RiskLevel, res.Reason)
			}
			if res.RequiresChallenge != tc.expectedChallenge {
				t.Fatalf("expected challenge %v, got %v", tc.expectedChallenge, res.RequiresChallenge)
			}
			if tc.expectedTable != "" && res.TargetTable != tc.expectedTable {
				t.Fatalf("expected target table %s, got %s", tc.expectedTable, res.TargetTable)
			}
		})
	}
}

func TestBackupManager_BackupTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-backup-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	ctx := context.Background()

	sess, err := driver.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	// Create table with data
	_, err = sess.Exec(ctx, `
		CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);
		INSERT INTO items (name) VALUES ('Keyboard'), ('Mouse');
	`)
	if err != nil {
		t.Fatal(err)
	}

	backupPath, dataIncluded, err := BackupTable(ctx, sess, filepath.Join(tempDir, "backups"), "items")
	if err != nil {
		t.Fatalf("BackupTable failed: %v", err)
	}

	if !dataIncluded {
		t.Fatal("expected data to be included in small table backup")
	}

	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(content) == 0 {
		t.Fatal("expected non-empty backup content")
	}
}
