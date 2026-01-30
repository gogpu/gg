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

## Current State: v0.22.0

✅ **Production-ready** for CPU rendering with full feature set:
- Canvas API, Text, Images, Clipping, Layers
- Anti-aliased rendering (4x supersampling)
- GPU backend (sparse strips, compute shaders)
- Enterprise architecture for UI integration
- **GGCanvas integration** with gpucontext.TextureDrawer interface

---

## Upcoming

### v0.23.0 — Polish & Performance
- [ ] Vello-style AA improvements
- [ ] Performance optimizations
- [ ] API cleanup before v1.0

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
| **SVG Support** | SVG import/export |
| **PDF Export** | Vector PDF generation |
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
                   RenderBackend Interface
                            │
           ┌────────────────┼────────────────┐
           │                │                │
      Software            SIMD              GPU
           │                │                │
           └────────────────┴────────────────┘
                            │
                 gogpu/wgpu (Pure Go WebGPU)
```

---

## Released Versions

| Version | Date | Highlights |
|---------|------|------------|
| **v0.22.x** | 2026-01 | gpucontext.TextureDrawer integration |
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
