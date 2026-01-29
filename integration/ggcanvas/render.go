// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package ggcanvas

import (
	"errors"
	"reflect"
)

// Rendering errors.
var (
	// ErrInvalidDrawContext is returned when the draw context doesn't implement
	// the required interface.
	ErrInvalidDrawContext = errors.New("ggcanvas: draw context must implement textureDrawer")

	// ErrInvalidRenderer is returned when the renderer doesn't implement
	// the required interface.
	ErrInvalidRenderer = errors.New("ggcanvas: renderer must implement textureCreator")
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

// textureDrawer is the interface for drawing textures.
// This matches gogpu.Context's drawing capabilities.
type textureDrawer interface {
	// DrawTexture draws a texture at the given position.
	DrawTexture(tex any, x, y float32) error

	// Renderer returns the renderer for texture creation.
	Renderer() any
}

// advancedTextureDrawer is an optional interface for advanced rendering.
// Not all draw contexts need to implement this.
type advancedTextureDrawer interface {
	// DrawTextureEx draws a texture with transform options.
	DrawTextureEx(tex any, x, y, scaleX, scaleY, alpha float32, flipY bool) error
}

// rendererWithTextureCreation wraps a renderer to get texture creation capability.
type rendererWithTextureCreation interface {
	NewTextureFromRGBA(width, height int, data []byte) (any, error)
}

// RenderTo draws the canvas content to a gogpu.Context.
// This is the primary integration method.
//
// The dc parameter should be a *gogpu.Context (or any type implementing textureDrawer).
// The canvas content is flushed to GPU and drawn at position (0, 0).
//
// Returns error if:
//   - Canvas is closed
//   - dc doesn't implement required interface
//   - Texture creation or drawing fails
func (c *Canvas) RenderTo(dc any) error {
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
//	canvas.RenderToEx(dc, opts)
func (c *Canvas) RenderToEx(dc any, opts RenderOptions) error {
	if c.closed {
		return ErrCanvasClosed
	}

	// Get texture drawer interface
	drawer, ok := dc.(textureDrawer)
	if !ok {
		return ErrInvalidDrawContext
	}

	// Flush canvas to ensure texture is up-to-date
	tex, err := c.Flush()
	if err != nil {
		return err
	}

	// If texture is pending (placeholder), create it now
	if pending, isPending := tex.(*pendingTexture); isPending {
		realTex, err := c.createRealTexture(drawer, pending)
		if err != nil {
			return err
		}
		c.texture = realTex
		tex = realTex
	}

	// Try advanced drawing if available and needed
	if opts.ScaleX != 1 || opts.ScaleY != 1 || opts.Alpha != 1 || opts.FlipY {
		if advanced, ok := dc.(advancedTextureDrawer); ok {
			return advanced.DrawTextureEx(tex, opts.X, opts.Y, opts.ScaleX, opts.ScaleY, opts.Alpha, opts.FlipY)
		}
		// Fall back to basic drawing (ignore scale/alpha/flip)
	}

	// Basic texture drawing
	return drawer.DrawTexture(tex, opts.X, opts.Y)
}

// createRealTexture creates a real GPU texture from a pending placeholder.
func (c *Canvas) createRealTexture(drawer textureDrawer, pending *pendingTexture) (any, error) {
	// Get renderer from drawer
	renderer := drawer.Renderer()

	// Check for nil renderer (handles both untyped nil and typed nil pointer in interface)
	if renderer == nil || isNilInterface(renderer) {
		return nil, ErrInvalidRenderer
	}

	// Get texture creator from renderer
	creator, ok := renderer.(rendererWithTextureCreation)
	if !ok {
		return nil, ErrInvalidRenderer
	}

	// Create the actual texture
	tex, err := creator.NewTextureFromRGBA(pending.width, pending.height, pending.data)
	if err != nil {
		return nil, err
	}

	return tex, nil
}

// isNilInterface checks if an interface contains a nil pointer.
// This handles the Go gotcha where (*T)(nil) wrapped in interface{} is not == nil.
func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// RenderToPosition is a convenience method for rendering at a specific position.
//
//	canvas.RenderToPosition(dc, 100, 50)
//
// is equivalent to:
//
//	canvas.RenderToEx(dc, RenderOptions{X: 100, Y: 50, ScaleX: 1, ScaleY: 1, Alpha: 1})
func (c *Canvas) RenderToPosition(dc any, x, y float32) error {
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
//	canvas.RenderToScaled(dc, 0.5) // Render at half size
func (c *Canvas) RenderToScaled(dc any, scale float32) error {
	return c.RenderToEx(dc, RenderOptions{
		X:      0,
		Y:      0,
		ScaleX: scale,
		ScaleY: scale,
		Alpha:  1,
	})
}
