package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/herliansyah/sqlfox/internal/cli"
	"github.com/herliansyah/sqlfox/internal/logger"
	"github.com/herliansyah/sqlfox/internal/server"
	"github.com/herliansyah/sqlfox/internal/vault"
	"github.com/herliansyah/sqlfox/web"

	_ "github.com/herliansyah/sqlfox/internal/driver/mysql"
	_ "github.com/herliansyah/sqlfox/internal/driver/postgres"
	_ "github.com/herliansyah/sqlfox/internal/driver/sqlite"
)

// Version is populated at build time via -ldflags "-X main.Version=..."
var Version = "v0.19.8"

func main() {
	cli.Version = Version
	initConsole()
	loadEnvFiles()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "query", "transfer", "job", "advise", "help":
			// ponytail: Setup default logger from environment before dispatching CLI
			logLvl := os.Getenv("SQLFOX_LOG_LEVEL")
			logFmt := os.Getenv("SQLFOX_LOG_FORMAT")
			logSink := os.Getenv("SQLFOX_LOG_FILE")
			var logCloser io.Closer
			if logLvl != "" || logSink != "" {
				res, err := logger.Setup(logger.Config{
					Level:    logLvl,
					Format:   logFmt,
					FilePath: logSink,
					Silent:   false,
				})
				if err == nil && res.Closer != nil {
					logCloser = res.Closer
				}
			}
			code := cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin)
			if logCloser != nil {
				_ = logCloser.Close()
			}
			os.Exit(code)

		case "stop":
			configDir, stopPort := parseStopArgs(os.Args[2:])
			lockPath := filepath.Join(configDir, "sqlfox.lock")
			var pid int
			if data, err := os.ReadFile(lockPath); err == nil {
				var info instanceInfo
				if json.Unmarshal(data, &info) == nil {
					pid = info.PID
				}
			}
			if err := stopExistingInstance(configDir, stopPort); err != nil {
				fmt.Printf("ℹ️  %v\n", err)
				os.Exit(0)
			}
			if pid > 0 {
				fmt.Printf("✅ SqlFox (PID %d) stopped successfully.\n", pid)
			} else {
				fmt.Println("✅ SqlFox stopped successfully.")
			}
			os.Exit(0)

		case "desktop":
			os.Exit(handleDesktopCommand(os.Args[2:]))
		}
	}

	portFlag := flag.Int("port", 8080, "Port to listen on (default 8080, with automatic scanning if busy)")
	hostFlag := flag.String("host", "127.0.0.1", "Host interface to bind (default 127.0.0.1; use --lan or 0.0.0.0 for LAN/Tablet access)")
	lanFlag := flag.Bool("lan", false, "Enable LAN and tablet access (binds to 0.0.0.0 and displays Master PIN)")
	behindProxyFlag := flag.Bool("behind-proxy", false, "Enable reverse proxy trust mode (inspects X-Forwarded-For and X-Real-IP headers)")
	trustedProxiesFlag := flag.String("trusted-proxies", "", "Comma-separated trusted proxy IPs or CIDRs (or SQLFOX_TRUSTED_PROXIES)")
	allowedHostsFlag := flag.String("allowed-hosts", "", "Comma-separated allowed Host headers for reverse proxies (or SQLFOX_ALLOWED_HOSTS)")
	tlsFlag := flag.Bool("tls", false, "Enable TLS/HTTPS listener (auto-generates self-signed ECDSA certificate if none provided)")
	tlsCertFlag := flag.String("tls-cert", "", "Path to custom TLS certificate file (PEM)")
	tlsKeyFlag := flag.String("tls-key", "", "Path to custom TLS private key file (PEM)")
	dbFlag := flag.String("db", "", "Path to config vault database (default: ~/.sqlfox/config.db)")
	passFlag := flag.String("pass", "", "Master password for vault encryption (leave blank to auto-generate or reuse ~/.sqlfox/master.key)")
	appFlag := flag.Bool("app", true, "Launch in standalone native window mode using Chrome/Edge/Chromium if available")
	browserFlag := flag.Bool("browser", false, "Force launch in standard web browser tab instead of native standalone app window")
	noBrowserFlag := flag.Bool("no-browser", false, "Do not automatically launch web browser or app window")
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

	preferApp := *appFlag && !*browserFlag
	if os.Getenv("SQLFOX_BROWSER") == "1" || os.Getenv("SQLFOX_APP") == "0" || os.Getenv("SQLFOX_APP_MODE") == "0" {
		preferApp = false
	}

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

	// Resolve config path first so persistent diagnostic logs default to <configDir>/logs/sqlfox.log
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

	logFile := *logFileFlag
	if logFile == "" {
		logFile = os.Getenv("SQLFOX_LOG_FILE")
	}
	if logFile == "" && !strings.EqualFold(level, "silent") && !strings.EqualFold(level, "quiet") {
		logFile = filepath.Join(configDir, "logs", "sqlfox.log")
	}

	isSilent := *silentFlag || *quietFlag

	// ponytail: Console logging to terminal is opt-in (active when -v, -verbose, -log-level, or SQLFOX_LOG_LEVEL is set).
	// Default interactive desktop server keeps terminal focused on clean banner while persistent file + broadcaster still capture diagnostics.
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
				openUI(activeURL, preferApp)
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

	// Capture the executable path at startup before any in-place self-update renames/unlinks
	// the running binary (on Linux, /proc/self/exe would otherwise gain a " (deleted)" suffix).
	startupExePath, _ := os.Executable()
	if startupExePath != "" {
		if evalPath, evalErr := filepath.EvalSymlinks(startupExePath); evalErr == nil {
			startupExePath = evalPath
		}
	}

	// Claim the lock atomically (after any interactive password prompt, so a waiting launch never
	// mistakes a prompting instance for a stuck one); if another launch won the race, defer to it.
	var claimRelease func()
	if !*multiFlag && os.Getenv("SQLFOX_MULTI_INSTANCE") != "1" {
		release, err := claimInstanceLock(configDir)
		if errors.Is(err, os.ErrExist) {
			if activeURL, _, isRunning := checkExistingInstance(configDir, *portFlag); isRunning {
				if !isSilent {
					fmt.Printf("ℹ️  SqlFox is already running at %s\n", activeURL)
				}
				if !*noBrowserFlag {
					openUI(activeURL, preferApp)
				}
				return
			}
			release, err = claimInstanceLock(configDir) // the stale lock was removed by the check
		}
		if err == nil {
			claimRelease = release
			defer func() {
				if claimRelease != nil {
					claimRelease()
				}
			}()
		}
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
	envLAN := os.Getenv("SQLFOX_LAN") == "1" || strings.ToLower(os.Getenv("SQLFOX_LAN")) == "true"
	isLAN := *lanFlag || envLAN || !isLoopbackBindHost(bindHost)
	if (*lanFlag || envLAN) && isLoopbackBindHost(bindHost) {
		bindHost = "0.0.0.0"
	}

	lanIP := getLocalNetworkIP()

	// Initialize HTTP server with embedded web assets and master password
	srv := server.NewServer(v, web.Assets(), backupDir, masterPassword)
	srv.SetVersion(Version)
	srv.SetLogger(logRes.Logger)
	srv.SetLANEnabled(isLAN)
	srv.SetLANIP(lanIP)
	isBehindProxy := *behindProxyFlag || os.Getenv("SQLFOX_BEHIND_PROXY") == "1" || strings.ToLower(os.Getenv("SQLFOX_BEHIND_PROXY")) == "true"
	srv.SetBehindProxy(isBehindProxy)

	trustedRaw := *trustedProxiesFlag
	if trustedRaw == "" {
		trustedRaw = os.Getenv("SQLFOX_TRUSTED_PROXIES")
	}
	if trustedRaw != "" {
		var proxies []string
		for _, p := range strings.Split(trustedRaw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				proxies = append(proxies, p)
			}
		}
		srv.SetTrustedProxies(proxies)
	}

	allowedRaw := *allowedHostsFlag
	if allowedRaw == "" {
		allowedRaw = os.Getenv("SQLFOX_ALLOWED_HOSTS")
	}
	if allowedRaw != "" {
		var hosts []string
		for _, h := range strings.Split(allowedRaw, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hosts = append(hosts, h)
			}
		}
		srv.SetAllowedHosts(hosts)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	restartChan := make(chan struct{}, 1)

	srv.SetOpenBrowserFn(func(targetURL string) error {
		return openDefaultBrowser(targetURL)
	})
	srv.SetShutdownFn(func() {
		select {
		case quit <- syscall.SIGTERM:
		default:
		}
	})
	srv.SetRestartFn(func() error {
		select {
		case restartChan <- struct{}{}:
		default:
		}
		return nil
	})

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

	envTLS := os.Getenv("SQLFOX_TLS") == "1" || strings.ToLower(os.Getenv("SQLFOX_TLS")) == "true"
	tlsCert := *tlsCertFlag
	if tlsCert == "" {
		tlsCert = os.Getenv("SQLFOX_TLS_CERT")
	}
	tlsKey := *tlsKeyFlag
	if tlsKey == "" {
		tlsKey = os.Getenv("SQLFOX_TLS_KEY")
	}
	isTLS := *tlsFlag || envTLS || tlsCert != "" || tlsKey != ""

	var tlsConfig *tls.Config
	if isTLS {
		certDir := filepath.Join(configDir, "certs")
		hostnames := []string{"localhost", "127.0.0.1"}
		if lanIP != "" {
			hostnames = append(hostnames, lanIP)
		}
		if bindHost != "127.0.0.1" && bindHost != "0.0.0.0" && bindHost != "" {
			hostnames = append(hostnames, bindHost)
		}
		cfg, _, _, err := server.LoadOrCreateTLSConfig(tlsCert, tlsKey, certDir, hostnames)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to initialize TLS: %v\n", err)
			os.Exit(1)
		}
		tlsConfig = cfg
	}

	scheme := "http"
	if isTLS {
		scheme = "https"
	}
	if !isTLS && (*lanFlag || bindHost == "0.0.0.0" || bindHost == "::") && !isSilent {
		fmt.Fprintln(os.Stderr, "⚠️  SECURITY WARNING: LAN access is running over plaintext HTTP. Pass --tls to encrypt LAN traffic, Master PIN, and bearer tokens.")
	}
	localURL := fmt.Sprintf("%s://localhost:%d", scheme, actualPort)

	var lockCleanup func()
	if !*multiFlag && os.Getenv("SQLFOX_MULTI_INSTANCE") != "1" {
		cleanup, err := writeInstanceLock(configDir, actualPort, localURL, bindHost)
		if err == nil {
			lockCleanup = cleanup
			defer lockCleanup()
		}
	}

	if isTLS && tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}

	// ponytail: WriteTimeout must remain disabled (0) so persistent SSE streams (/api/logs/stream)
	// and large streaming database dumps are not abruptly disconnected after 60 seconds.
	httpServer := &http.Server{
		Handler:           srv.Router(),
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
		}
	}()

	if !isSilent {
		tlsInfo := ""
		if isTLS {
			if tlsCert != "" {
				tlsInfo = fmt.Sprintf("Custom (%s)", filepath.Base(tlsCert))
			} else {
				tlsInfo = "Self-Signed ECDSA (HTTPS Enabled)"
			}
		}

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
			IsTLS:          isTLS,
			TLSCertInfo:    tlsInfo,
		})
	}

	logRes.Logger.Info("sqlfox_started",
		"version", Version,
		"port", actualPort,
		"local_url", localURL,
		"vault", dbPath,
	)

	// Auto launch browser or native app window to local URL
	if !*noBrowserFlag {
		go func() {
			time.Sleep(150 * time.Millisecond)
			openUI(localURL, preferApp)
		}()
	}

	// Graceful shutdown or restart
	isRestart := false
	select {
	case <-quit:
	case <-restartChan:
		isRestart = true
	}

	if isRestart {
		fmt.Println("\nRestarting SqlFox for update...")
	} else {
		fmt.Println("\nShutting down SqlFox gracefully...")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
	}
	srv.Close()
	if lockCleanup != nil {
		lockCleanup()
		lockCleanup = nil
	}
	if claimRelease != nil {
		claimRelease()
		claimRelease = nil
	}

	if isRestart {
		// Explicitly close vault SQLite handles and log files before syscall.Exec,
		// because deferred cleanups do not execute once the process image is replaced.
		_ = v.Close()
		if logRes.Closer != nil {
			_ = logRes.Closer.Close()
		}
		exePath := startupExePath
		if exePath == "" {
			if ep, err := os.Executable(); err == nil {
				if evalPath, err := filepath.EvalSymlinks(ep); err == nil {
					ep = evalPath
				}
				exePath = strings.TrimSuffix(ep, " (deleted)")
			}
		}
		restartArgs := appendNoBrowserArg(os.Args)
		if err := restartProcess(exePath, restartArgs); err != nil {
			// Fallback: spawn background child if restartProcess failed
			cmd := exec.Command(exePath, restartArgs[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			if startErr := cmd.Start(); startErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to restart SqlFox: %v\n", startErr)
			}
		}
		return
	}

	fmt.Println("SqlFox stopped.")
}

// parseStopArgs parses optional -db and -port flags for `sqlfox stop`.
func parseStopArgs(args []string) (configDir string, port int) {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "Path to encrypted vault SQLite file")
	portVal := fs.Int("port", 8080, "Port of running SqlFox instance")
	_ = fs.Parse(args)

	if *dbPath != "" {
		configDir = filepath.Dir(*dbPath)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		configDir = filepath.Join(home, ".sqlfox")
	}
	return configDir, *portVal
}

func resolveMasterPassword(flagPass string, configDir string) (string, bool, error) {
	// 1. Explicit CLI flag
	if flagPass != "" {
		return flagPass, false, nil
	}

	// 2. Environment variable SQLFOX_PASS or SQLFOX_MASTER_KEY
	if envPass := os.Getenv("SQLFOX_PASS"); envPass != "" {
		return envPass, false, nil
	}
	if envPass := os.Getenv("SQLFOX_MASTER_KEY"); envPass != "" {
		return envPass, false, nil
	}

	// 2b. Optional OS Keychain lookup when SQLFOX_KEYCHAIN=1
	useKeychain := os.Getenv("SQLFOX_KEYCHAIN") == "1"
	if useKeychain {
		if kcPass, ok := vault.ReadKeychainMasterPassword(); ok {
			return kcPass, false, nil
		}
	}

	// 3. Saved master.key file
	keyFile := filepath.Join(configDir, "master.key")
	if saved, err := vault.ReadMasterKeyFile(keyFile); err == nil && saved != "" {
		if useKeychain {
			_ = vault.WriteKeychainMasterPassword(saved)
		}
		return saved, false, nil
	}

	// 4. Generate cryptographically strong PIN with rejection sampling (no modulo bias, no hardcoded fallback).
	pin, err := generateFriendlyPIN()
	if err != nil {
		return "", false, fmt.Errorf("failed to generate master PIN: %w", err)
	}

	if useKeychain {
		if err := vault.WriteKeychainMasterPassword(pin); err == nil {
			return pin, true, nil
		}
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", false, fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}
	if err := os.WriteFile(keyFile, []byte(pin), 0600); err != nil {
		return "", false, fmt.Errorf("failed to persist master.key to %s: %w", keyFile, err)
	}
	return pin, true, nil
}

func generateFriendlyPIN() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const maxValid = 256 - (256 % len(charset)) // rejection threshold eliminates modulo bias
	out := make([]byte, 10)
	buf := make([]byte, 1)
	for i := 0; i < len(out); {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		if int(buf[0]) >= maxValid {
			continue
		}
		out[i] = charset[int(buf[0])%len(charset)]
		i++
	}
	return "fox-" + string(out), nil
}

// tryListen checks if port is free on loopback interfaces before binding.
// ponytail: on Windows Winsock and dual-stack setups, net.Listen("tcp", "127.0.0.1:p") or wildcard
// can bind without error even when another service (e.g. Pantau) already listens on [::1]:p or :p.
// Probing via DialTimeout and verifying IPv4 binding ensures busy ports trigger auto-scanning.
func tryListen(host string, port int) net.Listener {
	portStr := fmt.Sprintf("%d", port)
	for _, probeHost := range []string{"127.0.0.1", "::1"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(probeHost, portStr), 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
	}

	if host == "127.0.0.1" || host == "0.0.0.0" {
		if l4, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
			return nil
		} else {
			_ = l4.Close()
		}
	}

	l, err := net.Listen("tcp", net.JoinHostPort(host, portStr))
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

func isLoopbackBindHost(host string) bool {
	h := strings.Trim(strings.TrimSpace(host), "[]")
	if h == "" || strings.EqualFold(h, "localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
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

// loadEnvFiles loads environment variables ONLY from trusted user config paths (~/.sqlfox/).
// Untrusted CWD and executable directories (e.g. ~/Downloads) are intentionally NOT auto-loaded
// so a stray sqlfox.env next to a downloaded binary cannot inject SQLFOX_LAN or SQLFOX_PPROF.
func loadEnvFiles() {
	var candidates []string

	// In ~/.sqlfox/ (trusted user config directory)
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

func appendNoBrowserArg(args []string) []string {
	for _, a := range args {
		if a == "--no-browser" || a == "-no-browser" || strings.HasPrefix(a, "--no-browser=") || strings.HasPrefix(a, "-no-browser=") {
			return args
		}
	}
	res := make([]string, len(args), len(args)+1)
	copy(res, args)
	return append(res, "--no-browser")
}
