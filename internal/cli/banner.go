package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ServerBannerConfig specifies runtime metadata for rendering the Terminal Startup Banner.
type ServerBannerConfig struct {
	Version        string
	LocalURL       string
	IsLAN          bool
	LANIP          string
	Port           int
	MasterPassword string
	IsNewPIN       bool
	VaultPath      string
	BackupDir      string
}

// ponytail: check if output is an interactive character terminal and respects NO_COLOR/TERM
func isColorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if f, ok := w.(*os.File); ok {
		fi, err := f.Stat()
		if err == nil {
			return (fi.Mode() & os.ModeCharDevice) != 0
		}
	}
	return false
}

// RenderServerBanner writes the full-width Heavy Devtool Box banner to the provided writer.
func RenderServerBanner(w io.Writer, cfg ServerBannerConfig) {
	useColor := isColorEnabled(w)

	var (
		cOrange = ""
		cAmber  = ""
		cYellow = ""
		cGreen  = ""
		cCyan   = ""
		cDim    = ""
		cBold   = ""
		cReset  = ""
	)

	if useColor {
		cOrange = "\x1b[38;5;208m"
		cAmber = "\x1b[38;5;214m"
		cYellow = "\x1b[38;5;220m"
		cGreen = "\x1b[38;5;48m"
		cCyan = "\x1b[38;5;51m"
		cDim = "\x1b[38;5;242m"
		cBold = "\x1b[1m"
		cReset = "\x1b[0m"
	}

	versionTag := "SqlFox " + cfg.Version
	if cfg.Version == "" {
		versionTag = "SqlFox v0.9.0"
	}
	// Pad or truncate version tag to fit in 16-char slot
	if len(versionTag) > 16 {
		versionTag = versionTag[:16]
	}
	vPad := strings.Repeat(" ", 16-len(versionTag))

	topBorder := fmt.Sprintf("%s╔══════════════════════════════════════════════════════════════════════╗%s\n", cDim, cReset)
	midBorder := fmt.Sprintf("%s╠══════════════════════════════════════════════════════════════════════╣%s\n", cDim, cReset)
	botBorder := fmt.Sprintf("%s╚══════════════════════════════════════════════════════════════════════╝%s\n", cDim, cReset)

	logoLines := []string{
		fmt.Sprintf("%s║%s  %s███████╗ ██████╗ ██╗     ███████╗ ██████╗ ██╗  ██╗%s  %s%s%s%s%s║%s\n",
			cDim, cReset, cOrange, cReset, cDim, versionTag, vPad, cReset, cDim, cReset),
		fmt.Sprintf("%s║%s  %s██╔════╝██╔═══██╗██║     ██╔════╝██╔═══██╗╚██╗██╔╝%s  %s[STABLE]%s        %s║%s\n",
			cDim, cReset, cOrange, cReset, cGreen, cReset, cDim, cReset),
		fmt.Sprintf("%s║%s  %s███████╗██║   ██║██║     █████╗  ██║   ██║ ╚███╔╝%s   %s[ALL DRIVERS]%s   %s║%s\n",
			cDim, cReset, cAmber, cReset, cCyan, cReset, cDim, cReset),
		fmt.Sprintf("%s║%s  %s╚════██║██║▄▄ ██║██║     ██╔══╝  ██║   ██║ ██╔██╗%s                   %s║%s\n",
			cDim, cReset, cAmber, cReset, cDim, cReset),
		fmt.Sprintf("%s║%s  %s███████║╚██████╔╝███████╗██║     ╚██████╔╝██╔╝ ██╗%s  %sHTTP DAEMON%s     %s║%s\n",
			cDim, cReset, cYellow, cReset, cDim, cReset, cDim, cReset),
		fmt.Sprintf("%s║%s  %s╚══════╝ ╚══▀▀═╝ ╚══════╝╚═╝      ╚═════╝ ╚═╝  ╚═╝%s                  %s║%s\n",
			cDim, cReset, cYellow, cReset, cDim, cReset),
	}

	fmt.Fprint(w, "\n"+topBorder)
	for _, line := range logoLines {
		fmt.Fprint(w, line)
	}
	fmt.Fprint(w, midBorder)

	// Format metadata lines strictly to 70 interior characters
	renderMeta := func(label string, val string, valColor string) {
		plainText := fmt.Sprintf("  %-15s: %s", label, val)
		runes := []rune(plainText)
		if len(runes) > 70 {
			plainText = string(runes[:69]) + "…"
			runes = []rune(plainText)
		}
		pad := strings.Repeat(" ", 70-len(runes))

		if useColor {
			fmt.Fprintf(w, "%s║%s  %s%-15s%s: %s%s%s%s%s║%s\n",
				cDim, cReset, cBold, label, cReset, valColor, val, pad, cReset, cDim, cReset)
		} else {
			fmt.Fprintf(w, "║  %-15s: %s%s║\n", label, val, pad)
		}
	}

	localDisplay := cfg.LocalURL
	if localDisplay == "" {
		localDisplay = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	renderMeta("LOCAL ENDPOINT", localDisplay, cCyan)

	if cfg.IsLAN {
		lanDisplay := fmt.Sprintf("http://%s:%d (Tablet / Mobile)", cfg.LANIP, cfg.Port)
		if cfg.LANIP == "" {
			lanDisplay = fmt.Sprintf("http://0.0.0.0:%d (Tablet / Mobile)", cfg.Port)
		}
		renderMeta("LAN ENDPOINT", lanDisplay, cCyan)
		renderMeta("MASTER PIN", cfg.MasterPassword, cYellow)
	} else {
		renderMeta("LAN ACCESS", "Disabled (run with --lan for tablet/mobile)", cDim)
	}

	vaultDisplay := cfg.VaultPath
	if vaultDisplay == "" {
		vaultDisplay = "~/.sqlfox/config.db"
	}
	renderMeta("CONFIG VAULT", vaultDisplay, cReset)

	renderMeta("ENGINES", "MySQL • PostgreSQL • SQLite (Embedded Pure Go)", cGreen)
	renderMeta("RUNTIME STATE", "● Ready  •  Press [Ctrl+C] to exit", cGreen)

	fmt.Fprint(w, botBorder)
	if cfg.IsLAN && cfg.IsNewPIN {
		fmt.Fprintf(w, "  %sℹ  Master PIN auto-generated and saved to master.key%s\n", cDim, cReset)
	}
	fmt.Fprintln(w)
}

// RenderHelpHeader writes the compact CLI Hero Header at the top of sqlfox help.
func RenderHelpHeader(w io.Writer, version string) {
	useColor := isColorEnabled(w)

	var (
		cOrange = ""
		cAmber  = ""
		cYellow = ""
		cDim    = ""
		cBold   = ""
		cReset  = ""
	)

	if useColor {
		cOrange = "\x1b[38;5;208m"
		cAmber = "\x1b[38;5;214m"
		cYellow = "\x1b[38;5;220m"
		cDim = "\x1b[38;5;242m"
		cBold = "\x1b[1m"
		cReset = "\x1b[0m"
	}

	if version == "" {
		version = "v0.9.0"
	}

	header := fmt.Sprintf(`%s▄█████  ██████  ██      ███████  ██████  ██   ██%s
%s▀█▄▄▄▄  ██  ██  ██      ██▄▄▄▄   ██  ██   ▀███▀ %s
%s ▄▄▄▄█▀ ▀██████ ███████ ██       ██████   ██ ▀█▄%s  %s%s%s  %s• Modern DB Suite%s
`, cOrange, cReset, cAmber, cReset, cYellow, cReset, cBold, version, cReset, cDim, cReset)

	fmt.Fprint(w, header)
}
