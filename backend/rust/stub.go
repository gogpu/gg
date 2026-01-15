//go:build !rust

package rust

import "github.com/gogpu/gg/backend"

// init registers a nil-returning factory when rust tag is not set.
// This allows code to compile without the rust backend while still
// allowing backend.Get(backend.BackendRust) to return nil gracefully.
func init() {
	backend.Register(backend.BackendRust, func() backend.RenderBackend {
		return nil
	})
}
