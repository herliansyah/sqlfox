//go:build !windows

package main

import (
	"os"
	"syscall"
)

// restartProcess replaces the current process image in-place using syscall.Exec.
// On POSIX systems, this preserves the PID, eliminating race conditions with
// lock files, port binding, and duplicate UI launches.
func restartProcess(exePath string, args []string) error {
	return syscall.Exec(exePath, args, os.Environ())
}
