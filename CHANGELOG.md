# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned for v0.5.0
- SIMD optimization for blend functions
- Parallel tile-based rendering

## [0.4.0] - 2025-12-17

### Added

#### Color Pipeline (internal/color)
- **ColorSpace** — sRGB and Linear color space enum
- **ColorF32** — Float32 color type for precise computation
- **ColorU8** — Uint8 color type for storage
- **SRGBToLinear/LinearToSRGB** — Accurate color space conversions
- **Round-trip accuracy** — Max error < 1/255
- 100% test coverage

#### HSL Blend Modes (internal/blend/hsl)
- **Lum, Sat** — Luminance and saturation helpers (BT.601 coefficients)
- **SetLum, SetSat, ClipColor** — W3C spec helper functions
- **BlendHue** — Hue of source, saturation/luminosity of backdrop
- **BlendSaturation** — Saturation of source, hue/luminosity of backdrop
- **BlendColor** — Hue+saturation of source, luminosity of backdrop
- **BlendLuminosity** — Luminosity of source, hue+saturation of backdrop

#### Linear Space Blending (internal/blend/linear)
- **GetBlendFuncLinear** — Blend function with linear color space option
- **BlendLinear** — Convenience function for linear blending
- **Correct pipeline** — sRGB → Linear → Blend → sRGB
- **Alpha preservation** — Alpha channel never gamma-encoded
- Fixes dark halos and desaturated gradients

#### Layer API (context_layer.go)
- **PushLayer(blendMode, opacity)** — Create isolated drawing layer
- **PopLayer()** — Composite layer onto parent with blend mode
- **SetBlendMode(mode)** — Set blend mode for subsequent operations
- **Nested layers** — Arbitrary nesting depth support
- **Opacity control** — Per-layer opacity with automatic clamping

### Testing
- 83.8% overall coverage
- internal/color: 100% coverage
- internal/blend: 92.1% coverage

## [0.3.0] - 2025-12-16

### Added

#### Image Foundation
- **Format** — 7 pixel formats (Gray8, Gray16, RGB8, RGBA8, RGBAPremul, BGRA8, BGRAPremul)
- **FormatInfo** — Bytes-per-pixel, channel count, alpha detection
- **ImageBuf** — Core image buffer with lazy premultiplication
- **SubImage** — Zero-copy views into parent images
- **Thread-safe caching** — Premultiplied data computed once, cached with sync.RWMutex
- **PNG/JPEG I/O** — Load, save, encode, decode
- **FromStdImage/ToStdImage** — Full interoperability with standard library

#### Image Processing
- **Pool** — Memory-efficient image reuse (~3x faster allocation)
- **Interpolation** — Nearest (17ns), Bilinear (67ns), Bicubic (492ns)
- **Mipmap** — Automatic mipmap chain generation
- **Pattern** — Image patterns for fills with repeat modes
- **Affine transforms** — DrawImage with rotation, scale, translation

#### Clipping System (internal/clip)
- **EdgeClipper** — Cohen-Sutherland for lines, de Casteljau for curves
- **MaskClipper** — Alpha mask clipping with Gray8 buffers
- **ClipStack** — Hierarchical push/pop clipping with mask combination

#### Compositing System (internal/blend)
- **Porter-Duff** — 14 blend modes (Clear, Src, Dst, SrcOver, DstOver, SrcIn, DstIn, SrcOut, DstOut, SrcAtop, DstAtop, Xor, Plus, Modulate)
- **Advanced Blend** — 11 separable modes (Screen, Overlay, Darken, Lighten, ColorDodge, ColorBurn, HardLight, SoftLight, Difference, Exclusion, Multiply)
- **Layer System** — Isolated drawing surfaces with compositing on pop

#### Public API
- **DrawImage(img, x, y)** — Draw image at position
- **DrawImageEx(img, opts)** — Draw with transform, opacity, blend mode
- **CreateImagePattern** — Create pattern for fills
- **Clip()** — Clip to current path
- **ClipPreserve()** — Clip keeping path
- **ClipRect(x, y, w, h)** — Fast rectangular clipping
- **ResetClip()** — Clear clipping region

#### Examples
- `examples/images/` — Image loading and drawing demo
- `examples/clipping/` — Clipping API demonstration

### Testing
- 83.8% overall coverage
- internal/blend: 90.2% coverage
- internal/clip: 81.7% coverage
- internal/image: 87.0% coverage

## [0.2.0] - 2025-12-16

### Added

#### Text Rendering System
- **FontSource** — Heavyweight font resource with pluggable parser
- **Face interface** — Lightweight per-size font configuration
- **DrawString/DrawStringAnchored** — Text drawing at any position
- **MeasureString** — Accurate text measurement
- **LoadFontFace** — Convenience method for simple cases

#### Font Composition
- **MultiFace** — Font fallback chain for emoji/multi-language
- **FilteredFace** — Unicode range restriction (16 predefined ranges)
- Common ranges: BasicLatin, Cyrillic, CJK, Emoji, and more

#### Performance
- **LRU Cache** — Generic cache with soft limit eviction
- **RuneToBoolMap** — Bit-packed glyph presence cache (375x memory savings)
- **iter.Seq[Glyph]** — Go 1.25+ zero-allocation iterators

#### Architecture
- **FontParser interface** — Pluggable font parsing backends
- **golang.org/x/image** — Default parser implementation
- Copy protection using Ebitengine pattern

### Examples
- `examples/text/` — Basic text rendering demo
- `examples/text_fallback/` — MultiFace + FilteredFace demo

### Testing
- 64 tests, 83.8% coverage
- 14 benchmarks for cache and rendering performance
- Cross-platform system font detection

## [0.1.0] - 2025-12-12

### Added
- Initial release with software renderer
- Core drawing API (Context)
- Path building (lines, curves, arcs)
- Basic shapes (rectangles, circles, ellipses, polygons)
- Transformation stack (translate, rotate, scale)
- Color utilities (RGB, RGBA, HSL, Hex)
- PNG export
- Fill and stroke operations
- Scanline rasterization engine
- fogleman/gg API compatibility layer

[Unreleased]: https://github.com/gogpu/gg/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/gogpu/gg/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/gogpu/gg/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gogpu/gg/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gogpu/gg/releases/tag/v0.1.0
