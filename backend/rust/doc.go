// Package rust provides a GPU-accelerated rendering backend using go-webgpu/webgpu.
//
// This backend leverages the wgpu-native Rust WebGPU implementation via FFI bindings.
// It provides hardware-accelerated 2D graphics rendering using Vulkan, Metal, or DX12
// backends depending on the platform.
//
// # Architecture Overview
//
// The rust backend uses go-webgpu/webgpu for direct FFI access to wgpu-native:
//
//	Scene Commands -> Decoder -> RustBackend -> wgpu-native (Rust) -> Vulkan/Metal/DX12
//
// Key components:
//
//   - RustBackend: Main entry point implementing backend.RenderBackend
//   - GPU resources: Instance, Adapter, Device, Queue from wgpu-native
//
// # Registration and Selection
//
// The rust backend is automatically registered when this package is imported with
// the "rust" build tag:
//
//	// Build with: go build -tags rust
//	import _ "github.com/gogpu/gg/backend/rust"
//
// The backend will be preferred over native and software backends when available.
// Priority order: rust > native > software
//
// # Basic Usage
//
// Automatic backend selection (recommended):
//
//	b := backend.Default()  // Returns rust if available, otherwise native/software
//	if err := b.Init(); err != nil {
//	    log.Fatal(err)
//	}
//	defer b.Close()
//
// Explicit rust backend selection:
//
//	b := backend.Get(backend.BackendRust)
//	if b == nil {
//	    log.Fatal("rust backend not available")
//	}
//	if err := b.Init(); err != nil {
//	    log.Fatal(err)
//	}
//	defer b.Close()
//
// # Build Tags
//
// This package requires the "rust" build tag:
//
//	go build -tags rust ./...
//
// Without the tag, a stub implementation is compiled that returns nil from the factory.
//
// # Dependencies
//
// This backend requires wgpu-native library:
//   - Windows: wgpu_native.dll
//   - Linux: libwgpu_native.so
//   - macOS: libwgpu_native.dylib
//
// Download from: https://github.com/gfx-rs/wgpu-native/releases
//
// # Requirements
//
//   - Go 1.25+ (for generic features)
//   - go-webgpu/webgpu module (github.com/go-webgpu/webgpu)
//   - wgpu-native library in PATH or LD_LIBRARY_PATH
//   - A GPU that supports Vulkan, Metal, or DX12
//
// # Thread Safety
//
// RustBackend is safe for concurrent use from multiple goroutines.
// Internal synchronization is handled via mutexes.
//
// # Error Handling
//
// Common errors returned by this package:
//
//   - ErrNotInitialized: Backend must be initialized before use
//   - ErrNoGPU: No compatible GPU found
//   - ErrLibraryNotFound: wgpu-native library not found
//   - ErrDeviceLost: GPU device was lost (requires re-initialization)
//   - ErrNilTarget: Target pixmap is nil
//   - ErrNilScene: Scene is nil
//
// # Related Packages
//
//   - github.com/gogpu/gg: Core 2D graphics library
//   - github.com/gogpu/gg/scene: Scene graph and retained mode API
//   - github.com/gogpu/gg/backend: Backend interface and registry
//   - github.com/go-webgpu/webgpu: Zero-CGO WebGPU bindings
package rust
