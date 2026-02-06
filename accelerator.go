package gg

import (
	"errors"
	"sync"
)

// ErrFallbackToCPU indicates the GPU accelerator cannot handle this operation.
// The caller should transparently fall back to CPU rendering.
var ErrFallbackToCPU = errors.New("gg: falling back to CPU rendering")

// AcceleratedOp describes operation types for GPU capability checking.
type AcceleratedOp uint32

const (
	// AccelFill represents path fill operations.
	AccelFill AcceleratedOp = 1 << iota

	// AccelStroke represents path stroke operations.
	AccelStroke

	// AccelScene represents full scene rendering.
	AccelScene

	// AccelText represents text rendering.
	AccelText

	// AccelImage represents image compositing.
	AccelImage

	// AccelGradient represents gradient rendering.
	AccelGradient
)

// GPUAccelerator is an optional GPU acceleration provider.
//
// When registered via RegisterAccelerator, the Context tries GPU acceleration
// first for supported operations. If the accelerator returns ErrFallbackToCPU
// or any error, rendering transparently falls back to CPU.
//
// Implementations should be provided by GPU backend packages (e.g., gg/gpu/).
// Users opt in to GPU acceleration via blank import:
//
//	import _ "github.com/gogpu/gg/gpu" // enables GPU acceleration
type GPUAccelerator interface {
	// Name returns the accelerator name (e.g., "wgpu", "vulkan").
	Name() string

	// Init initializes GPU resources. Called once during registration.
	Init() error

	// Close releases GPU resources.
	Close()

	// CanAccelerate reports whether the accelerator supports the given operation.
	// This is a fast check used to skip GPU entirely for unsupported operations.
	CanAccelerate(op AcceleratedOp) bool
}

var (
	accelMu sync.RWMutex
	accel   GPUAccelerator
)

// RegisterAccelerator registers a GPU accelerator for optional GPU rendering.
//
// Only one accelerator can be registered. Subsequent calls replace the previous one.
// The accelerator's Init() method is called during registration.
// If Init() fails, the accelerator is not registered and the error is returned.
//
// Typical usage via blank import in GPU backend packages:
//
//	func init() {
//	    gg.RegisterAccelerator(NewWGPUAccelerator())
//	}
func RegisterAccelerator(a GPUAccelerator) error {
	if a == nil {
		return errors.New("gg: accelerator must not be nil")
	}
	if err := a.Init(); err != nil {
		return err
	}
	accelMu.Lock()
	old := accel
	accel = a
	accelMu.Unlock()
	if old != nil {
		old.Close()
	}
	return nil
}

// Accelerator returns the currently registered GPU accelerator, or nil if none.
func Accelerator() GPUAccelerator {
	accelMu.RLock()
	a := accel
	accelMu.RUnlock()
	return a
}
