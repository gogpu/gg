package native

import "errors"

// Package errors for wgpu backend.
var (
	// ErrNotInitialized is returned when operations are called before Init.
	ErrNotInitialized = errors.New("wgpu: backend not initialized")

	// ErrNoGPU is returned when no GPU adapter is available.
	ErrNoGPU = errors.New("wgpu: no GPU adapter available")

	// ErrDeviceLost is returned when the GPU device is lost.
	ErrDeviceLost = errors.New("wgpu: GPU device lost")

	// ErrNotImplemented is returned for stub operations not yet implemented.
	ErrNotImplemented = errors.New("wgpu: operation not implemented")

	// ErrInvalidDimensions is returned when width or height is invalid.
	ErrInvalidDimensions = errors.New("wgpu: invalid dimensions")

	// ErrNilTarget is returned when target pixmap is nil.
	ErrNilTarget = errors.New("wgpu: nil target pixmap")

	// ErrNilScene is returned when scene is nil.
	ErrNilScene = errors.New("wgpu: nil scene")
)
