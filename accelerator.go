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

	// AccelCircleSDF represents SDF-based circle rendering.
	AccelCircleSDF

	// AccelRRectSDF represents SDF-based rounded rectangle rendering.
	AccelRRectSDF
)

// GPURenderTarget provides pixel buffer access for GPU output.
// The Data slice must be in premultiplied RGBA format, 4 bytes per pixel,
// laid out row by row with the given Stride.
type GPURenderTarget struct {
	Data          []uint8
	Width, Height int
	Stride        int // bytes per row
}

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

	// FillPath renders a filled path to the target.
	// Returns ErrFallbackToCPU if the path cannot be GPU-accelerated.
	FillPath(target GPURenderTarget, path *Path, paint *Paint) error

	// StrokePath renders a stroked path to the target.
	// Returns ErrFallbackToCPU if the path cannot be GPU-accelerated.
	StrokePath(target GPURenderTarget, path *Path, paint *Paint) error

	// FillShape renders a detected shape using SDF.
	// This is the fast path for circles and rounded rectangles.
	// Returns ErrFallbackToCPU if the shape is not supported.
	FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error

	// StrokeShape renders a detected shape outline using SDF.
	// Returns ErrFallbackToCPU if the shape is not supported.
	StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error

	// Flush dispatches any pending GPU operations to the target pixel buffer.
	// Batch-capable accelerators accumulate shapes during FillShape/StrokeShape
	// and dispatch them all in a single GPU pass on Flush.
	// Immediate-mode accelerators (e.g., CPU SDF) return nil.
	Flush(target GPURenderTarget) error
}

// DeviceProviderAware is an optional interface for accelerators that can share
// GPU resources with an external provider (e.g., gogpu window).
// When SetDeviceProvider is called, the accelerator reuses the provided GPU
// device instead of creating its own.
type DeviceProviderAware interface {
	SetDeviceProvider(provider any) error
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

// SetAcceleratorDeviceProvider passes a device provider to the registered
// accelerator, enabling GPU device sharing. If no accelerator is registered
// or it doesn't support device sharing, this is a no-op.
//
// The provider should implement HalDevice() any and HalQueue() any methods
// that return wgpu/hal types.
func SetAcceleratorDeviceProvider(provider any) error {
	a := Accelerator()
	if a == nil {
		return nil
	}
	if dpa, ok := a.(DeviceProviderAware); ok {
		return dpa.SetDeviceProvider(provider)
	}
	return nil
}
