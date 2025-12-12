# gg Roadmap

> Pure Go 2D Graphics Library — Simple API, Zero Dependencies

## Current: v0.1.0

**Focus:** Core 2D drawing API with software renderer

### In Progress
- [x] Canvas API (NewContext, SetSize)
- [x] Basic shapes (DrawRectangle, DrawCircle, DrawEllipse, DrawLine)
- [x] Path operations (MoveTo, LineTo, QuadraticTo, CubicTo, ClosePath)
- [x] Fill and stroke (Fill, Stroke, SetColor, SetLineWidth)
- [x] Transformations (Translate, Scale, Rotate, Push/Pop)
- [x] Color support (RGBA, Hex parsing, named colors)
- [x] Image output (SavePNG, SaveJPG)
- [x] Software rasterizer (scanline algorithm)
- [ ] Text rendering (placeholder API)
- [ ] Pattern fills
- [ ] Complete test coverage

---

## Next: v0.2.0

**Focus:** Text rendering & image support

### Planned
- [ ] TrueType font loading (sfnt parsing)
- [ ] Text rendering (DrawString, DrawStringAnchored)
- [ ] Font metrics (MeasureString, WordWrap)
- [ ] Image loading (LoadPNG, LoadJPG)
- [ ] Image drawing (DrawImage, DrawImageAnchored)
- [ ] Clipping (Clip, ResetClip)

---

## Future: v0.3.0

**Focus:** Advanced features

### Planned
- [ ] Gradients (linear, radial)
- [ ] Pattern fills (image patterns)
- [ ] Blend modes
- [ ] Anti-aliasing improvements
- [ ] Path operations (Union, Intersect, Difference)

---

## Future: v0.4.0

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
- Text rendering implementation
- Additional shape primitives
- Test cases and benchmarks
- Documentation and examples

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
