//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func findChromiumWindows(targetURL string) (string, []string, bool) {
	appArgs := []string{"--app=" + targetURL}
	if strings.HasPrefix(targetURL, "https://localhost") || strings.HasPrefix(targetURL, "https://127.0.0.1") {
		appArgs = append(appArgs, "--allow-insecure-localhost")
	}

	// 1. Check in system PATH
	for _, name := range []string{"msedge.exe", "chrome.exe", "brave.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, appArgs, true
		}
	}

	// 2. Check standard Windows filesystem installation paths
	progFiles := os.Getenv("ProgramFiles")
	progFilesX86 := os.Getenv("ProgramFiles(x86)")
	localAppData := os.Getenv("LOCALAPPDATA")

	var candidates []string
	if progFilesX86 != "" {
		candidates = append(candidates, filepath.Join(progFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"))
		candidates = append(candidates, filepath.Join(progFilesX86, "Google", "Chrome", "Application", "chrome.exe"))
		candidates = append(candidates, filepath.Join(progFilesX86, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"))
	}
	if progFiles != "" {
		candidates = append(candidates, filepath.Join(progFiles, "Microsoft", "Edge", "Application", "msedge.exe"))
		candidates = append(candidates, filepath.Join(progFiles, "Google", "Chrome", "Application", "chrome.exe"))
		candidates = append(candidates, filepath.Join(progFiles, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"))
	}
	if localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "Microsoft", "Edge", "Application", "msedge.exe"))
		candidates = append(candidates, filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"))
		candidates = append(candidates, filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"))
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, appArgs, true
		}
	}

	return "", nil, false
}

func launchAppWindow(targetURL string) error {
	bin, args, found := findChromiumWindows(targetURL)
	if !found {
		return fmt.Errorf("no chromium-based browser found on windows")
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

func openDefaultBrowser(targetURL string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
