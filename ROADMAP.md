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

### v0.4.0 — Color Pipeline & Layers

See section below for details.

### v0.3.0 — Images, Clipping & Compositing

See section below for details.

### v0.2.0 — Text Rendering

See section below for details.

### v0.1.0 — Core 2D Drawing

- [x] Canvas API (NewContext, SetSize)
- [x] Basic shapes (Rectangle, Circle, Ellipse, Line, Arc)
- [x] Path operations (MoveTo, LineTo, QuadraticTo, CubicTo)
- [x] Fill and stroke (Fill, Stroke, SetColor, SetLineWidth)
- [x] Transformations (Translate, Scale, Rotate, Push/Pop)
- [x] Color support (RGBA, Hex parsing, named colors)
- [x] Image output (SavePNG, SaveJPG)
- [x] Software rasterizer (scanline algorithm)

### v0.2.0 — Text Rendering

- [x] TrueType font loading (FontSource, FontParser)
- [x] Text rendering (DrawString, DrawStringAnchored)
- [x] Font metrics (MeasureString, Metrics)
- [x] Face interface with Go 1.25+ iterators
- [x] MultiFace for font fallback
- [x] FilteredFace for Unicode ranges
- [x] LRU cache system
- [x] 64 tests, 83.8% coverage

### v0.3.0 — Images, Clipping & Compositing

- [x] Image format types (Gray8, Gray16, RGB8, RGBA8, RGBAPremul, BGRA8, BGRAPremul)
- [x] ImageBuf with lazy premultiplication
- [x] SubImage zero-copy views
- [x] Image pool for memory reuse (~3x faster)
- [x] PNG/JPEG I/O with std lib interop
- [x] Interpolation modes (Nearest 17ns, Bilinear 67ns, Bicubic 492ns)
- [x] Mipmap chain generation
- [x] ImagePattern for fills
- [x] DrawImage with affine transforms
- [x] Edge clipper (Cohen-Sutherland + curve extrema)
- [x] Mask clipper (alpha masks)
- [x] Clip stack (hierarchical clipping)
- [x] Porter-Duff (14 modes)
- [x] Advanced blend modes (11 modes)
- [x] Layer system (internal)
- [x] Context.DrawImage* methods
- [x] Context.Clip* methods

### v0.4.0 — Color Pipeline & Layers

- [x] Context.PushLayer/PopLayer API
- [x] HSL blend modes (Hue, Saturation, Color, Luminosity)
- [x] sRGB ↔ Linear color space conversion
- [x] ColorF32/ColorU8 types in internal/color
- [x] Linear space blending pipeline
- [x] 83.8% test coverage

---

## Planned

### v0.5.0 — SIMD Optimization

- [ ] Go 1.25+ SIMD intrinsics
- [ ] Batch pixel operations (8-16 pixels)
- [ ] SIMD blend functions
- [ ] SIMD sRGB conversion
- [ ] Scalar fallback for compatibility
- [ ] **Target: 3-5x faster blending**

### v0.6.0 — Parallel Rendering

- [ ] Tile-based rendering (64x64 tiles)
- [ ] WorkerPool with work stealing
- [ ] Parallel tile rasterization
- [ ] Lock-free dirty region tracking
- [ ] Tile compositor

### v0.7.0 — Scene Graph (Retained Mode)

- [ ] Encoding (command buffer)
- [ ] Scene API (retained mode)
- [ ] Layer stack with blending
- [ ] Layer caching
- [ ] Dirty region optimization
- [ ] SceneBuilder fluent API

### v0.8.0 — Backend Abstraction

- [ ] RenderBackend interface
- [ ] SoftwareBackend implementation
- [ ] Backend auto-selection
- [ ] Fallback mechanism
- [ ] Resource sharing between backends

### v0.9.0 — GPU Backend

- [ ] Integration with gogpu/wgpu
- [ ] GPU memory management
- [ ] Coarse rasterization (compute shader)
- [ ] Fine rasterization (compute shader)
- [ ] Texture atlas management
- [ ] GPU/CPU synchronization

---

## Target

### v1.0.0 — Production Release

**Timeline:** ~6 months from v0.4.0

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
- Layer API implementation
- SIMD optimization
- Test cases and benchmarks
- Documentation and examples
