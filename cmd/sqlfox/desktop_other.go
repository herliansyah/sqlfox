//go:build windows || darwin

package main

import (
	"fmt"
)

func handleDesktopCommand(args []string) int {
	fmt.Println("ℹ️  'sqlfox desktop' is designed for Linux desktop environments (X11 / Wayland).")
	fmt.Println("   On Windows, double-click sqlfox.exe directly (or build with -ldflags \"-H=windowsgui\").")
	fmt.Println("   On macOS, double-click or run SqlFox directly.")
	return 0
}
