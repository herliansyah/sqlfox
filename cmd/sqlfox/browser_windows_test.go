//go:build windows

package main

import (
	"testing"
)

func TestFindChromiumCandidateLookupWindows(t *testing.T) {
	bin, args, found := findChromiumWindows("http://127.0.0.1:8080")
	if found {
		t.Logf("Found Windows Chromium browser: %s with args: %v", bin, args)
	}
}
