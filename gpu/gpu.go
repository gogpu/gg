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
	"log"

	"github.com/gogpu/gg"
	gpuimpl "github.com/gogpu/gg/internal/gpu"
)

func init() {
	accel := &gpuimpl.SDFAccelerator{}
	if err := gg.RegisterAccelerator(accel); err != nil {
		log.Printf("gpu: GPU accelerator not available: %v", err)
	}
}
