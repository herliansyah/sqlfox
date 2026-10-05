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
	"sqlfox/internal/server"
	"sqlfox/internal/vault"
	"sqlfox/web"

	_ "sqlfox/internal/driver/mysql"
	_ "sqlfox/internal/driver/postgres"
	_ "sqlfox/internal/driver/sqlite"
)

// Version is populated at build time via -ldflags "-X main.Version=..."
var Version = "v0.2.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "query", "transfer", "job", "advise", "help":
			os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
		}
	}

	portFlag := flag.Int("port", 8080, "Port to listen on (default 8080, with automatic scanning if busy)")
	hostFlag := flag.String("host", "0.0.0.0", "Host interface to bind (0.0.0.0 for LAN/Tablet access, 127.0.0.1 for local only)")
	dbFlag := flag.String("db", "", "Path to config vault database (default: ~/.sqlfox/config.db)")
	passFlag := flag.String("pass", "", "Master password for vault encryption (leave blank to auto-generate or reuse ~/.sqlfox/master.key)")
	noBrowserFlag := flag.Bool("no-browser", false, "Do not automatically launch web browser")
	strictPortFlag := flag.Bool("strict-port", false, "Fail immediately if specified port is in use")
	versionFlag := flag.Bool("version", false, "Print SqlFox version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("SqlFox %s\n", Version)
		return
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

	// Initialize HTTP server with embedded web assets and master password
	srv := server.NewServer(v, web.Assets(), backupDir, masterPassword)
	srv.SetVersion(Version)

	maxAttempts := 50
	if *strictPortFlag {
		maxAttempts = 1
	}

	// Automatic port hunting
	listener, actualPort, err := listenWithPortHunting(*hostFlag, *portFlag, maxAttempts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to bind listener: %v\n", err)
		os.Exit(1)
	}

	localURL := fmt.Sprintf("http://localhost:%d", actualPort)
	lanIP := getLocalNetworkIP()

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

	fmt.Printf("\n🦊 SqlFox %s Database Manager is running!\n", Version)
	fmt.Printf("   ➜  Local:       %s\n", localURL)
	if lanIP != "" {
		fmt.Printf("   ➜  Network:     http://%s:%d (Buka dari Tablet/LAN)\n", lanIP, actualPort)
	}
	fmt.Printf("   ➜  Master PIN:  %s  (Ketik ini saat membuka dari Tablet/HP)\n", masterPassword)
	if isNewPIN {
		fmt.Printf("      (PIN baru dibuat dan disimpan di %s)\n", filepath.Join(configDir, "master.key"))
	}
	fmt.Printf("   ➜  Vault:       %s\n", dbPath)
	fmt.Printf("   ➜  Backups:     %s\n\n", backupDir)
	fmt.Println("   Press Ctrl+C to shut down gracefully.")

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
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	num := int(b[0])<<8 | int(b[1])
	return fmt.Sprintf("fox-%04d", num%10000), nil
}

func listenWithPortHunting(host string, startPort int, maxAttempts int) (net.Listener, int, error) {
	for i := 0; i < maxAttempts; i++ {
		port := startPort + i
		addr := fmt.Sprintf("%s:%d", host, port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			if i > 0 {
				fmt.Printf("⚠️  Port %d was busy. Automatically switched to free port %d.\n", startPort, port)
			}
			return l, port, nil
		}
	}
	return nil, 0, fmt.Errorf("could not find an available port after %d attempts starting at %d", maxAttempts, startPort)
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
