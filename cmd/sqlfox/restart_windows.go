//go:build windows

package main

import (
	"os"
	"os/exec"
)

// restartProcess starts a new background process for SqlFox on Windows.
// Because the server listener and instance lock have already been gracefully
// released prior to calling restartProcess, the child process can bind the port
// cleanly without collision.
func restartProcess(exePath string, args []string) error {
	cmd := exec.Command(exePath, args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Start()
}
