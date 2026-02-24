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

## Current State: v0.29.5

✅ **Production-ready** with GPU-accelerated rendering:
- Canvas API, Text, Images, Clipping, Layers
- Five-tier GPU render pipeline (SDF + Convex + Stencil-then-Cover + MSDF Text + Compute)
- Vello 8-stage compute pipeline for full-scene GPU rasterization
- GPU MSDF text pipeline (resolution-independent anti-aliased text)
- GPU stroke rendering (stroke-expand-then-fill via convex polygon renderer)
- RenderDirect zero-copy GPU surface rendering
- Analytic anti-aliasing (Vello tile-based AA)
- GPUAccelerator interface with transparent CPU fallback
- PipelineMode (Auto/RenderPass/Compute) for pipeline selection
- Recording System for vector export (PDF, SVG)
- GGCanvas integration for gogpu windowed rendering (auto-registration)
- Porter-Duff compositing for correct GPU readback
- Premultiplied alpha pipeline for correct compositing
- HarfBuzz-level text shaping via GoTextShaper
- Structured logging via log/slog

**New in v0.29.5:**
- AdvanceX drift fix for edge expansion (#95)
- coverageToRuns maxValue bug fix (#95)
- wgpu v0.16.13, gogpu v0.20.4

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

### v0.27.0 — GPU Acceleration Foundation ✅ Released
- [x] gg/gpu package with wgpu-based GPUAccelerator
- [x] SDF GPU acceleration for circles, ellipses, rects, rounded rects
- [x] Compute shader pipeline via naga WGSL compiler
- [x] Structured logging via log/slog

### v0.28.0 — Three-Tier GPU Rendering ✅ Released
- [x] Three-tier GPU render pipeline (SDF + Convex + Stencil-then-Cover)
- [x] Fan tessellator for path-to-triangle conversion
- [x] RenderDirect zero-copy GPU surface rendering
- [x] EvenOdd fill rule support in stencil pipeline
- [x] ggcanvas deferred texture destruction for DX12 stability

### v0.29.4 — Scene Renderer Bug Fixes ✅ Released
- [x] Delegate rasterization to gg.SoftwareRenderer (#124)
- [x] sync.Pool per-tile SoftwareRenderer/Pixmap reuse
- [x] Premultiplied source-over alpha compositing in tile compositor
- [x] Background preservation (no more tile data destruction)
- [x] Full curve support in scene strokes (CubicTo, QuadTo)

### v0.30.0 — Vello Compute Pipeline (In Progress)
- [x] 8-stage compute pipeline (pathtag → fine) ported from vello
- [x] 8 WGSL compute shaders with hal-based GPU dispatch
- [x] tilecompute CPU reference implementation (RasterizeScenePTCL)
- [x] Scene encoding: PathDef → EncodeScene → PackScene → flat u32 buffer
- [x] Euler spiral curve flattening (Levien's algorithm)
- [x] PipelineMode API (Auto/RenderPass/Compute)
- [x] GPU vs CPU golden tests (7 scenes)
- [ ] VelloAccelerator integration with GPUAccelerator interface
- [ ] Auto-selection heuristics for PipelineModeAuto

### v0.29.0 — GPU Text Rendering ✅ Released
- [x] MSDF text pipeline (Tier 4 in GPURenderSession)
- [x] WGSL MSDF fragment shader (median + smoothstep)
- [x] Persistent text vertex/index/uniform buffers
- [ ] Atlas generation from font glyphs
- [ ] DrawString → GPU text batch integration

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
| **v0.29.5** | 2026-02 | AdvanceX drift fix, coverageToRuns maxValue fix (#95) |
| v0.29.4 | 2026-02 | scene.Renderer delegation to SoftwareRenderer (#124) |
| v0.29.3 | 2026-02 | wgpu v0.16.12 (Vulkan debug object naming) |
| v0.29.2 | 2026-02 | wgpu v0.16.11 (Vulkan zero-extent swapchain fix) |
| v0.29.1 | 2026-02 | wgpu v0.16.10, naga v0.14.2 |
| v0.29.0 | 2026-02 | GPU MSDF text pipeline, four-tier rendering, GPU strokes |
| v0.28.6 | 2026-02 | wgpu v0.16.6 (Metal debug logging, goffi v0.3.9) |
| v0.28.5 | 2026-02 | wgpu v0.16.5 (per-encoder command pools) |
| v0.28.4 | 2026-02 | wgpu v0.16.4 (timeline semaphore, FencePool), naga v0.13.1 |
| v0.28.3 | 2026-02 | wgpu v0.16.3 (per-frame fence tracking, WaitIdle) |
| v0.28.2 | 2026-02 | Persistent GPU buffers, fence-free submit, buffer pooling |
| v0.28.1 | 2026-02 | Porter-Duff GPU compositing, event-driven example |
| v0.28.0 | 2026-02 | Three-tier GPU rendering, RenderDirect, ggcanvas improvements |
| v0.27.x | 2026-02 | SDF GPU acceleration, compute shaders, structured logging |
| v0.26.0 | 2026-02 | GPUAccelerator interface, architecture refactor, clean dependencies |
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
