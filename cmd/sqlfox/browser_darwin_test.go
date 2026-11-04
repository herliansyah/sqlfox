//go:build darwin

package main

import (
	"testing"
)

func TestFindChromiumCandidateLookupDarwin(t *testing.T) {
	bin, args, found := findChromiumDarwin("http://127.0.0.1:8080")
	if found {
		t.Logf("Found macOS Chromium browser: %s with args: %v", bin, args)
	}
}
