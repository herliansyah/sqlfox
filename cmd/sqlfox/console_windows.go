//go:build windows

package main

import (
	"os"
	"syscall"
)

// initConsole attaches to the parent console if launched from a terminal (PowerShell, CMD, Git Bash).
// When SqlFox is compiled with -ldflags "-H=windowsgui" for hidden-terminal background execution,
// this allows CLI commands (query, transfer, job, advise, stop, desktop, help) to still output text directly
// to the caller's console. If launched by double-clicking in Explorer, AttachConsole returns
// an error and no black CMD window is displayed.
func initConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attachConsole := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0) // (DWORD)-1

	r, _, _ := attachConsole.Call(attachParentProcess)
	if r != 0 {
		if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stdout = f
			os.Stderr = f
		}
		if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
			os.Stdin = f
		}
	}
}
