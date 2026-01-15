//go:build rust

package rust

import "errors"

// Package errors for rust backend.
var (
	// ErrNotInitialized is returned when operations are called before Init.
	ErrNotInitialized = errors.New("rust: backend not initialized")

	// ErrNoGPU is returned when no GPU adapter is available.
	ErrNoGPU = errors.New("rust: no GPU adapter available")

	// ErrLibraryNotFound is returned when wgpu-native library is not found.
	ErrLibraryNotFound = errors.New("rust: wgpu-native library not found")

	// ErrDeviceLost is returned when the GPU device is lost.
	ErrDeviceLost = errors.New("rust: GPU device lost")

	// ErrNotImplemented is returned for operations not yet implemented.
	ErrNotImplemented = errors.New("rust: operation not implemented")

	// ErrInvalidDimensions is returned when width or height is invalid.
	ErrInvalidDimensions = errors.New("rust: invalid dimensions")

	// ErrNilTarget is returned when target pixmap is nil.
	ErrNilTarget = errors.New("rust: nil target pixmap")

	// ErrNilScene is returned when scene is nil.
	ErrNilScene = errors.New("rust: nil scene")
)
