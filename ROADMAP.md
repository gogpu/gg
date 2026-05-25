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

## Current State: v0.48.6

✅ **Production-ready** with GPU-accelerated rendering:
- **Text stroke/outline** (ADR-033) — StrokeString + TextPath, Skia/Cairo/HTML5 pattern
- **Aliased text** (ADR-034) — TextModeAliased on GPU (Tier 6 NoAAFiller) AND CPU (per-glyph binary rasterization), Skia kAlias parity
- **Per-glyph text rendering** (ADR-039) — fractional glyph advances (Skia linearMetrics), hinted outlines + fractional positioning
- **Stroke inner join** (ADR-038) — tiny-skia parity, no teeth on thick strokes
- **SparseStripsFiller winding** — Vello backdrop.wgsl prefix-sum, windingDelta propagation
- **SDF thin stroke fallback** (ADR-040) — lineWidth < 2.0 → geometric expansion
- **NaN safety** (ADR-035) — depth guards on all 12 recursive flatten functions
- **Damage union** — forwardDamageRects unions explicit + frame damage
- **CJK text rendering** (ADR-027) — script-aware hinting, exact-size rasterization, dual MSDF atlas 64/128px
- **Damage-aware compositing** (ADR-026) — LoadOpLoad + scissor, TrackDamageRect, debug overlay dedup
- **Scene text TagText** (ADR-022) — glyph references, shape-once, DrawShapedGlyphs (Skia drawTextBlob)
- **ClearType LCD auto-detection** (ADR-024) — Windows SPI, macOS None, Linux Xft/Wayland
- **Four-level damage pipeline** (ADR-021) — Object Diff → Tile Dirty → GPU Scissor → OS Present
- Seven-tier GPU render pipeline (SDF + Convex + Stencil+Cover + Textured Quad + MSDF Text + Compute + Glyph Mask)
- Zero-readback compositor pipeline (ADR-015/016), single command buffer (ADR-017)
- All 5 backends: Vulkan, DX12, DX12+DXIL, GLES, Software
- Skia AAA pixel-perfect rasterizer, Vello 9-stage compute pipeline
- Smart multi-engine rasterizer — 6 algorithms with per-path auto-selection
- SVG renderer, Recording System (PDF/SVG export), premultiplied alpha, 29 blend modes

---

## Upcoming

### v0.49.0 — Next
- [ ] Gradient support — BrushLinearGradient/BrushRadialGradient in scene
- [ ] GPU-CLIP-003d — stencil-based arbitrary path clip for remaining shapes

### v0.48.6 — Current
- [x] **SparseStripsFiller winding propagation** (BUG-SPARSE-STRIPS-001) — Vello backdrop.wgsl parity
- [x] **SDF thin stroke fallback** (#346, ADR-040) — lineWidth < 2.0 → geometric expansion
- [x] **Present damage union** — forwardDamageRects unions explicit + frame damage

### v0.48.5 ✅ Released
- [x] **TextModeAliased CPU fallback** (#353) — per-glyph NoAAFiller, works without GPU
- [x] **Fractional glyph advances** (ADR-039) — Skia linearMetrics, letters no longer merge at 10-12px
- [x] **Per-glyph text rendering** — text.Draw replaced font.Drawer with RasterizeHinted

### v0.48.4 ✅ Released
- [x] **Stroke inner join teeth** (#354, #353, ADR-038) — tiny-skia stroker.rs parity

### v0.48.0–v0.48.3 ✅ Released
- [x] Text stroke (ADR-033), TextModeAliased GPU (ADR-034), zero-alloc paint (ADR-036)
- [x] Scissor coalescing (#335 @celer), NaN safety (ADR-035), polygon rotation (#334 @rcarlier)
- [x] GPU stroke polyline fix (#347 @TuSKan), stroke expander kurbo parity, BUG-SDF-001

### v0.47.0–v0.47.4 ✅ Released
- [x] Pixel-Perfect Mode (ADR-030), text batch coalescing (ADR-031)
- [x] HiDPI damage scaling, NewPixmapFromBuffer zero-copy

### v0.46.0–v0.46.11 ✅ Released

### v0.45.2–v0.45.3 ✅ Released
- [x] GPU scene clip: transform Push/Pop fix (BUG-GG-GPU-SCENE-CLIP-001)
- [x] Rect clips → hardware scissor in GPUSceneRenderer
- [x] SetDamageTracking API (ADR-021)
- [x] Flash-and-fade damage debug overlay (GOGPU_DEBUG_DAMAGE=1)
- [x] Scene Append layer-aware encoding
- [x] TagStroke LineCap/LineJoin/MiterLimit fix
- [x] Software backend softwareMode flag (lazy GPU init)

### v0.44.0–v0.45.1 ✅ Released
- [x] GPU-CLIP-003a: Depth-based arbitrary path clipping (Impeller/Graphite pattern)
- [x] GPU-CLIP-003b: Vello coarse.wgsl clip tag dispatch
- [x] dc.Clip() GPU bridge + stencil-then-cover-to-depth
- [x] Four-level damage pipeline (ADR-021, ADR-020)
- [x] Adapter-aware render mode (GOGPU_RENDER_MODE)
- [x] clip_path + damage_demo examples

### v0.43.3–v0.43.7 ✅ Released
- [x] Scene fixes (CTM, transform stack, LoadOp, image index, AppendWithTranslation)
- [x] DrawGPUTextureWithOpacity, ADR-018 type-safe handles, ADR-019 render pass blit
- [x] Auto-hinter stem collapse 12px, Retina text half-size fix (#276)
- [x] deps cascade: wgpu v0.26.12, gogpu v0.31.0, naga v0.17.10

### v0.43.1–v0.43.2 ✅ Released
- [x] Blit-only fix, type-safe GPU handles (ADR-018), overlay fix, 14 enterprise tests

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
| **v0.48.6** | 2026-05 | SparseStripsFiller winding (Vello parity), SDF thin stroke fallback (#346), damage union |
| v0.48.5 | 2026-05 | TextModeAliased CPU (#353), fractional advances (ADR-039), per-glyph rendering |
| v0.48.4 | 2026-05 | Stroke inner join teeth (#354, ADR-038, tiny-skia parity) |
| v0.48.0–3 | 2026-05 | Text stroke (ADR-033), aliased text GPU (ADR-034), GPU stroke fix (#347) |
| v0.47.0–4 | 2026-05 | Pixel-Perfect Mode (ADR-030), text batch coalescing, HiDPI damage |
| v0.46.0–11 | 2026-05 | CJK (ADR-027), damage pipeline (ADR-026), scene text (ADR-022), LCD auto-detect |
| v0.43.0–v0.45.4 | 2026-04 | Compositor APIs, single command buffer, damage pipeline, GPU clips |
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
