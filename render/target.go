// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package render

import (
	"image"
	"image/color"

	"github.com/gogpu/gputypes"
)

// RenderTarget defines where rendering output goes.
//
// A RenderTarget is an abstraction over different rendering destinations:
//   - PixmapTarget: CPU-backed *image.RGBA for software rendering
//   - TextureTarget: GPU texture for offscreen rendering
//   - SurfaceTarget: Window surface from the host application
//
// Targets may support CPU access (Pixels), GPU access (TextureView), or both.
// The Renderer implementation chooses the appropriate access method.
type RenderTarget interface {
	// Width returns the target width in pixels.
	Width() int

	// Height returns the target height in pixels.
	Height() int

	// Format returns the pixel format of the target.
	Format() gputypes.TextureFormat

	// TextureView returns the GPU texture view for this target.
	// Returns nil for CPU-only targets.
	TextureView() TextureView

	// Pixels returns direct access to pixel data.
	// Returns nil for GPU-only targets.
	// For RGBA format, each pixel is 4 bytes: R, G, B, A.
	Pixels() []byte

	// Stride returns the number of bytes per row.
	// For RGBA, this is typically Width * 4, but may include padding.
	Stride() int
}

// PixmapTarget is a CPU-backed render target using *image.RGBA.
//
// This target supports software rendering and provides direct pixel access.
// It is the default target for pure CPU rendering workflows.
//
// Example:
//
//	target := render.NewPixmapTarget(800, 600)
//	renderer.Render(target, scene)
//	img := target.Image()
type PixmapTarget struct {
	img *image.RGBA
}

// NewPixmapTarget creates a new CPU-backed render target.
func NewPixmapTarget(width, height int) *PixmapTarget {
	return &PixmapTarget{
		img: image.NewRGBA(image.Rect(0, 0, width, height)),
	}
}

// NewPixmapTargetFromImage wraps an existing *image.RGBA as a render target.
// The image is used directly without copying.
func NewPixmapTargetFromImage(img *image.RGBA) *PixmapTarget {
	return &PixmapTarget{img: img}
}

// Width returns the target width in pixels.
func (t *PixmapTarget) Width() int {
	return t.img.Bounds().Dx()
}

// Height returns the target height in pixels.
func (t *PixmapTarget) Height() int {
	return t.img.Bounds().Dy()
}

// Format returns the pixel format (RGBA8).
func (t *PixmapTarget) Format() gputypes.TextureFormat {
	return gputypes.TextureFormatRGBA8Unorm
}

// TextureView returns nil as this is a CPU-only target.
func (t *PixmapTarget) TextureView() TextureView {
	return nil
}

// Pixels returns direct access to the pixel data.
func (t *PixmapTarget) Pixels() []byte {
	return t.img.Pix
}

// Stride returns the number of bytes per row.
func (t *PixmapTarget) Stride() int {
	return t.img.Stride
}

// Image returns the underlying *image.RGBA.
// The returned image shares memory with the target.
func (t *PixmapTarget) Image() *image.RGBA {
	return t.img
}

// Clear fills the entire target with the given color.
func (t *PixmapTarget) Clear(c color.Color) {
	r, g, b, a := c.RGBA()
	// Convert from 16-bit to 8-bit (mask ensures value fits in uint8)
	//nolint:gosec // G115: mask ensures no overflow
	rgba := color.RGBA{
		R: uint8((r >> 8) & 0xFF),
		G: uint8((g >> 8) & 0xFF),
		B: uint8((b >> 8) & 0xFF),
		A: uint8((a >> 8) & 0xFF),
	}

	bounds := t.img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			t.img.SetRGBA(x, y, rgba)
		}
	}
}

// SetPixel sets a single pixel at the given coordinates.
func (t *PixmapTarget) SetPixel(x, y int, c color.Color) {
	t.img.Set(x, y, c)
}

// GetPixel returns the color at the given coordinates.
func (t *PixmapTarget) GetPixel(x, y int) color.Color {
	return t.img.At(x, y)
}

// Resize creates a new target with the given dimensions.
// The contents are not preserved.
func (t *PixmapTarget) Resize(width, height int) {
	t.img = image.NewRGBA(image.Rect(0, 0, width, height))
}

// Ensure PixmapTarget implements RenderTarget.
var _ RenderTarget = (*PixmapTarget)(nil)

// TextureTarget is a GPU texture-backed render target.
//
// This target wraps a GPU texture and allows rendering to offscreen surfaces
// for post-processing, texture caching, or multi-pass rendering.
//
// Note: Full implementation requires GPU backend support (Phase 3).
type TextureTarget struct {
	width  int
	height int
	format gputypes.TextureFormat
	view   TextureView
}

// NewTextureTarget creates a new GPU texture render target.
// Requires a DeviceHandle to create the texture.
//
// Note: This is a stub. Full implementation in Phase 3.
func NewTextureTarget(handle DeviceHandle, width, height int, format gputypes.TextureFormat) (*TextureTarget, error) {
	// TODO(Phase 3): Create actual GPU texture using handle.Device()
	return &TextureTarget{
		width:  width,
		height: height,
		format: format,
		view:   nil, // Will be created from texture
	}, nil
}

// Width returns the target width in pixels.
func (t *TextureTarget) Width() int {
	return t.width
}

// Height returns the target height in pixels.
func (t *TextureTarget) Height() int {
	return t.height
}

// Format returns the pixel format.
func (t *TextureTarget) Format() gputypes.TextureFormat {
	return t.format
}

// TextureView returns the GPU texture view.
func (t *TextureTarget) TextureView() TextureView {
	return t.view
}

// Pixels returns nil as this is a GPU-only target.
// Use ReadPixels for GPU readback (expensive).
func (t *TextureTarget) Pixels() []byte {
	return nil
}

// Stride returns 0 as this is a GPU-only target.
func (t *TextureTarget) Stride() int {
	return 0
}

// Destroy releases GPU resources.
func (t *TextureTarget) Destroy() {
	if t.view != nil {
		t.view.Destroy()
		t.view = nil
	}
}

// Ensure TextureTarget implements RenderTarget.
var _ RenderTarget = (*TextureTarget)(nil)

// SurfaceTarget wraps a window surface from the host application.
//
// This target allows gg to render directly to a window surface provided by
// gogpu or another host framework. This enables zero-copy rendering where
// gg draws directly to the display surface.
//
// Note: Full implementation requires GPU backend support (Phase 3).
type SurfaceTarget struct {
	width  int
	height int
	format gputypes.TextureFormat
	view   TextureView
}

// NewSurfaceTarget creates a render target from a window surface.
//
// Note: This is a stub. Full implementation in Phase 3 will accept
// a Surface interface from the host application.
func NewSurfaceTarget(width, height int, format gputypes.TextureFormat, view TextureView) *SurfaceTarget {
	return &SurfaceTarget{
		width:  width,
		height: height,
		format: format,
		view:   view,
	}
}

// Width returns the surface width in pixels.
func (t *SurfaceTarget) Width() int {
	return t.width
}

// Height returns the surface height in pixels.
func (t *SurfaceTarget) Height() int {
	return t.height
}

// Format returns the surface pixel format.
func (t *SurfaceTarget) Format() gputypes.TextureFormat {
	return t.format
}

// TextureView returns the current frame's texture view.
func (t *SurfaceTarget) TextureView() TextureView {
	return t.view
}

// Pixels returns nil as surfaces do not support CPU access.
func (t *SurfaceTarget) Pixels() []byte {
	return nil
}

// Stride returns 0 as surfaces do not support CPU access.
func (t *SurfaceTarget) Stride() int {
	return 0
}

// Ensure SurfaceTarget implements RenderTarget.
var _ RenderTarget = (*SurfaceTarget)(nil)
