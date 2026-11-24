//go:build integration

package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
)

func TestMySQLIntegrationLifecycle(t *testing.T) {
	dsn := os.Getenv("SQLFOX_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SQLFOX_TEST_MYSQL_DSN not set; skipping real MySQL integration test")
	}

	drv := &MySQLDriver{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Expect standard DSN or host/port/user/pass via Connect; parse minimal env for CI.
	sess, err := drv.Connect(ctx, driver.Config{
		Driver:       "mysql",
		Host:         envOr("SQLFOX_TEST_MYSQL_HOST", "127.0.0.1"),
		Port:         3306,
		User:         envOr("SQLFOX_TEST_MYSQL_USER", "root"),
		Password:     os.Getenv("SQLFOX_TEST_MYSQL_PASSWORD"),
		DatabaseName: envOr("SQLFOX_TEST_MYSQL_DB", "sqlfox_test"),
	})
	if err != nil {
		t.Fatalf("MySQL Connect failed: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Ping(ctx); err != nil {
		t.Fatalf("MySQL Ping failed: %v", err)
	}

	if _, err := sess.Exec(ctx, "CREATE TABLE IF NOT EXISTS integration_items (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL)"); err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	defer func() { _, _ = sess.Exec(context.Background(), "DROP TABLE IF EXISTS integration_items") }()

	if _, err := sess.Exec(ctx, "INSERT INTO integration_items (name) VALUES (?)", "alpha"); err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	schema, err := sess.InspectSchema(ctx, "")
	if err != nil || len(schema.Tables) == 0 {
		t.Fatalf("InspectSchema failed: err=%v tables=%d", err, len(schema.Tables))
	}

	stream, err := sess.QueryStream(ctx, "SELECT id, name FROM integration_items", 10)
	if err != nil {
		t.Fatalf("QueryStream failed: %v", err)
	}
	defer stream.Close()
	rows, err := stream.NextChunk(10)
	if err != nil || len(rows) == 0 {
		t.Fatalf("NextChunk failed: err=%v rows=%d", err, len(rows))
	}
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
