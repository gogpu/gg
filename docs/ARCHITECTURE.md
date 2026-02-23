# gg Architecture

This document describes the architecture of the gg 2D graphics library.

## Overview

gg is a 2D graphics library for Go, inspired by HTML5 Canvas API and modern Rust 2D graphics libraries (vello, tiny-skia).

**Core principle (v0.26.0): CPU raster is the foundation, GPU is an optional accelerator.**

```
                        ┌───────────────────┐
                        │ User Application  │
                        └─────────┬─────────┘
                                  │
                           ┌──────▼──────┐
                           │     gg      │
                           │  2D Canvas  │
                           └──────┬──────┘
                                  │
                    ┌─────────────┼─────────────┐
                    │                           │
             ┌──────▼──────┐             ┌──────▼──────┐
             │  CPU Raster │             │    GPU      │
             │  (default)  │             │ Accelerator │
             └──────┬──────┘             └──────┬──────┘
                    │                           │ (optional, 4 tiers)
             ┌──────▼──────┐             ┌──────▼──────┐
             │  internal/  │             │  internal/  │
             │   raster    │             │    gpu      │
             └─────────────┘             └──────┬──────┘
                                                │
                                         ┌──────▼──────┐
                                         │    wgpu     │
                                         └──────┬──────┘
                                                │
                                  ┌──────┬──────┼──────┬──────┐
                                  │      │      │      │      │
                               ┌──▼──┐┌──▼──┐┌──▼──┐┌──▼──┐┌──▼──┐
                               │ Vk  ││DX12 ││Metal││GLES ││Soft │
                               └─────┘└─────┘└─────┘└─────┘└─────┘
                                            wgpu/hal
```

## GPU Accelerator (v0.26.0+)

GPU acceleration is opt-in via the `GPUAccelerator` interface:

```go
// GPUAccelerator is an optional GPU acceleration provider.
type GPUAccelerator interface {
    Name() string
    Init() error
    Close()
    CanAccelerate(op AcceleratedOp) bool
    FillPath(target GPURenderTarget, path *Path, paint *Paint) error
    StrokePath(target GPURenderTarget, path *Path, paint *Paint) error
    FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error
    StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error
    Flush(target GPURenderTarget) error
}

// Register via blank import pattern
import _ "github.com/gogpu/gg/gpu"  // enables GPU acceleration
```

Optional extension interfaces for gogpu integration:

- **DeviceProviderAware** -- share GPU device with an external provider (e.g., gogpu window)
- **SurfaceTargetAware** -- render directly to a surface texture view (zero-copy windowed rendering)

### Four-Tier GPU Rendering

The GPU accelerator in `internal/gpu/` uses a unified render session (`GPURenderSession`) that
dispatches shapes and text to four rendering tiers within a single render pass:

| Tier | Name | Content | Technique |
|------|------|---------|-----------|
| **1** | SDF | Circles, ellipses, rounded rects | SDF shader evaluation per-pixel |
| **2a** | Convex | Convex polygons | Fan tessellation (no stencil needed) |
| **2b** | Stencil+Cover | Arbitrary paths | Stencil buffer for winding, then cover pass |
| **4** | MSDF Text | Text glyphs | Multi-channel SDF with median+smoothstep shader |

This mirrors enterprise engines (Skia Ganesh/Graphite, Flutter Impeller, Gio):
one render pass, multiple pipeline switches.

Key design:
- `RegisterAccelerator()` for opt-in GPU
- `ErrFallbackToCPU` sentinel error for graceful degradation
- `AcceleratedOp` bitfield for capability checking
- Shape detection (`DetectShape`) auto-identifies circles, rects, rounded rects from path data
- `Flush()` dispatches batched GPU operations in a single pass
- Two render modes: offscreen (readback to CPU) and surface (zero-copy to window)
- CPU raster always available as fallback

## Package Structure

```
gg/
├── context.go              # Canvas-like drawing context
├── software.go             # SoftwareRenderer (uses internal/raster directly)
├── accelerator.go          # GPUAccelerator interface + registration
├── sdf.go                  # CPU SDF coverage functions (circles, ellipses, rects)
├── sdf_accelerator.go      # SDFAccelerator (CPU-based SDF for simple shapes)
├── shape_detect.go         # DetectShape: auto-detect circles/rects/rrects from paths
├── options.go              # Configuration options
├── path.go                 # Vector path operations
├── paint.go                # Fill and stroke styles
├── pixmap.go               # Pixel buffer operations
├── text.go                 # Text rendering
│
├── gpu/                    # PUBLIC opt-in GPU registration (blank import)
│   └── gpu.go              # init() registers internal/gpu accelerator
│
├── integration/
│   └── ggcanvas/           # gogpu integration (Canvas for windowed rendering)
│       ├── canvas.go       # Canvas with Draw(), Flush(), Resize()
│       └── render.go       # RenderTo, RenderDirect (zero-copy)
│
├── render/                 # Cross-package rendering
│   ├── scene.go            # Scene with damage tracking
│   ├── software.go         # Software renderer
│   ├── layers.go           # LayeredTarget for z-ordering
│   ├── device.go           # DeviceHandle (gpucontext.DeviceProvider)
│   ├── target.go           # Render target abstraction
│   ├── renderer.go         # Renderer interface
│   └── gpu_renderer.go     # GPU-accelerated renderer
│
├── internal/
│   ├── raster/             # CPU rasterization core
│   │   ├── edge.go         # Line edge types
│   │   ├── edge_builder.go # Path to typed edges conversion
│   │   ├── alpha_runs.go   # RLE-encoded coverage buffer
│   │   ├── curve_edge.go   # QuadraticEdge, CubicEdge (forward diff)
│   │   ├── curve_aet.go    # CurveAwareAET (Active Edge Table)
│   │   ├── analytic_filler.go  # Trapezoidal integration filler
│   │   ├── fixed.go        # FDot6, FDot16 fixed-point types
│   │   ├── path_geometry.go    # Y-monotonic curve chopping
│   │   └── scene_adapter.go   # Scene to raster bridge
│   │
│   ├── gpu/                # GPU rendering pipeline (four-tier)
│   │   ├── backend.go      # GPU backend implementation
│   │   ├── sdf_gpu.go      # SDFAccelerator (GPU-based, wgpu HAL)
│   │   ├── sdf_render.go   # SDF render pipeline (Tier 1)
│   │   ├── convex_renderer.go  # Convex polygon renderer (Tier 2a)
│   │   ├── convexity.go    # Convexity detection algorithm
│   │   ├── stencil_renderer.go # Stencil+Cover renderer (Tier 2b)
│   │   ├── stencil_pipeline.go # Stencil render pipeline setup
│   │   ├── render_session.go   # GPURenderSession (unified render pass)
│   │   ├── gpu_textures.go # MSAA + stencil + resolve texture management
│   │   ├── tessellate.go   # Fan tessellation for paths
│   │   ├── adapter.go      # Analytic AA adapter
│   │   ├── analytic_filler.go  # GPU-side analytic filler
│   │   ├── analytic_filler_vello.go  # Vello tile rasterizer
│   │   ├── vello_tiles.go  # 16x16 tile binning + DDA
│   │   ├── coarse.go       # Coarse rasterization pass
│   │   ├── fine.go         # Fine rasterization pass
│   │   ├── pipeline.go     # Render pipeline management
│   │   ├── pipeline_cache_core.go  # PipelineCache (FNV-1a)
│   │   ├── command_encoder.go  # CommandEncoder state machine
│   │   ├── texture.go      # Texture with lazy default view
│   │   ├── buffer.go       # Buffer with async mapping
│   │   ├── text_pipeline.go    # MSDF text rendering pipeline (Tier 4)
│   │   ├── scene_bridge.go # Scene to native bridge
│   │   └── shaders/        # WGSL shaders
│   │       ├── sdf_render.wgsl    # SDF shape rendering (Tier 1)
│   │       ├── convex.wgsl        # Convex polygon fill (Tier 2a)
│   │       ├── stencil_fill.wgsl  # Stencil fill pass (Tier 2b)
│   │       ├── cover.wgsl         # Cover pass (Tier 2b)
│   │       ├── fine.wgsl          # Fine rasterization
│   │       ├── coarse.wgsl        # Coarse rasterization
│   │       ├── flatten.wgsl       # Path flattening
│   │       ├── blend.wgsl         # Blending operations
│   │       ├── blit.wgsl          # Blit / copy
│   │       ├── composite.wgsl     # Compositing
│   │       ├── strip.wgsl         # Strip rendering
│   │       └── msdf_text.wgsl     # MSDF text rendering
│   │
│   ├── cache/              # LRU caching infrastructure
│   │   ├── cache.go        # Generic cache
│   │   ├── lru.go          # LRU eviction
│   │   └── sharded.go      # Sharded cache for concurrency
│   │
│   ├── gpucore/            # GPU core types and shaders
│   │   ├── adapter.go      # Core GPU adapter
│   │   ├── pipeline.go     # Core pipeline
│   │   ├── types.go        # Core GPU types
│   │   └── shaders/        # WGSL compute shaders
│   │
│   ├── blend/              # Color blending (29 modes)
│   ├── parallel/           # Parallel tile rendering
│   ├── wide/               # SIMD operations
│   ├── stroke/             # Stroke expansion (kurbo/tiny-skia)
│   └── filter/             # Blur, shadow, color matrix
│
├── scene/                 # Retained-mode scene graph
│   ├── scene.go           # Scene encoding (draw commands → byte stream)
│   ├── renderer.go        # Tile-parallel renderer (delegates to SoftwareRenderer)
│   ├── builder.go         # Scene builder API
│   ├── path.go            # Scene path type (float32)
│   └── tile.go            # Tile grid and dirty region tracking
│
├── recording/              # Drawing recording for vector export
│   ├── recorder.go         # Command-based drawing recorder
│   ├── command.go          # Typed command definitions
│   ├── backend.go          # Backend interface (Writer, File)
│   ├── registry.go         # Backend registration
│   └── backends/raster/    # Built-in raster backend
│
├── surface/                # Render surfaces
│   ├── image_surface.go    # Image-based surface
│   └── path.go             # Surface path utilities
│
├── text/                   # Text rendering
│   ├── shaper.go           # Pluggable shaper interface
│   ├── shaper_builtin.go   # Default shaper (basic LTR)
│   ├── shaper_gotext.go    # HarfBuzz shaper (go-text)
│   ├── layout.go           # Multi-line layout engine
│   ├── glyph_cache.go      # LRU glyph cache (16-shard)
│   ├── glyph_run.go        # GlyphRunBuilder for batching
│   ├── msdf/               # MSDF text rendering
│   └── emoji/              # Color emoji support
│
└── internal/image/          # Image I/O (PNG, JPEG, WebP)
```

## Scene Renderer (scene/)

Retained-mode scene graph with tile-based parallel rendering. The `scene.Renderer`
handles orchestration (tile grid, worker pool, dirty regions, layer cache) while
delegating pixel rendering to `gg.SoftwareRenderer`.

### Architecture

```
scene.Scene (encoded draw commands)
       │
       ▼
scene.Renderer (orchestration)
       │
       ├── TileGrid (64x64 tiles)
       ├── DirtyRegion tracking
       ├── WorkerPool (parallel tiles)
       └── LayerCache (inter-frame reuse)
              │
              ▼ (per-tile)
       gg.SoftwareRenderer  ◄── delegation (v0.29.4)
              │
              ▼
       internal/raster (analytic AA)
```

### Delegation Pattern (v0.29.4)

Following the universal pattern confirmed by Qt Quick, Skia, Vello, and Flutter/Impeller:
**scene graph orchestrates, immediate-mode backend rasterizes.**

Per-tile rendering:
1. Acquire `SoftwareRenderer` + `Pixmap` from `sync.Pool`
2. Decode scene commands (fill, stroke, transform, etc.)
3. Convert `scene.Path` (float32) → `gg.Path` (float64) with tile offset subtraction
4. Convert `scene.Brush` → `gg.Paint` (fill rule, stroke params)
5. Delegate: `sr.Fill(pm, path, paint)` / `sr.Stroke(pm, path, paint)`
6. Composite tile onto target with premultiplied source-over alpha blending
7. Return resources to pool

### Key Components

| Component | Purpose |
|-----------|---------|
| `scene.Scene` | Encodes draw commands into byte stream |
| `scene.Renderer` | Orchestrates tile-parallel rendering |
| `TileGrid` | 64x64 tile partitioning |
| `WorkerPool` | Goroutine pool for parallel tile rendering |
| `DirtyRegion` | Tracks changed areas to minimize re-rendering |
| `LayerCache` | Caches rendered layers between frames |
| `tilePool` | sync.Pool for per-tile SoftwareRenderer/Pixmap reuse |

## Vello Tile Rasterizer (v0.25.0)

Port of vello_shaders CPU fine rasterizer to Go. Used in `internal/gpu/`.

### Architecture

```
Path → EdgeBuilder → VelloLines → binSegments → 16x16 Tiles
                                                     │
                                               collectSegments
                                                     │
                                              Analytic Coverage
                                                     │
                                               Pixel Output
```

### Key Components

- **VelloLine**: stores original float32 coordinates from curve flattening, bypassing fixed-point quantization (FDot6/FDot16 round-trip)
- **binSegments**: DDA-based segment distribution into 16x16 tiles with backdrop tracking
- **yEdge mechanism**: correct winding number propagation via backdrop prefix sum
- **Analytic trapezoidal coverage**: exact per-pixel area calculation (no supersampling)

### Fill Rules

- **NonZero** (default): winding number != 0 → filled
- **EvenOdd**: winding number is odd → filled

## Analytic Anti-Aliasing

Enterprise-grade curve rendering using forward differencing and trapezoidal coverage calculation.

### Forward Differencing Edges

```go
// O(1) per step - only additions, no multiplications
type QuadraticEdge struct {
    fFirstX, fFirstY FDot16  // Current position (fixed-point)
    fDx, fDy         FDot16  // First derivative
    fDDx, fDDy       FDot16  // Second derivative (constant)
    fLastY           int     // End scanline
}
```

### Fixed-Point Arithmetic

| Type | Format | Precision | Use Case |
|------|--------|-----------|----------|
| `FDot6` | 26.6 | 1/64 px | Y coordinates |
| `FDot16` | 16.16 | 1/65536 px | X coordinates, derivatives |
| `FDot8` | 24.8 | 1/256 | Coverage values |

## Stroke Expansion (internal/stroke)

Converts stroked paths to filled outlines using the kurbo/tiny-skia algorithm:

- **Line Caps**: Butt, Round, Square
- **Line Joins**: Miter (with limit), Round, Bevel
- **Curves**: Quadratic and cubic Bezier flattening with tolerance

## gogpu Integration (integration/ggcanvas)

The `integration/ggcanvas/` package bridges gg with gogpu for windowed rendering:

```go
import "github.com/gogpu/gg/integration/ggcanvas"

canvas := ggcanvas.New(provider, width, height)
// canvas auto-registers with App.TrackResource() — no manual Close needed

// Draw() marks canvas dirty atomically — recommended pattern:
canvas.Draw(func(dc *gg.Context) {
    dc.DrawCircle(400, 300, 100)
    dc.Fill()
})

// Zero-copy surface rendering (gg draws directly to window surface):
canvas.RenderDirect(surfaceView, surfaceWidth, surfaceHeight)

// Or readback-based rendering (GPU -> CPU -> texture):
canvas.RenderTo(drawContext)
```

Key implementation details:

- **`Draw()` helper** — draws with `gg.Context` and marks dirty atomically,
  skipping GPU upload when content is unchanged
- **Deferred texture destruction** — `Resize()` sets a `sizeChanged` flag instead
  of destroying the texture immediately, preventing DX12 descriptor heap issues
- **Porter-Duff compositing** — GPU readback uses "over" compositing
  (`compositeBGRAOverRGBA`) for correct multi-flush blending
- **Auto-registration** — Canvas detects if the provider implements
  `TrackResource(io.Closer)` (duck-typed interface) and auto-registers.
  On shutdown, gogpu closes all tracked resources in LIFO order — no manual
  `defer canvas.Close()` or `OnClose` wiring needed.

When used with gogpu, the accelerator shares the gogpu GPU device via `DeviceProviderAware`,
and can render directly to the window surface via `SurfaceTargetAware`, eliminating the
GPU->CPU->GPU round-trip.

## Recording System (v0.23.0)

Command-based drawing recording for vector export (Cairo/Skia-inspired).

```
User Code → Recorder → Commands → Recording → Backend → Output
                          ↓
                    ResourcePool
                   (paths, brushes)
```

### Available Backends

| Backend | Package | Format |
|---------|---------|--------|
| **Raster** | `recording/raster` | Built-in PNG/image |
| **PDF** | `github.com/gogpu/gg-pdf` | PDF documents |
| **SVG** | `github.com/gogpu/gg-svg` | SVG vector graphics |

## Relationship to gogpu Ecosystem

```
                    gpucontext (shared interfaces)
                           │
naga (shader compiler)     │
  │                        │
  └──► wgpu ◄──────────────┤
         │                 │
         ├──► gogpu ───────┤ (implements DeviceProvider)
         │                 │
         └──► gg ──────────┘ (consumes DeviceProvider)
                ↑
          this project
                │
         ┌──────┴──────┐
         │             │
      gg-pdf        gg-svg
    (PDF export)  (SVG export)
```

gg and gogpu are **independent libraries** that can interoperate via gpucontext:

| Aspect                | gg                    | gogpu                |
|-----------------------|-----------------------|----------------------|
| **Purpose**           | 2D graphics library   | GPU framework        |
| **CPU rendering**     | Built-in (core)       | No                   |
| **GPU rendering**     | Optional accelerator  | Primary              |
| **Dependencies**      | wgpu, naga, gpucontext | wgpu, gpucontext   |
| **gpucontext role**   | Consumer              | Provider             |

## Key Design Patterns

| Pattern | Source | Implementation |
|---------|--------|----------------|
| **Scene Delegation** | Qt/Skia/Vello/Flutter | Scene orchestrates tiles, SoftwareRenderer rasterizes |
| **GPU Accelerator** | gg v0.26.0 | Opt-in GPU via `import _ "github.com/gogpu/gg/gpu"` |
| **Four-Tier Rendering** | Skia Ganesh/Impeller | SDF, convex, stencil+cover, MSDF text in one render pass |
| **SDF Shape Rendering** | Shadertoy/GPU Gems | Per-pixel signed distance field for circles/rrects |
| **Stencil-Then-Cover** | GPU Gems 3, NV_path_rendering | Winding via stencil buffer, then cover fill |
| **Fan Tessellation** | Skia Ganesh | Convex path to triangle fan for GPU |
| **Shape Detection** | gg | Auto-detect circle/rect/rrect from path elements |
| **Lazy Default View** | wgpu-rs | `sync.Once` for thread-safe texture view |
| **State Machine** | wgpu | Command encoder lifecycle |
| **FNV-1a Hashing** | wgpu-core | Pipeline cache key generation |
| **Serial-Based LRU** | vello | Glyph cache eviction |
| **Stroke Expansion** | kurbo/tiny-skia | Forward/backward offset paths |
| **Forward Differencing** | Skia | O(1) curve edge stepping |
| **Fixed-Point Math** | Skia | FDot6/FDot16 sub-pixel precision |
| **Trapezoidal Coverage** | vello | Exact per-pixel AA calculation |
| **Tile Rasterization** | vello | 16x16 tile binning + DDA |
| **Command Pattern** | Cairo/Skia | Recording system for vector export |
| **Driver Pattern** | database/sql | Backend registration via blank import |
| **Device Sharing** | Skia Graphite | DeviceProviderAware for gogpu integration |
| **Zero-Copy Surface** | Flutter Impeller | SurfaceTargetAware for direct window rendering |

## See Also

- [README.md](../README.md) — Quick start guide
- [CHANGELOG.md](../CHANGELOG.md) — Version history
- [ROADMAP.md](../ROADMAP.md) — Development milestones
- [Examples](../examples/) — Code examples
