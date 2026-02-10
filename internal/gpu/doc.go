//go:build !nogpu

// Package gpu provides a Pure Go GPU-accelerated rendering backend.
//
// This is an internal package used by the gg library for GPU rendering.
// It leverages WebGPU for hardware-accelerated 2D graphics rendering
// via the gogpu/wgpu Pure Go WebGPU implementation (zero CGO), which supports
// Vulkan, Metal, and DX12 backends depending on the platform.
//
// # Architecture Overview
//
// The gpu backend implements a vello-style GPU rendering pipeline:
//
//	Scene Commands -> Decoder -> HybridPipeline (Flatten -> Coarse -> Fine) -> GPU -> Composite
//
// Key components:
//
//   - Backend: Main entry point for GPU rendering
//   - GPUSceneRenderer: Scene-to-GPU pipeline with HybridPipeline rasterization
//   - HybridPipeline: 3-stage path rasterization (Flatten, Coarse, Fine)
//   - MemoryManager: GPU texture memory with LRU eviction (configurable budget)
//   - TextureAtlas: Shelf-packing for efficient GPU memory usage
//   - PipelineCache: Pre-compiled GPU pipelines for all 29 blend modes
//   - ShaderModules: WGSL compute shaders for tile rasterization and blending
//
// # HybridPipeline (vello-style)
//
// Scene rendering uses a 3-stage tile-based pipeline inspired by Linebender's vello:
//
//  1. Flatten: Bezier curves are flattened to line segments using Wang's formula
//  2. Coarse: Line segments are binned into 4x4 pixel tiles with winding info
//  3. Fine: Each tile's coverage is computed with anti-aliased edges
//
// The pipeline automatically selects GPU or CPU execution per stage based on
// workload size. This hybrid approach provides optimal performance across
// different path complexities.
//
// # Blend Modes
//
// All 29 standard blend modes are supported via WGSL shaders:
//
// Standard modes:
//   - Normal, Multiply, Screen, Overlay
//   - Darken, Lighten, ColorDodge, ColorBurn
//   - HardLight, SoftLight, Difference, Exclusion
//
// HSL modes:
//   - Hue, Saturation, Color, Luminosity
//
// Porter-Duff compositing:
//   - Clear, Copy, Destination
//   - SourceOver, DestinationOver
//   - SourceIn, DestinationIn
//   - SourceOut, DestinationOut
//   - SourceAtop, DestinationAtop
//   - Xor, Plus
//
// # Usage
//
// Create and initialize the gpu backend directly:
//
//	b := gpu.NewBackend()
//	if err := b.Init(); err != nil {
//	    log.Fatal(err)
//	}
//	defer b.Close()
//
// # Rendering Scenes
//
// Build and render a scene:
//
//	builder := scene.NewSceneBuilder()
//	builder.FillRect(0, 0, 800, 600, scene.SolidBrush(gg.White))
//	builder.FillCircle(400, 300, 100, scene.SolidBrush(gg.Red))
//	s := builder.Build()
//
//	pm := gg.NewPixmap(800, 600)
//	if err := b.RenderScene(pm, s); err != nil {
//	    log.Printf("Render error: %v", err)
//	}
//
// # Memory Management
//
// The backend uses an LRU-based memory manager with configurable budget:
//
//	config := GPUSceneRendererConfig{
//	    Width:          1920,
//	    Height:         1080,
//	    MaxLayers:      16,
//	    MemoryBudgetMB: 256,
//	}
//
// When memory budget is exceeded, least-recently-used textures are evicted.
//
// # Requirements
//
//   - Go 1.25+ (for generic features)
//   - gogpu/wgpu module (github.com/gogpu/wgpu)
//   - A GPU that supports Vulkan, Metal, or DX12 (for actual GPU rendering)
//
// # Thread Safety
//
// Backend and GPUSceneRenderer are safe for concurrent use from multiple
// goroutines. Internal synchronization is handled via mutexes.
//
// # Error Handling
//
// Common errors returned by this package:
//
//   - ErrNotInitialized: Backend must be initialized before use
//   - ErrNoGPU: No compatible GPU found
//   - ErrDeviceLost: GPU device was lost (requires re-initialization)
//   - ErrNilTarget: Target pixmap is nil
//   - ErrNilScene: Scene is nil
//   - ErrRendererClosed: Renderer has been closed
//   - ErrEmptyScene: Scene contains no draw commands
//
// # Benchmarking
//
// Run benchmarks to compare GPU vs Software performance:
//
//	go test -bench=. ./internal/gpu/...
//
// # Related Packages
//
//   - github.com/gogpu/gg: Core 2D graphics library
//   - github.com/gogpu/gg/scene: Scene graph and retained mode API
//   - github.com/gogpu/wgpu: Pure Go WebGPU implementation
//
// # References
//
//   - W3C WebGPU Specification: https://www.w3.org/TR/webgpu/
//   - gogpu Organization: https://github.com/gogpu
//   - gogpu/wgpu: https://github.com/gogpu/wgpu
package gpu
