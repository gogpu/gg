// Package wgpu provides a GPU-accelerated rendering backend using gogpu/wgpu.
//
// This backend leverages WebGPU for hardware-accelerated 2D graphics rendering.
// It uses the gogpu/wgpu Pure Go WebGPU implementation, which supports Vulkan,
// Metal, and DX12 backends depending on the platform.
//
// # Architecture Overview
//
// The wgpu backend implements a vello-style GPU rendering pipeline:
//
//	Scene Commands -> Decoder -> HybridPipeline (Flatten → Coarse → Fine) -> GPU -> Composite
//
// Key components:
//
//   - WGPUBackend: Main entry point implementing backend.RenderBackend
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
//  2. Coarse: Line segments are binned into 4×4 pixel tiles with winding info
//  3. Fine: Each tile's coverage is computed with anti-aliased edges
//
// The pipeline automatically selects GPU or CPU execution per stage based on
// workload size. This hybrid approach provides optimal performance across
// different path complexities.
//
// Example tile data for a circle:
//
//	Tile(X=10, Y=5): Coverage[16]=[32, 128, 255, 255, ...]  // 4×4 pixel coverage
//	Tile(X=11, Y=5): Coverage[16]=[255, 255, 255, 192, ...] // Adjacent tile
//	... (sparse tiles only where path intersects)
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
// # Registration and Selection
//
// The wgpu backend is automatically registered when this package is imported:
//
//	import _ "github.com/gogpu/gg/backend/wgpu"
//
// The backend will be preferred over the software backend when available.
// If GPU initialization fails, the system will fall back to software rendering.
//
// # Basic Usage
//
// Automatic backend selection (recommended):
//
//	b := backend.Default()  // Returns wgpu if available, otherwise software
//	if err := b.Init(); err != nil {
//	    log.Fatal(err)
//	}
//	defer b.Close()
//
// Explicit wgpu backend selection:
//
//	b := backend.Get(backend.BackendWGPU)
//	if b == nil {
//	    log.Fatal("wgpu backend not available")
//	}
//	if err := b.Init(); err != nil {
//	    log.Fatal(err)
//	}
//	defer b.Close()
//
// # Rendering Scenes
//
// Build and render a scene:
//
//	// Create scene using SceneBuilder
//	builder := scene.NewSceneBuilder()
//	builder.FillRect(0, 0, 800, 600, scene.SolidBrush(gg.White))
//	builder.FillCircle(400, 300, 100, scene.SolidBrush(gg.Red))
//
//	// Add a blended layer
//	builder.Layer(scene.BlendMultiply, 0.8, nil, func(lb *scene.SceneBuilder) {
//	    lb.FillRect(300, 200, 200, 200, scene.SolidBrush(gg.Blue))
//	})
//
//	s := builder.Build()
//
//	// Render to pixmap
//	pm := gg.NewPixmap(800, 600)
//	if err := b.RenderScene(pm, s); err != nil {
//	    log.Printf("Render error: %v", err)
//	}
//
//	// Save result
//	pm.SavePNG("output.png")
//
// # Direct Scene Construction
//
// Lower-level scene construction:
//
//	s := scene.NewScene()
//	rect := scene.NewRectShape(10, 10, 80, 80)
//	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
//
//	circle := scene.NewCircleShape(50, 50, 30)
//	s.Fill(scene.FillNonZero, scene.TranslateAffine(100, 0), scene.SolidBrush(gg.Blue), circle)
//
//	pm := gg.NewPixmap(200, 100)
//	b.RenderScene(pm, s)
//
// # Performance Characteristics
//
// The GPU backend excels at:
//   - Large canvases (1080p and above)
//   - Many shapes with the same blend mode
//   - Complex layer compositing
//   - Parallel processing of independent draws
//
// Software backend may be faster for:
//   - Small canvases (< 256x256)
//   - Single shape renders
//   - Frequent GPU-CPU data transfers
//
// # Memory Management
//
// The backend uses an LRU-based memory manager with configurable budget:
//
//	// Configure via GPUSceneRendererConfig
//	config := GPUSceneRendererConfig{
//	    Width:          1920,
//	    Height:         1080,
//	    MaxLayers:      16,     // Maximum layer stack depth
//	    MemoryBudgetMB: 256,    // GPU texture memory budget
//	}
//
// When memory budget is exceeded, least-recently-used textures are evicted.
//
// # Current Status (v0.16.0)
//
// The GPU pipeline uses vello-style HybridPipeline architecture.
// The following components are fully implemented and tested:
//
//   - HybridPipeline: 3-stage path rasterization (Flatten → Coarse → Fine)
//   - Tile-based sparse coverage (4×4 pixel tiles)
//   - GPU/CPU automatic selection per stage
//   - Fill rule support (NonZero, EvenOdd)
//   - Anti-aliased coverage calculation
//   - Pipeline cache for all blend modes
//   - WGSL compute shaders (flatten.wgsl, coarse.wgsl, fine.wgsl)
//   - Memory management with LRU eviction
//   - Layer stack management
//   - Clip region support
//
// GPU compute shader execution uses CPU fallback until HAL bridge is complete.
// This will be enabled when core↔HAL device/queue bridge is implemented:
//
//   - HAL device/queue wiring to HybridPipeline
//   - Texture readback (for downloading GPU results to CPU)
//   - Buffer mapping (for uploading vertex/uniform data)
//
// All data flow through the pipeline is correct and tested.
//
// # Requirements
//
//   - Go 1.25+ (for generic features)
//   - gogpu/wgpu module (github.com/gogpu/wgpu)
//   - A GPU that supports Vulkan, Metal, or DX12 (for actual GPU rendering)
//
// # Thread Safety
//
// WGPUBackend and GPUSceneRenderer are safe for concurrent use from multiple
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
//	go test -bench=. ./backend/wgpu/...
//
// Key benchmarks:
//   - BenchmarkClear1080p: Full canvas clear comparison
//   - BenchmarkRect100: 100 rectangles comparison
//   - BenchmarkCircle50: Circle rendering comparison
//   - BenchmarkLayers4: Layer compositing comparison
//   - BenchmarkPipelineCreation: GPU pipeline cache performance
//
// # Related Packages
//
//   - github.com/gogpu/gg: Core 2D graphics library
//   - github.com/gogpu/gg/scene: Scene graph and retained mode API
//   - github.com/gogpu/gg/backend: Backend interface and registry
//   - github.com/gogpu/wgpu: Pure Go WebGPU implementation
//
// # References
//
//   - W3C WebGPU Specification: https://www.w3.org/TR/webgpu/
//   - gogpu Organization: https://github.com/gogpu
//   - gogpu/wgpu: https://github.com/gogpu/wgpu
package wgpu
