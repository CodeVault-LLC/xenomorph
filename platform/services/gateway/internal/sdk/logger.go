// Package sdk provides shared infrastructure for the gateway service.
//
// This package owns the structured logging initialization and trace ID
// propagation used across all gateway components. It does not own business
// logic or protocol handling.
package sdk

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// contextKey is an unexported type for context value keys to prevent collisions.
type contextKey string

const (
	// traceIDKey is the context key that carries the request trace identifier.
	traceIDKey contextKey = "trace_id"

	logDirPerm  os.FileMode = 0750
	logFilePerm os.FileMode = 0640
)

// InitLogger configures the global slog default logger with JSON-structured
// output. Output is written to stdout and, when filePath is non-empty, to an
// append-only log file on disk.
//
// The log directory is created with 0750 permissions. The log file is created
// or appended to with 0640 permissions. This function does not manage log
// rotation; operators must configure external rotation (e.g. logrotate).
//
// The configured handler automatically injects the "trace_id" attribute from
// context.Context into every log record. This enables request-scoped tracing
// without manual attribute plumbing at call sites.
//
// Call InitLogger exactly once during process startup before any component
// performs logging. A second call silently replaces the default handler.
func InitLogger(filePath string) error {
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	if filePath != "" {
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, logDirPerm); err != nil {
			return err
		}

		f, err := os.OpenFile(filepath.Clean(filePath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFilePerm)
		if err != nil {
			return err
		}

		writers = append(writers, f)
	}

	multi := io.MultiWriter(writers...)
	handler := slog.NewJSONHandler(multi, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	slog.SetDefault(slog.New(&contextHandler{handler: handler}))

	return nil
}

// WithTraceID returns a derived context that carries the given trace
// identifier. The trace ID is automatically injected into every log record
// emitted through slog.InfoContext, slog.ErrorContext, etc.
//
// The trace ID should be sourced from the incoming request's X-Trace-ID
// header when one exists, or generated as a new UUID when absent.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext returns the trace identifier stored in ctx. Returns an
// empty string when ctx is nil or no trace identifier is present.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}

	return ""
}

// contextHandler is a slog.Handler wrapper that automatically attaches the
// trace_id attribute from context.Context to every log record.
type contextHandler struct {
	handler slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle injects the trace_id from ctx into the record before delegating to
// the underlying handler. This ensures every log entry produced via
// slog.InfoContext or similar carries the request-scoped trace identifier
// without requiring callers to pass it explicitly.
func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		r.AddAttrs(slog.String("trace_id", traceID))
	}

	return h.handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{handler: h.handler.WithGroup(name)}
}
