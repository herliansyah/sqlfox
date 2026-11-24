package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxFileBytes caps each diagnostic log file at 5 MB before rotation.
	DefaultMaxFileBytes int64 = 5 * 1024 * 1024
	// DefaultMaxBackups retains 3 rotated backups (up to ~20 MB total footprint).
	DefaultMaxBackups = 3
)

var (
	activeLogFileMu sync.RWMutex
	activeLogFile   string
)

// ActiveLogFile returns the path of the currently configured persistent log file, if any.
func ActiveLogFile() string {
	activeLogFileMu.RLock()
	defer activeLogFileMu.RUnlock()
	return activeLogFile
}

// SetActiveLogFile records the active persistent log file path.
func SetActiveLogFile(path string) {
	activeLogFileMu.Lock()
	activeLogFile = path
	activeLogFileMu.Unlock()
}

// RotatingFileWriter is a thread-safe size-bounded log file writer using pure Go stdlib.
// ponytail: Replaces external log-rotation dependencies (e.g. lumberjack) with a ~60-line stdlib writer.
type RotatingFileWriter struct {
	mu         sync.Mutex
	path       string
	maxBytes   int64
	maxBackups int
	file       *os.File
	size       int64
}

// NewRotatingFileWriter opens or creates path and rotates it when size exceeds maxBytes.
func NewRotatingFileWriter(path string, maxBytes int64, maxBackups int) (*RotatingFileWriter, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFileBytes
	}
	if maxBackups < 0 {
		maxBackups = 0
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	var size int64
	if info, statErr := f.Stat(); statErr == nil {
		size = info.Size()
	}
	return &RotatingFileWriter{
		path:       path,
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
		file:       f,
		size:       size,
	}, nil
}

func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *RotatingFileWriter) rotateLocked() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.maxBackups > 0 {
		oldest := fmt.Sprintf("%s.%d", w.path, w.maxBackups)
		_ = os.Remove(oldest)
		for i := w.maxBackups - 1; i >= 1; i-- {
			src := fmt.Sprintf("%s.%d", w.path, i)
			dst := fmt.Sprintf("%s.%d", w.path, i+1)
			if _, err := os.Stat(src); err == nil {
				_ = os.Rename(src, dst)
			}
		}
		_ = os.Rename(w.path, w.path+".1")
	} else {
		_ = os.Remove(w.path)
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	w.file = f
	w.size = 0
	return nil
}

// LogTailResult holds persistent log lines and file metadata for diagnostic inspection.
type LogTailResult struct {
	Path         string   `json:"path"`
	SizeBytes    int64    `json:"size_bytes"`
	RotatedCount int      `json:"rotated_count"`
	Lines        []string `json:"lines"`
}

// ReadLogTail reads up to maxLines from the tail of filePath (bounded by maxBytes),
// spilling back into filePath.1 when the active file has fewer than maxLines lines.
func ReadLogTail(filePath string, maxBytes int64, maxLines int) (*LogTailResult, error) {
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	if maxLines <= 0 {
		maxLines = 500
	}
	res := &LogTailResult{
		Path:  filePath,
		Lines: []string{},
	}
	if filePath == "" {
		return res, nil
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return nil, err
	}
	res.SizeBytes = info.Size()
	for i := 1; i <= DefaultMaxBackups; i++ {
		if _, rErr := os.Stat(fmt.Sprintf("%s.%d", filePath, i)); rErr == nil {
			res.RotatedCount++
		}
	}

	lines, err := readFileTailLines(filePath, maxBytes, maxLines)
	if err != nil {
		return nil, err
	}
	if len(lines) < maxLines {
		backupPath := filePath + ".1"
		if _, bErr := os.Stat(backupPath); bErr == nil {
			need := maxLines - len(lines)
			if prevLines, pErr := readFileTailLines(backupPath, maxBytes, need); pErr == nil && len(prevLines) > 0 {
				lines = append(prevLines, lines...)
			}
		}
	}
	res.Lines = lines
	return res, nil
}

func readFileTailLines(path string, maxBytes int64, maxLines int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}
	offset := int64(0)
	if size > maxBytes {
		offset = size - maxBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
			data = data[idx+1:]
		}
	}
	rawLines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	var out []string
	for _, l := range rawLines {
		l = strings.TrimRight(l, "\r")
		if l != "" {
			out = append(out, l)
		}
	}
	if len(out) > maxLines {
		out = out[len(out)-maxLines:]
	}
	return out, nil
}

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
	if defaultBroadcaster != nil {
		defer defaultBroadcasterMu.RUnlock()
		return defaultBroadcaster
	}
	defaultBroadcasterMu.RUnlock()

	defaultBroadcasterMu.Lock()
	defer defaultBroadcasterMu.Unlock()
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
	if h.primary != nil {
		return h.primary.Enabled(ctx, level)
	}
	return level >= slog.LevelInfo
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
	Level        string // "debug", "info", "warn", "error", "silent"
	Format       string // "text", "json"
	FilePath     string // optional file path for log sink
	Silent       bool   // suppress console output
	MaxSizeBytes int64  // optional max bytes per log file before rotation (default 5MB)
	MaxBackups   int    // optional rotated backup files to retain (default 3; -1 = 0 backups)
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
		maxBytes := cfg.MaxSizeBytes
		if maxBytes <= 0 {
			maxBytes = DefaultMaxFileBytes
		}
		maxBackups := cfg.MaxBackups
		if maxBackups == 0 {
			maxBackups = DefaultMaxBackups
		} else if maxBackups < 0 {
			maxBackups = 0
		}
		rw, err := NewRotatingFileWriter(cfg.FilePath, maxBytes, maxBackups)
		if err != nil {
			return nil, err
		}
		SetActiveLogFile(cfg.FilePath)
		fileWriter = rw
		writers = append(writers, rw)
	} else {
		SetActiveLogFile("")
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

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// HTTPMiddleware logs incoming HTTP requests with credential masking and static asset filtering.
func HTTPMiddleware(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Filter out non-API noise (static files, external probes like /ws) and streaming SSE telemetry endpoint
			if !strings.HasPrefix(path, "/api/") || path == "/api/logs/stream" {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			durMs := time.Since(start).Milliseconds()

			// Filter out repetitive background polling noise (reminders, pings, processlist, flight recorder status)
			if (sw.status < 400 || sw.status == http.StatusUnauthorized) && (path == "/api/notes/reminders/due" || strings.HasSuffix(path, "/ping") || strings.HasSuffix(path, "/flight-recorder/status") || strings.HasSuffix(path, "/processlist")) {
				return
			}

			// Credential masking: sensitive endpoints do not log raw parameters.
			// Never trust raw X-Forwarded-For blindly in logs (spoofable when not behind a trusted proxy).
			isAuth := strings.HasPrefix(path, "/api/auth/")
			clientIP := r.RemoteAddr

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
