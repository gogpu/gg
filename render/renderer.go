// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

// Renderer executes drawing commands to a render target.
//
// The Renderer interface is the primary abstraction for rendering backends.
// Different implementations provide CPU or GPU rendering:
//
//   - SoftwareRenderer: CPU-based rendering using core/ package algorithms
//   - GPURenderer: GPU-accelerated rendering using WebGPU (Phase 3)
//
// Renderers are stateless between Render calls, allowing the same renderer
// to be used with different targets and scenes.
//
// Thread Safety: Renderers are NOT thread-safe. Each renderer should be used
// from a single goroutine, or external synchronization must be used.
//
// Example:
//
//	// Create renderer
//	renderer := render.NewSoftwareRenderer()
//
//	// Create target
//	target := render.NewPixmapTarget(800, 600)
//
//	// Render scene
//	if err := renderer.Render(target, scene); err != nil {
//	    log.Printf("render failed: %v", err)
//	}
type Renderer interface {
	// Render draws the scene to the target.
	//
	// The scene is processed and drawn to the target in order of commands.
	// Returns an error if rendering fails (e.g., incompatible target format).
	//
	// The scene is not modified by this operation and can be rendered
	// multiple times to different targets.
	Render(target RenderTarget, scene *Scene) error

	// Flush ensures all pending rendering operations are complete.
	//
	// For CPU renderers, this is typically a no-op as operations are
	// synchronous. For GPU renderers, this may submit command buffers
	// and wait for completion.
	//
	// Returns an error if flushing fails.
	Flush() error
}

// RendererCapabilities describes the features supported by a renderer.
type RendererCapabilities struct {
	// IsGPU indicates if this is a GPU-accelerated renderer.
	IsGPU bool

	// SupportsAntialiasing indicates if anti-aliased rendering is supported.
	SupportsAntialiasing bool

	// SupportsBlendModes indicates if custom blend modes are supported.
	SupportsBlendModes bool

	// SupportsGradients indicates if gradient fills are supported.
	SupportsGradients bool

	// SupportsTextures indicates if texture sampling is supported.
	SupportsTextures bool

	// MaxTextureSize is the maximum texture dimension (0 = unlimited).
	MaxTextureSize int
}

// CapableRenderer is an optional interface for renderers that can
// report their capabilities.
type CapableRenderer interface {
	Renderer

	// Capabilities returns the renderer's capabilities.
	Capabilities() RendererCapabilities
}
