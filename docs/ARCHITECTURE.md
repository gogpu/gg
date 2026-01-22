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

## Backend Selection

```go
import "github.com/gogpu/gg/backend"

// Auto-select best available
b := backend.Default()

// Get specific backend by name
b := backend.Get(backend.BackendNative)   // Pure Go GPU
b := backend.Get(backend.BackendGo)       // Alias for Native
b := backend.Get(backend.BackendRust)     // Rust FFI GPU
b := backend.Get(backend.BackendSoftware) // CPU fallback

// Initialize default backend
b, err := backend.InitDefault()
```

## Software Rendering: Two Levels

There are **two different** software rendering options in the ecosystem:

| Component              | Level   | Purpose                              |
|------------------------|---------|--------------------------------------|
| `wgpu/hal/software`    | HAL     | Full WebGPU emulation on CPU         |
| `gg/backend/software`  | Backend | Lightweight 2D rasterizer (no wgpu)  |

- **wgpu/hal/software** — Used when Native backend needs CPU fallback
- **gg/backend/software** — Direct 2D rendering without WebGPU overhead

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
├── backend/            # Backend abstraction
│   ├── backend.go      # RenderBackend interface
│   ├── registry.go     # Auto-registration
│   ├── software/       # CPU rasterizer
│   ├── native/         # Pure Go GPU backend (v0.20.0)
│   │   ├── command_encoder.go   # CommandEncoder with state machine
│   │   ├── texture.go           # Texture with lazy default view
│   │   ├── buffer.go            # Buffer with async mapping
│   │   ├── render_pass.go       # RenderPassEncoder
│   │   ├── compute_pass.go      # ComputePassEncoder
│   │   ├── pipeline_cache_core.go # PipelineCache (FNV-1a hashing)
│   │   └── shaders/             # WGSL compute shaders
│   └── rust/           # Rust FFI backend
│
├── scene/              # Retained mode rendering
│   ├── scene.go        # Scene graph
│   └── renderer.go     # Parallel tile renderer
│
├── text/               # Text rendering
│   ├── shaper.go       # Pluggable text shaping
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
└── image/              # Image loading
```

## Native Backend Architecture (v0.20.0)

The `backend/native/` package provides enterprise-grade GPU rendering:

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
naga (shader compiler)
  │
  └──► wgpu (Pure Go WebGPU)
         │
         ├──► gogpu (framework)
         │
         └──► gg (2D graphics) ◄── this project
```

gg and gogpu are **independent libraries**:

| Aspect                | gg                    | gogpu                |
|-----------------------|-----------------------|----------------------|
| **Purpose**           | 2D graphics library   | GPU framework        |
| **Dependencies**      | wgpu, naga            | wgpu                 |
| **Backend interface** | 6 methods             | 120+ methods         |
| **Software fallback** | Yes                   | No                   |

Both use **gogpu/wgpu** as the shared WebGPU implementation.

## Key Design Patterns

| Pattern | Source | Implementation |
|---------|--------|----------------|
| **Lazy Default View** | wgpu-rs | `sync.Once` for thread-safe texture view |
| **State Machine** | wgpu | Command encoder lifecycle |
| **Async Mapping** | wgpu | Buffer map with callbacks |
| **FNV-1a Hashing** | wgpu-core | Pipeline cache key generation |
| **Serial-Based LRU** | vello | Glyph cache eviction |
| **Stroke Expansion** | kurbo/tiny-skia | Forward/backward offset paths |

## See Also

- [README.md](../README.md) — Quick start guide
- [CHANGELOG.md](../CHANGELOG.md) — Version history
- [ROADMAP.md](../ROADMAP.md) — Development milestones
- [Examples](../examples/) — Code examples
