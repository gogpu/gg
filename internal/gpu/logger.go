//go:build !nogpu

package gpu

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// nopHandler silently discards all log records.
type nopHandler struct{}

func (nopHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (nopHandler) Handle(context.Context, slog.Record) error { return nil }
func (nopHandler) WithAttrs([]slog.Attr) slog.Handler        { return nopHandler{} }
func (nopHandler) WithGroup(string) slog.Handler             { return nopHandler{} }

// loggerPtr stores the active logger. Accessed atomically for thread safety.
var loggerPtr atomic.Pointer[slog.Logger]

func init() {
	l := slog.New(nopHandler{})
	loggerPtr.Store(l)
}

// slogger returns the current package logger.
// All logging in internal/gpu goes through this function.
func slogger() *slog.Logger { return loggerPtr.Load() }

// setLogger updates the package-level logger.
// Called from SDFAccelerator.SetLogger when gg.SetLogger propagates.
func setLogger(l *slog.Logger) {
	if l == nil {
		l = slog.New(nopHandler{})
	}
	loggerPtr.Store(l)
}
