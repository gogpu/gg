package recording

import (
	"image"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// mockBackend is a minimal backend implementation for testing.
type mockBackend struct {
	name       string
	beginCalls int
	endCalls   int
	width      int
	height     int
}

func newMockBackend(name string) *mockBackend {
	return &mockBackend{name: name}
}

func (b *mockBackend) Begin(width, height int) error {
	b.beginCalls++
	b.width = width
	b.height = height
	return nil
}

func (b *mockBackend) End() error {
	b.endCalls++
	return nil
}

func (b *mockBackend) Save()    {}
func (b *mockBackend) Restore() {}

func (b *mockBackend) SetTransform(_ Matrix)                                 {}
func (b *mockBackend) SetClip(_ *gg.Path, _ FillRule)                        {}
func (b *mockBackend) ClearClip()                                            {}
func (b *mockBackend) FillPath(_ *gg.Path, _ Brush, _ FillRule)              {}
func (b *mockBackend) StrokePath(_ *gg.Path, _ Brush, _ Stroke)              {}
func (b *mockBackend) FillRect(_ Rect, _ Brush)                              {}
func (b *mockBackend) DrawImage(_ image.Image, _, _ Rect, _ ImageOptions)    {}
func (b *mockBackend) DrawText(_ string, _, _ float64, _ text.Face, _ Brush) {}

// resetRegistry clears all registered backends for test isolation.
func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	backends = make(map[string]BackendFactory)
}

func TestRegisterAndNewBackend(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	// Register a test backend
	Register("test", func() Backend {
		return newMockBackend("test")
	})

	// Create instance
	backend, err := NewBackend("test")
	if err != nil {
		t.Fatalf("NewBackend failed: %v", err)
	}

	// Verify it's the right type
	mock, ok := backend.(*mockBackend)
	if !ok {
		t.Fatal("backend is not a mockBackend")
	}
	if mock.name != "test" {
		t.Errorf("got name %q, want %q", mock.name, "test")
	}
}

func TestNewBackendUnknown(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	_, err := NewBackend("unknown")
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}

func TestRegisterNilFactory(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil factory")
		}
	}()

	Register("nil", nil)
}

func TestRegisterDuplicate(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	factory := func() Backend { return newMockBackend("dup") }

	Register("dup", factory)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate registration")
		}
	}()

	Register("dup", factory)
}

func TestUnregister(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("temp", func() Backend {
		return newMockBackend("temp")
	})

	if !IsRegistered("temp") {
		t.Error("backend should be registered")
	}

	Unregister("temp")

	if IsRegistered("temp") {
		t.Error("backend should not be registered after Unregister")
	}

	// Unregister non-existent should not panic
	Unregister("nonexistent")
}

func TestBackends(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	// Register in non-alphabetical order
	Register("charlie", func() Backend { return newMockBackend("c") })
	Register("alpha", func() Backend { return newMockBackend("a") })
	Register("bravo", func() Backend { return newMockBackend("b") })

	names := Backends()

	if len(names) != 3 {
		t.Fatalf("expected 3 backends, got %d", len(names))
	}

	// Verify sorted order
	expected := []string{"alpha", "bravo", "charlie"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestIsRegistered(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	if IsRegistered("notregistered") {
		t.Error("should not be registered")
	}

	Register("registered", func() Backend {
		return newMockBackend("r")
	})

	if !IsRegistered("registered") {
		t.Error("should be registered")
	}
}

func TestCount(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	if Count() != 0 {
		t.Errorf("expected count 0, got %d", Count())
	}

	Register("one", func() Backend { return newMockBackend("1") })
	if Count() != 1 {
		t.Errorf("expected count 1, got %d", Count())
	}

	Register("two", func() Backend { return newMockBackend("2") })
	if Count() != 2 {
		t.Errorf("expected count 2, got %d", Count())
	}
}

func TestMustBackend(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("must", func() Backend {
		return newMockBackend("must")
	})

	// Should not panic
	backend := MustBackend("must")
	if backend == nil {
		t.Error("expected non-nil backend")
	}
}

func TestMustBackendPanic(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown backend")
		}
	}()

	_ = MustBackend("unknown")
}

func TestBackendLifecycle(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("lifecycle", func() Backend {
		return newMockBackend("lifecycle")
	})

	backend, err := NewBackend("lifecycle")
	if err != nil {
		t.Fatalf("NewBackend failed: %v", err)
	}

	mock := backend.(*mockBackend)

	// Test Begin
	if err := backend.Begin(800, 600); err != nil {
		t.Errorf("Begin failed: %v", err)
	}
	if mock.beginCalls != 1 {
		t.Errorf("expected 1 Begin call, got %d", mock.beginCalls)
	}
	if mock.width != 800 || mock.height != 600 {
		t.Errorf("got dimensions %dx%d, want 800x600", mock.width, mock.height)
	}

	// Test End
	if err := backend.End(); err != nil {
		t.Errorf("End failed: %v", err)
	}
	if mock.endCalls != 1 {
		t.Errorf("expected 1 End call, got %d", mock.endCalls)
	}
}

func TestConcurrentRegistration(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	// This test verifies thread-safety of the registry.
	// Multiple goroutines should be able to register and query safely.

	done := make(chan bool)

	// Goroutine 1: Register backends
	go func() {
		for i := 0; i < 100; i++ {
			name := "concurrent" + string(rune('A'+i%26)) + string(rune('0'+i/26))
			// Wrap in recovery to handle panics from duplicate names
			func() {
				defer func() { _ = recover() }()
				Register(name, func() Backend { return newMockBackend(name) })
			}()
		}
		done <- true
	}()

	// Goroutine 2: Query backends
	go func() {
		for i := 0; i < 100; i++ {
			_ = Backends()
			_ = Count()
			_ = IsRegistered("nonexistent")
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}
