//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
)

func TestPostgresIntegrationLifecycle(t *testing.T) {
	if os.Getenv("SQLFOX_TEST_PG_ENABLE") == "" {
		t.Skip("SQLFOX_TEST_PG_ENABLE not set; skipping real PostgreSQL integration test")
	}

	drv := &PostgresDriver{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sess, err := drv.Connect(ctx, driver.ConnectionConfig{
		Driver:       "postgres",
		Host:         envOr("SQLFOX_TEST_PG_HOST", "127.0.0.1"),
		Port:         5432,
		User:         envOr("SQLFOX_TEST_PG_USER", "postgres"),
		Password:     envOr("SQLFOX_TEST_PG_PASSWORD", "postgres"),
		DatabaseName: envOr("SQLFOX_TEST_PG_DB", "sqlfox_test"),
	})
	if err != nil {
		t.Fatalf("Postgres Connect failed: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Ping(ctx); err != nil {
		t.Fatalf("Postgres Ping failed: %v", err)
	}

	if _, err := sess.Exec(ctx, "CREATE TABLE IF NOT EXISTS integration_items (id SERIAL PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	defer func() { _, _ = sess.Exec(context.Background(), "DROP TABLE IF EXISTS integration_items") }()

	if _, err := sess.Exec(ctx, "INSERT INTO integration_items (name) VALUES ($1)", "alpha"); err != nil {
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
