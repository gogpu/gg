# gg Roadmap

> Pure Go 2D Graphics Library — Simple API, Zero Dependencies

## Released: v0.1.0

**Focus:** Core 2D drawing API with software renderer

- [x] Canvas API (NewContext, SetSize)
- [x] Basic shapes (DrawRectangle, DrawCircle, DrawEllipse, DrawLine)
- [x] Path operations (MoveTo, LineTo, QuadraticTo, CubicTo, ClosePath)
- [x] Fill and stroke (Fill, Stroke, SetColor, SetLineWidth)
- [x] Transformations (Translate, Scale, Rotate, Push/Pop)
- [x] Color support (RGBA, Hex parsing, named colors)
- [x] Image output (SavePNG, SaveJPG)
- [x] Software rasterizer (scanline algorithm)

---

## Current: v0.2.0

**Focus:** Text rendering

### Completed
- [x] TrueType font loading (FontSource, FontParser)
- [x] Text rendering (DrawString, DrawStringAnchored)
- [x] Font metrics (MeasureString, Metrics)
- [x] Face interface with Go 1.25+ iterators
- [x] MultiFace for font fallback
- [x] FilteredFace for Unicode ranges
- [x] LRU cache system
- [x] 64 tests, 83.8% coverage

---

## Next: v0.3.0

**Focus:** Image support & clipping

### Planned
- [ ] Image loading (LoadPNG, LoadJPG)
- [ ] Image drawing (DrawImage, DrawImageAnchored)
- [ ] Clipping (Clip, ResetClip)
- [ ] Image patterns

---

## Future: v0.4.0

**Focus:** Advanced features

### Planned
- [ ] Gradients (linear, radial)
- [ ] Pattern fills
- [ ] Blend modes
- [ ] Anti-aliasing improvements

---

## Future: v0.5.0

**Focus:** GPU acceleration (optional)

### Planned
- [ ] GPU renderer using gogpu/gogpu
- [ ] Automatic fallback to software
- [ ] Shader-based path rendering
- [ ] Hardware-accelerated compositing

---

## Goal: v1.0.0

**Focus:** Production ready

### Requirements
- [ ] Full fogleman/gg API compatibility
- [ ] Comprehensive test suite (90%+ coverage)
- [ ] Stable public API
- [ ] Performance benchmarks
- [ ] Documentation with examples

---

## Non-Goals (for now)

- 3D graphics
- Animation system
- GUI widgets (see gogpu/ui)
- Platform-specific rendering

---

## Contributing

Help wanted on:
- Image loading implementation
- Additional shape primitives
- Test cases and benchmarks
- Documentation and examples

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
