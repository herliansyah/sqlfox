//go:build !windows && !darwin

package main

import (
	"testing"
)

func TestFindChromiumCandidateLookupLinux(t *testing.T) {
	bin, args, found := findChromiumLinux("http://127.0.0.1:8080")
	if found {
		t.Logf("Found Linux Chromium browser: %s with args: %v", bin, args)
	}
}
