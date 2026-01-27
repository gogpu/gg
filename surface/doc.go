// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Package surface provides a unified surface abstraction for 2D rendering.
//
// Surface is the core rendering target abstraction that decouples drawing
// operations from their implementation. This allows the same drawing code
// to work with:
//
//   - CPU-based software rendering (ImageSurface)
//   - GPU-accelerated rendering (GPUSurface)
//   - Third-party backends via registry
//
// # Architecture
//
// The surface package follows the Cairo/Skia pattern where surfaces are
// rendering targets independent of the drawing context. This separation
// enables:
//
//   - Backend switching without code changes
//   - Testing with mock surfaces
//   - Third-party backend integration (RFC #46)
//
// # Surface Types
//
//   - ImageSurface: CPU-based rendering to *image.RGBA using core.AnalyticFiller
//   - GPUSurface: GPU-accelerated rendering wrapper (requires external device)
//
// # Registry (RFC #46)
//
// Third-party backends can register surfaces via the registry:
//
//	surface.Register("vulkan", func(w, h int, opts Options) (Surface, error) {
//	    return NewVulkanSurface(w, h)
//	})
//
//	// Later:
//	s, err := surface.NewSurfaceByName("vulkan", 800, 600, nil)
//
// # Usage
//
// Basic usage with ImageSurface:
//
//	// Create a CPU-based surface
//	s := surface.NewImageSurface(800, 600)
//	defer s.Close()
//
//	// Clear with white background
//	s.Clear(color.White)
//
//	// Create a path
//	path := surface.NewPath()
//	path.MoveTo(100, 100)
//	path.LineTo(200, 100)
//	path.LineTo(150, 200)
//	path.Close()
//
//	// Fill with red
//	s.Fill(path, surface.FillStyle{
//	    Color: color.RGBA{255, 0, 0, 255},
//	    Rule:  surface.FillRuleNonZero,
//	})
//
//	// Get the result
//	img := s.Snapshot()
//
// # Integration with gg.Context
//
// The surface package is designed to integrate with gg.Context (see INT-001).
// Context will use Surface as its rendering target, allowing backend switching.
//
// # References
//
//   - Cairo: https://cairographics.org/manual/cairo-Image-Surfaces.html
//   - Skia: https://skia.org/docs/user/api/skcanvas_overview/
//   - RFC #46: Third-party backend support
package surface
