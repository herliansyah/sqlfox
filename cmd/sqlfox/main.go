package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"sqlfox/internal/cli"
	"sqlfox/internal/logger"
	"sqlfox/internal/server"
	"sqlfox/internal/vault"
	"sqlfox/web"

	_ "sqlfox/internal/driver/mysql"
	_ "sqlfox/internal/driver/postgres"
	_ "sqlfox/internal/driver/sqlite"
)

// Version is populated at build time via -ldflags "-X main.Version=..."
var Version = "v0.10.5"

func main() {
	loadEnvFiles()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "query", "transfer", "job", "advise", "help":
			// ponytail: Setup default logger from environment before dispatching CLI
			logLvl := os.Getenv("SQLFOX_LOG_LEVEL")
			logFmt := os.Getenv("SQLFOX_LOG_FORMAT")
			logSink := os.Getenv("SQLFOX_LOG_FILE")
			if logLvl != "" || logSink != "" {
				res, err := logger.Setup(logger.Config{
					Level:    logLvl,
					Format:   logFmt,
					FilePath: logSink,
					Silent:   false,
				})
				if err == nil && res.Closer != nil {
					defer res.Closer.Close()
				}
			}
			os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
		}
	}

	portFlag := flag.Int("port", 8080, "Port to listen on (default 8080, with automatic scanning if busy)")
	hostFlag := flag.String("host", "127.0.0.1", "Host interface to bind (default 127.0.0.1; use --lan or 0.0.0.0 for LAN/Tablet access)")
	lanFlag := flag.Bool("lan", false, "Enable LAN and tablet access (binds to 0.0.0.0 and displays Master PIN)")
	dbFlag := flag.String("db", "", "Path to config vault database (default: ~/.sqlfox/config.db)")
	passFlag := flag.String("pass", "", "Master password for vault encryption (leave blank to auto-generate or reuse ~/.sqlfox/master.key)")
	noBrowserFlag := flag.Bool("no-browser", false, "Do not automatically launch web browser")
	strictPortFlag := flag.Bool("strict-port", false, "Fail immediately if specified port is in use")
	multiFlag := flag.Bool("multi", false, "Allow multiple SqlFox instances to run concurrently")
	versionFlag := flag.Bool("version", false, "Print SqlFox version and exit")
	logLevelFlag := flag.String("log-level", "", "Logging verbosity: debug, info, warn, error, silent (or SQLFOX_LOG_LEVEL)")
	verboseFlag := flag.Bool("v", false, "Enable verbose debug logging (shortcut for -log-level=debug)")
	verboseLongFlag := flag.Bool("verbose", false, "Enable verbose debug logging")
	logFormatFlag := flag.String("log-format", "", "Log output format: text, json (default text or SQLFOX_LOG_FORMAT)")
	logFileFlag := flag.String("log-file", "", "Target file for log output sink (or SQLFOX_LOG_FILE)")
	silentFlag := flag.Bool("silent", false, "Suppress terminal console output (shortcut for -q)")
	quietFlag := flag.Bool("q", false, "Suppress terminal console output")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("SqlFox %s\n", Version)
		return
	}

	// Resolve logging configuration
	level := *logLevelFlag
	if level == "" {
		level = os.Getenv("SQLFOX_LOG_LEVEL")
	}
	if *verboseFlag || *verboseLongFlag {
		level = "debug"
	}
	if level == "" {
		level = "info"
	}

	format := *logFormatFlag
	if format == "" {
		format = os.Getenv("SQLFOX_LOG_FORMAT")
	}
	if format == "" {
		format = "text"
	}

	logFile := *logFileFlag
	if logFile == "" {
		logFile = os.Getenv("SQLFOX_LOG_FILE")
	}

	isSilent := *silentFlag || *quietFlag

	// ponytail: Console logging to terminal is opt-in (active when -v, -verbose, -log-level, or SQLFOX_LOG_LEVEL is set).
	// Default interactive desktop server keeps terminal focused on clean banner while broadcaster still logs to Activity Console.
	hasExplicitLogLevel := *logLevelFlag != "" || os.Getenv("SQLFOX_LOG_LEVEL") != "" || *verboseFlag || *verboseLongFlag
	consoleSilent := isSilent || !hasExplicitLogLevel

	logRes, err := logger.Setup(logger.Config{
		Level:    level,
		Format:   format,
		FilePath: logFile,
		Silent:   consoleSilent,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	if logRes.Closer != nil {
		defer logRes.Closer.Close()
	}

	// Resolve config path
	dbPath := *dbFlag
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dbPath = filepath.Join(home, ".sqlfox", "config.db")
	}

	configDir := filepath.Dir(dbPath)
	backupDir := filepath.Join(configDir, "backups")

	// Single-Instance Guard: prevent duplicate instances and unneeded port hunting
	if !*multiFlag && os.Getenv("SQLFOX_MULTI_INSTANCE") != "1" {
		if activeURL, activePID, isRunning := checkExistingInstance(configDir, *portFlag); isRunning {
			if !isSilent {
				if activePID > 0 {
					fmt.Printf("ℹ️  SqlFox is already running at %s (PID %d)\n", activeURL, activePID)
				} else {
					fmt.Printf("ℹ️  SqlFox is already running at %s\n", activeURL)
				}
			}
			if !*noBrowserFlag {
				go openBrowser(activeURL)
				time.Sleep(300 * time.Millisecond)
			}
			return
		}
	}

	// Resolve or auto-generate Master Password / PIN
	masterPassword, isNewPIN, err := resolveMasterPassword(*passFlag, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error resolving master password: %v\n", err)
		os.Exit(1)
	}

	// Open or create encrypted connection vault
	v, err := vault.OpenVault(dbPath, masterPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error initializing vault (%s): %v\n", dbPath, err)
		fmt.Fprintf(os.Stderr, "   Tip: If this is an existing database, run with: ./sqlfox -pass \"your_password\"\n")
		fmt.Fprintf(os.Stderr, "   Or remove %s to start with a fresh vault.\n", dbPath)
		os.Exit(1)
	}
	defer v.Close()

	bindHost := *hostFlag
	isLAN := *lanFlag || bindHost == "0.0.0.0"
	if *lanFlag && bindHost == "127.0.0.1" {
		bindHost = "0.0.0.0"
	}

	// Initialize HTTP server with embedded web assets and master password
	srv := server.NewServer(v, web.Assets(), backupDir, masterPassword)
	srv.SetVersion(Version)
	srv.SetLogger(logRes.Logger)
	srv.SetLANEnabled(isLAN)

	maxAttempts := 50
	if *strictPortFlag {
		maxAttempts = 1
	}

	// Automatic port hunting
	listener, actualPort, err := listenWithPortHunting(bindHost, *portFlag, maxAttempts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to bind listener: %v\n", err)
		os.Exit(1)
	}

	localURL := fmt.Sprintf("http://localhost:%d", actualPort)
	lanIP := getLocalNetworkIP()

	var lockCleanup func()
	if !*multiFlag && os.Getenv("SQLFOX_MULTI_INSTANCE") != "1" {
		cleanup, err := writeInstanceLock(configDir, actualPort, localURL)
		if err == nil {
			lockCleanup = cleanup
			defer lockCleanup()
		}
	}

	httpServer := &http.Server{
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
		}
	}()

	if !isSilent {
		cli.RenderServerBanner(os.Stdout, cli.ServerBannerConfig{
			Version:        Version,
			LocalURL:       localURL,
			IsLAN:          isLAN,
			LANIP:          lanIP,
			Port:           actualPort,
			MasterPassword: masterPassword,
			IsNewPIN:       isNewPIN,
			VaultPath:      dbPath,
			BackupDir:      backupDir,
		})
	}

	logRes.Logger.Info("sqlfox_started",
		"version", Version,
		"port", actualPort,
		"local_url", localURL,
		"vault", dbPath,
	)

	// Auto launch browser to local URL
	if !*noBrowserFlag {
		go openBrowser(localURL)
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down SqlFox gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
	}
	srv.Close()
	if lockCleanup != nil {
		lockCleanup()
	}
	fmt.Println("SqlFox stopped.")
}

func resolveMasterPassword(flagPass string, configDir string) (string, bool, error) {
	// 1. Explicit CLI flag
	if flagPass != "" {
		return flagPass, false, nil
	}

	// 2. Environment variable SQLFOX_PASS
	if envPass := os.Getenv("SQLFOX_PASS"); envPass != "" {
		return envPass, false, nil
	}

	// 3. Saved master.key file
	keyFile := filepath.Join(configDir, "master.key")
	if data, err := os.ReadFile(keyFile); err == nil {
		saved := strings.TrimSpace(string(data))
		if saved != "" {
			return saved, false, nil
		}
	}

	// 4. Generate friendly PIN
	pin, err := generateFriendlyPIN()
	if err != nil {
		return "fox-1234", true, nil
	}

	_ = os.MkdirAll(configDir, 0700)
	_ = os.WriteFile(keyFile, []byte(pin), 0600)
	return pin, true, nil
}

func generateFriendlyPIN() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return "fox-" + string(b), nil
}

// tryListen checks if port is free on loopback interfaces before binding.
// ponytail: on Windows Winsock and dual-stack setups, net.Listen("tcp", "127.0.0.1:p") or wildcard
// can bind without error even when another service (e.g. Pantau) already listens on [::1]:p or :p.
// Probing via DialTimeout and verifying IPv4 binding ensures busy ports trigger auto-scanning.
func tryListen(host string, port int) net.Listener {
	for _, probeHost := range []string{"127.0.0.1", "[::1]"} {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", probeHost, port), 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
	}

	l4, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil
	}
	_ = l4.Close()

	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil
	}
	return l
}

func listenWithPortHunting(host string, startPort int, maxAttempts int) (net.Listener, int, error) {
	for i := 0; i < maxAttempts; i++ {
		port := startPort + i
		l := tryListen(host, port)
		if l != nil {
			if i > 0 {
				fmt.Printf("⚠️  Port %d was busy. Automatically switched to free port %d.\n", startPort, port)
			}
			return l, port, nil
		}
	}
	if maxAttempts == 1 {
		return nil, startPort, fmt.Errorf("port %d is already in use", startPort)
	}
	return nil, startPort, fmt.Errorf("could not find an available port after %d attempts starting at %d", maxAttempts, startPort)
}

func getLocalNetworkIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

func openBrowser(url string) {
	time.Sleep(200 * time.Millisecond)
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	_ = exec.Command(cmd, args...).Start()
}

// loadEnvFiles loads environment variables from .env or sqlfox.env if present.
// ponytail: Lightweight zero-dependency env loader allows Windows double-click configuration.
func loadEnvFiles() {
	var candidates []string

	// Next to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "sqlfox.env"), filepath.Join(exeDir, ".env"))
	}

	// In current working directory
	candidates = append(candidates, "sqlfox.env", ".env")

	// In ~/.sqlfox/
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".sqlfox", "sqlfox.env"), filepath.Join(home, ".sqlfox", ".env"))
	}

	loadEnvFilesFromPaths(candidates...)
}

func loadEnvFilesFromPaths(paths ...string) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				k = strings.TrimSpace(k)
				v = strings.Trim(strings.TrimSpace(v), `"'`)
				if os.Getenv(k) == "" {
					_ = os.Setenv(k, v)
				}
			}
		}
	}
}

