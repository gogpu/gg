package gogpu

import (
	"fmt"

	"github.com/gogpu/gg"
)

// GPURenderer is a GPU-backed renderer for immediate mode drawing.
// It implements the gg.Renderer interface.
//
// The renderer uses gogpu's gpu.Backend for GPU operations,
// allowing both Rust and Pure Go GPU backends to be used.
//
// Phase 1 Implementation:
// Currently uses software rasterization via SoftwareRenderer,
// with future phases adding GPU texture upload and native GPU path rendering.
type GPURenderer struct {
	backend          *Backend
	width            int
	height           int
	softwareRenderer *gg.SoftwareRenderer
}

// newGPURenderer creates a new GPU renderer.
func newGPURenderer(b *Backend, width, height int) *GPURenderer {
	return &GPURenderer{
		backend:          b,
		width:            width,
		height:           height,
		softwareRenderer: gg.NewSoftwareRenderer(width, height),
	}
}

// Fill fills a path with the given paint.
//
// Phase 1 Implementation:
// Uses software rasterization via SoftwareRenderer. Future phases will add
// GPU texture upload and native GPU path tessellation.
func (r *GPURenderer) Fill(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Phase 1: Delegate to software renderer
	if err := r.softwareRenderer.Fill(pixmap, path, paint); err != nil {
		return fmt.Errorf("fill: %w", err)
	}

	// TODO Phase 2: Upload pixmap to GPU texture for compositing
	// TODO Phase 3: Native GPU path tessellation

	return nil
}

// Stroke strokes a path with the given paint.
//
// Phase 1 Implementation:
// Uses software rasterization via SoftwareRenderer. Future phases will add
// GPU texture upload and native GPU stroke expansion.
func (r *GPURenderer) Stroke(pixmap *gg.Pixmap, path *gg.Path, paint *gg.Paint) error {
	if pixmap == nil {
		return ErrNilTarget
	}
	if path == nil || paint == nil {
		return nil // No-op for nil path or paint
	}

	// Phase 1: Delegate to software renderer
	if err := r.softwareRenderer.Stroke(pixmap, path, paint); err != nil {
		return fmt.Errorf("stroke: %w", err)
	}

	// TODO Phase 2: Upload pixmap to GPU texture for compositing
	// TODO Phase 3: Native GPU stroke expansion and tessellation

	return nil
}

// Width returns the renderer width.
func (r *GPURenderer) Width() int {
	return r.width
}

// Height returns the renderer height.
func (r *GPURenderer) Height() int {
	return r.height
}

// Close releases renderer resources.
//
// Note: This is a stub implementation. GPU resource cleanup will be
// implemented when the rendering pipeline is complete.
func (r *GPURenderer) Close() {
	// TODO: Release GPU resources
	// - Vertex buffers
	// - Texture views
	// - Pipelines
}
