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
                    │                           │ (optional, 6 tiers)
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

## GPU Accelerator (v0.26.0+, Compute v0.30.0)

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
- **Per-pass render target** -- `GPURenderTarget.View` (`gpucontext.TextureView`) enables GPU-direct rendering to any texture view (surface or offscreen). Follows WebGPU spec per-render-pass target pattern. ~~SurfaceTargetAware~~ (deprecated, use GPURenderTarget.View)

### Six-Tier GPU Rendering

The GPU accelerator in `internal/gpu/` uses a unified render session (`GPURenderSession`) that
dispatches shapes and text to six rendering tiers:

| Tier | Name | Content | Technique |
|------|------|---------|-----------|
| **1** | SDF | Circles, ellipses, rounded rects | SDF shader evaluation per-pixel |
| **2a** | Convex | Convex polygons | Fan tessellation (no stencil needed) |
| **2b** | Stencil+Cover | Arbitrary paths | Stencil buffer for winding, then cover pass |
| **4** | MSDF Text | Text glyphs (dynamic/animated) | Multi-channel SDF with median+smoothstep shader |
| **5** | Compute | Full scenes (many paths) | Vello-style 9-stage compute pipeline (GPU or CPU fallback) |
| **6** | Glyph Mask | Text glyphs (static/UI, ≤48px) | CPU-rasterized R8 alpha atlas, GPU textured quads, ClearType LCD subpixel, font hinting |

Tiers 1–4, 6 use a render-pass pipeline (one render pass, multiple pipeline switches).
Tier 5 uses a compute-only pipeline (9 dispatch stages, no render pass).
Auto-selection routes horizontal text ≤48px to Tier 6 (pixel-perfect with font hinting and optional ClearType LCD subpixel rendering), else Tier 4 (scalable MSDF).

This mirrors enterprise engines (Skia Ganesh/Graphite, Flutter Impeller, Gio).

Key design:
- `RegisterAccelerator()` for opt-in GPU
- `ErrFallbackToCPU` sentinel error for graceful degradation
- `AcceleratedOp` bitfield for capability checking
- Shape detection (`DetectShape`) auto-identifies circles, rects, rounded rects from path data
- `Flush()` dispatches batched GPU operations in a single pass
- Two render modes: offscreen (readback to CPU) and surface (zero-copy to window)
- `PipelineMode` (Auto/RenderPass/Compute) selects the appropriate pipeline
- CPU raster always available as fallback

### Vello Compute Pipeline (Tier 5, v0.30.0)

The compute pipeline is a port of [vello](https://github.com/linebender/vello)'s
9-stage GPU compute architecture to Go. It processes entire scenes (many paths)
in parallel on the GPU using compute shaders.

#### 9-Stage Pipeline

```
Scene → [1] pathtag_reduce → [2] pathtag_scan → [3] draw_reduce → [4] draw_leaf
                                                                        │
Output ← [9] fine ← [8] path_tiling ← [7] coarse ← [6] backdrop ← [5] path_count
```

| Stage | Shader | Purpose |
|-------|--------|---------|
| 1 | `pathtag_reduce.wgsl` | Parallel reduction of path tag monoids |
| 2 | `pathtag_scan.wgsl` | Prefix scan producing cumulative offsets |
| 3 | `draw_reduce.wgsl` | Parallel reduction of draw monoids |
| 4 | `draw_leaf.wgsl` | Prefix scan + draw info extraction |
| 5 | `path_count.wgsl` | Per-tile segment counting + line flattening |
| 6 | `backdrop.wgsl` | Left-to-right backdrop prefix sum per row |
| 7 | `coarse.wgsl` | Per-tile command list (PTCL) generation |
| 8 | `path_tiling.wgsl` | Segment clipping and tile assignment |
| 9 | `fine.wgsl` | Per-pixel rasterization (16×16 tiles, 64 threads × 4 pixels each) |

#### PipelineMode Selection

`PipelineMode` controls which GPU pipeline is used:

| Mode | Behavior |
|------|----------|
| `PipelineModeAuto` | Auto-select based on scene complexity heuristics |
| `PipelineModeRenderPass` | Force render-pass pipeline (Tiers 1–4) |
| `PipelineModeCompute` | Force compute pipeline (Tier 5) |

In Auto mode, the compute pipeline is preferred for scenes with many paths,
while the render-pass pipeline is preferred for simple scenes with few shapes.

#### CPU Reference Implementation

The `tilecompute/` package contains a complete CPU reference implementation
of the 9-stage pipeline. `RasterizeScenePTCL()` runs the full pipeline on
CPU, producing identical output to the GPU compute shaders. This enables:
- Golden tests (GPU vs CPU pixel comparison)
- Debugging shader correctness
- CPU-only fallback when no GPU is available

## Smart Rasterizer Selection (v0.32.0)

gg uses multi-factor auto-selection to choose the optimal rasterization algorithm per-path.
Five algorithms are available, each optimal for different scenarios:

| Algorithm | Type | Tiles | Origin | Optimal For | Location |
|-----------|------|-------|--------|-------------|----------|
| **AnalyticFiller** | CPU | — (scanline) | Coverage: Vello fine.rs; Edges: tiny-skia/Skia | Simple paths, small shapes | `internal/raster/` |
| **SparseStrips** | CPU | 4×4 | Vello sparse_strips | Complex paths, CPU/SIMD workloads | `internal/gpu/sparse_strips*.go`, `fine.go`, `coarse.go` |
| **TileCompute** | CPU | 16×16 | Vello 9-stage compute (CPU port) | Extreme complexity (10K+ segments) | `internal/gpu/tilecompute/` |
| **SDFAccelerator** | CPU+GPU | — (per-pixel) | Original (gg) | Geometric shapes (circles, rrects) | `sdf_accelerator.go`, `internal/gpu/sdf_gpu.go` |
| **Vello PTCL** | GPU | 16×16 | Vello 9-stage compute | Full scenes (many paths, GPU compute) | `internal/gpu/vello_*.go` |

### Auto-Selection Flow

```
Context.Fill()
    │
    ├── RasterizerMode == SDF? → force SDF on accelerator → try GPU
    ├── RasterizerMode == Auto? → try GPU accelerator (SDF/Stencil/Compute)
    │
    └── SoftwareRenderer.Fill()
         │
         ├── RasterizerMode == Analytic? → AnalyticFiller (scanline)
         ├── RasterizerMode == SparseStrips? → forced SparseStrips (4×4)
         ├── RasterizerMode == TileCompute? → forced TileCompute (16×16)
         │
         └── RasterizerMode == Auto?
              │
              ├── bbox < 32px? → AnalyticFiller (tile overhead exceeds benefit)
              ├── elements > adaptiveThreshold(bboxArea)? → CoverageFiller
              │    └── AdaptiveFiller auto-selects:
              │         ├── segments > 10K AND area > 2MP → TileCompute (16×16)
              │         └── otherwise → SparseStrips (4×4)
              └── otherwise → AnalyticFiller (scanline)
```

### Adaptive Threshold Formula

```
threshold = clamp(2048 / sqrt(bboxArea), 32, 256)
```

| Bounding Box | Threshold | Rationale |
|-------------|-----------|-----------|
| 50×50 px | 29 elements | Small area — scanline is cheap |
| 100×100 px | 20 elements | Medium — tile rasterizer starts winning |
| 200×200 px | 10 elements | Large — tile overhead amortized |
| 500×500 px | 4 elements | Very large — almost always tile |

### CoverageFiller Interface

```go
// CoverageFiller is a tile-based coverage rasterizer for complex paths.
type CoverageFiller interface {
    FillCoverage(path *Path, width, height int, fillRule FillRule,
        callback func(x, y int, coverage uint8))
}

// ForceableFiller allows forced algorithm selection.
type ForceableFiller interface {
    CoverageFiller
    SparseFiller() CoverageFiller   // 4×4 tiles
    ComputeFiller() CoverageFiller  // 16×16 tiles
}
```

Registration follows the same pattern as `GPUAccelerator`:
- `RegisterCoverageFiller()` / `GetCoverageFiller()` — registration pair
- `gg/gpu/` registers `AdaptiveFiller` (auto 4×4 vs 16×16)
- `gg/raster/` registers `AdaptiveFiller` independently (CPU-only, no GPU deps)

### RasterizerMode API

Per-context force override for debugging, benchmarking, and known workloads:

```go
dc := gg.NewContext(800, 600)

dc.SetRasterizerMode(gg.RasterizerSparseStrips) // force 4×4 tiles
dc.SetRasterizerMode(gg.RasterizerSDF)          // force SDF for shapes
dc.SetRasterizerMode(gg.RasterizerAuto)          // restore auto-selection
```

| Mode | Behavior |
|------|----------|
| `RasterizerAuto` | Multi-factor auto-selection (default) |
| `RasterizerAnalytic` | Force scanline, bypass tile rasterizer |
| `RasterizerSparseStrips` | Force 4×4 tiles via `ForceableFiller` |
| `RasterizerTileCompute` | Force 16×16 tiles via `ForceableFiller` |
| `RasterizerSDF` | Force SDF for shapes, bypass min-size check |

## Text Rendering Pipeline (v0.29.0+, CPU Transform v0.32.1, Hinting+ClearType v0.36.0)

Text rendering uses a multi-tier strategy. GPU MSDF handles text when available;
the CPU pipeline uses a hybrid decision tree for transform-aware rendering.
Glyph Mask (Tier 6) provides pixel-perfect rendering with auto-hinting and
optional ClearType LCD subpixel rendering for small axis-aligned text.

### Pipeline Flow

```
dc.DrawString(s, x, y)
    │
    ├── [1] GPU MSDF Text (if GPUTextAccelerator registered)
    │       CTM passed to vertex shader → correct scale/rotation/skew
    │
    └── [2] CPU Pipeline (drawStringCPU — 3-tier decision tree)
             │
             ├── Tier 0: Translation-only? → bitmap fast path (text.Draw)
             │            No quality loss, position transformed by CTM
             │
             ├── Tier 1: Uniform positive scale ≤256px?
             │            → bitmap at device size (Strategy A, Skia pattern)
             │            FontSource.Face(fontSize * scale) at transformed position
             │
             └── Tier 2: Everything else (rotation, shear, non-uniform scale,
                          negative scale, scale >256px)
                          → glyph outlines as vector paths (Strategy B, Vello pattern)
                          OutlineExtractor → Path (all glyphs, one fill)
                          → path.Transform(CTM) → SoftwareRenderer.Fill()
```

### Design Decisions (Enterprise References)

| Decision | Reference | Rationale |
|----------|-----------|-----------|
| 256px atlas threshold | Skia `kSkSideTooBigForAtlas` | Above this, bitmap quality degrades; outlines scale perfectly |
| Translation-only fast path | Cairo `_cairo_gstate_get_font_ctm` | Most common case, zero overhead vs identity |
| Glyph outlines as Path | Vello `resolve_glyph_run` | Exact rendering at any transform, no quality loss |
| Y-flip (`y - outlineY`) | TrueType/PostScript (Y-up) → screen (Y-down) | Industry standard for font outline conversion |
| All glyphs in one Path | `scene/text.go:ToCompositePath` | Single fill call, more efficient than per-glyph |
| FillRuleNonZero | Font outline convention | Standard winding rule for TrueType/OpenType contours |
| MultiFace fallback | `Source() == nil` → bitmap | Graceful degradation for composite faces |
| Lazy OutlineExtractor | GC-managed lifecycle | No changes to `NewContext()` or `Close()` |

### Key Files

| File | Content |
|------|---------|
| `text.go` | `drawStringCPU` decision tree, `drawStringBitmap/Scaled/AsOutlines` |
| `context.go` | `outlineExtractor` field (lazy init) |
| `text/glyph_outline.go` | `OutlineExtractor`, `GlyphOutline`, `OutlineSegment` |
| `text/face.go` | `Face.Glyphs()`, `Face.Source()`, `Face.Size()` |

## HiDPI/Retina Device Scale

gg uses the Cairo-pattern `device_scale` for DPI-transparent drawing. User code
operates in logical coordinates (points/DIP), while the internal pixmap is
allocated at physical pixel resolution. A permanent base scale transform maps
logical to physical automatically.

### API

```go
// Create HiDPI-aware context (800x600 logical, 1600x1200 physical on Retina 2x)
dc := gg.NewContextWithScale(800, 600, 2.0)

// Or via functional option
dc := gg.NewContext(800, 600, gg.WithDeviceScale(2.0))

// Or set dynamically
dc.SetDeviceScale(2.0)

dc.Width()       // 800 (logical)
dc.Height()      // 600 (logical)
dc.PixelWidth()  // 1600 (physical)
dc.PixelHeight() // 1200 (physical)
dc.DeviceScale() // 2.0

// Drawing uses logical coordinates — no DPI awareness needed in user code
dc.DrawCircle(400, 300, 100) // centered in logical space
dc.Fill()                     // rasterized at physical resolution
```

### Implementation

- `NewContextWithScale(w, h, scale)` allocates pixmap at `w*scale × h*scale`
- A permanent `Scale(scale, scale)` base matrix is applied to the transform stack
- `Identity()` resets to the base matrix (not the identity matrix)
- All drawing operations flow through the transform stack and are automatically scaled
- `SoftwareRenderer` receives device scale for DPI-aware tolerances

### DPI-Aware Rasterization (femtovg pattern)

On HiDPI displays, rasterization tolerances are adjusted for sharper output:

| Parameter | Formula | Effect on Retina 2x |
|-----------|---------|---------------------|
| Curve flatten tolerance | `baseTol / deviceScale` | 0.05 instead of 0.1 — finer subdivision |
| Stroke expansion tolerance | `baseTol / deviceScale` | Tighter stroke edges |

### gogpu Integration (ggcanvas)

```go
// ggcanvas automatically uses platform scale factor
canvas := ggcanvas.MustNewWithScale(provider, logicalW, logicalH, scaleFactor)
```

`ggcanvas.NewWithScale()` creates the GPU texture at physical pixel dimensions
while the gg Context operates in logical coordinates. The scale factor typically
comes from `gogpu.App.ScaleFactor()`.

## Package Structure

```
gg/
├── context.go              # Canvas-like drawing context
├── software.go             # SoftwareRenderer (adaptive threshold + force mode)
├── accelerator.go          # GPUAccelerator interface + registration
├── coverage_filler.go      # CoverageFiller + ForceableFiller interfaces
├── rasterizer_mode.go      # RasterizerMode type (Auto/Analytic/SparseStrips/TileCompute/SDF)
├── sdf.go                  # CPU SDF coverage functions (circles, ellipses, rects)
├── sdf_accelerator.go      # SDFAccelerator (CPU-based SDF, ForceSDFAware)
├── shape_detect.go         # DetectShape: auto-detect circles/rects/rrects from paths
├── pipeline_mode.go        # PipelineMode (Auto/RenderPass/Compute)
├── options.go              # Configuration options
├── path.go                 # Vector path operations (SetPath, DrawPath, FillPath)
├── path_svg.go             # SVG path data parser (ParseSVGPath)
├── paint.go                # Fill and stroke styles
├── lcd_layout.go           # LCD ClearType layout types (LCDLayoutRGB/BGR/None)
├── pixmap.go               # Pixel buffer operations
├── text.go                 # Text rendering
│
├── gpu/                    # PUBLIC opt-in: GPU accelerator + tile rasterizer
│   └── gpu.go              # init() registers SDFAccelerator + AdaptiveFiller
│
├── raster/                 # PUBLIC opt-in: tile rasterizer only (no GPU)
│   └── raster.go           # init() registers AdaptiveFiller (CPU-only)
│
├── svg/                    # SVG renderer (Parse + Render SVG XML → image.RGBA)
│   ├── svg.go              # Public API: Parse, Render, RenderWithColor
│   ├── parser.go           # SVG XML parser → Document tree
│   ├── renderer.go         # Element rendering via gg.Context
│   ├── colors.go           # SVG color parsing (#hex, rgb(), named)
│   ├── transform.go        # SVG transform parsing (translate, rotate, scale)
│   └── document.go         # Document, Element types
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
│   ├── gpu/                # GPU rendering pipeline (six-tier) + tile rasterizers
│   │   ├── backend.go      # GPU backend implementation
│   │   ├── sdf_gpu.go      # SDFAccelerator (GPU-based, wgpu HAL, ForceSDFAware)
│   │   ├── sdf_render.go   # SDF render pipeline (Tier 1)
│   │   ├── adaptive_filler.go    # AdaptiveFiller (auto 4×4 vs 16×16 tiles)
│   │   ├── sparse_strips_filler.go  # SparseStripsFiller (4×4 tiles, CoverageFiller)
│   │   ├── tilecompute_filler.go    # TileComputeFiller (16×16 tiles, CoverageFiller)
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
│   │   ├── glyph_mask_engine.go   # Glyph mask engine (Tier 6, shaping → atlas → quads)
│   │   ├── glyph_mask_pipeline.go # Glyph mask GPU pipeline (Tier 6, R8 alpha atlas)
│   │   ├── vello_accelerator.go  # VelloAccelerator (Tier 5 compute integration)
│   │   ├── vello_compute.go     # VelloComputeDispatcher (hal-based GPU dispatch)
│   │   ├── scene_bridge.go # Scene to native bridge
│   │   ├── golden_test.go  # GPU vs CPU golden comparison tests
│   │   │
│   │   ├── tilecompute/    # Vello compute pipeline CPU reference
│   │   │   ├── types.go         # PathDef, SceneElement, LineSoup, Tile, PathSegment
│   │   │   ├── scene_encode.go  # EncodeScene/EncodeSceneDef, PackScene
│   │   │   ├── flatten.go       # Euler spiral curve flattening
│   │   │   ├── pathtag.go       # Path tag monoid reduce/scan
│   │   │   ├── draw_leaf.go     # Draw monoid reduce/scan + ClipInp generation
│   │   │   ├── clip_leaf.go     # Clip matching (sequential stack, Vello parity)
│   │   │   ├── path_count.go    # Per-tile segment counting
│   │   │   ├── rasterizer.go    # RasterizeScenePTCL/SceneDefPTCL (full pipeline)
│   │   │   ├── coarse.go        # Coarse rasterization + PTCL + clip state
│   │   │   ├── fine.go          # Fine per-pixel rasterization + packed blend stack
│   │   │   └── shaders/         # WGSL compute shaders (9 stages)
│   │   │       ├── pathtag_reduce.wgsl
│   │   │       ├── pathtag_scan.wgsl
│   │   │       ├── draw_reduce.wgsl
│   │   │       ├── draw_leaf.wgsl
│   │   │       ├── path_count.wgsl
│   │   │       ├── backdrop.wgsl
│   │   │       ├── coarse.wgsl
│   │   │       ├── path_tiling.wgsl
│   │   │       └── fine.wgsl
│   │   │
│   │   └── shaders/        # WGSL render-pass shaders
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
│   │       ├── msdf_text.wgsl     # MSDF text rendering (Tier 4)
│   │       ├── glyph_mask.wgsl    # Glyph mask rendering (Tier 6)
│   │       └── glyph_mask_lcd.wgsl # LCD ClearType subpixel (Tier 6)
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
│   ├── glyph_outline.go    # Outline extraction + grid-fit hinting
│   ├── glyph_mask_rasterizer.go # CPU rasterization (grayscale + LCD/ClearType)
│   ├── glyph_mask_atlas.go # R8 alpha atlas (shelf packing, LRU, LCD 3x support)
│   ├── lcd_filter.go       # ClearType 5-tap FIR filter, LCDLayout (RGB/BGR)
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
| `RoundRectShape` | SDF-based rounded rect with per-pixel tile rendering (~5x faster) |
| `BeginClip/EndClip` | Alpha mask compositing for clip regions (Cairo/Skia pattern) |

## CPU Tile Rasterizers (v0.25.0+)

Two tile-based CPU rasterizers ported from [Vello](https://github.com/linebender/vello), each optimized for different scenarios:

### SparseStrips (4×4 tiles)

Port of Vello's `sparse_strips` rasterizer. Default CoverageFiller for complex paths.

```
Path → FlattenContext → Line Segments → CoarseRasterizer (bin to 4×4 tiles)
  → FineRasterizer (per-tile coverage) → StripRenderer → Blend
```

- **Location:** `internal/gpu/sparse_strips.go`, `fine.go`, `coarse.go`, `segment.go`
- **Tile size:** 4×4 (16 pixels) — optimized for CPU/SIMD (4px = f32x4 lane width)
- **Winding:** int32 backdrop prefix sum between tiles, float accumulation within tiles
- **Best for:** Complex paths with many elements, general-purpose CPU workloads

### TileCompute (16×16 tiles)

Port of Vello's 9-stage compute pipeline running on CPU. Produces bit-exact results with the GPU compute path (Tier 5).

```
Path → FlattenContext → Line Segments → Coarse (bin to 16×16 tiles)
  → fillPath() (per-tile, Vello area formula) → float32 alpha → Blend
```

- **Location:** `internal/gpu/tilecompute/`
- **Tile size:** 16×16 (256 pixels) — matches GPU compute workgroup dimensions
- **Area accumulation:** Per-tile `area[256]`, full reset per tile (zero drift by design)
- **Best for:** Extreme complexity (10K+ segments), GPU compute validation/fallback

### Shared Components

- **FlattenContext** (`internal/gpu/flatten.go`): Euler spiral curve flattening (Vello)
- **CoarseRasterizer** (`internal/gpu/coarse.go`): Segment-to-tile binning with DDA
- **Backdrop** (`internal/gpu/coarse.go`): int32 prefix sum for winding propagation
- **Analytic coverage**: Exact per-pixel trapezoidal area calculation (no supersampling)
- **Fill rules:** NonZero (winding != 0) and EvenOdd (winding is odd)

## AnalyticFiller (CPU Scanline AA)

Hybrid architecture combining Vello's coverage formula with Skia/tiny-skia's scanline infrastructure.

**From Vello fine.rs** (`internal/raster/analytic_filler.go`):
- Trapezoidal area integration per pixel (`area*sign + acc`)
- Left-to-right accumulator propagation (`acc += pixelH * sign`)

**From tiny-skia / Skia** (`internal/raster/`):
- EdgeBuilder: path → Y-monotonic edges (`edge_builder.go`)
- CurveEdge: forward differencing for quadratic/cubic curves (`curve_edge.go`)
- CurveAwareAET: Active Edge Table for scanline intersections (`curve_aet.go`)
- AlphaRuns: RLE-encoded coverage buffer (`alpha_runs.go`)
- FDot6/FDot16: fixed-point arithmetic (`fixed.go`)

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
and can render directly to any texture view (surface or offscreen) via `GPURenderTarget.View`
(`gpucontext.TextureView`), eliminating the GPU->CPU->GPU round-trip. Target is per-pass
(WebGPU spec pattern), enabling multi-context rendering (RepaintBoundary, offscreen export).

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
| **Six-Tier Rendering** | Skia Ganesh/Impeller/Vello | SDF, convex, stencil+cover, MSDF text, glyph mask (render pass) + compute pipeline |
| **9-Stage Compute** | vello | pathtag→draw→path_count→backdrop→coarse→path_tiling→fine GPU compute |
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
| **Multi-Engine Rasterizer** | coregex/gg | Adaptive algorithm selection per-path (analytic/4×4/16×16/SDF) |
| **Adaptive Threshold** | gg | `2048/sqrt(bboxArea)` — scales threshold with shape size |
| **CoverageFiller Registration** | accelerator.go pattern | Tile rasterizer registration via `RegisterCoverageFiller()` |
| **Hybrid Text Transform** | Skia/Cairo/Vello | 3-tier decision tree: bitmap → scaled bitmap → outline paths |
| **Font Hinting** | FreeType auto-hinter | Grid-fit outline Y/X coordinates for crisp stems at small sizes |
| **ClearType LCD** | FreeType/Microsoft | 3× horizontal oversampling + 5-tap FIR filter for per-channel RGB alpha |
| **Command Pattern** | Cairo/Skia | Recording system for vector export |
| **Driver Pattern** | database/sql | Backend registration via blank import |
| **Device Sharing** | Skia Graphite | DeviceProviderAware for gogpu integration |
| **Per-Pass Render Target** | WebGPU spec, Skia GrContext | GPURenderTarget.View for per-pass target (surface or offscreen) |

## See Also

- [README.md](../README.md) — Quick start guide
- [CHANGELOG.md](../CHANGELOG.md) — Version history
- [ROADMAP.md](../ROADMAP.md) — Development milestones
- [Examples](../examples/) — Code examples
