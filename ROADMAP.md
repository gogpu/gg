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

### v0.8.0 — Backend Abstraction

See section below for details.

### v0.7.0 — Scene Graph (Retained Mode)

See section below for details.

### v0.6.0 — Parallel Rendering

See section below for details.

### v0.5.0 — SIMD Optimization

See section below for details.

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

### v0.5.0 — SIMD Optimization

- [x] Fast div255 (shift approximation, 2.4x faster)
- [x] sRGB LUTs (260x faster than math.Pow)
- [x] Wide types: U16x16 (16 pixels), F32x8 (8 pixels)
- [x] Batch blending (14 Porter-Duff + 7 advanced modes)
- [x] Auto-vectorization via fixed-size arrays
- [x] Rasterizer integration (SpanFiller, FillSpan, FillSpanBlend)
- [x] Visual regression tests
- [x] Comprehensive benchmarks
- [x] **Achieved: 2-260x faster operations**

### v0.6.0 — Parallel Rendering

- [x] Tile-based rendering (64x64 tiles)
- [x] TileGrid with dynamic resizing
- [x] TilePool (sync.Pool, 0 allocs)
- [x] WorkerPool with work stealing
- [x] ParallelRasterizer (Clear, FillRect, Composite)
- [x] Lock-free DirtyRegion (atomic bitmap, 10.9ns/mark)
- [x] Scaling benchmarks (1, 2, 4, 8+ cores)
- [x] Visual regression tests (pixel-perfect)
- [x] **6,372 LOC, 120+ tests, race-free**

---

### v0.7.0 — Scene Graph (Retained Mode)

- [x] Dual-stream Encoding (command buffer, vello pattern)
- [x] Scene API (Fill, Stroke, DrawImage, PushLayer/PopLayer)
- [x] 13 Shape types (Rect, Circle, Ellipse, Polygon, Star, etc.)
- [x] Layer stack with blending (29 blend modes)
- [x] Filter effects (Blur, DropShadow, ColorMatrix)
- [x] Layer caching (64MB LRU, 90ns Get)
- [x] SceneBuilder fluent API
- [x] Parallel Renderer (TileGrid + WorkerPool integration)
- [x] **15,376 LOC, 89% coverage, 25 benchmarks**

### v0.8.0 — Backend Abstraction

- [x] RenderBackend interface
- [x] SoftwareBackend implementation (wraps existing renderer)
- [x] Backend registry with auto-selection
- [x] Priority-based fallback (wgpu > software)
- [x] **595 LOC, 89.4% coverage, 16 tests**

---

## Planned

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
| `docs/dev/research/RESEARCH-006-scene-graph.md` | v0.7.0 scene graph research |
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
- Scene graph implementation
- GPU backend integration
- Test cases and benchmarks
- Documentation and examples
