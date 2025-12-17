# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned for v0.9.0
- GPU backend (gogpu/wgpu integration)
- GPU memory management
- Compute shader rasterization

## [0.8.0] - 2025-12-18

### Added

#### Backend Abstraction (backend/)

**RenderBackend Interface**
- **RenderBackend** — Pluggable interface for rendering backends
  - Name() — Backend identifier
  - Init()/Close() — Lifecycle management
  - NewRenderer() — Create immediate mode renderer
  - RenderScene() — Retained mode scene rendering
- **Common errors** — ErrBackendNotAvailable, ErrNotInitialized

**Backend Registry**
- **Register/Unregister** — Backend factory registration
- **Get** — Get backend by name
- **Default** — Priority-based selection (wgpu > software)
- **MustDefault** — Panic on missing backend
- **Available** — List registered backends
- **IsRegistered** — Check backend availability

**SoftwareBackend**
- **SoftwareBackend** — CPU-based rendering implementation
- **Auto-registration** — Registered on package import
- **Lazy scene renderer** — Created on first RenderScene call
- **Resize support** — Recreates renderer on target size change

### Statistics
- **595 LOC added** across 5 files
- **89.4% test coverage** (16 tests)
- **0 linter issues**

## [0.7.0] - 2025-12-18

### Added

#### Scene Graph (Retained Mode)

**Encoding System (scene/)**
- **Tag** — 22 command types (0x01-0x51) for path, draw, layer, clip operations
- **Encoding** — Dual-stream command buffer (vello pattern)
  - Separate streams: tags, pathData, drawData, transforms, brushes
  - Hash() for cache keys (FNV-64a)
  - Append() for encoding composition
  - Clone() for independent copies
- **EncodingPool** — sync.Pool-based zero-allocation reuse

**Scene API**
- **Scene** — Retained mode drawing surface
  - Fill(style, transform, brush, shape) — Fill shape
  - Stroke(style, transform, brush, shape) — Stroke shape
  - DrawImage(img, transform) — Draw image
  - PushLayer/PopLayer — Compositing layers
  - PushClip/PopClip — Clipping regions
  - PushTransform/PopTransform — Transform stack
  - Flatten() — Composite all layers to encoding
- **13 Shape types** — Rect, Circle, Ellipse, Line, Polygon, RoundedRect, Star, Arc, Sector, Ring, Capsule, Triangle, PathShape
- **Path** — float32 points with MoveTo, LineTo, QuadTo, CubicTo, Close
- **29 BlendModes** — 14 Porter-Duff + 11 Advanced + 4 HSL

**Layer System**
- **LayerKind** — Regular, Filtered, Clip (memory-optimized)
- **LayerStack** — Nested layer management with pooling
- **LayerState** — Blend mode, alpha, clip, encoding per layer
- **ClipStack** — Hierarchical clip region management
- 100-level nesting tested

**Filter Effects (internal/filter/)**
- **BlurFilter** — Separable Gaussian blur, O(n) per radius
- **DropShadowFilter** — Offset + blur + colorize
- **ColorMatrixFilter** — 4x5 matrix with 10 presets
  - Grayscale, Sepia, Invert, Brightness, Contrast
  - Saturation, HueRotate, Opacity, Tint
- **FilterChain** — Sequential filter composition
- **GaussianKernel** — Cached kernel generation

**Layer Caching**
- **LayerCache** — LRU cache for rendered layers
  - 64MB default, configurable via NewLayerCache(mb)
  - Thread-safe with sync.RWMutex
  - Atomic statistics (hits, misses, evictions)
  - Performance: Get 90ns, Put 393ns, Stats 26ns

**SceneBuilder (Fluent API)**
- **NewSceneBuilder()** — Create builder
- **Fill/Stroke** — Drawing operations
- **FillRect/StrokeRect/FillCircle/StrokeCircle** — Convenience methods
- **Layer/Clip/Group** — Nested operations with callbacks
- **Transform/Translate/Scale/Rotate** — Transform operations
- **Build()** — Return scene and reset builder

**Renderer & Integration**
- **Renderer** — Parallel tile-based scene renderer
  - Render(target, scene) — Full scene rendering
  - RenderDirty(target, scene, dirty) — Incremental rendering
  - Stats() — Render statistics
  - CacheStats() — Cache statistics
- **Decoder** — Sequential encoding command reader
  - Next(), Tag(), MoveTo(), LineTo(), etc.
  - CollectPath() — Read complete path
- Integration with TileGrid, WorkerPool, DirtyRegion

**Examples**
- **examples/scene/** — Scene API demonstration

### Performance

| Operation | Time | Notes |
|-----------|------|-------|
| LayerCache.Get | 90ns | 4x faster than target |
| LayerCache.Put | 393ns | 25x faster than target |
| LayerCache.Stats | 26ns | Atomic reads |
| Blur (r=5, 1080p) | ~5ms | Separable algorithm |
| ColorMatrix (1080p) | ~2ms | Per-pixel |

### Statistics
- **15,376 LOC added** across 37 files
- **scene package**: 89% coverage
- **internal/filter**: 93% coverage
- **25 benchmarks** for performance validation
- **0 linter issues**

## [0.6.0] - 2025-12-17

### Added

#### Tile-Based Infrastructure (internal/parallel)
- **Tile** — 64x64 pixel tile with local data buffer (16KB per tile)
- **TileGrid** — 2D grid manager with dynamic resizing
  - TileAt, TileAtPixel — O(1) tile access
  - TilesInRect — Tiles intersecting a rectangle
  - MarkDirty, MarkRectDirty — Dirty region tracking
  - ForEach, ForEachDirty — Tile iteration
- **TilePool** — sync.Pool-based memory reuse (0 allocs/op in hot path)
  - Get/Put with automatic data clearing
  - Edge tile support for non-64-aligned canvases

#### WorkerPool with Work Stealing
- **WorkerPool** — Goroutine pool for parallel execution
  - Per-worker buffered channels (256 items)
  - Work stealing from other workers when idle
  - ExecuteAll — Distribute work and wait for completion
  - ExecuteAsync — Fire-and-forget execution
  - Submit — Single work item submission
  - Graceful shutdown with Close()
- No goroutine leaks (verified by tests)

#### ParallelRasterizer
- **ParallelRasterizer** — High-level parallel rendering coordinator
  - Clear — Parallel tile clearing with solid color
  - FillRect — Parallel rectangle filling across tiles
  - FillTiles — Custom tile processing with callback
  - Composite — Merge all tiles to output buffer
  - CompositeDirty — Merge only dirty tiles
- Automatic tile grid and worker pool management
- Integration with DirtyRegion for efficient updates

#### Lock-Free DirtyRegion
- **DirtyRegion** — Atomic bitmap for dirty tile tracking
  - Mark — O(1) lock-free marking using atomic.Uint64.Or()
  - MarkRect — Mark all tiles in rectangle
  - IsDirty — Check single tile status
  - GetDirtyTiles — Return list of dirty tiles
  - GetAndClear — Atomic get and reset
  - Count — Number of dirty tiles
- Performance: 10.9 ns/mark, 0 allocations
- Uses bits.TrailingZeros64 for efficient iteration

#### Benchmarks & Visual Tests
- **Component benchmarks** — TileGrid, WorkerPool, TilePool, DirtyRegion, ParallelRasterizer
- **Scaling benchmarks** — 1, 2, 4, 8, Max cores with GOMAXPROCS control
- **Visual regression tests** — 7 test suites comparing parallel vs serial output
  - ParallelClear, ParallelFillRect, ParallelComposite
  - TileBoundaries, EdgeTiles, MultipleOperations
  - Pixel-perfect comparison (tolerance 0)

### Performance

| Operation | Time | Allocations |
|-----------|------|-------------|
| DirtyRegion.Mark | 10.9 ns | 0 |
| TilePool.GetPut | ~50 ns | 0 |
| WorkerPool.ExecuteAll/100 | ~15 µs | 0 (hot path) |
| Clear 1920x1080 | ~1.4 ms (1 core) → ~0.7 ms (2 cores) | — |

### Testing
- 120+ tests in internal/parallel/
- All tests pass with race detector (-race)
- 83.8% overall coverage

## [0.5.0] - 2025-12-17

### Added

#### Fast Math (internal/blend)
- **div255** — Shift approximation `(x + 255) >> 8` (2.4x faster than division)
- **mulDiv255** — Multiply and divide by 255 in one operation
- **inv255** — Fast complement calculation (255 - x)
- **clamp255** — Branchless clamping to [0, 255]

#### sRGB Lookup Tables (internal/color)
- **sRGBToLinearLUT** — 256-entry lookup table for sRGB to linear conversion
- **linearToSRGBLUT** — 4096-entry lookup table for linear to sRGB
- **SRGBToLinearFast** — 260x faster than math.Pow (0.16ns vs 40.93ns)
- **LinearToSRGBFast** — 23x faster than math.Pow (1.81ns vs 41.92ns)
- Total memory: ~5KB for both tables

#### Wide Types (internal/wide)
- **U16x16** — 16-element uint16 vector for lowp batch operations
  - Add, Sub, Mul, MulDiv255, Inv, And, Or, Min, Max
  - Zero allocations, 3.8ns per 16-element Add
- **F32x8** — 8-element float32 vector for highp operations
  - Add, Sub, Mul, Div, Sqrt, Min, Max, Clamp
  - Zero allocations, 1.9ns per 8-element Add
- **BatchState** — Structure for 16-pixel batch processing
  - LoadSrc/LoadDst from []byte buffers
  - StoreDst back to []byte buffers
  - AoS (Array of Structures) storage, SoA processing

#### Batch Blending (internal/blend)
- **14 Porter-Duff batch modes** — Clear, Source, Destination, SourceOver, DestinationOver, SourceIn, DestinationIn, SourceOut, DestinationOut, SourceAtop, DestinationAtop, Xor, Plus, Modulate
- **7 Advanced batch modes** — Multiply, Screen, Darken, Lighten, Overlay, HardLight, SoftLight
- **BlendBatch** — Generic batch blending function
- **SourceOverBatch** — Optimized source-over (11.9ns per pixel)
- All modes operate on premultiplied alpha, ±2 tolerance for div255 approximation

#### Rasterizer Integration
- **SpanFiller interface** — Optional interface for optimized span filling
- **FillSpan** — Fill horizontal span with solid color (no blending)
  - Pattern-based optimization for spans ≥16 pixels
  - Uses copy() for efficient memory filling
- **FillSpanBlend** — Fill horizontal span with source-over blending
  - Falls back to scalar for spans <16 pixels
  - Optimized for common opaque case (alpha ≥ 0.9999)

#### Benchmarks & Tests
- **Visual regression tests** — All 14 Porter-Duff modes tested at boundary sizes
- **Batch boundary tests** — Edge cases around n % 16
- **SIMD benchmarks** — div255, sRGB LUTs, wide types
- **Pixmap benchmarks** — FillSpan vs SetPixel comparison
- **BENCHMARK_RESULTS_v0.5.0.md** — Comprehensive benchmark documentation

### Performance
| Operation | Before | After | Improvement |
|-----------|--------|-------|-------------|
| div255 | ~0.4ns | ~0.17ns | 2.4x |
| sRGB→Linear | 40.93ns | 0.16ns | 260x |
| Linear→sRGB | 41.92ns | 1.81ns | 23x |
| SourceOver/16px | ~300ns | 190ns | 1.6x |
| U16x16.Add | — | 3.8ns | new |
| F32x8.Add | — | 1.9ns | new |

### Testing
- 83.8% overall coverage
- All batch modes: 0 allocations per operation
- Visual regression tests pass with ±2 tolerance

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

[Unreleased]: https://github.com/gogpu/gg/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/gogpu/gg/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/gogpu/gg/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/gogpu/gg/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/gogpu/gg/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/gogpu/gg/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gogpu/gg/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gogpu/gg/releases/tag/v0.1.0
