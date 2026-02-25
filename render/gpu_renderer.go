// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package render

import (
	"errors"
)

// GPURenderer is a GPU-accelerated renderer using WebGPU.
//
// This renderer uses the GPU device provided by the host application
// to perform hardware-accelerated 2D rendering. It leverages compute
// shaders for path rasterization and the render pipeline for compositing.
//
// Note: This is a stub implementation for Phase 1. Full GPU rendering
// will be implemented in Phase 3.
//
// Example:
//
//	app := gogpu.NewApp(gogpu.Config{...})
//	var renderer *render.GPURenderer
//	var initialized bool
//
//	app.OnDraw(func(dc *gogpu.Context) {
//	    if !initialized {
//	        provider := app.GPUContextProvider()
//	        if provider != nil {
//	            renderer, _ = render.NewGPURenderer(provider)
//	            initialized = true
//	        }
//	    }
//	    // Render scene (CPU target for now, GPU targets in Phase 3)
//	    target := render.NewPixmapTarget(800, 600)
//	    renderer.Render(target, scene)
//	})
type GPURenderer struct {
	// handle is the GPU device handle from the host application.
	handle DeviceHandle

	// softwareFallback is used when GPU rendering is not available.
	softwareFallback *SoftwareRenderer
}

// NewGPURenderer creates a new GPU-accelerated renderer.
//
// The DeviceHandle must be provided by the host application (e.g., gogpu.App).
// The renderer does NOT create its own GPU device.
//
// Returns an error if the device handle is invalid or GPU rendering
// is not supported on this device.
//
// Note: Phase 1 implementation falls back to software rendering.
func NewGPURenderer(handle DeviceHandle) (*GPURenderer, error) {
	if handle == nil {
		return nil, errors.New("render: nil device handle")
	}

	// Phase 1: Create software fallback
	// Phase 3: Initialize GPU pipeline
	return &GPURenderer{
		handle:           handle,
		softwareFallback: NewSoftwareRenderer(),
	}, nil
}

// Render draws the scene to the target.
//
// For GPU targets (TextureView != nil), rendering is performed on the GPU.
// For CPU targets (Pixels != nil), rendering falls back to software.
//
// Note: Phase 1 implementation always uses software fallback.
func (r *GPURenderer) Render(target RenderTarget, scene *Scene) error {
	if target == nil {
		return errors.New("render: nil target")
	}

	// Phase 1: Always use software fallback
	// Check if target supports CPU rendering
	if target.Pixels() != nil {
		return r.softwareFallback.Render(target, scene)
	}

	// GPU targets not yet supported in Phase 1
	// Phase 3 will implement GPU pipeline rendering
	return errors.New("render: GPU targets not yet implemented (Phase 1)")
}

// Flush ensures all GPU commands are submitted and complete.
//
// This method:
//   - Submits any pending command buffers
//   - Waits for GPU completion
//
// For CPU targets, this is a no-op.
func (r *GPURenderer) Flush() error {
	// Phase 1: No GPU commands to flush
	// Phase 3: Submit command buffers and poll device
	return nil
}

// Capabilities returns the renderer's capabilities.
func (r *GPURenderer) Capabilities() RendererCapabilities {
	// Phase 1: Report software capabilities
	// Phase 3: Query actual GPU capabilities
	return RendererCapabilities{
		IsGPU:                true, // Intent, not current reality
		SupportsAntialiasing: true,
		SupportsBlendModes:   false, // TODO: Phase 3
		SupportsGradients:    false, // TODO: Phase 3
		SupportsTextures:     false, // TODO: Phase 3
		MaxTextureSize:       8192,  // Typical GPU limit
	}
}

// DeviceHandle returns the underlying device handle.
// This allows advanced users to access the GPU device for custom rendering.
func (r *GPURenderer) DeviceHandle() DeviceHandle {
	return r.handle
}

// CreateTextureTarget creates a GPU texture render target.
//
// Note: Phase 1 implementation returns an error.
// Phase 3 will implement actual texture creation.
func (r *GPURenderer) CreateTextureTarget(width, height int) (*TextureTarget, error) {
	// Phase 1: Not implemented
	return nil, errors.New("render: GPU texture targets not yet implemented (Phase 1)")
}

// Ensure GPURenderer implements Renderer and CapableRenderer.
var (
	_ Renderer        = (*GPURenderer)(nil)
	_ CapableRenderer = (*GPURenderer)(nil)
)
