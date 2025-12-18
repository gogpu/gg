// Package wgpu provides a GPU-accelerated rendering backend using gogpu/wgpu.
//
// This backend leverages WebGPU for hardware-accelerated 2D graphics rendering.
// It uses the gogpu/wgpu Pure Go WebGPU implementation, which supports Vulkan,
// Metal, and DX12 backends depending on the platform.
//
// # Architecture Overview
//
// The wgpu backend implements a complete GPU rendering pipeline:
//
//	Scene Commands -> Decoder -> Tessellator -> Strip Buffer -> GPU -> Composite
//
// Key components:
//
//   - WGPUBackend: Main entry point implementing backend.RenderBackend
//   - GPUSceneRenderer: Scene-to-GPU pipeline with tessellation and compositing
//   - MemoryManager: GPU texture memory with LRU eviction (configurable budget)
//   - TextureAtlas: Shelf-packing for efficient GPU memory usage
//   - Tessellator: Path-to-strips using Active Edge Table algorithm
//   - PipelineCache: Pre-compiled GPU pipelines for all 29 blend modes
//   - ShaderModules: WGSL shaders for strip rasterization and blending
//
// # Sparse Strips Algorithm
//
// Scene rendering uses the Sparse Strips algorithm optimized for GPU execution:
//
//  1. Paths are tessellated into horizontal coverage strips (one per scanline)
//  2. Each strip stores anti-aliased coverage values (0-255) for a contiguous range
//  3. Adjacent strips on the same row are merged for efficiency
//  4. GPU compute shaders rasterize strips to textures in parallel
//  5. Layer compositing uses fragment shaders with blend modes
//
// Example strip data for a circle:
//
//	Y=10, X=45, Width=10, Coverage=[32, 128, 255, 255, 255, 255, 255, 128, 32, 8]
//	Y=11, X=43, Width=14, Coverage=[64, 192, 255, ... ]
//	... (one strip per visible scanline)
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
// # Current Status (v0.9.0)
//
// The GPU pipeline implementation is complete. The following components are
// fully implemented and tested:
//
//   - Path tessellation to sparse strips
//   - Strip buffer packing for GPU upload
//   - Active Edge Table scanline conversion
//   - Fill rule support (NonZero, EvenOdd)
//   - Anti-aliased coverage calculation
//   - Pipeline cache for all blend modes
//   - WGSL shader generation
//   - Memory management with LRU eviction
//   - Layer stack management
//   - Clip region support
//
// GPU operations currently run as stubs that prepare all data but don't
// execute actual GPU commands. This will be enabled when gogpu/wgpu
// implements the remaining core functionality:
//
//   - Texture readback (for downloading GPU results to CPU)
//   - Buffer mapping (for uploading vertex/uniform data)
//   - Command buffer execution
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
//   - BenchmarkTessellation: Path to strips conversion
//   - BenchmarkStripPacking: GPU data preparation
//   - BenchmarkClear1080p: Full canvas clear comparison
//   - BenchmarkRect100: 100 rectangles comparison
//   - BenchmarkLayers4: Layer compositing comparison
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
