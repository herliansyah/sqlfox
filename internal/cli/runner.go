package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/vault"

	gomysql "github.com/go-sql-driver/mysql"
	_ "github.com/herliansyah/sqlfox/internal/driver/mysql"
	_ "github.com/herliansyah/sqlfox/internal/driver/postgres"
	_ "github.com/herliansyah/sqlfox/internal/driver/sqlite"
)

// Version is overridden by cmd/sqlfox from main.Version so CLI help always matches the binary version.
var Version = "v0.19.8"

// PrintHelp writes the CLI command help to the provided writer.
func PrintHelp(w io.Writer) {
	RenderHelpHeader(w, Version)
	help := `
Usage:
  sqlfox [command] [flags]
  sqlfox [server-flags] (launches desktop web UI)

Commands:
  query     Execute SQL query or script with streaming output formats
  transfer  Perform direct cross-connection data transfer between databases
  job       Execute declarative database automation jobs or dry-run checks
  advise    Analyze SQL query and generate heuristic index recommendations and bloat warnings
  stop      Stop a running background SqlFox instance
  desktop   Manage desktop application launcher (install, uninstall, status)
  help      Show help for commands

Server Flags (Default Mode):
  -port int       Port to listen on (default 8080)
  -host string    Host interface to bind (default "127.0.0.1")
  --lan           Enable LAN and tablet access (binds to 0.0.0.0)
  -db string      Path to config vault database (default: ~/.sqlfox/config.db)
  -pass string    Master password for vault encryption
  -app            Launch in standalone native window mode using Chrome/Edge/Chromium (default true)
  -browser        Force launch in standard web browser tab instead of native standalone app window
  -no-browser     Do not automatically launch web browser or app window

Run 'sqlfox <command> --help' for details on a specific command.
`
	fmt.Fprint(w, help)
}

// Run dispatches subcommands based on the supplied arguments.
func Run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(args) == 0 {
		PrintHelp(stderr)
		return 0
	}

	cmd := args[0]
	subArgs := args[1:]

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "help", "-h", "--help":
		PrintHelp(stdout)
		return 0
	case "query":
		return RunQueryContext(ctx, subArgs, stdout, stderr, stdin)
	case "transfer":
		return RunTransferContext(ctx, subArgs, stdout, stderr, stdin)
	case "job":
		return RunJobContext(ctx, subArgs, stdout, stderr, stdin)
	case "advise":
		return RunAdviseContext(ctx, subArgs, stdout, stderr, stdin)
	case "stop":
		fmt.Fprintln(stdout, "To stop a running background SqlFox instance, run 'sqlfox stop'.")
		return 0
	case "desktop":
		fmt.Fprintln(stdout, "To manage the desktop application launcher, run 'sqlfox desktop [install|uninstall|status]'.")
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown command '%s'. Run 'sqlfox help' for usage.\n", cmd)
		return 1
	}
}

// SessionTarget holds connection metadata resolved from a DSN or Vault profile.
type SessionTarget struct {
	Session     driver.Session
	Driver      string
	Environment string
	DBName      string
	Cleanup     func()
}

// ResolveVaultTarget always resolves a named connection profile from the local Vault,
// ensuring `--conn <profile> --driver postgres|sqlite` never misinterprets the profile name as a DSN/file path.
func ResolveVaultTarget(ctx context.Context, target string, driverOverride string, vaultPath string, pass string) (*SessionTarget, error) {
	if target == "" {
		return nil, fmt.Errorf("connection target cannot be empty (specify --dsn or --conn)")
	}
	return resolveFromVault(ctx, target, driverOverride, vaultPath, pass)
}

// ResolveTarget parses a direct DSN or resolves a named profile from the Vault.
func ResolveTarget(ctx context.Context, target string, driverOverride string, vaultPath string, pass string) (*SessionTarget, error) {
	if target == "" {
		return nil, fmt.Errorf("connection target cannot be empty (specify --dsn or --conn)")
	}

	// 1. Direct DSN checks
	drv, cfg, ok := parseDirectDSN(target, driverOverride)
	if ok {
		sess, err := driver.Connect(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect via DSN: %w", err)
		}
		return &SessionTarget{
			Session:     sess,
			Driver:      drv,
			Environment: "Development",
			DBName:      cfg.DatabaseName,
			Cleanup: func() {
				_ = sess.Close()
			},
		}, nil
	}

	// 2. Resolve via local Vault
	return resolveFromVault(ctx, target, driverOverride, vaultPath, pass)
}

func resolveFromVault(ctx context.Context, target string, driverOverride string, vaultPath string, pass string) (*SessionTarget, error) {
	if vaultPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		vaultPath = filepath.Join(home, ".sqlfox", "config.db")
	}

	configDir := filepath.Dir(vaultPath)
	masterPass := pass
	if masterPass == "" {
		if envPass := os.Getenv("SQLFOX_PASS"); envPass != "" {
			masterPass = envPass
		} else if envPass := os.Getenv("SQLFOX_MASTER_KEY"); envPass != "" {
			masterPass = envPass
		}
	}
	if masterPass == "" {
		// Attempt to read from ~/.sqlfox/master.key
		keyPath := filepath.Join(configDir, "master.key")
		if saved, err := vault.ReadMasterKeyFile(keyPath); err == nil {
			masterPass = saved
		}
	}

	v, err := vault.OpenVault(vaultPath, masterPass)
	if err != nil {
		return nil, fmt.Errorf("failed to open vault (%s): %w", vaultPath, err)
	}

	connRec, err := v.GetConnectionByName(target)
	if err != nil {
		_ = v.Close()
		return nil, fmt.Errorf("connection '%s' not found in vault: %w", RedactDSN(target), err)
	}

	drvName := connRec.Driver
	if driverOverride != "" {
		drvName = driverOverride
	}

	drvCfg := driver.Config{
		Driver:       drvName,
		Host:         connRec.Host,
		Port:         connRec.Port,
		DatabaseName: connRec.DatabaseName,
		User:         connRec.User,
		Password:     connRec.Password,
		UseSSL:       connRec.UseSSL,
		SSLCA:        connRec.SSLCA,
		SSLCert:      connRec.SSLCert,
		SSLKey:       connRec.SSLKey,
	}

	sess, err := driver.Connect(ctx, drvCfg)
	if err != nil {
		_ = v.Close()
		return nil, fmt.Errorf("failed to connect to vault connection '%s': %w", RedactDSN(target), err)
	}

	env := connRec.Environment
	if env == "" {
		env = "Development"
	}

	return &SessionTarget{
		Session:     sess,
		Driver:      drvName,
		Environment: env,
		DBName:      connRec.DatabaseName,
		Cleanup: func() {
			_ = sess.Close()
			_ = v.Close()
		},
	}, nil
}

func parseDirectDSN(raw string, driverOverride string) (string, driver.Config, bool) {
	trimmed := strings.TrimSpace(raw)

	// Case A: SQLite URI prefix (sqlite://... or sqlite3://...)
	if strings.HasPrefix(trimmed, "sqlite://") || strings.HasPrefix(trimmed, "sqlite3://") {
		path := strings.TrimPrefix(strings.TrimPrefix(trimmed, "sqlite3://"), "sqlite://")
		return "sqlite", driver.Config{Driver: "sqlite", DatabaseName: path}, true
	}

	// Case B: SQLite file path (.db, .sqlite, .sqlite3)
	if strings.HasSuffix(trimmed, ".db") || strings.HasSuffix(trimmed, ".sqlite") || strings.HasSuffix(trimmed, ".sqlite3") {
		return "sqlite", driver.Config{Driver: "sqlite", DatabaseName: trimmed}, true
	}

	// Case C: MySQL URI (mysql://user:pass@host:port/dbname)
	if strings.HasPrefix(trimmed, "mysql://") {
		u, err := url.Parse(trimmed)
		if err == nil {
			var user, pass string
			if u.User != nil {
				user = u.User.Username()
				pass, _ = u.User.Password()
			}
			host := u.Hostname()
			port := 3306
			if pStr := u.Port(); pStr != "" {
				if p, err := strconv.Atoi(pStr); err == nil {
					port = p
				}
			}
			dbName := strings.TrimPrefix(u.Path, "/")
			useSSL := u.Query().Get("ssl") == "true" || u.Query().Get("tls") == "true"

			return "mysql", driver.Config{
				Driver:       "mysql",
				Host:         host,
				Port:         port,
				DatabaseName: dbName,
				User:         user,
				Password:     pass,
				UseSSL:       useSSL,
			}, true
		}
	}

	// Case D: Native Go MySQL DSN (user:pass@tcp(host:port)/dbname)
	if strings.Contains(trimmed, "@tcp(") || strings.Contains(trimmed, "@unix(") {
		cfg, err := gomysql.ParseDSN(trimmed)
		if err == nil {
			host := "127.0.0.1"
			port := 3306
			if cfg.Addr != "" {
				if h, pStr, err := net.SplitHostPort(cfg.Addr); err == nil {
					host = h
					if p, err := strconv.Atoi(pStr); err == nil {
						port = p
					}
				} else {
					host = cfg.Addr
				}
			}
			return "mysql", driver.Config{
				Driver:       "mysql",
				Host:         host,
				Port:         port,
				DatabaseName: cfg.DBName,
				User:         cfg.User,
				Password:     cfg.Passwd,
				UseSSL:       cfg.TLSConfig != "" && cfg.TLSConfig != "false",
			}, true
		}
	}

	// Case E: PostgreSQL URI (postgres://user:pass@host:port/dbname or postgresql://...)
	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		u, err := url.Parse(trimmed)
		if err == nil {
			var user, pass string
			if u.User != nil {
				user = u.User.Username()
				pass, _ = u.User.Password()
			}
			host := u.Hostname()
			port := 5432
			if pStr := u.Port(); pStr != "" {
				if p, err := strconv.Atoi(pStr); err == nil {
					port = p
				}
			}
			dbName := strings.TrimPrefix(u.Path, "/")
			if dbName == "" {
				dbName = "postgres"
			}
			q := u.Query()
			sslMode := q.Get("sslmode")
			useSSL := sslMode != "" && sslMode != "disable"

			return "postgres", driver.Config{
				Driver:       "postgres",
				Host:         host,
				Port:         port,
				DatabaseName: dbName,
				User:         user,
				Password:     pass,
				UseSSL:       useSSL,
				SSLMode:      sslMode,
				SSLCA:        q.Get("sslrootcert"),
				SSLCert:      q.Get("sslcert"),
				SSLKey:       q.Get("sslkey"),
			}, true
		}
	}

	// Case F: Explicit driver override provided (used for direct DSN targets; --conn uses ResolveVaultTarget)
	if driverOverride == "sqlite" {
		return "sqlite", driver.Config{Driver: "sqlite", DatabaseName: trimmed}, true
	}
	if driverOverride == "postgres" || driverOverride == "postgresql" || driverOverride == "pg" {
		return "postgres", driver.Config{Driver: "postgres", DatabaseName: trimmed}, true
	}

	return "", driver.Config{}, false
}

// SplitStatements breaks SQL text into executable individual statements (see driver.SplitSQLStatements).

// RedactDSN masks embedded passwords in URI or Go MySQL DSNs before printing to logs or errors.
func RedactDSN(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		if u, err := url.Parse(trimmed); err == nil && u.User != nil {
			if _, hasPass := u.User.Password(); hasPass {
				return u.Redacted()
			}
			return trimmed
		}
		// Malformed URI with credentials (e.g. unescaped special char in password): strip userinfo before '@'.
		if schemeIdx := strings.Index(trimmed, "://"); schemeIdx >= 0 {
			rest := trimmed[schemeIdx+3:]
			if atIdx := strings.LastIndex(rest, "@"); atIdx >= 0 {
				userPart := rest[:atIdx]
				if colonIdx := strings.IndexByte(userPart, ':'); colonIdx >= 0 {
					return trimmed[:schemeIdx+3] + userPart[:colonIdx] + ":xxxxx@" + rest[atIdx+1:]
				}
			}
		}
	}
	if strings.Contains(trimmed, "@tcp(") || strings.Contains(trimmed, "@unix(") {
		atIdx := strings.Index(trimmed, "@")
		if atIdx > 0 {
			userPart := trimmed[:atIdx]
			if colonIdx := strings.IndexByte(userPart, ':'); colonIdx >= 0 {
				return userPart[:colonIdx] + ":xxxxx" + trimmed[atIdx:]
			}
		}
	}
	return trimmed
}
func SplitStatements(reader io.Reader, driverName ...string) []string {
	const maxCLIInputBytes = 100 << 20
	content, err := io.ReadAll(io.LimitReader(reader, maxCLIInputBytes))
	if err != nil {
		return nil
	}
	drv := ""
	if len(driverName) > 0 {
		drv = driverName[0]
	}
	return driver.SplitSQLStatements(string(content), drv)
}
