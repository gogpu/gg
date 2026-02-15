//go:build !nogpu

// Package gpu registers the GPU accelerator for hardware-accelerated rendering.
//
// Import this package to enable GPU-based rendering. The GPU accelerator uses
// wgpu/hal compute shaders for parallel evaluation, providing hardware-accelerated
// anti-aliased rendering for shapes like circles and rounded rectangles.
//
// If GPU initialization fails (no Vulkan/Metal/DX12 available), the
// registration is silently skipped and rendering falls back to CPU.
//
// Usage:
//
//	import _ "github.com/gogpu/gg/gpu" // enable GPU acceleration
package gpu

import (
	"github.com/gogpu/gg"
	gpuimpl "github.com/gogpu/gg/internal/gpu"
)

func init() {
	accel := &gpuimpl.SDFAccelerator{}
	if err := gg.RegisterAccelerator(accel); err != nil {
		gg.Logger().Warn("GPU accelerator not available", "err", err)
	}
}

// SetDeviceProvider configures the GPU accelerator to use a shared GPU device
// from an external provider (e.g., gogpu). This avoids creating a separate
// GPU instance and enables efficient device sharing.
//
// The provider should be a gpucontext.DeviceProvider that also implements
// gpucontext.HalProvider for direct HAL access.
//
// Call this before drawing operations, typically from ggcanvas.New() or
// manually after registering the accelerator.
func SetDeviceProvider(provider any) error {
	return gg.SetAcceleratorDeviceProvider(provider)
}
