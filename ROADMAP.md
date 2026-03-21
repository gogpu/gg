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

## Current State: v0.37.4

✅ **Production-ready** with GPU-accelerated rendering, 81.5% test coverage:
- Canvas API, Text, Images, Clipping, Layers
- **Six-tier GPU render pipeline** (SDF + Convex + Stencil-then-Cover + MSDF Text + Compute + Glyph Mask)
- **Smart multi-engine rasterizer** — 5 algorithms with per-path auto-selection
- Vello 9-stage compute pipeline for full-scene GPU rasterization
- GPU MSDF text + Glyph mask dual-strategy text rendering
- GPU stroke rendering, RenderDirect zero-copy GPU surface rendering
- **GPU RRect clip** — analytic SDF in fragment shaders (GPU-CLIP-002)
- **GPU scissor rect clip** — hardware scissor for rectangular clips (GPU-CLIP-001)
- **Separated deviceMatrix/userMatrix** — Cairo/Skia/Blend2D pattern for correct HiDPI
- **SVG renderer** (`gg/svg` package) — parse + render SVG XML for JB-quality icons
- **SVG path parser** — `ParseSVGPath()` for SVG `d` attribute
- Recording System for vector export (PDF, SVG)
- wgpu public API migration (zero hal imports in production GPU code)
- Font hinting, ClearType LCD subpixel rendering
- Premultiplied alpha, 29 blend modes, structured logging

---

## Upcoming

### v0.38.0 — Planned
- [ ] GPU-CLIP-003: Stencil-based path clipping for text + arbitrary shapes (#205)
- [ ] GPU-LAYER-001: GPU render-to-texture layer compositing
- [ ] Restore LCD ClearType in Tier 6 (Intel Vulkan compatible)
- [ ] naga MSL compute shader support (threadgroup vars, mem_flags)
- [ ] Fix dashQuad/dashCubic off-by-one iteration bug (found by tests)

### v0.37.4 — HiDPI Coordinate Fix 🔧 In Progress
- [x] Separate deviceMatrix from user CTM (Cairo/Skia/Blend2D pattern) (#218)
- [x] Test coverage 77% → 81.5% for awesome-go submission
- [x] Found dashQuad/dashCubic off-by-one bug via tests

### v0.37.0–v0.37.3 ✅ Released
- [x] Migrate internal/gpu from hal to wgpu public API (zero hal imports)
- [x] Explicit SetViewport in all GPU render passes (#171)
- [x] Universal `ggcanvas.Render()` — one call, all backends
- [x] GLES/Software backend support
- [x] Pipeline clip recreation fix
- [x] naga v0.14.7–v0.14.8, wgpu v0.21.0–v0.21.3

### v0.36.0–v0.36.4 ✅ Released
- [x] GPU Glyph Mask Cache (Tier 6) — CPU rasterize → R8 alpha atlas → GPU composite
- [x] RoundRectShape with SDF tile rendering
- [x] BeginClip/EndClip in tile renderer
- [x] Font hinting (auto-hinter ≤48px), ClearType LCD infrastructure
- [x] GPU scissor rect clipping (GPU-CLIP-001)
- [x] GPU RRect SDF clip in fragment shaders (GPU-CLIP-002)
- [x] naga MSL buffer(0) collision fix (v0.14.7)

### v0.33.0–v0.35.3 ✅ Released
- [x] Text DPI scaling, MSDF fixes, TextMode API, vector text
- [x] HiDPI/Retina platform support (gogpu v0.23.0+)
- [x] MSDF atlas FontID collision fix

### v0.25.0–v0.32.1 ✅ Released
- [x] Vello tile-based AA, architecture refactor, GPU acceleration
- [x] Five-tier GPU rendering, MSDF text, Vello 9-stage compute
- [x] Smart multi-engine rasterizer, CoverageFiller, RasterizerMode
- [x] Text API redesign, CPU text transform, Recording System

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
   (Context API)      (scene.Renderer)    (Images, Fonts)
         │                   │                   │
         │              orchestration            │
         │         (tiles, workers, cache)       │
         │                   │                   │
         └───────────────────┼───────────────────┘
                             │ delegation
              ┌──────────────┴──────────────┐
              │                             │
      SoftwareRenderer                GPUAccelerator
      (always available)              (opt-in via gpu/)
              │                             │
    internal/raster              internal/gpu (five-tier)
                                 ├── Tiers 1-4 (render pass)
                                 └── Tier 5 (compute pipeline)
```

---

## Released Versions

| Version | Date | Highlights |
|---------|------|------------|
| **v0.37.4** | 2026-03 | Separate deviceMatrix/userMatrix (#218), test coverage 81.5% |
| v0.37.3 | 2026-03 | Universal `ggcanvas.Render()`, GLES/Software support |
| v0.37.2 | 2026-03 | Pipeline clip recreation + wgpu v0.21.2 validation |
| v0.37.1 | 2026-03 | wgpu v0.21.1, gogpu v0.24.2 |
| v0.37.0 | 2026-03 | wgpu public API migration, SetViewport fix (#171), naga v0.14.7 |
| v0.36.4 | 2026-03 | GPU RRect SDF clip (GPU-CLIP-002), ClipRoundRect API |
| v0.36.0–3 | 2026-03 | Glyph Mask Tier 6, RoundRectShape, scissor clip, font hinting, ClearType |
| v0.35.x | 2026-03 | MSDF atlas fixes, text DPI scaling |
| v0.34.x | 2026-03 | HiDPI/Retina support, diagnostic logging |
| v0.33.x | 2026-03 | TextMode API, MSDF quality improvements |
| v0.32.x | 2026-02 | Smart multi-engine rasterizer, CPU text transform |
| v0.31.x | 2026-02 | Text API redesign, DrawStringWrapped |
| v0.30.x | 2026-02 | Vello 9-stage compute pipeline |
| v0.29.x | 2026-02 | GPU MSDF text, scene.Renderer delegation |
| v0.28.x | 2026-02 | Three-tier GPU rendering, RenderDirect |
| v0.27.x | 2026-02 | SDF GPU acceleration, compute shaders |
| v0.25–26 | 2026-02 | Vello AA rasterizer, architecture refactor |
| v0.1–24 | 2025-12 – 2026-02 | Core features, GPU backend, text, images, recording |

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
