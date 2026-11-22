// Package logging ports logging_utils.configure_logging on top of log/slog.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LevelCritical is Python's CRITICAL, above slog.LevelError.
const LevelCritical = slog.Level(12)

type handler struct {
	mu    *sync.Mutex
	out   io.Writer
	level slog.Level
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }
func (h *handler) WithAttrs([]slog.Attr) slog.Handler           { return h }
func (h *handler) WithGroup(string) slog.Handler                { return h }

// Handle writes "<asctime> <LEVEL> <message>" like the Python format minus the logger name.
func (h *handler) Handle(_ context.Context, r slog.Record) error {
	name := strings.ToUpper(r.Level.String())
	switch {
	case r.Level >= LevelCritical:
		name = "CRITICAL"
	case r.Level >= slog.LevelError:
		name = "ERROR"
	case r.Level >= slog.LevelWarn:
		name = "WARNING"
	}
	ts := r.Time.Format("2006-01-02 15:04:05") + fmt.Sprintf(",%03d", r.Time.Nanosecond()/int(time.Millisecond))
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprintf(h.out, "%s %s %s\n", ts, name, r.Message)
	return err
}

func parseLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	case "CRITICAL":
		return LevelCritical
	}
	return slog.LevelInfo
}

// Configure sends logs to stderr and, when file is not empty, to that file
// (parent directories are created, the file is appended to).
func Configure(level, file string) error {
	var out io.Writer = os.Stderr
	if file != "" {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		out = io.MultiWriter(os.Stderr, f)
	}
	slog.SetDefault(slog.New(&handler{mu: &sync.Mutex{}, out: out, level: parseLevel(level)}))
	return nil
}
