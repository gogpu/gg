package backend

import (
	"sync"
)

// BackendFactory creates a new backend instance.
type BackendFactory func() RenderBackend

// registry holds registered backends.
var (
	registryMu sync.RWMutex
	backends   = make(map[string]BackendFactory)
	// Priority order for backend selection (first available wins).
	// Rust > Native > Software (Rust is fastest, Software is fallback).
	backendPriority = []string{BackendRust, BackendNative, BackendSoftware}
)

// Register registers a backend factory with the given name.
// This is typically called from init() functions in backend packages.
// If a backend with the same name is already registered, it will be replaced.
func Register(name string, factory BackendFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	backends[name] = factory
}

// Unregister removes a backend from the registry.
// This is useful for testing.
func Unregister(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(backends, name)
}

// Available returns a list of registered backend names.
func Available() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	return names
}

// IsRegistered checks if a backend with the given name is registered.
func IsRegistered(name string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := backends[name]
	return ok
}

// Get returns a backend instance by name.
// Returns nil if the backend is not registered.
func Get(name string) RenderBackend {
	registryMu.RLock()
	defer registryMu.RUnlock()

	factory, ok := backends[name]
	if !ok {
		return nil
	}
	return factory()
}

// Default returns the best available backend based on priority.
// Priority order: wgpu > software
// Returns nil if no backends are registered.
func Default() RenderBackend {
	registryMu.RLock()
	defer registryMu.RUnlock()

	for _, name := range backendPriority {
		if factory, ok := backends[name]; ok {
			b := factory()
			if b != nil {
				return b
			}
		}
	}

	// Fallback: return first available
	for _, factory := range backends {
		if b := factory(); b != nil {
			return b
		}
	}

	return nil
}

// MustDefault returns the default backend or panics.
func MustDefault() RenderBackend {
	b := Default()
	if b == nil {
		panic("backend: no backend available")
	}
	return b
}

// InitDefault initializes the default backend based on availability.
// This is called automatically when using gg.NewContext() without
// explicit backend selection.
func InitDefault() (RenderBackend, error) {
	b := Default()
	if b == nil {
		return nil, ErrBackendNotAvailable
	}

	if err := b.Init(); err != nil {
		return nil, err
	}

	return b, nil
}
