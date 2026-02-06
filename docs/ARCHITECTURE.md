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
                    │                           │ (optional)
             ┌──────▼──────┐             ┌──────▼──────┐
             │  internal/  │             │  internal/  │
             │   raster    │             │   native    │
             └─────────────┘             └──────┬──────┘
                                                │
                                         ┌──────▼──────┐
                                         │    wgpu     │
                                         └──────┬──────┘
                                                │
                                      ┌─────────┼─────────┐
                                      │         │         │
                                   ┌──▼──┐  ┌───▼───┐  ┌──▼──┐
                                   │ Vk  │  │ Metal │  │Soft │
                                   └─────┘  └───────┘  └─────┘
                                           wgpu/hal
```

## GPU Accelerator (v0.26.0)

GPU acceleration is opt-in via the `GPUAccelerator` interface:

```go
// GPUAccelerator provides optional GPU acceleration
type GPUAccelerator interface {
    Name() string
    AccelerateOp(op AcceleratedOp, args any) (any, error)
    Close()
}

// Register via blank import pattern
import _ "github.com/gogpu/gg/internal/native"
```

Key design:
- `RegisterAccelerator()` for opt-in GPU
- `ErrFallbackToCPU` sentinel error for graceful degradation
- `AcceleratedOp` bitfield for capability checking
- Zero overhead (~17ns) when no GPU registered
- CPU raster always available as fallback

## Package Structure

```
gg/
├── context.go              # Canvas-like drawing context
├── software.go             # SoftwareRenderer (uses internal/raster directly)
├── accelerator.go          # GPUAccelerator interface + registration
├── options.go              # Configuration options
├── path.go                 # Vector path operations
├── paint.go                # Fill and stroke styles
├── pixmap.go               # Pixel buffer operations
├── text.go                 # Text rendering
│
├── render/                 # Cross-package rendering
│   ├── scene.go            # Scene with damage tracking
│   ├── software.go         # Software renderer
│   ├── layers.go           # LayeredTarget for z-ordering
│   ├── surface.go          # Render target abstraction
│   └── types.go            # DeviceHandle (gpucontext.DeviceProvider)
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
│   ├── native/             # GPU rendering pipeline
│   │   ├── backend.go      # GPU backend implementation
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
│   │   ├── shaders/        # WGSL compute shaders
│   │   └── scene_bridge.go # Scene to native bridge
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
│   ├── tile/               # Parallel rendering
│   ├── wide/               # SIMD operations
│   ├── stroke/             # Stroke expansion (kurbo/tiny-skia)
│   └── filter/             # Blur, shadow, color matrix
│
├── recording/              # Drawing recording for vector export
│   ├── recorder.go         # Command-based drawing recorder
│   ├── recording.go        # Recording with ResourcePool
│   ├── commands.go         # Typed command definitions
│   ├── backend.go          # Backend interface (Writer, File)
│   ├── registry.go         # Backend registration
│   └── raster/             # Built-in raster backend
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
├── font/                   # Font loading
└── image/                  # Image I/O (PNG, JPEG, WebP)
```

## Vello Tile Rasterizer (v0.25.0)

Port of vello_shaders CPU fine rasterizer to Go. Used in `internal/native/`.

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
| **Dependencies**      | wgpu, naga, gpucontext| wgpu, gpucontext    |
| **gpucontext role**   | Consumer              | Provider             |

## Key Design Patterns

| Pattern | Source | Implementation |
|---------|--------|----------------|
| **GPU Accelerator** | gg v0.26.0 | Opt-in GPU via blank import |
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

## See Also

- [README.md](../README.md) — Quick start guide
- [CHANGELOG.md](../CHANGELOG.md) — Version history
- [ROADMAP.md](../ROADMAP.md) — Development milestones
- [Examples](../examples/) — Code examples
