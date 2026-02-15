package gg

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestNopHandler_Enabled(t *testing.T) {
	h := nopHandler{}
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if h.Enabled(context.Background(), level) {
			t.Errorf("nopHandler.Enabled(%v) = true, want false", level)
		}
	}
}

func TestNopHandler_Handle(t *testing.T) {
	h := nopHandler{}
	if err := h.Handle(context.Background(), slog.Record{}); err != nil {
		t.Errorf("nopHandler.Handle() = %v, want nil", err)
	}
}

func TestNopHandler_WithAttrs(t *testing.T) {
	h := nopHandler{}
	got := h.WithAttrs([]slog.Attr{slog.String("key", "val")})
	if _, ok := got.(nopHandler); !ok {
		t.Errorf("nopHandler.WithAttrs() returned %T, want nopHandler", got)
	}
}

func TestNopHandler_WithGroup(t *testing.T) {
	h := nopHandler{}
	got := h.WithGroup("group")
	if _, ok := got.(nopHandler); !ok {
		t.Errorf("nopHandler.WithGroup() returned %T, want nopHandler", got)
	}
}

func TestLoggerDefaultSilent(t *testing.T) {
	l := Logger()
	if l == nil {
		t.Fatal("Logger() returned nil")
	}
	// Default logger must be disabled at all levels.
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn} {
		if l.Enabled(context.Background(), level) {
			t.Errorf("default logger should not be enabled for %v", level)
		}
	}
}

func TestSetLogger(t *testing.T) {
	orig := Logger()
	t.Cleanup(func() { SetLogger(orig) })

	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	SetLogger(custom)

	got := Logger()
	if got != custom {
		t.Error("Logger() did not return the custom logger set via SetLogger")
	}

	// Verify output is captured.
	got.Info("test message", "key", "value")
	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("expected log output to contain 'test message', got: %s", buf.String())
	}
}

func TestSetLoggerNilRestoresSilent(t *testing.T) {
	orig := Logger()
	t.Cleanup(func() { SetLogger(orig) })

	// First set a real logger.
	SetLogger(slog.Default())

	// Then set nil to restore silence.
	SetLogger(nil)

	l := Logger()
	if l == nil {
		t.Fatal("SetLogger(nil) should set nop logger, not nil")
	}
	if l.Enabled(context.Background(), slog.LevelError) {
		t.Error("SetLogger(nil) should produce a disabled logger")
	}
}

func TestSetLoggerPropagatesToAccelerator(t *testing.T) {
	orig := Logger()
	t.Cleanup(func() {
		SetLogger(orig)
		resetAccelerator()
	})
	resetAccelerator()

	mock := &mockAccelerator{name: "logger-test"}
	accelMu.Lock()
	accel = mock
	accelMu.Unlock()

	custom := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	SetLogger(custom)

	if mock.logger != custom {
		t.Error("SetLogger did not propagate to accelerator via loggerSetter")
	}
}

func TestRegisterAcceleratorPropagatesCurrentLogger(t *testing.T) {
	orig := Logger()
	t.Cleanup(func() {
		SetLogger(orig)
		resetAccelerator()
	})
	resetAccelerator()

	// Set a custom logger before registration.
	custom := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	SetLogger(custom)

	// Register accelerator — it should receive the current logger.
	mock := &mockAccelerator{name: "propagation-test"}
	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator() = %v", err)
	}

	if mock.logger != custom {
		t.Error("RegisterAccelerator did not propagate current logger to accelerator")
	}
}

func TestLoggerConcurrentAccess(t *testing.T) {
	orig := Logger()
	t.Cleanup(func() { SetLogger(orig) })

	var wg sync.WaitGroup
	const goroutines = 100

	// Concurrent readers.
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := Logger()
			if l == nil {
				t.Error("Logger() returned nil during concurrent access")
			}
			// Exercise the logger — must not panic.
			l.Debug("concurrent read")
		}()
	}

	// Concurrent writers.
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			SetLogger(slog.Default())
			SetLogger(nil)
		}()
	}

	wg.Wait()
}

func BenchmarkLoggerLoad(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		l := Logger()
		_ = l
	}
}

func BenchmarkLoggerDisabledLog(b *testing.B) {
	// Benchmark the hot path: calling a log method on a disabled logger.
	l := Logger()
	b.ReportAllocs()
	for b.Loop() {
		l.Debug("message", "key", "value")
	}
}
