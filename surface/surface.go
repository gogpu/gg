// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package surface

import (
	"image"
	"image/color"
)

// Surface is the core rendering target abstraction.
//
// A Surface represents a 2D canvas that can be drawn to. Implementations may
// use CPU-based software rendering, GPU acceleration, or any other backend.
//
// Surfaces are NOT thread-safe. Each surface should be used from a single
// goroutine, or external synchronization must be used.
//
// Example usage:
//
//	s := surface.NewImageSurface(800, 600)
//	defer s.Close()
//
//	s.Clear(color.White)
//	s.Fill(path, surface.FillStyle{Color: color.RGBA{255, 0, 0, 255}})
//	img := s.Snapshot()
type Surface interface {
	// Width returns the surface width in pixels.
	Width() int

	// Height returns the surface height in pixels.
	Height() int

	// Clear fills the entire surface with the given color.
	// This is typically the fastest way to reset the surface.
	Clear(c color.Color)

	// Fill fills the given path using the specified style.
	// The path is not modified or consumed.
	Fill(path *Path, style FillStyle)

	// Stroke strokes the given path using the specified style.
	// The path is not modified or consumed.
	Stroke(path *Path, style StrokeStyle)

	// DrawImage draws an image at the specified position.
	// If opts is nil, default options are used.
	DrawImage(img image.Image, at Point, opts *DrawImageOptions)

	// Flush ensures all pending drawing operations are complete.
	// For CPU surfaces, this is typically a no-op.
	// For GPU surfaces, this may submit commands and wait for completion.
	// Returns an error if flushing fails.
	Flush() error

	// Snapshot returns the current surface contents as an RGBA image.
	// The returned image is a copy; modifications to it do not affect the surface.
	// This may be slow for GPU surfaces as it requires readback.
	Snapshot() *image.RGBA

	// Close releases all resources associated with the surface.
	// After Close, the surface must not be used.
	// Close is idempotent; multiple calls are safe.
	Close() error
}

// SubSurface is an optional interface for surfaces that support sub-regions.
type SubSurface interface {
	Surface

	// CreateSubSurface creates a new surface backed by a region of this surface.
	// Drawing to the sub-surface affects the parent surface.
	CreateSubSurface(bounds image.Rectangle) (Surface, error)
}

// ResizableSurface is an optional interface for surfaces that support resizing.
type ResizableSurface interface {
	Surface

	// Resize changes the surface dimensions.
	// Existing content may be discarded or preserved depending on implementation.
	Resize(width, height int) error
}

// ClippableSurface is an optional interface for surfaces with clipping support.
type ClippableSurface interface {
	Surface

	// SetClip sets the clipping region.
	// Only pixels within the path will be affected by subsequent operations.
	SetClip(path *Path)

	// ClearClip removes the clipping region.
	ClearClip()

	// PushClip saves the current clip state.
	PushClip()

	// PopClip restores the previous clip state.
	PopClip()
}

// BlendableSurface is an optional interface for surfaces with blend mode support.
type BlendableSurface interface {
	Surface

	// SetBlendMode sets the blend mode for subsequent operations.
	SetBlendMode(mode BlendMode)

	// BlendMode returns the current blend mode.
	BlendMode() BlendMode
}

// BlendMode specifies how source and destination colors are combined.
type BlendMode uint8

const (
	// BlendModeSourceOver is the default Porter-Duff source-over mode.
	BlendModeSourceOver BlendMode = iota

	// BlendModeMultiply multiplies source and destination colors.
	BlendModeMultiply

	// BlendModeScreen is the inverse of multiply.
	BlendModeScreen

	// BlendModeOverlay combines multiply and screen.
	BlendModeOverlay

	// BlendModeClear clears the destination.
	BlendModeClear

	// BlendModeCopy replaces destination with source.
	BlendModeCopy
)

// Capabilities describes the optional features a surface supports.
type Capabilities struct {
	// SupportsSubSurface indicates CreateSubSurface is available.
	SupportsSubSurface bool

	// SupportsResize indicates Resize is available.
	SupportsResize bool

	// SupportsClipping indicates clipping operations are available.
	SupportsClipping bool

	// SupportsBlendModes indicates blend mode control is available.
	SupportsBlendModes bool

	// SupportsAntialias indicates anti-aliased rendering is available.
	SupportsAntialias bool

	// MaxWidth is the maximum supported width (0 = unlimited).
	MaxWidth int

	// MaxHeight is the maximum supported height (0 = unlimited).
	MaxHeight int
}

// CapableSurface is an optional interface for querying surface capabilities.
type CapableSurface interface {
	Surface

	// Capabilities returns the surface's capabilities.
	Capabilities() Capabilities
}
