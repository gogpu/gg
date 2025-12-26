// Package gg provides a simple 2D graphics library for Go.
//
// # Overview
//
// gg is a Pure Go 2D graphics library inspired by fogleman/gg and designed
// to integrate with the GoGPU ecosystem. It provides an immediate-mode drawing
// API similar to HTML Canvas, with both software and GPU rendering backends.
//
// # Quick Start
//
//	import "github.com/gogpu/gg"
//
//	// Create a drawing context (dc = drawing context convention)
//	dc := gg.NewContext(512, 512)
//
//	// Draw shapes
//	dc.SetRGB(1, 0, 0)
//	dc.DrawCircle(256, 256, 100)
//	dc.Fill()
//
//	// Save to PNG
//	dc.SavePNG("output.png")
//
// # API Compatibility
//
// The API is designed to be compatible with fogleman/gg for easy migration.
// Most fogleman/gg code should work with minimal changes.
//
// # Renderers
//
// The library includes both software and GPU-accelerated renderers:
//   - Software rasterizer for broad compatibility
//   - GPU renderer via gogpu/wgpu for high performance
//
// # Architecture
//
// The library is organized into:
//   - Public API: Context, Path, Paint, Matrix, Point
//   - Internal: raster (scanline), path (tessellation), blend (compositing)
//   - Renderers: software, gpu (wgpu)
//
// # Coordinate System
//
// Uses standard computer graphics coordinates:
//   - Origin (0,0) at top-left
//   - X increases right
//   - Y increases down
//   - Angles in radians, 0 is right, increases counter-clockwise
//
// # Performance
//
// The software renderer prioritizes correctness.
// For performance-critical applications, use the GPU renderer.
package gg
