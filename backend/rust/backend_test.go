//go:build rust

package rust

import (
	"errors"
	"testing"

	"github.com/gogpu/gg/backend"
)

func TestBackendRegistration(t *testing.T) {
	// Verify backend is registered
	if !backend.IsRegistered(backend.BackendRust) {
		t.Error("rust backend should be registered")
	}

	// Verify we can get the backend
	b := backend.Get(backend.BackendRust)
	if b == nil {
		t.Fatal("backend.Get(BackendRust) should not return nil")
	}

	// Verify name
	if b.Name() != backend.BackendRust {
		t.Errorf("Name() = %q, want %q", b.Name(), backend.BackendRust)
	}
}

func TestBackendNotInitialized(t *testing.T) {
	b := NewRustBackend()

	// Should not be initialized initially
	if b.IsInitialized() {
		t.Error("backend should not be initialized initially")
	}

	// Device and Queue should be nil
	if b.Device() != nil {
		t.Error("Device() should return nil before Init()")
	}
	if b.Queue() != nil {
		t.Error("Queue() should return nil before Init()")
	}

	// NewRenderer should return nil or handle gracefully
	renderer := b.NewRenderer(800, 600)
	if renderer != nil {
		t.Error("NewRenderer should return nil on uninitialized backend")
	}
}

func TestBackendInvalidDimensions(t *testing.T) {
	b := &RustBackend{initialized: true}

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 600},
		{"zero height", 800, 0},
		{"negative width", -1, 600},
		{"negative height", 800, -1},
		{"both zero", 0, 0},
		{"both negative", -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer := b.NewRenderer(tt.width, tt.height)
			if renderer != nil {
				t.Errorf("NewRenderer(%d, %d) should return nil", tt.width, tt.height)
			}
		})
	}
}

func TestRenderSceneErrors(t *testing.T) {
	b := NewRustBackend()

	// Should return error when not initialized
	err := b.RenderScene(nil, nil)
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("RenderScene on uninitialized backend should return ErrNotInitialized, got %v", err)
	}
}
