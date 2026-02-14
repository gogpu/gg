// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package ggcanvas

import (
	"errors"
	"fmt"

	"github.com/gogpu/gpucontext"
)

// Rendering errors.
var (
	// ErrInvalidDrawContext is returned when the draw context doesn't implement
	// gpucontext.TextureDrawer.
	ErrInvalidDrawContext = errors.New("ggcanvas: dc must implement gpucontext.TextureDrawer")

	// ErrInvalidRenderer is returned when the renderer doesn't implement
	// gpucontext.TextureCreator.
	ErrInvalidRenderer = errors.New("ggcanvas: renderer must implement gpucontext.TextureCreator")
)

// RenderOptions controls how canvas is rendered to the target.
type RenderOptions struct {
	// X, Y is the position to draw the texture (default: 0, 0)
	X, Y float32

	// ScaleX, ScaleY are the scale factors (default: 1, 1)
	// Values < 1 shrink, values > 1 enlarge
	ScaleX float32
	ScaleY float32

	// Alpha is the opacity from 0 (transparent) to 1 (opaque) (default: 1)
	Alpha float32

	// FlipY flips the texture vertically (default: false)
	// Useful when coordinate systems differ between gg and GPU
	FlipY bool
}

// DefaultRenderOptions returns options with sensible defaults.
func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		X:      0,
		Y:      0,
		ScaleX: 1,
		ScaleY: 1,
		Alpha:  1,
		FlipY:  false,
	}
}

// RenderTo draws the canvas content to a gpucontext.TextureDrawer.
// This is the primary integration method.
//
// The dc parameter should be obtained from gogpu.Context.AsTextureDrawer().
// The canvas content is flushed to GPU and drawn at position (0, 0).
//
// Example:
//
//	app.OnDraw(func(dc *gogpu.Context) {
//	    canvas.RenderTo(dc.AsTextureDrawer())
//	})
//
// Returns error if:
//   - Canvas is closed
//   - Texture creation or drawing fails
func (c *Canvas) RenderTo(dc gpucontext.TextureDrawer) error {
	return c.RenderToEx(dc, DefaultRenderOptions())
}

// RenderToEx draws the canvas with additional options.
// Use this when you need positioning, scaling, or transparency control.
//
// Example:
//
//	opts := ggcanvas.RenderOptions{
//	    X: 100, Y: 50,
//	    ScaleX: 0.5, ScaleY: 0.5,
//	    Alpha: 0.8,
//	}
//	canvas.RenderToEx(dc.AsTextureDrawer(), opts)
func (c *Canvas) RenderToEx(dc gpucontext.TextureDrawer, opts RenderOptions) error {
	if c.closed {
		return ErrCanvasClosed
	}

	// Flush canvas to ensure pixmap is up-to-date
	tex, err := c.Flush()
	if err != nil {
		return err
	}

	// If texture is pending (placeholder), create real GPU texture now
	if pending, isPending := tex.(*pendingTexture); isPending {
		creator := dc.TextureCreator()
		if creator == nil {
			return ErrInvalidRenderer
		}

		// NewTextureFromRGBA calls WriteTexture which does waitForGPU internally.
		// After this returns, ALL prior GPU work is complete, so it's safe to
		// destroy the old texture (its descriptor heap entries are no longer in use).
		realTex, err := creator.NewTextureFromRGBA(pending.width, pending.height, pending.data)
		if err != nil {
			return fmt.Errorf("ggcanvas: NewTextureFromRGBA failed: %w", err)
		}

		// gg pixmap data is premultiplied alpha — mark texture accordingly
		// so gogpu uses BlendFactorOne pipeline for correct compositing.
		if pt, ok := realTex.(interface{ SetPremultiplied(bool) }); ok {
			pt.SetPremultiplied(true)
		}

		c.texture = realTex
		tex = realTex

		// NOW safe to destroy the old texture — GPU is idle after WriteTexture's wait.
		// This prevents use-after-free where the GPU reads freed descriptor heap entries.
		if c.oldTexture != nil {
			if destroyer, ok := c.oldTexture.(textureDestroyer); ok {
				destroyer.Destroy()
			}
			c.oldTexture = nil
		}
	}

	// Get gpucontext.Texture for drawing
	gpuTex, ok := tex.(gpucontext.Texture)
	if !ok {
		return ErrInvalidDrawContext
	}

	// Draw texture at position
	// Note: ScaleX, ScaleY, Alpha, FlipY are currently ignored (basic rendering)
	// Advanced rendering with transforms can be added in future versions
	return dc.DrawTexture(gpuTex, opts.X, opts.Y)
}

// RenderToPosition is a convenience method for rendering at a specific position.
//
//	canvas.RenderToPosition(dc.AsTextureDrawer(), 100, 50)
//
// is equivalent to:
//
//	canvas.RenderToEx(dc.AsTextureDrawer(), RenderOptions{X: 100, Y: 50, ScaleX: 1, ScaleY: 1, Alpha: 1})
func (c *Canvas) RenderToPosition(dc gpucontext.TextureDrawer, x, y float32) error {
	return c.RenderToEx(dc, RenderOptions{
		X:      x,
		Y:      y,
		ScaleX: 1,
		ScaleY: 1,
		Alpha:  1,
	})
}

// RenderToScaled is a convenience method for rendering with uniform scaling.
//
//	canvas.RenderToScaled(dc.AsTextureDrawer(), 0.5) // Render at half size
func (c *Canvas) RenderToScaled(dc gpucontext.TextureDrawer, scale float32) error {
	return c.RenderToEx(dc, RenderOptions{
		X:      0,
		Y:      0,
		ScaleX: scale,
		ScaleY: scale,
		Alpha:  1,
	})
}
