// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Package render provides the integration layer between gg and GPU frameworks.
//
// This package defines the core abstractions for device integration, allowing
// gg to render to GPU surfaces provided by host applications (like gogpu.App).
//
// # Key Principle
//
// gg RECEIVES a GPU device from the host application, it does NOT create its own.
// This follows the Vello/femtovg/Skia pattern where the rendering library is
// injected with GPU resources rather than managing them itself.
//
// # Core Interfaces
//
//   - DeviceHandle: Provides GPU device access from the host application
//   - RenderTarget: Defines where rendering output goes (Pixmap, Texture, Surface)
//   - Renderer: Executes drawing commands to a target
//   - EventSource: Input events for UI integration (future)
//
// # Renderer Implementations
//
//   - SoftwareRenderer: CPU-based rendering using the core/ package
//   - GPURenderer: GPU-accelerated rendering (stub for Phase 3)
//
// # RenderTarget Implementations
//
//   - PixmapTarget: CPU-backed *image.RGBA target
//   - TextureTarget: GPU texture target (stub)
//   - SurfaceTarget: Window surface from host (stub)
//
// # Usage
//
// Integration with gogpu:
//
//	app := gogpu.NewApp(gogpu.Config{...})
//	var renderer render.Renderer
//	var scene *Scene
//
//	app.OnInit(func(gc *gogpu.Context) {
//	    // gg receives GPU device from gogpu (zero overhead)
//	    renderer, _ = render.NewGPURenderer(gc.DeviceHandle())
//
//	    // Build scene (retained mode)
//	    scene = NewScene()
//	    scene.SetFillColor(Red)
//	    scene.Circle(100, 100, 50)
//	    scene.Fill()
//	})
//
//	app.OnDraw(func(gc *gogpu.Context) {
//	    // Render scene to window surface (zero-copy)
//	    renderer.Render(gc.SurfaceTarget(), scene)
//	})
//
// Software rendering fallback:
//
//	// Create CPU-backed target
//	target := render.NewPixmapTarget(800, 600)
//
//	// Create software renderer
//	renderer := render.NewSoftwareRenderer()
//
//	// Render scene
//	renderer.Render(target, scene)
//
//	// Get rendered image
//	img := target.Image()
//
// # Architecture
//
//	                 User Application
//	                       │
//	      ┌────────────────┼────────────────┐
//	      │                │                │
//	      ▼                ▼                ▼
//	 gogpu.App       gg.Context         gg.Scene
//	 (windowing)     (immediate)        (retained)
//	      │                │                │
//	      └────────────────┼────────────────┘
//	                       │
//	                       ▼
//	               gg/render package
//	      ┌────────────────┼────────────────┐
//	      │                │                │
//	      ▼                ▼                ▼
//	DeviceHandle     RenderTarget       Renderer
//	(GPU access)    (output target)   (execution)
//	      │                │                │
//	      └────────────────┼────────────────┘
//	                       │
//	                       ▼
//	               gg/core package
//	               (CPU rasterization)
//
// # Thread Safety
//
// Renderers are NOT thread-safe. Each renderer should be used from a single
// goroutine, or external synchronization must be used.
//
// # References
//
//   - Vello DeviceProvider pattern: https://github.com/AhornGraphics/vello
//   - femtovg Renderer trait: https://github.com/AhornGraphics/femtovg
//   - Skia GrDirectContext: https://skia.org/docs/user/api/
package render
