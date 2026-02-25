// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"errors"
	"testing"
)

// TestRegistryRegister tests backend registration.
func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()

	factory := func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}

	r.Register("test", 50, factory, nil)

	entry, ok := r.Get("test")
	if !ok {
		t.Fatal("registered backend not found")
	}

	if entry.Name != "test" {
		t.Errorf("Name = %s, want test", entry.Name)
	}
	if entry.Priority != 50 {
		t.Errorf("Priority = %d, want 50", entry.Priority)
	}
	if !entry.Available() {
		t.Error("backend should be available (nil Available func)")
	}
}

// TestRegistryUnregister tests backend removal.
func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()

	r.Register("temp", 10, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	_, ok := r.Get("temp")
	if !ok {
		t.Fatal("backend should exist before unregister")
	}

	r.Unregister("temp")

	_, ok = r.Get("temp")
	if ok {
		t.Error("backend should not exist after unregister")
	}
}

// TestRegistryList tests listing backends.
func TestRegistryList(t *testing.T) {
	r := NewRegistry()

	r.Register("low", 10, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	r.Register("high", 100, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	r.Register("mid", 50, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	list := r.List()

	if len(list) != 3 {
		t.Fatalf("expected 3 backends, got %d", len(list))
	}

	// Should be sorted by priority (highest first)
	if list[0] != "high" {
		t.Errorf("first should be high (priority 100), got %s", list[0])
	}
	if list[1] != "mid" {
		t.Errorf("second should be mid (priority 50), got %s", list[1])
	}
	if list[2] != "low" {
		t.Errorf("third should be low (priority 10), got %s", list[2])
	}
}

// TestRegistryAvailable tests filtering by availability.
func TestRegistryAvailable(t *testing.T) {
	r := NewRegistry()

	r.Register("available", 100, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, func() bool { return true })

	r.Register("unavailable", 200, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, func() bool { return false })

	available := r.Available()

	if len(available) != 1 {
		t.Fatalf("expected 1 available backend, got %d", len(available))
	}

	if available[0] != "available" {
		t.Errorf("expected 'available', got %s", available[0])
	}
}

// TestRegistryNewSurface tests creating surfaces via registry.
func TestRegistryNewSurface(t *testing.T) {
	r := NewRegistry()

	r.Register("test", 50, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	s, err := r.NewSurface(Options{Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("NewSurface failed: %v", err)
	}
	defer s.Close()

	if s.Width() != 100 || s.Height() != 100 {
		t.Errorf("size = %dx%d, want 100x100", s.Width(), s.Height())
	}
}

// TestRegistryNewSurfaceByName tests creating named surfaces.
func TestRegistryNewSurfaceByName(t *testing.T) {
	r := NewRegistry()

	r.Register("specific", 50, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	s, err := r.NewSurfaceByName("specific", Options{Width: 50, Height: 50})
	if err != nil {
		t.Fatalf("NewSurfaceByName failed: %v", err)
	}
	defer s.Close()

	if s.Width() != 50 {
		t.Errorf("Width = %d, want 50", s.Width())
	}
}

// TestRegistryNewSurfaceByNameNotFound tests error for unknown backend.
func TestRegistryNewSurfaceByNameNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.NewSurfaceByName("nonexistent", Options{Width: 100, Height: 100})
	if err == nil {
		t.Fatal("expected error for nonexistent backend")
	}

	var notFound *BackendNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("expected BackendNotFoundError, got %T", err)
	}

	if notFound.Name != "nonexistent" {
		t.Errorf("error name = %s, want nonexistent", notFound.Name)
	}
}

// TestRegistryNewSurfaceByNameUnavailable tests error for unavailable backend.
func TestRegistryNewSurfaceByNameUnavailable(t *testing.T) {
	r := NewRegistry()

	r.Register("unavailable", 50, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, func() bool { return false })

	_, err := r.NewSurfaceByName("unavailable", Options{Width: 100, Height: 100})
	if err == nil {
		t.Fatal("expected error for unavailable backend")
	}

	var unavailable *BackendUnavailableError
	if !errors.As(err, &unavailable) {
		t.Errorf("expected BackendUnavailableError, got %T", err)
	}
}

// TestRegistryNoBackend tests error when no backends available.
func TestRegistryNoBackend(t *testing.T) {
	r := NewRegistry()

	_, err := r.NewSurface(Options{Width: 100, Height: 100})
	if err == nil {
		t.Fatal("expected error with no backends")
	}

	if !errors.Is(err, ErrNoBackendAvailable) {
		t.Errorf("expected ErrNoBackendAvailable, got %v", err)
	}
}

// TestRegistryFactoryError tests handling of factory errors.
func TestRegistryFactoryError(t *testing.T) {
	r := NewRegistry()

	expectedErr := errors.New("creation failed")
	r.Register("failing", 50, func(opts Options) (Surface, error) {
		return nil, expectedErr
	}, nil)

	_, err := r.NewSurfaceByName("failing", Options{Width: 100, Height: 100})
	if err == nil {
		t.Fatal("expected error from factory")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected factory error, got %v", err)
	}
}

// TestRegistryPrioritySelection tests that highest priority is selected.
func TestRegistryPrioritySelection(t *testing.T) {
	r := NewRegistry()

	var selected string

	r.Register("low", 10, func(opts Options) (Surface, error) {
		selected = "low"
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	r.Register("high", 100, func(opts Options) (Surface, error) {
		selected = "high"
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	s, err := r.NewSurface(Options{Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("NewSurface failed: %v", err)
	}
	defer s.Close()

	if selected != "high" {
		t.Errorf("selected = %s, want high (highest priority)", selected)
	}
}

// TestRegistryOverwrite tests that re-registering overwrites.
func TestRegistryOverwrite(t *testing.T) {
	r := NewRegistry()

	r.Register("test", 10, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	r.Register("test", 50, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)

	entry, _ := r.Get("test")
	if entry.Priority != 50 {
		t.Errorf("Priority = %d, want 50 (should be overwritten)", entry.Priority)
	}
}

// TestGlobalRegistry tests the global registry functions.
func TestGlobalRegistry(t *testing.T) {
	// The global registry should have "image" registered from init()
	available := Available()

	found := false
	for _, name := range available {
		if name == "image" {
			found = true
			break
		}
	}

	if !found {
		t.Error("'image' backend should be in global registry")
	}

	// Test global NewSurface
	s, err := NewSurface(100, 100)
	if err != nil {
		t.Fatalf("global NewSurface failed: %v", err)
	}
	defer s.Close()

	if s.Width() != 100 {
		t.Errorf("Width = %d, want 100", s.Width())
	}
}

// TestBackendNotFoundError tests error message formatting.
func TestBackendNotFoundError(t *testing.T) {
	err := &BackendNotFoundError{Name: "vulkan"}
	msg := err.Error()

	if msg != "surface: backend not found: vulkan" {
		t.Errorf("error message = %q, unexpected format", msg)
	}
}

// TestBackendUnavailableError tests error message formatting.
func TestBackendUnavailableError(t *testing.T) {
	err := &BackendUnavailableError{Name: "metal"}
	msg := err.Error()

	if msg != "surface: backend unavailable: metal" {
		t.Errorf("error message = %q, unexpected format", msg)
	}
}
