# gg Roadmap

> **Enterprise-Grade 2D Graphics Library for Go**
>
> Designed to power IDEs, browsers, and professional graphics applications.

---

## Vision

**gg** aims to become the **reference 2D graphics library** for the Go ecosystem — comparable to:
- **tiny-skia** (Rust) — Software rendering
- **vello** (Rust) — GPU rendering
- **Skia** (C++) — Industry standard

---

## Released

### v0.1.0 — Core 2D Drawing

- [x] Canvas API (NewContext, SetSize)
- [x] Basic shapes (Rectangle, Circle, Ellipse, Line, Arc)
- [x] Path operations (MoveTo, LineTo, QuadraticTo, CubicTo)
- [x] Fill and stroke (Fill, Stroke, SetColor, SetLineWidth)
- [x] Transformations (Translate, Scale, Rotate, Push/Pop)
- [x] Color support (RGBA, Hex parsing, named colors)
- [x] Image output (SavePNG, SaveJPG)
- [x] Software rasterizer (scanline algorithm)

### v0.2.0 — Text Rendering ✅

- [x] TrueType font loading (FontSource, FontParser)
- [x] Text rendering (DrawString, DrawStringAnchored)
- [x] Font metrics (MeasureString, Metrics)
- [x] Face interface with Go 1.25+ iterators
- [x] MultiFace for font fallback
- [x] FilteredFace for Unicode ranges
- [x] LRU cache system
- [x] 64 tests, 83.8% coverage

---

## In Progress

### v0.3.0 — Images, Clipping & Compositing

**Timeline:** ~3 weeks | **Tasks:** 20

#### Foundation ✅
- [x] Image format types (Gray8, Gray16, RGB8, RGBA8, RGBAPremul, BGRA8, BGRAPremul)
- [x] ImageBuf with lazy premultiplication
- [x] SubImage zero-copy views
- [x] Image pool for memory reuse (~3x faster)
- [x] PNG/JPEG I/O with std lib interop

#### Image Core ✅
- [x] Interpolation modes (Nearest 17ns, Bilinear 67ns, Bicubic 492ns)

#### Image Drawing
- [ ] DrawImage with affine transforms
- [ ] Mipmap chain generation
- [ ] ImagePattern for fills

#### Clipping
- [ ] Edge clipper (Cohen-Sutherland + curve extrema)
- [ ] Mask clipper (alpha masks)
- [ ] Clip stack (hierarchical clipping)

#### Compositing
- [ ] Porter-Duff (12+ modes)
- [ ] Advanced blend modes (15+ modes)
- [ ] Layer system (push/pop compositing)

#### Public API
- [ ] Context.DrawImage* methods
- [ ] Context.Clip* methods
- [ ] Context.PushLayer/PopLayer

---

## Planned

### v0.4.0 — Color Pipeline

**Timeline:** +4 weeks

- [ ] sRGB ↔ Linear color space conversion
- [ ] Premultiplied alpha pipeline
- [ ] ColorF32 computation type
- [ ] ICC profile support (basic)
- [ ] Correct blending in linear space

### v0.5.0 — SIMD Optimization

**Timeline:** +3 weeks

- [ ] Go 1.25+ SIMD intrinsics
- [ ] Batch pixel operations (8-16 pixels)
- [ ] SIMD blend functions
- [ ] SIMD sRGB conversion
- [ ] Scalar fallback for compatibility
- [ ] **Target: 3-5x faster blending**

### v0.6.0 — Parallel Rendering

**Timeline:** +4 weeks

- [ ] Tile-based rendering (64x64 tiles)
- [ ] WorkerPool with work stealing
- [ ] Parallel tile rasterization
- [ ] Lock-free dirty region tracking
- [ ] Tile compositor

### v0.7.0 — Scene Graph (Retained Mode)

**Timeline:** +4 weeks

- [ ] Encoding (command buffer)
- [ ] Scene API (retained mode)
- [ ] Layer stack with blending
- [ ] Layer caching
- [ ] Dirty region optimization
- [ ] SceneBuilder fluent API

### v0.8.0 — Backend Abstraction

**Timeline:** +3 weeks

- [ ] RenderBackend interface
- [ ] SoftwareBackend implementation
- [ ] Backend auto-selection
- [ ] Fallback mechanism
- [ ] Resource sharing between backends

### v0.9.0 — GPU Backend

**Timeline:** +6 weeks

- [ ] Integration with gogpu/wgpu
- [ ] GPU memory management
- [ ] Coarse rasterization (compute shader)
- [ ] Fine rasterization (compute shader)
- [ ] Texture atlas management
- [ ] GPU/CPU synchronization

---

## Target

### v1.0.0 — Production Release

**Timeline:** +4 weeks (Total: ~7 months from v0.3.0)

- [ ] API review and cleanup
- [ ] Comprehensive documentation
- [ ] Performance benchmarks
- [ ] Cross-platform testing
- [ ] Example applications
- [ ] 90%+ test coverage
- [ ] Stable public API

---

## Architecture (v1.0.0 Target)

```
                         gg (Public API)
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
    Immediate Mode      Retained Mode         Resources
    (Context API)       (Scene Graph)      (Images, Fonts)
         │                    │                    │
         └────────────────────┼────────────────────┘
                              │
                     RenderBackend Interface
                              │
              ┌───────────────┼───────────────┐
              │               │               │
         Software          SIMD           GPU
         (v0.1.0+)       (v0.5.0)     (gogpu/wgpu)
```

---

## Reference Documents

| Document | Purpose |
|----------|---------|
| `docs/dev/research/DESIGN-001-images-clipping-v3.md` | v0.3.0 design |
| `docs/dev/research/ARCHITECTURE-001-enterprise-grade.md` | v1.0.0 blueprint |

---

## Non-Goals (for now)

- 3D graphics (see gogpu/gogpu)
- Animation system
- GUI widgets (see gogpu/ui)
- Platform-specific rendering

---

## Contributing

Help wanted on all phases! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Priority areas:
- Image loading/drawing implementation
- Clipping algorithms
- SIMD optimization
- Test cases and benchmarks
- Documentation and examples
