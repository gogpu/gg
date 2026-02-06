# gg Roadmap

> **Enterprise-Grade 2D Graphics Library for Go**
>
> Designed to power IDEs, browsers, and professional graphics applications.

---

## Vision

**gg** is a Pure Go 2D graphics library following modern Rust patterns (vello, tiny-skia, kurbo). Our goal is to provide production-ready 2D rendering for Go applications without CGO dependencies.

### Core Principles

1. **Pure Go** — No CGO, easy cross-compilation, single binary deployment
2. **GPU-First** — Designed for GPU acceleration from day one
3. **Production-Ready** — Enterprise-grade error handling and patterns
4. **API Stability** — Semantic versioning with clear deprecation policy

---

## Current State: v0.26.0

✅ **Production-ready** for CPU rendering with full feature set:
- Canvas API, Text, Images, Clipping, Layers
- Analytic anti-aliasing (Vello tile-based AA)
- GPUAccelerator interface for optional GPU acceleration
- Internal architecture: CPU raster core + optional GPU
- Recording System for vector export (PDF, SVG)
- GGCanvas integration with gpucontext.TextureDrawer interface
- Premultiplied alpha pipeline for correct compositing
- HarfBuzz-level text shaping via GoTextShaper

---

## Upcoming

### v0.25.0 — Rendering Quality ✅ Released
- [x] Vello tile-based analytic AA rasterizer
- [x] VelloLine float pipeline (bypass fixed-point quantization)

### v0.26.0 — Architecture Refactor ✅ Released
- [x] Extract internal/raster core package
- [x] GPUAccelerator interface with transparent fallback
- [x] Remove backend/ abstraction layer
- [x] Move implementation details to internal/
- [x] Clean go.mod dependencies

### v0.27.0 — GPU Acceleration (Planned)
- [ ] gg/gpu package with wgpu-based GPUAccelerator
- [ ] GPU-accelerated fill and stroke operations
- [ ] Native curve evaluation in tiles
- [ ] Performance optimizations

### v1.0.0 — Production Release
- [ ] API stability guarantee
- [ ] Semantic versioning commitment
- [ ] Long-term support plan
- [ ] Enterprise deployment guide
- [ ] Comprehensive documentation

---

## Future Ideas

| Theme | Description |
|-------|-------------|
| **WebAssembly** | WASM target for browser rendering |
| **SVG Import** | SVG file parsing and rendering |
| **Advanced Text** | Complex text shaping (HarfBuzz-style) |

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
              ┌──────────────┴──────────────┐
              │                             │
         CPU Raster                   GPUAccelerator
      (always available)             (optional, planned)
              │
    internal/raster + internal/native
```

---

## Released Versions

| Version | Date | Highlights |
|---------|------|------------|
| **v0.26.0** | 2026-02 | GPUAccelerator interface, architecture refactor, clean dependencies |
| v0.25.0 | 2026-02 | Vello tile-based analytic AA rasterizer, VelloLine float pipeline |
| v0.24.x | 2026-02 | Premultiplied alpha, HarfBuzz shaping, WebP, gogpu_integration |
| v0.23.0 | 2026-02 | Recording System for vector export (PDF, SVG backends) |
| v0.22.x | 2026-01/02 | gpucontext.TextureDrawer integration, naga v0.10.0, wgpu v0.13.0 |
| v0.21.x | 2026-01 | Enterprise architecture, stroke quality fixes |
| v0.20.x | 2026-01 | GPU backend completion |
| v0.19.x | 2026-01 | Anti-aliased rendering |
| v0.15.x | 2025-12 | GPU compute shaders |
| v0.12-14 | 2025-12 | Brush/Gradients, Go 1.25+, Masks |
| v0.9-11 | 2025-12 | GPU backend, Scene graph, Text pipeline |
| v0.1-8 | 2025-12 | Core features |

→ **See [CHANGELOG.md](CHANGELOG.md) for detailed release notes**

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

## Non-Goals

- **3D graphics** — See gogpu/gogpu
- **Animation system** — Application layer concern
- **GUI widgets** — See gogpu/ui (planned)

---

## License

MIT License — see [LICENSE](LICENSE) for details.
