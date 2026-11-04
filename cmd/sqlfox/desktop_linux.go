//go:build !windows && !darwin

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sqlfox/web"
)

func handleDesktopCommand(args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to determine user home directory: %v\n", err)
		return 1
	}

	appsDir := filepath.Join(home, ".local", "share", "applications")
	desktopPath := filepath.Join(appsDir, "sqlfox.desktop")
	iconDir := filepath.Join(home, ".local", "share", "icons", "hicolor", "512x512", "apps")
	iconPath := filepath.Join(iconDir, "sqlfox.png")
	legacyIconPath := filepath.Join(home, ".local", "share", "icons", "sqlfox.png")

	switch sub {
	case "install":
		exePath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to determine executable path: %v\n", err)
			return 1
		}
		exePath, err = filepath.EvalSymlinks(exePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to resolve symlinks for executable: %v\n", err)
			return 1
		}

		if err := os.MkdirAll(iconDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create icon directory: %v\n", err)
			return 1
		}
		_ = os.MkdirAll(filepath.Dir(legacyIconPath), 0755)

		// Extract embedded icon
		assets := web.Assets()
		f, err := assets.Open("icon-512.png")
		if err == nil {
			defer f.Close()
			data, readErr := io.ReadAll(f)
			if readErr == nil {
				_ = os.WriteFile(iconPath, data, 0644)
				_ = os.WriteFile(legacyIconPath, data, 0644)
			}
		}

		withLAN := false
		for _, arg := range args {
			if arg == "--lan" || arg == "-lan" {
				withLAN = true
			}
		}

		execTarget := fmt.Sprintf(`"%s"`, exePath)
		if withLAN {
			execTarget = fmt.Sprintf(`"%s" --lan`, exePath)
		}

		// Write .desktop entry
		desktopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=SqlFox
GenericName=Database Manager
Comment=Lightweight High-Performance Database Manager
Exec=%s
Icon=sqlfox
Terminal=false
Categories=Development;Database;
StartupWMClass=SqlFox
`, execTarget)

		if err := os.MkdirAll(appsDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create applications directory: %v\n", err)
			return 1
		}
		if err := os.WriteFile(desktopPath, []byte(desktopContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to write desktop entry: %v\n", err)
			return 1
		}

		// Update desktop database and icon cache if available
		if updateBin, err := exec.LookPath("update-desktop-database"); err == nil {
			_ = exec.Command(updateBin, appsDir).Run()
		}
		if iconUpdateBin, err := exec.LookPath("gtk-update-icon-cache"); err == nil {
			_ = exec.Command(iconUpdateBin, "-q", "-t", "-f", filepath.Join(home, ".local", "share", "icons", "hicolor")).Run()
		}

		fmt.Println("✅ SqlFox desktop launcher installed successfully!")
		fmt.Printf("   Entry: %s\n", desktopPath)
		fmt.Printf("   Icon:  %s\n", iconPath)
		fmt.Printf("   Exec:  %s\n", execTarget)
		if withLAN {
			fmt.Println("   LAN Access: ENABLED (binds to 0.0.0.0 with Master PIN authentication guard)")
		} else {
			fmt.Println("   LAN Access: Disabled (localhost only; pass --lan to enable)")
		}
		fmt.Println("   You can now launch SqlFox from your desktop application menu without opening a terminal.")
		return 0

	case "uninstall":
		_ = os.Remove(desktopPath)
		_ = os.Remove(iconPath)
		_ = os.Remove(legacyIconPath)
		if updateBin, err := exec.LookPath("update-desktop-database"); err == nil {
			_ = exec.Command(updateBin, appsDir).Run()
		}
		if iconUpdateBin, err := exec.LookPath("gtk-update-icon-cache"); err == nil {
			_ = exec.Command(iconUpdateBin, "-q", "-t", "-f", filepath.Join(home, ".local", "share", "icons", "hicolor")).Run()
		}
		fmt.Println("✅ SqlFox desktop launcher uninstalled.")
		return 0

	case "status":
		if data, err := os.ReadFile(desktopPath); err == nil {
			fmt.Println("ℹ️  SqlFox desktop launcher is INSTALLED:")
			fmt.Printf("   Location: %s\n", desktopPath)
			if strings.Contains(string(data), "--lan") {
				fmt.Println("   LAN Access: ENABLED (Tablet / LAN connections accepted)")
			} else {
				fmt.Println("   LAN Access: Localhost only")
			}
		} else {
			fmt.Println("ℹ️  SqlFox desktop launcher is NOT installed.")
			fmt.Println("   Run 'sqlfox desktop install' (or 'sqlfox desktop install --lan') to create shortcut.")
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "Usage: sqlfox desktop [install|uninstall|status]\n")
		return 1
	}
}
