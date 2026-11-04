//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func findChromiumDarwin(targetURL string) (string, []string, bool) {
	home, _ := os.UserHomeDir()
	appNames := []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium"}

	for _, app := range appNames {
		sysApp := filepath.Join("/Applications", app+".app")
		userApp := filepath.Join(home, "Applications", app+".app")

		// Prefer invoking the inner executable directly to bypass open -n profile locking issues
		for _, appDir := range []string{sysApp, userApp} {
			innerBin := filepath.Join(appDir, "Contents", "MacOS", app)
			if _, err := os.Stat(innerBin); err == nil {
				return innerBin, []string{"--app=" + targetURL}, true
			}
		}

		if _, err := os.Stat(sysApp); err == nil {
			return "open", []string{"-a", sysApp, "--args", "--app=" + targetURL}, true
		}
		if _, err := os.Stat(userApp); err == nil {
			return "open", []string{"-a", userApp, "--args", "--app=" + targetURL}, true
		}
	}

	for _, bin := range []string{"google-chrome", "chromium"} {
		if p, err := exec.LookPath(bin); err == nil {
			return p, []string{"--app=" + targetURL}, true
		}
	}

	return "", nil, false
}

func launchAppWindow(targetURL string) error {
	cmdName, args, found := findChromiumDarwin(targetURL)
	if !found {
		return fmt.Errorf("no chromium-based browser found on macos")
	}

	cmd := exec.Command(cmdName, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

func openDefaultBrowser(targetURL string) error {
	cmd := exec.Command("open", targetURL)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
