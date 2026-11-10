//go:build !windows && !darwin

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// chromiumLinuxCandidates lists common Chromium-based browser binaries on Linux desktop environments.
var chromiumLinuxCandidates = []string{
	"google-chrome-stable",
	"google-chrome",
	"chromium",
	"chromium-browser",
	"microsoft-edge-stable",
	"microsoft-edge",
	"brave-browser",
	"/snap/bin/chromium",
	"/snap/bin/brave",
}

func findChromiumLinux(targetURL string) (string, []string, bool) {
	for _, name := range chromiumLinuxCandidates {
		if path, err := exec.LookPath(name); err == nil {
			args := []string{"--app=" + targetURL, "--class=SqlFox"}
			if strings.HasPrefix(targetURL, "https://localhost") || strings.HasPrefix(targetURL, "https://127.0.0.1") {
				args = append(args, "--allow-insecure-localhost")
			}
			return path, args, true
		}
	}
	return "", nil, false
}

func launchAppWindow(targetURL string) error {
	bin, args, found := findChromiumLinux(targetURL)
	if !found {
		return fmt.Errorf("no chromium-based browser found on linux")
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	return cmd.Start()
}

func openDefaultBrowser(targetURL string) error {
	cmdName := "xdg-open"
	if _, err := exec.LookPath(cmdName); err != nil {
		if _, err := exec.LookPath("sensible-browser"); err == nil {
			cmdName = "sensible-browser"
		} else if _, err := exec.LookPath("x-www-browser"); err == nil {
			cmdName = "x-www-browser"
		}
	}

	c := exec.Command(cmdName, targetURL)
	c.Stdin = nil
	c.Stdout = nil
	c.Stderr = nil
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return c.Start()
}
