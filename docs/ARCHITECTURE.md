# gg Architecture

This document describes the architecture of the gg 2D graphics library.

## Overview

gg is a 2D graphics library for Go, inspired by HTML5 Canvas API and modern Rust 2D graphics libraries (vello, tiny-skia).

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
         ┌────────────────────────┼────────────────────────┐
         │                        │                        │
  ┌──────▼──────┐          ┌──────▼──────┐          ┌──────▼──────┐
  │   backend   │          │   backend   │          │   backend   │
  │    rust     │          │   native    │          │  software   │
  └──────┬──────┘          └──────┬──────┘          └──────┬──────┘
         │                        │                        │
         │                 ┌──────▼──────┐                 │
         │                 │    wgpu     │                 │
         │                 │    core     │                 │
         │                 └──────┬──────┘                 │
         │                        │                        │
         │              ┌─────────┼─────────┐              │
         │              │         │         │              │
         │           ┌──▼──┐  ┌───▼───┐  ┌──▼──┐           │
         │           │ Vk  │  │ Metal │  │Soft │           │
         │           └─────┘  └───────┘  └─────┘           │
         │                   wgpu/hal                      │
         │                                                 │
  ┌──────▼──────┐                                   ┌──────▼──────┐
  │ wgpu-native │                                   │    CPU      │
  │ (Rust FFI)  │                                   │ Rasterizer  │
  └─────────────┘                                   └─────────────┘
         │                                                 │
      GPU API                                      Direct 2D render
  (Vulkan/Metal/DX12)                               (no GPU needed)
```

## Backend System

gg supports three rendering backends:

| Backend      | Constant           | Description               | GPU Required |
|--------------|--------------------|---------------------------|--------------|
| **Rust**     | `BackendRust`      | wgpu-native via FFI       | Yes          |
| **Native**   | `BackendNative`    | Pure Go via gogpu/wgpu    | Yes          |
| **Software** | `BackendSoftware`  | CPU 2D rasterizer         | No           |

Aliases for convenience:
- `BackendGo` = `BackendNative`

### Backend Priority

When multiple backends are available, gg selects automatically:

```
Rust → Native → Software
 (1)     (2)       (3)
```

1. **Rust** — Maximum performance (if compiled with `-tags rust`)
2. **Native** — Good performance, zero dependencies (default)
3. **Software** — Always available fallback

### Build Tags

```bash
# Default: Native + Software backends
go build ./...

# With Rust backend
go build -tags rust ./...
```

## Backend Usage

```go
import "github.com/gogpu/gg/internal/native"

// Create and initialize GPU backend directly
nb := native.NewNativeBackend()
if err := nb.Init(); err != nil {
    log.Fatal(err)
}
defer nb.Close()

// For software rendering, use scene.Renderer or gg.NewContext() directly
```

## Software Rendering

| Component              | Level   | Purpose                              |
|------------------------|---------|--------------------------------------|
| `wgpu/hal/software`    | HAL     | Full WebGPU emulation on CPU         |
| Core `gg` package      | Default | Built-in 2D rasterizer (no wgpu)    |

## RenderBackend Interface

gg uses a simple 6-method interface:

```go
type RenderBackend interface {
    // Identification
    Name() string

    // Lifecycle
    Init() error
    Close()

    // Rendering
    NewRenderer(width, height int) gg.Renderer
    RenderScene(target *gg.Pixmap, scene *scene.Scene) error
}
```

This is intentionally simpler than gogpu's 120+ method interface.

## Package Structure

```
gg/
├── context.go          # Canvas-like drawing context
├── path.go             # Vector path operations
├── paint.go            # Fill and stroke styles
├── pixmap.go           # Pixel buffer operations
├── text.go             # Text rendering
│
├── core/               # Shared rendering primitives (v0.21.0)
│   ├── path.go         # Unified Path type
│   ├── paint.go        # Unified paint/brush definitions
│   ├── transform.go    # 2D affine transformations
│   └── scene.go        # Scene command builder
│
├── render/             # Cross-package rendering (v0.21.0)
│   ├── renderer.go     # GPU/Software renderer interface
│   ├── scene.go        # Scene with damage tracking
│   ├── layers.go       # LayeredTarget for z-ordering
│   ├── surface.go      # Render target abstraction
│   └── types.go        # DeviceHandle (gpucontext.DeviceProvider)
│
├── backend/            # Backend abstraction
│   ├── backend.go      # RenderBackend interface
│   ├── registry.go     # Auto-registration
│   ├── software/       # CPU rasterizer
│   ├── native/         # Pure Go GPU backend (v0.20.0+)
│   │   ├── command_encoder.go   # CommandEncoder with state machine
│   │   ├── texture.go           # Texture with lazy default view
│   │   ├── buffer.go            # Buffer with async mapping
│   │   ├── render_pass.go       # RenderPassEncoder
│   │   ├── compute_pass.go      # ComputePassEncoder
│   │   ├── pipeline_cache_core.go # PipelineCache (FNV-1a hashing)
│   │   ├── shaders/             # WGSL compute shaders
│   │   │   # Analytic Anti-Aliasing (v0.21.0)
│   │   ├── fixed_point.go       # FDot6, FDot16 fixed-point types
│   │   ├── curve_edge.go        # QuadraticEdge, CubicEdge (forward diff)
│   │   ├── path_geometry.go     # Y-monotonic curve chopping
│   │   ├── edge_builder.go      # Path to typed edges conversion
│   │   ├── alpha_runs.go        # RLE-encoded coverage buffer
│   │   ├── curve_aet.go         # CurveAwareAET (Active Edge Table)
│   │   ├── analytic_filler.go   # Trapezoidal integration filler
│   │   └── analytic_adapter.go  # SoftwareRenderer integration
│   └── rust/           # Rust FFI backend
│
├── scene/              # Retained mode rendering
│   ├── scene.go        # Scene graph
│   └── renderer.go     # Parallel tile renderer
│
├── recording/          # Drawing recording for vector export (v0.23.0)
│   ├── recorder.go     # Command-based drawing recorder
│   ├── recording.go    # Recording with ResourcePool
│   ├── commands.go     # Typed command definitions
│   ├── brush.go        # Brush types (Solid, Gradient)
│   ├── backend.go      # Backend interface (Writer, File)
│   ├── registry.go     # Backend registration
│   └── raster/         # Built-in raster backend
│
├── text/               # Text rendering
│   ├── shaper.go       # Pluggable shaper interface + global registry
│   ├── shaper_builtin.go # Default shaper (x/image, basic LTR)
│   ├── shaper_gotext.go  # HarfBuzz shaper (go-text/typesetting)
│   ├── layout.go       # Multi-line layout engine
│   ├── glyph_cache.go  # LRU glyph cache (16-shard)
│   ├── glyph_run.go    # GlyphRunBuilder for batching
│   ├── msdf/           # MSDF text rendering
│   └── emoji/          # Color emoji support
│
├── internal/
│   ├── raster/         # Scanline rasterization
│   ├── blend/          # Color blending (29 modes)
│   ├── tile/           # Parallel rendering
│   ├── wide/           # SIMD operations
│   ├── stroke/         # Stroke expansion (v0.20.0)
│   │   └── expander.go # kurbo/tiny-skia pattern
│   └── filter/         # Blur, shadow, color matrix
│
├── font/               # Font loading
└── image/              # Image I/O (PNG, JPEG, WebP)
```

## Native Backend Architecture (v0.20.0)

The `internal/native/` package provides enterprise-grade GPU rendering:

### Command Encoder

```go
// State machine: Recording → Locked → Finished → Consumed
type CommandEncoder struct {
    core      *core.CoreCommandEncoder
    state     encoderState
    errors    []error
}

// Create and record commands
encoder := native.NewCommandEncoder(device, nil)
pass := encoder.BeginRenderPass(&RenderPassDescriptor{...})
pass.SetPipeline(pipeline)
pass.Draw(3, 1, 0, 0)
pass.End()
cmdBuffer := encoder.Finish(nil)
queue.Submit(cmdBuffer)
```

### Texture Management

```go
// Lazy default view with sync.Once for thread-safety
type Texture struct {
    hal         hal.Texture
    defaultView *TextureView
    viewOnce    sync.Once
}

texture := native.CreateCoreTexture(device, &TextureDescriptor{
    Size:   Extent3D{Width: 512, Height: 512, DepthOrArrayLayers: 1},
    Format: types.TextureFormatRGBA8Unorm,
    Usage:  types.TextureUsageRenderAttachment | types.TextureUsageTextureBinding,
})
view := texture.GetDefaultView() // Thread-safe, created once
```

### Buffer Mapping

```go
// Async mapping with state machine
type Buffer struct {
    hal       hal.Buffer
    mapState  BufferMapState  // Unmapped → Pending → Mapped
}

buffer.MapAsync(types.MapModeRead, 0, size, func(status BufferMapAsyncStatus) {
    if status == BufferMapAsyncStatusSuccess {
        data := buffer.GetMappedRange(0, size)
        // Use data...
        buffer.Unmap()
    }
})
device.Poll(true) // Wait for mapping
```

### Pipeline Cache

```go
// FNV-1a descriptor hashing with double-check locking
type PipelineCacheCore struct {
    device    hal.Device
    cache     map[uint64]*RenderPipeline
    mu        sync.RWMutex
    hits      atomic.Uint64
    misses    atomic.Uint64
}

cache := native.NewPipelineCacheCore(device)
pipeline := cache.GetOrCreateRenderPipeline(&RenderPipelineDescriptor{...})
```

## Stroke Expansion (internal/stroke)

Converts stroked paths to filled outlines using the kurbo/tiny-skia algorithm:

```go
expander := stroke.NewStrokeExpander()
expander.SetWidth(2.0)
expander.SetLineCap(stroke.LineCapRound)
expander.SetLineJoin(stroke.LineJoinMiter)
expander.SetMiterLimit(4.0)

filledPath := expander.Expand(strokePath)
```

Supports:
- **Line Caps**: Butt, Round, Square
- **Line Joins**: Miter (with limit), Round, Bevel
- **Curves**: Quadratic and cubic Bezier flattening with tolerance

## Glyph Batching (text/glyph_run.go)

Efficient glyph batching for GPU text rendering:

```go
builder := text.NewGlyphRunBuilder(cache)
builder.AddShapedRun(shapedRun, origin)
commands := builder.Build(createGlyphFunc)

for _, cmd := range commands {
    // Render glyph with transform
}
```

## Analytic Anti-Aliasing (v0.21.0)

Enterprise-grade curve rendering using forward differencing and trapezoidal coverage calculation (vello/tiny-skia pattern).

### Problem: Pre-Flattening

Traditional approach flattens bezier curves to line segments before rasterization:

```
Curve → Flatten (Tolerance=0.1) → Lines → Rasterize → Visible segments!
```

This causes visible segmentation on smooth curves, especially at larger sizes.

### Solution: Forward Differencing

Analytic AA processes curves directly with O(1) per-step evaluation:

```
Curve → Forward Differencing → Direct scanline intersection → Smooth result
```

### Architecture

```
Path → EdgeBuilder → Typed Edges → AnalyticFiller → AlphaRuns → Pixels
         │              │              │
    Y-monotonic    Line/Quad/Cubic   Trapezoidal
      chopping     CurveEdgeVariant   integration
```

### Forward Differencing Edges

```go
// O(1) per step - only additions, no multiplications
type QuadraticEdge struct {
    fFirstX, fFirstY FDot16  // Current position (fixed-point)
    fDx, fDy         FDot16  // First derivative
    fDDx, fDDy       FDot16  // Second derivative (constant)
    fLastY           int     // End scanline
}

// Step advances one scanline with just additions
func (e *QuadraticEdge) Step() {
    e.fFirstX += e.fDx
    e.fFirstY += e.fDy
    e.fDx += e.fDDx
    e.fDy += e.fDDy
}
```

### Fixed-Point Arithmetic

Sub-pixel precision without floating-point overhead:

| Type | Format | Precision | Use Case |
|------|--------|-----------|----------|
| `FDot6` | 26.6 | 1/64 px | Y coordinates |
| `FDot16` | 16.16 | 1/65536 px | X coordinates, derivatives |
| `FDot8` | 24.8 | 1/256 | Coverage values |

### Curve-Aware Active Edge Table

```go
type CurveAwareAET struct {
    lines     []LineEdge
    quads     []QuadraticEdge
    cubics    []CubicEdge
}

// StepCurves advances all curve edges one scanline
func (aet *CurveAwareAET) StepCurves() {
    for i := range aet.quads {
        aet.quads[i].Step()  // O(1)
    }
    for i := range aet.cubics {
        aet.cubics[i].Step() // O(1)
    }
}
```

### Trapezoidal Coverage

Exact per-pixel coverage using trapezoidal integration (vello algorithm):

```go
// Coverage = area of trapezoid formed by edge crossing pixel
coverage := computeTrapezoidArea(x0, x1, y0, y1)
alphaRuns.Add(x, uint8(coverage * 255))
```

### Performance

| Operation | Time | vs Naive |
|-----------|------|----------|
| QuadraticEdge.Step | 7ns | O(1) vs O(n) |
| CubicEdge.Step | 9ns | O(1) vs O(n) |
| Memory (1080p circle) | ~27KB | -75% vs supersampling |

### Integration

```go
// SoftwareRenderer with analytic AA
renderer := gg.NewSoftwareRenderer(800, 600)
renderer.SetRenderMode(gg.RenderModeAnalytic) // Default

// Curves render smoothly without segmentation
dc.CubicTo(100, 50, 200, 150, 300, 100)
dc.Fill()
```

## Cross-Package Integration (render/)

The `render/` package (v0.21.0+) enables gg to integrate with external GPU frameworks like gogpu:

### DeviceHandle Pattern

```go
// DeviceHandle is an alias for gpucontext.DeviceProvider
type DeviceHandle = gpucontext.DeviceProvider

// Create renderer with injected device
renderer := render.NewGPURenderer(deviceHandle)
```

### Scene with Damage Tracking

```go
scene := render.NewScene()

// Partial invalidation for UI efficiency
scene.Invalidate(render.DirtyRect{X: 100, Y: 100, Width: 50, Height: 50})

// Check what needs redrawing
if scene.NeedsFullRedraw() {
    // Full redraw
} else {
    // Partial redraw using scene.DirtyRects()
}
```

### LayeredTarget for Z-Ordering

```go
type LayeredTarget interface {
    RenderTarget
    CreateLayer(z int) (RenderTarget, error)
    RemoveLayer(z int) error
    SetLayerVisible(z int, visible bool)
    Layers() []int
    Composite()
}
```

This enables UI frameworks to manage popups, dropdowns, and overlays efficiently.

## Rendering Modes

### Immediate Mode

Traditional draw-as-you-go approach:

```go
dc := gg.NewContext(800, 600)
dc.SetRGB(1, 0, 0)
dc.DrawCircle(400, 300, 100)
dc.Fill()
dc.SavePNG("output.png")
```

### Retained Mode

Scene graph approach for complex scenes:

```go
s := scene.New()
s.PushLayer(scene.LayerConfig{})
s.Fill(path, paint)
s.PopLayer()

renderer.RenderScene(pixmap, s)
```

## Relationship to gogpu Ecosystem

```
                    gpucontext (shared interfaces)
                    gputypes (shared types) [planned]
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
| **Dependencies**      | wgpu, naga, gpucontext| wgpu, gpucontext    |
| **Backend interface** | 6 methods             | 120+ methods         |
| **Software fallback** | Yes                   | No                   |
| **gpucontext role**   | Consumer              | Provider             |

### Cross-Package Integration (v0.21.0+)

gg can receive GPU device from gogpu via `gpucontext.DeviceProvider`:

```go
// gogpu provides device
provider := app.GPUContextProvider()

// gg receives device for rendering
renderer := render.NewGPURenderer(provider)
```

This enables enterprise-grade dependency injection without circular imports.

## Recording System (v0.23.0)

The `recording/` package provides command-based drawing recording for vector export.

### Architecture (Cairo/Skia-inspired)

```
User Code → Recorder → Commands → Recording → Backend → Output
                          ↓
                    ResourcePool
                   (paths, brushes)
```

### Core Types

```go
// Recorder captures drawing operations
rec := recording.NewRecorder(800, 600)
rec.SetColor(gg.Blue)
rec.DrawCircle(400, 300, 100)
rec.Fill()

// Recording holds captured commands
recording := rec.Recording()

// Export via pluggable backends
recording.SaveToFile("output.pdf", "pdf")
recording.SaveToFile("output.svg", "svg")
```

### Command Pattern

All drawing operations become typed commands:

```go
type FillPathCmd struct {
    PathRef  PathRef    // Reference to pooled path
    BrushRef BrushRef   // Reference to pooled brush
}

type SetTransformCmd struct {
    Matrix Matrix  // 3x3 affine transform
}
```

### Resource Pooling

Deduplication for paths, brushes, images:

```go
type ResourcePool struct {
    paths   []Path
    brushes []Brush
    images  []image.Image
}

// Same brush returns same reference
ref1 := pool.AddBrush(solidBlue)
ref2 := pool.AddBrush(solidBlue) // ref1 == ref2
```

### Backend Interface

Pluggable renderers via database/sql driver pattern:

```go
type Backend interface {
    Name() string
    Play(recording *Recording) error
}

type WriterBackend interface {
    Backend
    PlayToWriter(recording *Recording, w io.Writer) error
}

type FileBackend interface {
    Backend
    PlayToFile(recording *Recording, filename string) error
}
```

### Backend Registration

```go
// External backends register via init()
func init() {
    recording.Register("pdf", &pdfBackend{})
    recording.Register("svg", &svgBackend{})
}

// Users enable via blank import
import _ "github.com/gogpu/gg-pdf"
import _ "github.com/gogpu/gg-svg"
```

### Available Backends

| Backend | Package | Format |
|---------|---------|--------|
| **Raster** | `recording/raster` | Built-in PNG/image |
| **PDF** | `github.com/gogpu/gg-pdf` | PDF documents |
| **SVG** | `github.com/gogpu/gg-svg` | SVG vector graphics |

## Key Design Patterns

| Pattern | Source | Implementation |
|---------|--------|----------------|
| **Lazy Default View** | wgpu-rs | `sync.Once` for thread-safe texture view |
| **State Machine** | wgpu | Command encoder lifecycle |
| **Async Mapping** | wgpu | Buffer map with callbacks |
| **FNV-1a Hashing** | wgpu-core | Pipeline cache key generation |
| **Serial-Based LRU** | vello | Glyph cache eviction |
| **Stroke Expansion** | kurbo/tiny-skia | Forward/backward offset paths |
| **Forward Differencing** | Skia | O(1) curve edge stepping |
| **Fixed-Point Math** | Skia | FDot6/FDot16 sub-pixel precision |
| **Trapezoidal Coverage** | vello | Exact per-pixel AA calculation |
| **Y-Monotonic Chopping** | tiny-skia | Curve splitting for scanline traversal |
| **Command Pattern** | Cairo/Skia | Recording system for vector export |
| **Driver Pattern** | database/sql | Backend registration via blank import |

## See Also

- [README.md](../README.md) — Quick start guide
- [CHANGELOG.md](../CHANGELOG.md) — Version history
- [ROADMAP.md](../ROADMAP.md) — Development milestones
- [Examples](../examples/) — Code examples
