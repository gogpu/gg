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

## Current State: v0.12.0

**46,000+ LOC** | **87.6% Coverage** | **0 Linter Issues**

| Version | Focus |
|---------|-------|
| v0.1.0 | Canvas API |
| v0.2.0 | Text Rendering |
| v0.3.0 | Images, Clipping |
| v0.4.0 | Colors, Layers |
| v0.5.0 | SIMD Optimization |
| v0.6.0 | Parallel Rendering |
| v0.7.0 | Scene Graph |
| v0.8.0 | Backend Abstraction |
| v0.9.0 | GPU Backend |
| v0.10.0 | Text Pipeline |
| v0.11.0 | MSDF, Emoji |
| **v0.12.0** | **Brush, Gradients, Stroke** |

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

**Status:** Planned | **Target:** Q1 2025

Full adoption of Go 1.25+ features for modern, idiomatic API.

| Feature | Description |
|---------|-------------|
| **context.Context** | Cancellation for long operations |
| **Generic Cache** | `Cache[K, V]` type-safe caching |
| **iter.Seq** | Shape interface with iterator protocol |
| **Text Wrapping** | Word-wrap with configurable break points |

```go
// Context support
ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
defer cancel()
err := renderer.RenderWithContext(ctx, scene) // Cancellable!

// Generic cache
cache := gg.NewCache[string, *gg.Texture](100)

// Iterator-based shapes
for elem := range path.Elements() {
    // Process path elements
}
```

### v0.14.0 — Advanced Features

**Status:** Planned | **Target:** Q2 2025

Professional graphics features for complex applications.

| Feature | Description |
|---------|-------------|
| **Masks** | AsMask, SetMask, InvertMask operations |
| **PathBuilder** | Fluent path construction pattern |
| **Resource Cleanup** | Context.Close() for deterministic cleanup |
| **Streaming PNG** | EncodePNG(io.Writer) for large images |

### v0.15.0 — Documentation & RC

**Status:** Planned | **Target:** Q2 2025

Comprehensive documentation and release candidate.

| Deliverable | Description |
|-------------|-------------|
| **API Documentation** | Every public symbol documented |
| **Migration Guide** | fogleman/gg → gogpu/gg |
| **Examples** | 20+ working examples |
| **Benchmarks** | Performance comparison suite |

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
