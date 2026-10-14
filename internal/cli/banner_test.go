package cli

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestBanner_LocalhostMode(t *testing.T) {
	_ = os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	var buf bytes.Buffer
	cfg := ServerBannerConfig{
		Version:   "v0.9.0",
		LocalURL:  "http://localhost:8080",
		IsLAN:     false,
		Port:      8080,
		VaultPath: "/home/user/.sqlfox/config.db",
	}

	RenderServerBanner(&buf, cfg)
	out := buf.String()

	// Assert required domain elements
	if !strings.Contains(out, "SqlFox") {
		t.Errorf("banner missing product name 'SqlFox'")
	}
	if !strings.Contains(out, "http://localhost:8080") {
		t.Errorf("banner missing local endpoint URL")
	}
	if !strings.Contains(out, "LAN ACCESS") || !strings.Contains(out, "Disabled") {
		t.Errorf("banner missing localhost LAN access status note")
	}
	if !strings.Contains(out, "MySQL • PostgreSQL • SQLite") {
		t.Errorf("banner missing embedded engines list")
	}

	// Verify line lengths and box integrity
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, l := range lines {
		clean := ansiRegex.ReplaceAllString(l, "")
		runes := []rune(clean)
		// Box lines (starting with ╔, ║, ╠, ╚) must be exactly 72 runes wide
		if strings.HasPrefix(clean, "╔") || strings.HasPrefix(clean, "║") || strings.HasPrefix(clean, "╠") || strings.HasPrefix(clean, "╚") {
			if len(runes) != 72 {
				t.Errorf("line %d has length %d runes, expected 72:\n%s", i+1, len(runes), clean)
			}
		}
	}
}

func TestBanner_LANMode(t *testing.T) {
	_ = os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	var buf bytes.Buffer
	cfg := ServerBannerConfig{
		Version:        "v0.9.0",
		LocalURL:       "http://localhost:8080",
		IsLAN:          true,
		LANIP:          "192.168.1.100",
		Port:           8080,
		MasterPassword: "fox-secret-pin",
		IsNewPIN:       true,
		VaultPath:      "/tmp/config.db",
	}

	RenderServerBanner(&buf, cfg)
	out := buf.String()

	if !strings.Contains(out, "LAN ENDPOINT") || !strings.Contains(out, "192.168.1.100:8080") {
		t.Errorf("expected LAN endpoint in banner, got:\n%s", out)
	}
	if !strings.Contains(out, "MASTER PIN") || !strings.Contains(out, "fox-secret-pin") {
		t.Errorf("expected master PIN in banner, got:\n%s", out)
	}
	if !strings.Contains(out, "master.key") {
		t.Errorf("expected new PIN note in footer, got:\n%s", out)
	}
}

func TestBanner_HelpHeader(t *testing.T) {
	_ = os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	var buf bytes.Buffer
	RenderHelpHeader(&buf, "v0.9.0")
	out := buf.String()

	if !strings.Contains(out, "v0.9.0") {
		t.Errorf("expected version tag in help header, got:\n%s", out)
	}
	if !strings.Contains(out, "Modern DB Suite") {
		t.Errorf("expected tagline in help header, got:\n%s", out)
	}
}
