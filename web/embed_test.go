package web

import (
	"io"
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedAssetsIntegrity(t *testing.T) {
	fsys := Assets()

	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	defer f.Close()

	htmlBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	html := string(htmlBytes)

	if !strings.Contains(html, `<script type="module"`) {
		t.Fatalf("dist/index.html does not reference a module bundle")
	}
	if !strings.Contains(html, `href="/vendor/fonts/fonts.css"`) {
		t.Errorf("index.html does not link /vendor/fonts/fonts.css")
	}
	if !strings.Contains(html, `id="remote-lock-modal"`) {
		t.Errorf("index.html missing #remote-lock-modal shell for remote access guard")
	}

	entries, err := fs.ReadDir(distFS, "dist/assets")
	if err != nil {
		t.Fatalf("read dist/assets: %v", err)
	}
	var hasJS, hasCSS bool
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "index-") && strings.HasSuffix(name, ".js") {
			b, err := distFS.ReadFile("dist/assets/" + name)
			if err != nil || len(b) == 0 {
				t.Fatalf("invalid JS bundle %s: err=%v len=%d", name, err, len(b))
			}
			hasJS = true
		}
		if strings.HasPrefix(name, "index-") && strings.HasSuffix(name, ".css") {
			b, err := distFS.ReadFile("dist/assets/" + name)
			if err != nil || len(b) == 0 {
				t.Fatalf("invalid CSS bundle %s: err=%v len=%d", name, err, len(b))
			}
			hasCSS = true
		}
	}
	if !hasJS || !hasCSS {
		t.Fatalf("expected both index-*.js (%v) and index-*.css (%v) in dist/assets", hasJS, hasCSS)
	}
}

func TestEmbeddedFonts(t *testing.T) {
	fsys := Assets()

	requiredFiles := []string{
		"vendor/fonts/fonts.css",
		"vendor/fonts/inter-latin.woff2",
		"vendor/fonts/inter-latin-ext.woff2",
		"vendor/fonts/jetbrains-mono-latin.woff2",
		"vendor/fonts/jetbrains-mono-latin-ext.woff2",
		"vendor/fonts/jetbrains-mono-italic-latin.woff2",
	}

	for _, relPath := range requiredFiles {
		f, err := fsys.Open(relPath)
		if err != nil {
			t.Fatalf("required font asset %q missing in embedded FS: %v", relPath, err)
		}
		buf := make([]byte, 64)
		n, _ := f.Read(buf)
		_ = f.Close()
		if n == 0 {
			t.Errorf("font asset %q is empty", relPath)
		}
		if strings.HasSuffix(relPath, ".woff2") {
			if n < 4 || string(buf[:4]) != "wOF2" {
				t.Errorf("font asset %q does not contain valid WOFF2 header magic (got %q)", relPath, string(buf[:min(n, 4)]))
			}
		}
	}
}
