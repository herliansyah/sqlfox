package logger

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Event represents a structured diagnostic event emitted to live streams.
type Event struct {
	ID        int64          `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Module    string         `json:"module"`
	Message   string         `json:"message"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

// Broadcaster manages telemetry subscribers and a ring buffer of recent events.
// ponytail: Uses in-memory fixed-capacity ring buffer and standard channel fan-out with zero external dependencies.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	ringBuffer  []Event
	ringCap     int
	nextID      int64
}

var (
	defaultBroadcasterMu sync.RWMutex
	defaultBroadcaster   *Broadcaster
)

// NewBroadcaster creates a Broadcaster with a fixed ring buffer capacity.
func NewBroadcaster(capacity int) *Broadcaster {
	if capacity <= 0 {
		capacity = 100
	}
	return &Broadcaster{
		subscribers: make(map[chan Event]struct{}),
		ringBuffer:  make([]Event, 0, capacity),
		ringCap:     capacity,
	}
}

// DefaultBroadcaster returns the global default telemetry broadcaster.
func DefaultBroadcaster() *Broadcaster {
	defaultBroadcasterMu.RLock()
	defer defaultBroadcasterMu.RUnlock()
	if defaultBroadcaster == nil {
		defaultBroadcaster = NewBroadcaster(100)
	}
	return defaultBroadcaster
}

// SetDefaultBroadcaster sets the global default telemetry broadcaster.
func SetDefaultBroadcaster(b *Broadcaster) {
	defaultBroadcasterMu.Lock()
	defaultBroadcaster = b
	defaultBroadcasterMu.Unlock()
}

// Broadcast distributes an event to all subscribers and records it in the ring buffer.
func (b *Broadcaster) Broadcast(evt Event) {
	b.mu.Lock()
	b.nextID++
	evt.ID = b.nextID
	if len(b.ringBuffer) >= b.ringCap {
		copy(b.ringBuffer, b.ringBuffer[1:])
		b.ringBuffer[len(b.ringBuffer)-1] = evt
	} else {
		b.ringBuffer = append(b.ringBuffer, evt)
	}

	subs := make([]chan Event, 0, len(b.subscribers))
	for ch := range b.subscribers {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
			// Non-blocking drop on slow subscriber to protect daemon throughput
		}
	}
}

// Subscribe returns a receiving channel and an unsubscription cleanup function.
func (b *Broadcaster) Subscribe() (chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, 64)
	b.subscribers[ch] = struct{}{}
	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subscribers, ch)
	}
	return ch, unsub
}

// History returns a snapshot copy of the ring buffer.
func (b *Broadcaster) History() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make([]Event, len(b.ringBuffer))
	copy(res, b.ringBuffer)
	return res
}

type broadcastHandler struct {
	primary     slog.Handler
	broadcaster *Broadcaster
	attrs       []slog.Attr
	groups      []string
}

func (h *broadcastHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *broadcastHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.primary != nil && h.primary.Enabled(ctx, r.Level) {
		_ = h.primary.Handle(ctx, r)
	}

	if h.broadcaster == nil {
		return nil
	}

	attrs := make(map[string]any)
	module := "app"

	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.Any()
		if a.Key == "module" {
			module = a.Value.String()
		}
	}

	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		if a.Key == "module" {
			module = a.Value.String()
		}
		return true
	})

	if module == "app" {
		if _, ok := attrs["sql"]; ok {
			module = "query"
		} else if _, ok := attrs["path"]; ok {
			module = "http"
		} else if strings.HasPrefix(r.Message, "schema_") {
			module = "schema"
		} else if strings.HasPrefix(r.Message, "connection_") {
			module = "connection"
		} else if strings.HasPrefix(r.Message, "ai_") {
			module = "ai"
		} else if strings.HasPrefix(r.Message, "sync_") {
			module = "sync"
		}
	}

	lvlStr := r.Level.String()
	evt := Event{
		Timestamp: r.Time,
		Level:     lvlStr,
		Module:    module,
		Message:   r.Message,
		Attrs:     attrs,
	}

	h.broadcaster.Broadcast(evt)
	return nil
}

func (h *broadcastHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var nextPrimary slog.Handler
	if h.primary != nil {
		nextPrimary = h.primary.WithAttrs(attrs)
	}
	return &broadcastHandler{
		primary:     nextPrimary,
		broadcaster: h.broadcaster,
		attrs:       append(append([]slog.Attr(nil), h.attrs...), attrs...),
		groups:      h.groups,
	}
}

func (h *broadcastHandler) WithGroup(name string) slog.Handler {
	var nextPrimary slog.Handler
	if h.primary != nil {
		nextPrimary = h.primary.WithGroup(name)
	}
	return &broadcastHandler{
		primary:     nextPrimary,
		broadcaster: h.broadcaster,
		attrs:       h.attrs,
		groups:      append(append([]string(nil), h.groups...), name),
	}
}

// Config configures the Diagnostic Logger.
type Config struct {
	Level    string // "debug", "info", "warn", "error", "silent"
	Format   string // "text", "json"
	FilePath string // optional file path for log sink
	Silent   bool   // suppress console output
}

// Result holds the configured logger and any file closer.
type Result struct {
	Logger      *slog.Logger
	Closer      io.Closer
	Broadcaster *Broadcaster
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

	broadcaster := NewBroadcaster(100)
	SetDefaultBroadcaster(broadcaster)

	bHandler := &broadcastHandler{
		primary:     handler,
		broadcaster: broadcaster,
	}

	l := slog.New(bHandler)
	slog.SetDefault(l)

	return &Result{
		Logger:      l,
		Closer:      fileWriter,
		Broadcaster: broadcaster,
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

			// Filter out static asset noise and streaming SSE logs endpoint
			if path == "/api/logs/stream" || strings.HasPrefix(path, "/assets/") || strings.HasSuffix(path, ".svg") ||
				strings.HasSuffix(path, ".ico") || strings.HasSuffix(path, ".png") ||
				strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			durMs := time.Since(start).Milliseconds()

			// Filter out repetitive background polling noise on 200 OK to keep activity log focused on real actions
			if sw.status < 400 && (path == "/api/notes/reminders/due" || strings.HasSuffix(path, "/flight-recorder/status") || strings.HasSuffix(path, "/processlist")) {
				return
			}

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
					"module", "http",
					"method", r.Method,
					"path", path,
					"status", sw.status,
					"duration_ms", durMs,
					"ip", clientIP,
					"auth", "[MASKED]",
				)
			} else {
				l.Log(context.Background(), level, "http_request",
					"module", "http",
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
