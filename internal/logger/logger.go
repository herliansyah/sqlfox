package logger

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config configures the Diagnostic Logger.
type Config struct {
	Level    string // "debug", "info", "warn", "error", "silent"
	Format   string // "text", "json"
	FilePath string // optional file path for log sink
	Silent   bool   // suppress console output
}

// Result holds the configured logger and any file closer.
type Result struct {
	Logger *slog.Logger
	Closer io.Closer
}

// LevelFromString parses string into slog.Level.
func LevelFromString(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	case "silent", "quiet":
		return slog.Level(999), true // Suppress all standard logs
	default:
		return slog.LevelInfo, false
	}
}

// Setup initializes the global and returned slog.Logger based on Config.
// ponytail: Uses pure Go standard library log/slog without external logging dependencies.
func Setup(cfg Config) (*Result, error) {
	lvl, _ := LevelFromString(cfg.Level)
	if cfg.Silent && cfg.FilePath == "" {
		lvl = slog.Level(999)
	}

	var fileWriter io.WriteCloser
	var writers []io.Writer

	if cfg.FilePath != "" {
		if dir := filepath.Dir(cfg.FilePath); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
		f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		fileWriter = f
		writers = append(writers, f)
	}

	// Console sink: if not silent, output to stderr
	if !cfg.Silent {
		writers = append(writers, os.Stderr)
	}

	var sink io.Writer
	if len(writers) == 0 || (cfg.Silent && cfg.FilePath == "") {
		sink = io.Discard
	} else if len(writers) == 1 {
		sink = writers[0]
	} else {
		sink = io.MultiWriter(writers...)
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "json") {
		handler = slog.NewJSONHandler(sink, opts)
	} else {
		handler = slog.NewTextHandler(sink, opts)
	}

	l := slog.New(handler)
	slog.SetDefault(l)

	return &Result{
		Logger: l,
		Closer: fileWriter,
	}, nil
}

// MaskSecret redacts sensitive strings like passwords or PINs.
func MaskSecret(s string) string {
	if s == "" {
		return ""
	}
	return "[REDACTED]"
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// HTTPMiddleware logs incoming HTTP requests with credential masking and static asset filtering.
func HTTPMiddleware(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Filter out static asset noise
			if strings.HasPrefix(path, "/assets/") || strings.HasSuffix(path, ".svg") ||
				strings.HasSuffix(path, ".ico") || strings.HasSuffix(path, ".png") ||
				strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			durMs := time.Since(start).Milliseconds()

			// Credential masking: sensitive endpoints do not log raw parameters
			isAuth := strings.HasPrefix(path, "/api/auth/")
			clientIP := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				clientIP = forwarded
			}

			level := slog.LevelDebug
			if sw.status >= 500 {
				level = slog.LevelError
			} else if sw.status >= 400 {
				level = slog.LevelWarn
			}

			if isAuth {
				l.Log(context.Background(), level, "http_request",
					"method", r.Method,
					"path", path,
					"status", sw.status,
					"duration_ms", durMs,
					"ip", clientIP,
					"auth", "[MASKED]",
				)
			} else {
				l.Log(context.Background(), level, "http_request",
					"method", r.Method,
					"path", path,
					"status", sw.status,
					"duration_ms", durMs,
					"ip", clientIP,
				)
			}
		})
	}
}
