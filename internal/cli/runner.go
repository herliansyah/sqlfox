package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sqlfox/internal/driver"
	"sqlfox/internal/vault"

	gomysql "github.com/go-sql-driver/mysql"
	_ "sqlfox/internal/driver/mysql"
	_ "sqlfox/internal/driver/postgres"
	_ "sqlfox/internal/driver/sqlite"
)

// PrintHelp writes the CLI command help to the provided writer.
func PrintHelp(w io.Writer) {
	help := `SqlFox - Modern Database Management & Automation Suite

Usage:
  sqlfox [command] [flags]
  sqlfox [server-flags] (launches desktop web UI)

Commands:
  query     Execute SQL query or script with streaming output formats
  transfer  Perform direct cross-connection data transfer between databases
  help      Show help for commands

Server Flags (Default Mode):
  -port int       Port to listen on (default 8080)
  -host string    Host interface to bind (default "0.0.0.0")
  -db string      Path to config vault database (default: ~/.sqlfox/config.db)
  -pass string    Master password for vault encryption
  -no-browser     Do not automatically launch web browser

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

	switch cmd {
	case "help", "-h", "--help":
		PrintHelp(stdout)
		return 0
	case "query":
		return RunQuery(subArgs, stdout, stderr, stdin)
	case "transfer":
		return RunTransfer(subArgs, stdout, stderr, stdin)
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
		// Attempt to read from ~/.sqlfox/master.key
		keyPath := filepath.Join(configDir, "master.key")
		if keyBytes, err := os.ReadFile(keyPath); err == nil {
			masterPass = strings.TrimSpace(string(keyBytes))
		}
	}

	v, err := vault.OpenVault(vaultPath, masterPass)
	if err != nil {
		return nil, fmt.Errorf("failed to open vault (%s): %w", vaultPath, err)
	}

	connRec, err := v.GetConnectionByName(target)
	if err != nil {
		_ = v.Close()
		return nil, fmt.Errorf("connection '%s' not found in vault: %w", target, err)
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
		return nil, fmt.Errorf("failed to connect to vault connection '%s': %w", target, err)
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
			sslMode := u.Query().Get("sslmode")
			useSSL := sslMode != "" && sslMode != "disable"

			return "postgres", driver.Config{
				Driver:       "postgres",
				Host:         host,
				Port:         port,
				DatabaseName: dbName,
				User:         user,
				Password:     pass,
				UseSSL:       useSSL,
			}, true
		}
	}

	// Case F: Explicit driver override provided
	if driverOverride == "sqlite" {
		return "sqlite", driver.Config{Driver: "sqlite", DatabaseName: trimmed}, true
	}
	if driverOverride == "postgres" || driverOverride == "postgresql" || driverOverride == "pg" {
		return "postgres", driver.Config{Driver: "postgres", DatabaseName: trimmed}, true
	}

	return "", driver.Config{}, false
}

// SplitStatements breaks SQL text into executable individual statements.
// ponytail: Token-aware splitter handling quotes, backticks, and comments without third-party lexers.
func SplitStatements(reader io.Reader) []string {
	var statements []string
	content, err := io.ReadAll(reader)
	if err != nil || len(content) == 0 {
		return nil
	}

	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false

	runes := []rune(string(content))
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]

		if inLineComment {
			if r == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inLineComment = true
				i++
				continue
			}
			if r == '#' {
				inLineComment = true
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if r == '\'' && !inDoubleQuote && !inBacktick {
			if i > 0 && runes[i-1] == '\\' {
				// escaped
			} else {
				inSingleQuote = !inSingleQuote
			}
		} else if r == '"' && !inSingleQuote && !inBacktick {
			if i > 0 && runes[i-1] == '\\' {
				// escaped
			} else {
				inDoubleQuote = !inDoubleQuote
			}
		} else if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
		}

		if r == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(r)
	}

	last := strings.TrimSpace(current.String())
	if last != "" {
		statements = append(statements, last)
	}

	return statements
}
