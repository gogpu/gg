// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"errors"
	"sort"
	"sync"
)

// SurfaceFactory creates a new Surface with the given options.
// Implementations should validate options and return descriptive errors.
type SurfaceFactory func(opts Options) (Surface, error)

// RegistryEntry represents a registered surface backend.
type RegistryEntry struct {
	// Name is the unique identifier for this backend.
	Name string

	// Priority determines selection order (higher = preferred).
	// Standard priorities:
	//   - 100: GPU backends (Vulkan, Metal, D3D12)
	//   - 50: Hardware-accelerated software (WARP)
	//   - 10: Pure software backends
	Priority int

	// Factory creates surface instances.
	Factory SurfaceFactory

	// Available reports if the backend is available on this system.
	// This is called during registration and cached.
	Available func() bool
}

// globalRegistry is the default registry.
var globalRegistry = &Registry{}

// Registry manages registered surface backends.
//
// The registry enables third-party backends to register themselves
// without requiring changes to the core library (RFC #46).
//
// Example registration:
//
//	func init() {
//	    surface.Register("vulkan", 100, vulkanFactory, vulkanAvailable)
//	}
//
// Example usage:
//
//	s, err := surface.NewSurfaceByName("vulkan", 800, 600)
//	// or auto-select best available:
//	s, err := surface.NewSurface(800, 600)
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*RegistryEntry
}

// NewRegistry creates a new empty registry.
// Most code should use the global registry via Register and NewSurface.
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]*RegistryEntry),
	}
}

// Register adds a backend to the global registry.
//
// Parameters:
//   - name: unique identifier (e.g., "vulkan", "metal", "software")
//   - priority: selection priority (higher = preferred)
//   - factory: function to create surface instances
//   - available: function to check if backend is available
//
// If available is nil, the backend is assumed always available.
// Registering a name that already exists replaces the previous entry.
func Register(name string, priority int, factory SurfaceFactory, available func() bool) {
	globalRegistry.Register(name, priority, factory, available)
}

// Unregister removes a backend from the global registry.
func Unregister(name string) {
	globalRegistry.Unregister(name)
}

// List returns all registered backend names sorted by priority (highest first).
func List() []string {
	return globalRegistry.List()
}

// Available returns names of all available backends sorted by priority.
func Available() []string {
	return globalRegistry.Available()
}

// Get returns information about a specific backend.
func Get(name string) (*RegistryEntry, bool) {
	return globalRegistry.Get(name)
}

// NewSurface creates a surface using the best available backend.
// Returns an error if no backends are available.
func NewSurface(width, height int) (Surface, error) {
	return globalRegistry.NewSurface(Options{Width: width, Height: height})
}

// NewSurfaceWithOptions creates a surface using the best available backend.
func NewSurfaceWithOptions(opts Options) (Surface, error) {
	return globalRegistry.NewSurface(opts)
}

// NewSurfaceByName creates a surface using a specific named backend.
func NewSurfaceByName(name string, width, height int) (Surface, error) {
	return globalRegistry.NewSurfaceByName(name, Options{Width: width, Height: height})
}

// NewSurfaceByNameWithOptions creates a surface using a specific backend.
func NewSurfaceByNameWithOptions(name string, opts Options) (Surface, error) {
	return globalRegistry.NewSurfaceByName(name, opts)
}

// Register adds a backend to this registry.
func (r *Registry) Register(name string, priority int, factory SurfaceFactory, available func() bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entries == nil {
		r.entries = make(map[string]*RegistryEntry)
	}

	if available == nil {
		available = func() bool { return true }
	}

	r.entries[name] = &RegistryEntry{
		Name:      name,
		Priority:  priority,
		Factory:   factory,
		Available: available,
	}
}

// Unregister removes a backend from this registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.entries, name)
}

// List returns all registered backend names sorted by priority.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sortedNames(false)
}

// Available returns names of all available backends sorted by priority.
func (r *Registry) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sortedNames(true)
}

// Get returns information about a specific backend.
func (r *Registry) Get(name string) (*RegistryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.entries[name]
	if !ok {
		return nil, false
	}

	// Return a copy to prevent modification
	entryCopy := *entry
	return &entryCopy, true
}

// NewSurface creates a surface using the best available backend.
func (r *Registry) NewSurface(opts Options) (Surface, error) {
	r.mu.RLock()
	available := r.sortedNames(true)
	r.mu.RUnlock()

	if len(available) == 0 {
		return nil, ErrNoBackendAvailable
	}

	// Try each available backend in priority order
	var lastErr error
	for _, name := range available {
		s, err := r.NewSurfaceByName(name, opts)
		if err == nil {
			return s, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoBackendAvailable
}

// NewSurfaceByName creates a surface using a specific backend.
func (r *Registry) NewSurfaceByName(name string, opts Options) (Surface, error) {
	r.mu.RLock()
	entry, ok := r.entries[name]
	r.mu.RUnlock()

	if !ok {
		return nil, &BackendNotFoundError{Name: name}
	}

	if !entry.Available() {
		return nil, &BackendUnavailableError{Name: name}
	}

	return entry.Factory(opts)
}

// sortedNames returns backend names sorted by priority (highest first).
// If onlyAvailable is true, filters to available backends only.
// Must be called with lock held.
func (r *Registry) sortedNames(onlyAvailable bool) []string {
	if len(r.entries) == 0 {
		return nil
	}

	type entry struct {
		name     string
		priority int
	}

	entries := make([]entry, 0, len(r.entries))
	for name, e := range r.entries {
		if onlyAvailable && !e.Available() {
			continue
		}
		entries = append(entries, entry{name: name, priority: e.Priority})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// Errors.
var (
	// ErrNoBackendAvailable is returned when no surface backends are registered
	// or available on the current system.
	ErrNoBackendAvailable = errors.New("surface: no backend available")
)

// BackendNotFoundError indicates a named backend is not registered.
type BackendNotFoundError struct {
	Name string
}

func (e *BackendNotFoundError) Error() string {
	return "surface: backend not found: " + e.Name
}

// BackendUnavailableError indicates a backend exists but is not available.
type BackendUnavailableError struct {
	Name string
}

func (e *BackendUnavailableError) Error() string {
	return "surface: backend unavailable: " + e.Name
}

// init registers the built-in ImageSurface backend.
func init() {
	Register("image", 10, func(opts Options) (Surface, error) {
		return NewImageSurface(opts.Width, opts.Height), nil
	}, nil)
}
