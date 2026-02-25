// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"errors"
	"image"
	"image/color"
)

// GPUSurface is a GPU-accelerated surface wrapper.
//
// This is a minimal stub implementation that wraps an external GPU backend.
// The actual GPU implementation is provided by the backend (e.g., gogpu/wgpu).
//
// To use GPUSurface, you must provide a GPUBackend implementation.
// This allows gg to remain independent of specific GPU libraries.
//
// Example integration with gogpu:
//
//	// In gogpu package:
//	type gogpuBackend struct {
//	    device *wgpu.Device
//	    queue  *wgpu.Queue
//	    // ...
//	}
//
//	func (b *gogpuBackend) Clear(c color.Color) { ... }
//	func (b *gogpuBackend) Fill(path *surface.Path, style surface.FillStyle) { ... }
//	// ... implement GPUBackend interface
//
//	// Register the backend:
//	surface.Register("vulkan", 100, func(opts surface.Options) (surface.Surface, error) {
//	    backend := createVulkanBackend(opts)
//	    return surface.NewGPUSurface(opts.Width, opts.Height, backend), nil
//	}, vulkanAvailable)
type GPUSurface struct {
	width   int
	height  int
	backend GPUBackend
	closed  bool
}

// GPUBackend is the interface that GPU implementations must provide.
//
// This abstraction allows different GPU backends (Vulkan, Metal, D3D12)
// to be used with the same Surface API.
type GPUBackend interface {
	// Clear fills the surface with a color.
	Clear(c color.Color)

	// Fill fills a path with the given style.
	Fill(path *Path, style FillStyle)

	// Stroke strokes a path with the given style.
	Stroke(path *Path, style StrokeStyle)

	// DrawImage draws an image at the specified position.
	DrawImage(img image.Image, at Point, opts *DrawImageOptions)

	// Flush ensures all pending operations are submitted.
	Flush() error

	// Readback reads the surface contents to an image.
	Readback() (*image.RGBA, error)

	// Close releases GPU resources.
	Close() error
}

// NewGPUSurface creates a new GPU surface with the given backend.
// Returns an error if backend is nil.
func NewGPUSurface(width, height int, backend GPUBackend) (*GPUSurface, error) {
	if backend == nil {
		return nil, errors.New("surface: GPUBackend cannot be nil")
	}

	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}

	return &GPUSurface{
		width:   width,
		height:  height,
		backend: backend,
	}, nil
}

// Width returns the surface width.
func (s *GPUSurface) Width() int {
	return s.width
}

// Height returns the surface height.
func (s *GPUSurface) Height() int {
	return s.height
}

// Clear fills the entire surface with the given color.
func (s *GPUSurface) Clear(c color.Color) {
	if s.closed || s.backend == nil {
		return
	}
	s.backend.Clear(c)
}

// Fill fills the given path using the specified style.
func (s *GPUSurface) Fill(path *Path, style FillStyle) {
	if s.closed || s.backend == nil || path == nil {
		return
	}
	s.backend.Fill(path, style)
}

// Stroke strokes the given path using the specified style.
func (s *GPUSurface) Stroke(path *Path, style StrokeStyle) {
	if s.closed || s.backend == nil || path == nil {
		return
	}
	s.backend.Stroke(path, style)
}

// DrawImage draws an image at the specified position.
func (s *GPUSurface) DrawImage(img image.Image, at Point, opts *DrawImageOptions) {
	if s.closed || s.backend == nil || img == nil {
		return
	}
	s.backend.DrawImage(img, at, opts)
}

// Flush ensures all pending operations are complete.
func (s *GPUSurface) Flush() error {
	if s.closed || s.backend == nil {
		return nil
	}
	return s.backend.Flush()
}

// Snapshot returns the current surface contents as an image.
// This performs a GPU readback, which may be slow.
func (s *GPUSurface) Snapshot() *image.RGBA {
	if s.closed || s.backend == nil {
		return nil
	}
	img, err := s.backend.Readback()
	if err != nil {
		return nil
	}
	return img
}

// Close releases all resources associated with the surface.
func (s *GPUSurface) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true

	if s.backend != nil {
		return s.backend.Close()
	}
	return nil
}

// Backend returns the underlying GPU backend.
// Returns nil if the surface is closed.
func (s *GPUSurface) Backend() GPUBackend {
	if s.closed {
		return nil
	}
	return s.backend
}

// Capabilities returns the surface capabilities.
func (s *GPUSurface) Capabilities() Capabilities {
	return Capabilities{
		SupportsSubSurface: false, // Depends on backend
		SupportsResize:     true,  // Most GPU surfaces support resize
		SupportsClipping:   true,  // GPU shaders support clipping
		SupportsBlendModes: true,  // GPU supports blend modes
		SupportsAntialias:  true,  // GPU supports MSAA or analytical AA
		MaxWidth:           16384, // Typical GPU texture limit
		MaxHeight:          16384,
	}
}

// Verify GPUSurface implements Surface interface.
var _ Surface = (*GPUSurface)(nil)
var _ CapableSurface = (*GPUSurface)(nil)
