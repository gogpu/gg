package recording

import (
	"fmt"
	"sort"
	"sync"
)

// BackendFactory is a function that creates a new backend instance.
// Factories are registered via Register() and called by NewBackend().
type BackendFactory func() Backend

// Registry state - protected by mutex for thread-safe access.
var (
	registryMu sync.RWMutex
	backends   = make(map[string]BackendFactory)
)

// Register registers a backend factory with the given name.
// This function is typically called from init() in backend packages,
// following the database/sql driver pattern:
//
//	func init() {
//	    recording.Register("pdf", func() recording.Backend {
//	        return NewPDFBackend()
//	    })
//	}
//
// Register panics if:
//   - factory is nil
//   - a backend with the same name is already registered
//
// This ensures that duplicate registrations are caught early during
// program initialization rather than silently overwriting backends.
func Register(name string, factory BackendFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if factory == nil {
		panic("recording: Register factory is nil")
	}
	if _, dup := backends[name]; dup {
		panic("recording: Register called twice for " + name)
	}
	backends[name] = factory
}

// Unregister removes a backend from the registry.
// This is primarily useful for testing to clean up between tests.
// If the backend is not registered, this is a no-op.
func Unregister(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(backends, name)
}

// NewBackend creates a new backend instance by name.
// The name must match a previously registered backend.
//
// Example:
//
//	import _ "github.com/gogpu/gg-pdf" // Register PDF backend
//
//	backend, err := recording.NewBackend("pdf")
//	if err != nil {
//	    // Handle error - backend not registered
//	}
//
// Returns an error if the backend is not registered.
// The error message includes a hint about forgotten imports.
func NewBackend(name string) (Backend, error) {
	registryMu.RLock()
	factory, ok := backends[name]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("recording: unknown backend %q (forgotten import?)", name)
	}
	return factory(), nil
}

// MustBackend creates a new backend instance by name, panicking on error.
// This is useful when backend availability is guaranteed.
//
// Example:
//
//	backend := recording.MustBackend("raster")
func MustBackend(name string) Backend {
	b, err := NewBackend(name)
	if err != nil {
		panic(err)
	}
	return b
}

// Backends returns a sorted list of registered backend names.
// The list is sorted alphabetically for consistent output.
func Backends() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsRegistered checks if a backend with the given name is registered.
func IsRegistered(name string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := backends[name]
	return ok
}

// Count returns the number of registered backends.
func Count() int {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return len(backends)
}
