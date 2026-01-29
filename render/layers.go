// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"slices"

	"github.com/gogpu/gputypes"
)

// LayeredTarget supports z-ordered layers for popups, dropdowns, and tooltips.
//
// This interface extends RenderTarget with layer management capabilities.
// Layers are rendered in ascending z-order (lower z values behind higher ones).
// This is useful for UI frameworks that need to render overlays without
// managing separate surfaces.
type LayeredTarget interface {
	RenderTarget

	// CreateLayer creates a new layer at the specified z-order.
	// Higher z values are rendered on top of lower values.
	// Returns an error if a layer with the same z-order already exists.
	CreateLayer(z int) (RenderTarget, error)

	// RemoveLayer removes a layer by z-order.
	// Returns an error if the layer does not exist.
	RemoveLayer(z int) error

	// SetLayerVisible controls layer visibility without removing it.
	// Invisible layers are not composited but retain their content.
	SetLayerVisible(z int, visible bool)

	// Layers returns all layer z-orders in render order (ascending).
	Layers() []int

	// Composite blends all visible layers onto the base target.
	// This should be called after drawing to layers is complete.
	Composite()
}

// layer represents a single compositing layer.
type layer struct {
	img     *image.RGBA
	visible bool
}

// LayeredPixmapTarget is a CPU-backed implementation of LayeredTarget.
// It uses *image.RGBA for each layer and composites them in z-order.
type LayeredPixmapTarget struct {
	base   *image.RGBA    // Base layer (z=0 equivalent, always visible)
	layers map[int]*layer // Additional layers by z-order
	zOrder []int          // Cached sorted z-order list
	width  int
	height int
}

// NewLayeredPixmapTarget creates a new layered CPU render target.
func NewLayeredPixmapTarget(width, height int) *LayeredPixmapTarget {
	return &LayeredPixmapTarget{
		base:   image.NewRGBA(image.Rect(0, 0, width, height)),
		layers: make(map[int]*layer),
		zOrder: nil,
		width:  width,
		height: height,
	}
}

// Width returns the target width in pixels.
func (t *LayeredPixmapTarget) Width() int {
	return t.width
}

// Height returns the target height in pixels.
func (t *LayeredPixmapTarget) Height() int {
	return t.height
}

// Format returns the pixel format (RGBA8).
func (t *LayeredPixmapTarget) Format() gputypes.TextureFormat {
	return gputypes.TextureFormatRGBA8Unorm
}

// TextureView returns nil as this is a CPU-only target.
func (t *LayeredPixmapTarget) TextureView() TextureView {
	return nil
}

// Pixels returns direct access to the base layer pixel data.
// Note: This returns the base layer, not the composited result.
// Call Composite() first to get the composited image.
func (t *LayeredPixmapTarget) Pixels() []byte {
	return t.base.Pix
}

// Stride returns the number of bytes per row.
func (t *LayeredPixmapTarget) Stride() int {
	return t.base.Stride
}

// Image returns the base layer image.
// Note: This returns the base layer, not the composited result.
// Call Composite() first, then Image() to get the composited image.
func (t *LayeredPixmapTarget) Image() *image.RGBA {
	return t.base
}

// CreateLayer creates a new layer at the specified z-order.
// Returns a RenderTarget that can be used for drawing to the layer.
func (t *LayeredPixmapTarget) CreateLayer(z int) (RenderTarget, error) {
	if _, exists := t.layers[z]; exists {
		return nil, fmt.Errorf("layer with z=%d already exists", z)
	}

	l := &layer{
		img:     image.NewRGBA(image.Rect(0, 0, t.width, t.height)),
		visible: true,
	}
	t.layers[z] = l

	// Invalidate cached z-order
	t.zOrder = nil

	// Return a PixmapTarget wrapping the layer's image
	return NewPixmapTargetFromImage(l.img), nil
}

// RemoveLayer removes a layer by z-order.
func (t *LayeredPixmapTarget) RemoveLayer(z int) error {
	if _, exists := t.layers[z]; !exists {
		return fmt.Errorf("layer with z=%d does not exist", z)
	}

	delete(t.layers, z)

	// Invalidate cached z-order
	t.zOrder = nil

	return nil
}

// SetLayerVisible controls layer visibility.
func (t *LayeredPixmapTarget) SetLayerVisible(z int, visible bool) {
	if l, exists := t.layers[z]; exists {
		l.visible = visible
	}
}

// Layers returns all layer z-orders in render order (ascending).
func (t *LayeredPixmapTarget) Layers() []int {
	if t.zOrder == nil {
		t.zOrder = make([]int, 0, len(t.layers))
		for z := range t.layers {
			t.zOrder = append(t.zOrder, z)
		}
		slices.Sort(t.zOrder)
	}
	// Return a copy to prevent modification
	result := make([]int, len(t.zOrder))
	copy(result, t.zOrder)
	return result
}

// Composite blends all visible layers onto the base target in z-order.
// Layers are composited using standard alpha blending (source over).
func (t *LayeredPixmapTarget) Composite() {
	// Get sorted z-orders
	orders := t.Layers()

	// Composite each visible layer onto base
	for _, z := range orders {
		l := t.layers[z]
		if l.visible {
			// Use draw.Over for alpha compositing
			draw.Draw(t.base, t.base.Bounds(), l.img, image.Point{}, draw.Over)
		}
	}
}

// Clear fills the base layer with the given color.
// Does not affect other layers.
func (t *LayeredPixmapTarget) Clear(c color.Color) {
	r, g, b, a := c.RGBA()
	// Convert from 16-bit to 8-bit
	//nolint:gosec // G115: mask ensures no overflow
	rgba := color.RGBA{
		R: uint8((r >> 8) & 0xFF),
		G: uint8((g >> 8) & 0xFF),
		B: uint8((b >> 8) & 0xFF),
		A: uint8((a >> 8) & 0xFF),
	}

	bounds := t.base.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			t.base.SetRGBA(x, y, rgba)
		}
	}
}

// ClearLayer fills a specific layer with a color.
// Returns error if the layer does not exist.
func (t *LayeredPixmapTarget) ClearLayer(z int, c color.Color) error {
	l, exists := t.layers[z]
	if !exists {
		return fmt.Errorf("layer with z=%d does not exist", z)
	}

	r, g, b, a := c.RGBA()
	//nolint:gosec // G115: mask ensures no overflow
	rgba := color.RGBA{
		R: uint8((r >> 8) & 0xFF),
		G: uint8((g >> 8) & 0xFF),
		B: uint8((b >> 8) & 0xFF),
		A: uint8((a >> 8) & 0xFF),
	}

	bounds := l.img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			l.img.SetRGBA(x, y, rgba)
		}
	}

	return nil
}

// GetLayer returns the RenderTarget for a specific layer.
// Returns nil if the layer does not exist.
func (t *LayeredPixmapTarget) GetLayer(z int) RenderTarget {
	l, exists := t.layers[z]
	if !exists {
		return nil
	}
	return NewPixmapTargetFromImage(l.img)
}

// Ensure LayeredPixmapTarget implements both RenderTarget and LayeredTarget.
var (
	_ RenderTarget  = (*LayeredPixmapTarget)(nil)
	_ LayeredTarget = (*LayeredPixmapTarget)(nil)
)
