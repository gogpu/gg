package gg

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// nopHandler is a slog.Handler that silently discards all log records.
// The Enabled method returns false so the caller skips message formatting
// entirely, making disabled logging effectively zero-cost.
type nopHandler struct{}

func (nopHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (nopHandler) Handle(context.Context, slog.Record) error { return nil }
func (nopHandler) WithAttrs([]slog.Attr) slog.Handler        { return nopHandler{} }
func (nopHandler) WithGroup(string) slog.Handler             { return nopHandler{} }

// newNopLogger creates a logger that silently discards all output.
func newNopLogger() *slog.Logger { return slog.New(nopHandler{}) }

// loggerPtr stores the active logger. Accessed atomically so that
// SetLogger can be called concurrently with logging from any goroutine.
var loggerPtr atomic.Pointer[slog.Logger]

func init() {
	l := newNopLogger()
	loggerPtr.Store(l)
}

// SetLogger configures the logger for gg and all its sub-packages.
// By default, gg produces no log output. Call SetLogger to enable logging.
//
// SetLogger is safe for concurrent use: it stores the new logger atomically.
// Pass nil to disable logging (restore default silent behavior).
//
// Log levels used by gg:
//   - [slog.LevelDebug]: internal diagnostics (GPU pipeline state, buffer sizes)
//   - [slog.LevelInfo]: important lifecycle events (GPU adapter selected)
//   - [slog.LevelWarn]: non-fatal issues (CPU fallback, resource release errors)
//
// Example:
//
//	// Enable info-level logging to stderr:
//	gg.SetLogger(slog.Default())
//
//	// Enable debug-level logging for full diagnostics:
//	gg.SetLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	})))
func SetLogger(l *slog.Logger) {
	if l == nil {
		l = newNopLogger()
	}
	loggerPtr.Store(l)

	// Propagate to GPU accelerator if it supports logging.
	accelMu.RLock()
	a := accel
	accelMu.RUnlock()
	if a != nil {
		propagateLogger(a, l)
	}
}

// Logger returns the current logger used by gg.
// Sub-packages (gpu/, integration/ggcanvas/) call this to share the same
// logger configuration without introducing import cycles.
//
// Logger is safe for concurrent use.
func Logger() *slog.Logger {
	return loggerPtr.Load()
}

// loggerSetter is implemented by accelerators that accept a logger.
type loggerSetter interface {
	SetLogger(*slog.Logger)
}

// propagateLogger passes the logger to an accelerator if it implements
// the loggerSetter interface. Called from both SetLogger and
// RegisterAccelerator to ensure the accelerator always has the current logger.
func propagateLogger(a GPUAccelerator, l *slog.Logger) {
	if ls, ok := a.(loggerSetter); ok {
		ls.SetLogger(l)
	}
}
