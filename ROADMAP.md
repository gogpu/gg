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

## Current State: v0.43.1

✅ **Production-ready** with GPU-accelerated rendering, 81.5% test coverage:
- Canvas API, Text, Images, Clipping, Layers
- **Seven-tier GPU render pipeline** (SDF + Convex + Stencil-then-Cover + Textured Quad + GPU Texture Composite + MSDF Text + Compute + Glyph Mask)
- **Zero-readback compositor pipeline** (ADR-015/016) — FlushPixmap, DrawGPUTextureBase, BeginGPUFrame, FillRectCPU, non-MSAA blit path (93% bandwidth reduction)
- **Single command buffer compositor** (ADR-017, Flutter Impeller pattern) — CreateSharedEncoder + SetSharedEncoder + SubmitSharedEncoder for multi-context frames
- **GPU-to-GPU texture compositing** — DrawGPUTexture + CreateOffscreenTexture (Flutter pattern, zero readback)
- **Bullet-proof encoder lifecycle** — defer-based safety, no silently swallowed errors
- **Per-context GPU accelerator** (ADR-013) — Skia GrContext pattern, multi-context isolation
- **Skia AAA pixel-perfect rasterizer** — trapezoid decomposition, diff=0 vs C++ Skia
- **GPU DrawImage** — Tier 3 textured quad, zero mid-frame CPU flush
- **Scene GPU auto-select** — GPU rendering for retained-mode scenes
- **Smart multi-engine rasterizer** — 5 algorithms with per-path auto-selection
- **Zero-alloc hot path** — QueueShape 26ns/0allocs, ScissorSegment 13ns/0allocs
- **TexturePool** — Flutter RenderTargetCache pattern, per-frame lifecycle
- **Path SOA representation** — `[]PathVerb` + `[]float64` (ADR-010, enterprise standard)
- **Vello compute clip pipeline** — BeginClip/EndClip with packed blend stack (ADR-012)
- **Alpha mask API** — per-shape, per-layer, luminance masks, GPU interface
- Vello 9-stage compute pipeline for full-scene GPU rasterization
- GPU MSDF text + Glyph mask dual-strategy text rendering
- GPU stroke rendering, RenderDirect zero-copy GPU surface rendering
- **GPU RRect clip** — analytic SDF in fragment shaders (GPU-CLIP-002)
- **GPU scissor rect clip** — hardware scissor for rectangular clips (GPU-CLIP-001)
- **Separated deviceMatrix/userMatrix** — Cairo/Skia/Blend2D pattern for correct HiDPI
- **SVG renderer** (`gg/svg` package) — parse + render SVG XML for JB-quality icons
- Recording System for vector export (PDF, SVG)
- Font hinting, ClearType LCD subpixel rendering
- Premultiplied alpha, 29 blend modes, structured logging

---

## Upcoming

### v0.43.1 — In Progress
- [x] Type-safe GPU handles (ADR-018) — `any` → `unsafe.Pointer` opaque structs in gpucontext
- [x] Blit-only black screen fix + GPU texture resource leak fix
- [x] GPU texture overlay stretched fix (BUG-GG-GPU-TEXTURE-OVERLAY-SIZE) — separate vertex buffers
- [x] Enterprise GPU texture tests (14 tests: vertices, ortho, queueing, isBlitOnly, regression)
- [x] `blit_only` example + documentation

### v0.44.0 — Planned
- [ ] GPU-CLIP-003: Stencil-based path clipping for text + arbitrary shapes (#205)
- [ ] GPU-LAYER-001: GPU render-to-texture layer compositing
- [ ] Restore LCD ClearType in Tier 6 (Intel Vulkan compatible)
- [ ] Vello compute clip GPU shaders (clip_reduce.wgsl + clip_leaf.wgsl)

### Pre-1.0.0 — Public API Freeze Blockers
- [ ] **API-001: GPU handle API shape** — generics `Handle[Tag]` vs plain structs (ADR-018 follow-up). Must decide before 1.0.0 — changing after = breaking. See `docs/dev/kanban/0-backlog/API-001-gpu-handle-generics-vs-struct.md`
- [ ] **API-002: Eliminate remaining `any`** — `face any` in DrawText (circular dep), `Canvas.texture any` (internal), `PresentTexture(tex any)` (cross-package token)
- [ ] **API-003: Public API review** — full audit of exported types, methods, interfaces before freeze

### v0.43.0–v0.43.1 ✅ Released
- [x] Zero-readback compositor pipeline (ADR-015/016) — FlushPixmap, DrawGPUTextureBase, BeginGPUFrame, non-MSAA blit path
- [x] Single command buffer compositor (ADR-017, Flutter Impeller pattern) — CreateSharedEncoder + SubmitSharedEncoder
- [x] Non-MSAA blit-only fast path — 93% bandwidth reduction for compositor-only frames
- [x] FillRectCPU + Pixmap.FillRect — CPU-only rect fill bypassing GPU accelerator
- [x] FlushGPUWithViewDamage — damage-aware sub-region compositing
- [x] Blit-only black screen fix — early return skipped baseLayer-only frames
- [x] GPU texture resource leak fix — session-level persistent buffers (grow-only)
- [x] `blit_only` example — standalone non-MSAA compositor demo
- [x] Dependencies: wgpu v0.26.4, gogpu v0.29.2

### v0.42.0–v0.42.1 ✅ Released
- [x] GPU-to-GPU texture compositing — DrawGPUTexture + CreateOffscreenTexture (Flutter pattern)
- [x] Bullet-proof encoder lifecycle — defer-based safety, MinBindingSize
- [x] DrawGPUTexture deep-copy fix (v0.42.1)

### v0.41.0 ✅ Released
- [x] Per-context GPU accelerator (ARCH-GG-001, ADR-013) — Skia GrContext pattern
- [x] GPU textured quad pipeline (Tier 3) — DrawImage GPU rendering
- [x] Skia AAA pixel-perfect rasterizer — coverage diff=0 vs C++ Skia
- [x] Convex fast path (RAST-012) — 1.6x faster, zero allocs
- [x] Scene GPU auto-select — GPU rendering for retained-mode scenes
- [x] TexturePool — Flutter RenderTargetCache pattern
- [x] Per-pass View routing (PR #255) — WebGPU spec alignment
- [x] BUG-RAST-011 shadow fix (#235) — near-horizontal edge bleed eliminated

### v0.40.0–v0.40.1 ✅ Released
- [x] Alpha mask API — per-shape, per-layer, luminance, GPU interface (v0.40.0)
- [x] Adreno Vulkan fix — packed blend stack + PIXELS_PER_THREAD=4 (#252)
- [x] Vello compute clip pipeline — SceneElement BeginClip/EndClip (ADR-012)
- [x] Buffer.Map API migration, deps update (wgpu v0.25.1, naga v0.17.4)
- [x] Removed incorrect gogpu dependency from go.mod

### v0.39.0–v0.39.4 ✅ Released
- [x] Path SOA representation — zero per-verb allocs (ADR-010)
- [x] Zero-alloc rasterizer — FillRect/FillCircle 0 allocs/op
- [x] Comprehensive allocation reduction (gradients, scene, stroke, worker pool)
- [x] 3 dead naga SPIR-V workarounds removed (GPU golden verified)
- [x] Clear() API fix (#227), MSDF Retina fix (#247), ParseHex (#237)
- [x] dashQuad/dashCubic off-by-one fix

### v0.37.4–v0.38.1 ✅ Released
- [x] Separate deviceMatrix from user CTM (#218), test coverage 81.5%
- [x] Enterprise SVG renderer, SVG path parser, ClearType LCD pipeline
- [x] DrawImage rotation fix (#224), GLES blit fix (#226)

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
    internal/raster              internal/gpu (seven-tier)
                                 ├── Tiers 1-4, 6 (render pass)
                                 └── Tier 5 (compute pipeline)
```

---

## Released Versions

| Version | Date | Highlights |
|---------|------|------------|
| **v0.42.0** | 2026-04 | GPU texture compositing (DrawGPUTexture + CreateOffscreenTexture), Flutter pattern |
| v0.41.0–2 | 2026-04 | Per-context GPU (ADR-013), Tier 3 textured quad, Skia AAA, ImageCache genID, text kerning |
| v0.40.1 | 2026-04 | Adreno fix (#252), Vello compute clip, Buffer.Map, deps update |
| v0.40.0 | 2026-04 | Alpha mask API — per-shape, per-layer, luminance, GPU |
| v0.39.0–4 | 2026-04 | Path SOA (ADR-010), zero-alloc rasterizer, MSDF Retina fix |
| v0.38.0–2 | 2026-03 | SVG renderer, Clear() fix (#227), DrawImage rotation (#224) |
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
