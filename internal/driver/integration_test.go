//go:build integration

package driver_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
	_ "github.com/herliansyah/sqlfox/internal/driver/mysql"
	_ "github.com/herliansyah/sqlfox/internal/driver/postgres"
)

func TestMySQLIntegration(t *testing.T) {
	host := os.Getenv("SQLFOX_TEST_MYSQL_HOST")
	if host == "" {
		t.Skip("SQLFOX_TEST_MYSQL_HOST not set; skipping live MySQL integration test")
	}
	port, _ := strconv.Atoi(os.Getenv("SQLFOX_TEST_MYSQL_PORT"))
	if port == 0 {
		port = 3306
	}
	cfg := driver.Config{
		Driver:       "mysql",
		Host:         host,
		Port:         port,
		User:         envOr("SQLFOX_TEST_MYSQL_USER", "root"),
		Password:     os.Getenv("SQLFOX_TEST_MYSQL_PASS"),
		DatabaseName: envOr("SQLFOX_TEST_MYSQL_DB", "mysql"),
	}
	runLiveDriverSmoke(t, cfg, "SELECT 1 AS ok")
}

func TestPostgresIntegration(t *testing.T) {
	host := os.Getenv("SQLFOX_TEST_POSTGRES_HOST")
	if host == "" {
		t.Skip("SQLFOX_TEST_POSTGRES_HOST not set; skipping live PostgreSQL integration test")
	}
	port, _ := strconv.Atoi(os.Getenv("SQLFOX_TEST_POSTGRES_PORT"))
	if port == 0 {
		port = 5432
	}
	cfg := driver.Config{
		Driver:       "postgres",
		Host:         host,
		Port:         port,
		User:         envOr("SQLFOX_TEST_POSTGRES_USER", "postgres"),
		Password:     os.Getenv("SQLFOX_TEST_POSTGRES_PASS"),
		DatabaseName: envOr("SQLFOX_TEST_POSTGRES_DB", "postgres"),
	}
	runLiveDriverSmoke(t, cfg, "SELECT 1 AS ok")
}

func runLiveDriverSmoke(t *testing.T, cfg driver.Config, pingSQL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := driver.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("Connect(%s) failed: %v", cfg.Driver, err)
	}
	defer sess.Close()

	if _, err := sess.Ping(ctx); err != nil {
		t.Fatalf("Ping(%s) failed: %v", cfg.Driver, err)
	}
	info, err := sess.ServerInfo(ctx)
	if err != nil || strings.TrimSpace(info.Version) == "" {
		t.Fatalf("ServerInfo(%s) failed: info=%+v err=%v", cfg.Driver, info, err)
	}
	stream, err := sess.QueryStream(ctx, pingSQL, 10)
	if err != nil {
		t.Fatalf("QueryStream(%s) failed: %v", cfg.Driver, err)
	}
	_ = stream.Close()

	tx, err := sess.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx(%s) failed: %v", cfg.Driver, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback(%s) failed: %v", cfg.Driver, err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
