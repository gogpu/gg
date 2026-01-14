// Package gogpu provides a GPU-accelerated rendering backend for gg
// using the gogpu/gogpu framework.
//
// This backend uses gogpu's gpu.Backend interface, which supports both
// Rust (wgpu-native) and Pure Go (gogpu/wgpu) implementations. Users can
// select the underlying GPU backend by importing the appropriate package:
//
//	import _ "github.com/gogpu/gogpu/gpu/backend/rust"   // Rust backend
//	import _ "github.com/gogpu/gogpu/gpu/backend/native" // Pure Go backend
package gogpu

import "errors"

// Package errors for gogpu backend.
var (
	// ErrNotInitialized is returned when operations are called before Init.
	ErrNotInitialized = errors.New("gogpu: backend not initialized")

	// ErrNoGPUBackend is returned when no GPU backend is available.
	ErrNoGPUBackend = errors.New("gogpu: no GPU backend available")

	// ErrDeviceCreationFailed is returned when GPU device creation fails.
	ErrDeviceCreationFailed = errors.New("gogpu: device creation failed")

	// ErrNotImplemented is returned for stub operations not yet implemented.
	ErrNotImplemented = errors.New("gogpu: operation not implemented")

	// ErrInvalidDimensions is returned when width or height is invalid.
	ErrInvalidDimensions = errors.New("gogpu: invalid dimensions")

	// ErrNilTarget is returned when target pixmap is nil.
	ErrNilTarget = errors.New("gogpu: nil target pixmap")

	// ErrNilScene is returned when scene is nil.
	ErrNilScene = errors.New("gogpu: nil scene")
)
