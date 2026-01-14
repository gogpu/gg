package gogpu

import (
	"github.com/gogpu/gg/backend"
)

// init registers the gogpu backend on package import.
// This enables automatic backend selection when using backend.Default().
//
// To use the gogpu backend, import this package:
//
//	import _ "github.com/gogpu/gg/backend/gogpu"
//
// The gogpu backend requires a GPU backend to be registered with gogpu.
// Import one of the following to enable GPU support:
//
//	import _ "github.com/gogpu/gogpu/gpu/backend/rust"   // Rust (wgpu-native)
//	import _ "github.com/gogpu/gogpu/gpu/backend/native" // Pure Go
func init() {
	backend.Register(BackendGoGPU, func() backend.RenderBackend {
		return &Backend{}
	})
}
