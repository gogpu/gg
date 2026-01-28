# gg Roadmap

> **Enterprise-Grade 2D Graphics Library for Go**
>
> Designed to power IDEs, browsers, and professional graphics applications.

---

## Design Philosophy

**gg** is built from the ground up following **modern Rust 2D graphics patterns**:

| Pattern | Inspiration | Implementation |
|---------|-------------|----------------|
| **Dual-Stream Encoding** | vello | Scene graph with GPU-ready command buffers |
| **Sparse Strips** | vello 2025 | CPU tessellation → GPU rasterization |
| **Paint/Brush System** | tiny-skia, peniko | Pattern interface with extensible types |
| **29 Blend Modes** | vello, W3C | Porter-Duff + Advanced + HSL modes |
| **Layer Compositing** | Skia, vello | Isolated layers with blend/opacity |
| **LRU Caching** | Industry standard | Sharded thread-safe caches |

### Core Principles

1. **Pure Go** — No CGO, easy cross-compilation, single binary deployment
2. **GPU-First** — Designed for GPU acceleration from day one
3. **Production-Ready** — Enterprise-grade error handling, logging, monitoring
4. **API Stability** — Semantic versioning, deprecation policy, migration guides

---

## Current State: v0.21.1

| Milestone | Focus | Status |
|-----------|-------|--------|
| **Analytic Anti-Aliasing** | Smooth bezier curves | ✅ v0.20.2 |
| Anti-Aliased Rendering | Smooth edges for all shapes | ✅ v0.19.0 |
| GPU Backend Completion | Enterprise-grade GPU types | ✅ v0.20.0 |
| Canvas API | Core drawing operations | ✅ |
| Text Rendering | Font loading, layout, rendering | ✅ |
| Images, Clipping | Image loading, clip paths | ✅ |
| Colors, Layers | Color pipeline, layer compositing | ✅ |
| SIMD Optimization | Performance improvements | ✅ |
| Parallel Rendering | Multi-threaded rasterization | ✅ |
| Scene Graph | Retained mode rendering | ✅ |
| Backend Abstraction | GPU/CPU backend interface | ✅ |
| GPU Backend | Sparse strips, compute shaders | ✅ |
| Text Pipeline | GPU-accelerated text | ✅ |
| MSDF, Emoji | Signed distance fonts, emoji support | ✅ |
| Brush, Gradients | Gradient fills, stroke system | ✅ |
| Go 1.25+ Modernization | Modern Go features | ✅ |
| Advanced Features | Masks, PathBuilder, streaming I/O | ✅ |
| GPU Compute Shaders | Fine, coarse, flatten shaders | ✅ |
| Renderer DI | Dependency injection for GPU integration | ✅ |

---

## Roadmap to v1.0.0

### v0.12.0 — Rust-First API Modernization

**Status:** Released | **Date:** 2025-12-24

Complete API modernization following Rust 2D graphics best practices.

| Feature | Pattern Source | Description |
|---------|---------------|-------------|
| **Brush Enum** | vello/peniko | Replace Pattern with sealed Brush interface |
| **Gradients** | vello/tiny-skia | Linear, Radial, Sweep with ExtendMode |
| **Stroke Struct** | tiny-skia/kurbo | Unified stroke parameters with Dash |
| **Error Wrapping** | Go 1.13+ | All errors use `%w` with context |

```go
// Before (v0.11.0)
ctx.SetColor(gg.Red)
ctx.Fill()

// After (v0.12.0) — Same simple API, more powerful internals
ctx.SetColor(gg.Red)                    // Still works!
ctx.SetFillBrush(gg.Solid(gg.Red))     // Or use Brush directly
ctx.SetFillBrush(gg.NewLinearGradient(0, 0, 100, 0).
    AddColorStop(0, gg.Red).
    AddColorStop(1, gg.Blue))
ctx.Fill()
```

### v0.13.0 — Go 1.25+ Modernization

**Status:** Released | **Date:** 2025-12-24

Full adoption of Go 1.25+ features for modern, idiomatic API.

| Feature | Description |
|---------|-------------|
| **context.Context** | Cancellation for long operations |
| **Generic Cache** | `Cache[K, V]` type-safe caching |
| **iter.Seq** | Path iterators with zero allocation |
| **Text Wrapping** | UAX #14 line breaking with CJK support |

```go
// Context support
ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
defer cancel()
err := renderer.RenderWithContext(ctx, scene) // Cancellable!

// Generic cache
cache := cache.New[string, *Texture](1024, 2048)

// Iterator-based paths
for elem := range path.Elements() {
    // Process path elements
}

// Unicode-aware text wrapping
opts := text.LayoutOptions{
    MaxWidth: 400,
    WrapMode: text.WrapWordChar, // Word-first, char fallback
}
```

### v0.14.0 — Advanced Features

**Status:** Released | **Date:** 2025-12-24

Professional graphics features for complex applications.

| Feature | Description |
|---------|-------------|
| **Alpha Masks** | AsMask, SetMask, GetMask, InvertMask, ClearMask |
| **PathBuilder** | Fluent path construction with 13 shape methods |
| **Context.Close()** | io.Closer implementation for deterministic cleanup |
| **Path Helpers** | GetCurrentPoint(), HasCurrentPoint(), Path.Clone() |
| **Streaming I/O** | EncodePNG/EncodeJPEG to io.Writer |

```go
// PathBuilder — fluent API
path := gg.BuildPath().
    MoveTo(100, 100).
    LineTo(200, 100).
    Circle(150, 150, 50).
    Star(250, 150, 40, 20, 5).
    Build()

// Alpha Masks
ctx.DrawCircle(200, 200, 100)
ctx.Fill()
mask := ctx.AsMask()

ctx2 := gg.NewContext(400, 400)
ctx2.SetMask(mask)
ctx2.DrawRectangle(0, 0, 400, 400)
ctx2.Fill() // Only visible through mask

// Streaming output
file, _ := os.Create("output.png")
defer file.Close()
ctx.EncodePNG(file) // Write directly to file
```

### v0.15.0 — GPU Compute Shaders

**Status:** Released | **Date:** 2025-12-26

Implements vello-style GPU compute shader pipeline for high-performance 2D rasterization.

| Feature | Description |
|---------|-------------|
| **Fine Shader** | GPU coverage calculation with analytic anti-aliasing |
| **Coarse Shader** | Tile binning with atomic operations |
| **Flatten Shader** | Quadratic and cubic Bezier curve flattening |
| **HybridPipeline** | Unified GPU/CPU pipeline with automatic selection |

```go
// GPU-accelerated path rasterization
pipeline := wgpu.NewHybridPipeline(device, queue, wgpu.HybridPipelineConfig{
    FlattenThreshold: 100,  // Use GPU for 100+ curves
    CoarseThreshold:  50,   // Use GPU for 50+ segments
    FineThreshold:    20,   // Use GPU for 20+ tiles
})

// Rasterize path with optimal GPU/CPU balance
coverage := pipeline.RasterizePath(path, transform, scene.FillNonZero)
```

**Statistics:** +6,470 LOC, 3 WGSL shaders, 17 tests, 87.6% coverage

### v0.19.0 — Anti-Aliased Rendering

**Status:** Released | **Date:** 2026-01-22

Professional-grade anti-aliasing using the tiny-skia algorithm.

| Feature | Pattern Source | Description |
|---------|---------------|-------------|
| **4x Supersampling** | tiny-skia | Sub-pixel accuracy with SUPERSAMPLE_SHIFT=2 |
| **AlphaRuns RLE** | tiny-skia | Memory-efficient sparse alpha buffer |
| **SuperBlitter** | tiny-skia | Coverage accumulation coordinator |
| **SIMD Optimization** | internal/wide | Batch processing for 16 pixels |

**Fixes:** [#43](https://github.com/gogpu/gg/issues/43) — Pixelated circles

### v0.21.0 — Enterprise Architecture (Current)

**Status:** Released | **Date:** 2026-01-27

Enterprise-grade architecture for gogpu/ui integration following Skia, Vello, Cairo patterns.

| Feature | Package | Description |
|---------|---------|-------------|
| **core/** | ARCH-003 | CPU rendering separated from GPU |
| **surface/** | ARCH-004 | Unified Surface interface (Image, GPU) |
| **render/** | INT-001 | Device integration (DeviceHandle, RenderTarget, Scene) |
| **gpucontext** | ARCH-006 | Shared interfaces (DeviceProvider, EventSource, Registry) |
| **Damage Tracking** | UI-ARCH-001 | Dirty region tracking for partial redraw |
| **LayeredTarget** | UI-ARCH-001 | Z-ordered layers for popups/dropdowns |
| **IME Support** | UI-ARCH-001 | CJK input support via gpucontext |

```go
// New: Device integration with gogpu
renderer := render.NewGPURenderer(app.DeviceHandle())
scene := render.NewScene()
scene.Rectangle(10, 10, 100, 50)
scene.SetFillColor(color.Red)
scene.Fill()
renderer.Render(target, scene)

// New: Damage tracking for efficient UI
scene.Invalidate(render.DirtyRect{X: 10, Y: 10, Width: 100, Height: 50})
if scene.NeedsFullRedraw() {
    // Redraw everything
} else {
    for _, rect := range scene.DirtyRects() {
        // Redraw only dirty regions
    }
}

// New: Layered surfaces for popups
layered := render.NewLayeredPixmapTarget(800, 600)
popup, _ := layered.CreateLayer(100) // z=100 on top
layered.Composite() // Blend all layers
```

**Also includes:**
- **BUG-001**: Dash pattern fix for analytic AA ([#52](https://github.com/gogpu/gg/issues/52))
- **FEAT-001**: Direct Matrix API ([#51](https://github.com/gogpu/gg/issues/51))
- **Context.Resize()**: Frame reuse without allocation

### v0.21.1 — Subpath Fix (Current)

**Status:** Released | **Date:** 2026-01-28

Critical fix for dashed strokes with scale transformation.

| Feature | Pattern Source | Description |
|---------|---------------|-------------|
| **EdgeIter** | tiny-skia | Edge iterator with proper subpath handling |
| **FillAAFromEdges** | tiny-skia | Edge-based rasterization without subpath leaks |

**Fixes:** [#54](https://github.com/gogpu/gg/issues/54) — Dashed strokes with scale rendered incorrectly

**Root Cause:** `path.Flatten()` returned flat point list, losing subpath boundaries. Rasterizer created incorrect "connecting edges" between separate subpaths.

**Solution:** New `path.EdgeIter` following tiny-skia pattern — iterates over edges directly, tracks `moveTo` per subpath, never creates inter-subpath edges.

### v0.20.1 — Dependency Update

**Status:** Released | **Date:** 2026-01-24

- **wgpu v0.10.2** — FFI build tag fix for CGO compatibility

### v0.20.0 — GPU Backend Completion

**Status:** Released | **Date:** 2026-01-22

Enterprise-grade GPU backend implementation following wgpu-rs and vello patterns.

| Feature | Pattern Source | Description |
|---------|---------------|-------------|
| **Command Encoder** | wgpu | State machine with deferred errors |
| **Texture Management** | wgpu-rs | Lazy default view via `sync.Once` |
| **Buffer Mapping** | wgpu | Async mapping with state machine |
| **Render/Compute Pass** | wgpu | Full pass encoder implementation |
| **Pipeline Cache** | wgpu-core | FNV-1a hashing, double-check locking |
| **Stroke Expansion** | kurbo/tiny-skia | Forward/backward offset paths |
| **Glyph Run Builder** | vello | Efficient glyph batching |
| **Color Emoji** | skrifa/swash | CBDT/CBLC, COLR format support |

```go
// Command Encoder with state machine
encoder := native.NewCommandEncoder(device, nil)
pass := encoder.BeginRenderPass(&RenderPassDescriptor{...})
pass.SetPipeline(pipeline)
pass.Draw(3, 1, 0, 0)
pass.End()
cmdBuffer := encoder.Finish(nil)

// Texture with lazy view
texture := native.CreateCoreTexture(device, descriptor)
view := texture.GetDefaultView() // Thread-safe, created once

// Pipeline caching
cache := native.NewPipelineCacheCore(device)
pipeline := cache.GetOrCreateRenderPipeline(descriptor)
```

**Statistics:** +8,700 LOC, 9 tasks completed, enterprise-grade patterns

**Fixes:** [#45](https://github.com/gogpu/gg/issues/45) — Color emoji rendering

### v1.0.0 — Production Release

**Status:** Target | **ETA:** After community validation

- API stability guarantee
- Semantic versioning commitment
- Long-term support plan
- Enterprise deployment guide

---

## Architecture

```
                           gg (Public API)
                                │
            ┌───────────────────┼───────────────────┐
            │                   │                   │
      Immediate Mode       Retained Mode        Resources
      (Context API)        (Scene Graph)     (Images, Fonts)
            │                   │                   │
            └───────────────────┼───────────────────┘
                                │
                       RenderBackend Interface
                                │
               ┌────────────────┼────────────────┐
               │                │                │
          Software            SIMD              GPU
          (v0.1.0+)         (v0.5.0)         (v0.9.0)
               │                │                │
               └────────────────┴────────────────┘
                                │
                     gogpu/wgpu (Pure Go WebGPU)
```

### Key Patterns (Rust-Inspired)

**Dual-Stream Encoding (vello)**
```
Scene → Tags Stream + Data Stream → GPU Commands
         [Fill, Stroke, Layer...]   [coords, colors, transforms...]
```

**Sparse Strips (vello 2025)**
```
Path → CPU Tessellation → Strips → GPU Rasterization → Composite
```

**Layer Compositing (Skia/vello)**
```
PushLayer(blend, opacity) → Draw operations → PopLayer() → Composite
```

---

## Released Versions

| Version | Date | Highlights | LOC |
|---------|------|------------|-----|
| **v0.21.1** | **2026-01-28** | **Subpath fix for dashed strokes ([#54](https://github.com/gogpu/gg/issues/54))** | **+200** |
| v0.21.0 | 2026-01-26 | Enterprise Architecture, UI integration | +8,163 |
| v0.20.1 | 2026-01-24 | wgpu v0.10.2 (CGO fix) | — |
| v0.20.0 | 2026-01-22 | GPU Backend Completion (enterprise-grade) | +8,700 |
| v0.19.0 | 2026-01-22 | Anti-Aliased Rendering (tiny-skia) | +700 |
| v0.18.x | 2026-01 | Renderer DI, Backend refactoring | +400 |
| v0.17.x | 2026-01 | Dependency updates | — |
| v0.16.0 | 2026-01 | wgpu v0.9.0, cleanup | — |
| v0.15.x | 2025-12/2026-01 | GPU compute shaders, dependency updates | +6,470 |
| v0.14.0 | 2025-12-24 | Masks, PathBuilder, Close, EncodePNG | +800 |
| v0.13.0 | 2025-12-24 | Go 1.25+: Iterators, Cache, Context, Wrapping | +1,700 |
| v0.12.0 | 2025-12-24 | Brush, Gradients, Stroke, Dash | +4,337 |
| v0.11.0 | 2025-12-24 | MSDF, Emoji, Subpixel text | +16,200 |
| v0.10.0 | 2025-12-24 | GPU Text Pipeline | +2,500 |
| v0.9.0 | 2025-12-18 | GPU Backend (Sparse Strips) | +9,930 |
| v0.8.0 | 2025-12-18 | Backend Abstraction | +595 |
| v0.7.0 | 2025-12-18 | Scene Graph (Retained Mode) | +15,376 |
| v0.6.0 | 2025-12-17 | Parallel Rendering | +6,372 |
| v0.5.0 | 2025-12-17 | SIMD Optimization | +3,200 |
| v0.4.0 | 2025-12-17 | Color Pipeline, Layers | +1,500 |
| v0.3.0 | 2025-12-16 | Images, Clipping, Compositing | +4,800 |
| v0.2.0 | 2025-12-16 | Text Rendering | +2,200 |
| v0.1.0 | 2025-12-12 | Core 2D Drawing | +3,500 |

---

## Research Documents

| Document | Purpose |
|----------|---------|
| `RESEARCH-010-api-review-v1.0.0.md` | API compatibility analysis |
| `RESEARCH-011-rust-2d-deep-dive.md` | Rust patterns deep dive (vello, tiny-skia, kurbo) |
| `RESEARCH-007-gpu-backend.md` | GPU backend architecture |
| `RESEARCH-006-scene-graph.md` | Scene graph design |

---

## Non-Goals

- **3D graphics** — See gogpu/gogpu
- **Animation system** — Application layer concern
- **GUI widgets** — See gogpu/ui (planned)
- **Platform rendering** — Pure Go, platform-independent

---

## Contributing

We welcome contributions! Priority areas:

1. **API Feedback** — Try the library and report pain points
2. **Test Cases** — Expand test coverage
3. **Examples** — Real-world usage examples
4. **Documentation** — Improve docs and guides
5. **Performance** — Benchmark and optimize hot paths

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## License

MIT License — see [LICENSE](LICENSE) for details.
