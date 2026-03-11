# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.35.3] - 2026-03-11

### Fixed

- **MSDF atlas FontID collision when mixing fonts from same family** —
  `computeFontID()` hashed `source.Name()` (family name, e.g., "Go") instead of
  `parsed.FullName()` (e.g., "Go Regular" / "Go Bold"). Fonts within the same family
  that share the same glyph count produced identical FontIDs, causing atlas cache
  collisions: Bold glyphs silently overwrote Regular glyphs (or vice versa), resulting
  in per-glyph weight inconsistency when rendering mixed-font text.

### Changed

- Update gogpu v0.23.1 → v0.23.2 in examples (Retina contentsScale fix)

## [0.35.2] - 2026-03-11

### Fixed

- **GPU surface not cleared between frames (progressive drift on Retina)** —
  `GPURenderSession.BeginFrame()` was never called, so `frameRendered` stayed `true`
  after the first frame, causing all subsequent frames to use `LoadOpLoad` instead of
  `LoadOpClear`. Previous frame content persisted and new shapes accumulated on top,
  producing progressive stretching and drift on macOS Retina displays. Fix: add
  `FrameAware` interface and `BeginAcceleratorFrame()`, called from
  `ggcanvas.RenderDirect()`. Also auto-detect new frame via swapchain TextureView
  pointer change in `SetSurfaceTarget`. Mid-frame flushes correctly use `LoadOpLoad`
  to preserve previously drawn content.
  ([#171](https://github.com/gogpu/gg/issues/171))

- **TextModeVector text invisible with GPU SurfaceTarget** —
  `drawStringAsOutlines()` rendered glyph outlines directly to CPU pixmap via
  `renderer.Fill()`, bypassing the GPU pipeline. In zero-copy surface mode
  (`ggcanvas.RenderDirect`), the pixmap was never composited onto the GPU surface.
  Fix: route device-space glyph path through `doFill()` — the same multi-tier pipeline
  used by all shapes (GPU stencil+cover → surface, or CPU fallback → pixmap). Also
  removed unnecessary `flushGPUAccelerator()` call that created a mid-frame render pass
  with `LoadOpClear`, wiping previously drawn content.
  ([#184](https://github.com/gogpu/gg/issues/184))

### Dependencies

- Update wgpu v0.20.0 → v0.20.1 (Metal stencil attachment fix for Retina)

## [0.35.1] - 2026-03-11

### Changed

- **scene.TextRenderer uses GlyphCache** — `RenderGlyph`, `RenderGlyphs`, and
  `RenderTextToScene` now use the global `GlyphCache` for outline reuse across frames,
  matching the pattern established in `Context.drawStringAsOutlines()`. Eliminates
  redundant outline extraction when rendering text through the scene pipeline.

## [0.35.0] - 2026-03-11

### Added

- **TextMode API** — per-Context text rendering strategy selection with four modes:
  `TextModeAuto` (default), `TextModeMSDF` (GPU atlas), `TextModeVector` (glyph outlines),
  `TextModeBitmap` (CPU bitmap). Set via `SetTextMode()` / query via `TextMode()`.
- **DPI-aware MSDF text pipeline** — `deviceScale` propagated through the GPU MSDF
  pipeline. On HiDPI displays (2× Retina), MSDF `screenPxRange` scales proportionally
  with physical font size, producing crisper anti-aliased text without atlas changes.
- **MSDF stem darkening** — shader-level stem darkening (FreeType/macOS/Pathfinder
  pattern) counteracts gamma-induced thinning at small text sizes. Applied to all three
  MSDF entry points (fill, outline, shadow). Starts at `screenPxRange=2`, fades to zero
  at `screenPxRange≥8` (large text unaffected).
- **GlyphCache integration for vector text** — `drawStringAsOutlines()` now caches
  glyph outlines via `text.GlyphCache.GetOrCreate()`, avoiding repeated `ExtractOutline()`
  calls on every frame. Uses the global shared cache for cross-Context reuse.
- **Text-aware rasterizer routing** — area-based tile rasterizer selection replaces
  per-dimension check. Wide-but-short text paths (400+ elements at 16px height) now
  route to SparseStrips tile rasterizer instead of always using AnalyticFiller.
- **Visual regression tests** — 6 test functions covering text quality across strategies
  (Bitmap/Vector), sizes (12-48px), thin strokes, and GlyphCache integration.

### Changed

- **MSDF `pxRange` tuned from 8.0 to 4.0** — doubles effective `screenPxRange` at
  all font sizes, improving anti-aliasing quality especially at 12-16px body text.
- **MSDF error correction threshold raised from 0.25 to 0.40** — more aggressive
  artifact correction for cleaner glyph edges.
- **MSDF `screenPxRange` minimum clamp raised from 1.0 to 1.5** — prevents AA
  failure on very small characters where the range would collapse below usable threshold.

## [0.34.2] - 2026-03-11

### Fixed

- **`DrawRoundedRectangle` HiDPI/Retina rendering** — fix coordinate space mismatch
  where rounded rectangles appeared at half size in the wrong position on HiDPI displays.
  The method now uses Context drawing methods (with matrix transform) instead of direct
  Path methods, matching the pattern used by `DrawCircle` and `DrawEllipse`.
  ([#171](https://github.com/gogpu/gg/issues/171))

## [0.34.1] - 2026-03-11

### Added

- **GPU pipeline diagnostic logging** — comprehensive structured `slog` logging
  across the entire GPU rendering dimensional handoff chain. All logs are
  zero-cost when disabled (default `nopHandler`). Enable via `gg.SetLogger()`.
  ([#171](https://github.com/gogpu/gg/issues/171))
  - `NewContext` / `SetDeviceScale` — log logical/physical dimensions and scale
  - `ggcanvas.NewWithScale` — log canvas creation with logical, scale, physical dims
  - `ggcanvas.RenderDirect` — log surface dimensions per frame
  - `SetDeviceProvider` — log shared GPU device type on success
  - `SetSurfaceTarget` — log surface dimensions and mode/size changes
  - `RenderFrame` — log effective viewport dimensions (target vs surface override)
  - `EnsureTextures` — log MSAA/stencil texture creation dimensions
  - `FlushGPU` — log target dimensions on entry
  - `makeSDFRenderUniform` — log viewport uniform dimensions passed to shader
  - `Flush` — log pending shape counts per tier and pipeline mode

### Fixed

- **`ggcanvas.NewWithScale` no longer silently discards `SetAcceleratorDeviceProvider`
  errors** — now logs `Warn` on failure instead of `_ =` discard.

## [0.34.0] - 2026-03-11

### Added

- **HiDPI/Retina device scale** — Cairo-pattern `SetDeviceScale()` for
  DPI-transparent drawing. User code draws in logical coordinates (points/DIP),
  the Context automatically scales to physical pixel resolution internally.
  ([#171](https://github.com/gogpu/gg/issues/171),
  [#175](https://github.com/gogpu/gg/issues/175))
  - `NewContextWithScale(w, h, scale)` — create HiDPI-aware context
  - `WithDeviceScale(scale)` — functional option for `NewContext`
  - `SetDeviceScale(scale)` — set device scale on existing context
  - `DeviceScale()` — query current device scale
  - `PixelWidth()/PixelHeight()` — physical pixel dimensions
  - `Width()/Height()` — logical dimensions (unchanged)
- **DPI-aware rasterization tolerances** — curve flattening tolerance and stroke
  expansion tolerance now scale with device DPI (femtovg pattern:
  `tolerance = baseTolerance / deviceScale`). Produces sharper curves on
  Retina/HiDPI displays.
- **ggcanvas HiDPI auto-detection** — `ggcanvas.New()` auto-detects HiDPI scale
  via `gpucontext.WindowProvider` interface (no manual scale parameter needed).
  `ggcanvas.NewWithScale()` and `MustNewWithScale()` for explicit control.
  `DeviceScale()` and `SetDeviceScale()` methods on Canvas.

## [0.33.6] - 2026-03-10

### Changed

- **Update wgpu v0.19.7 → v0.20.0** — enterprise-grade validation layer:
  core validation (30+ WebGPU spec rules), 7 typed error types with `errors.As()`,
  WebGPU deferred error pattern, HAL defense-in-depth.
- **Update gputypes v0.2.0 → v0.3.0** — `TextureUsage.ContainsUnknownBits()`.

## [0.33.5] - 2026-03-08

### Fixed

- **Fix stroke join artifacts at acute/near-reversal angles** — implement
  Skia/tiny-skia inner join handling: at acute angles, the outer (convex) side
  receives join decoration (miter/bevel/round) while the inner (concave) side
  routes through the pivot point to prevent self-intersection. Previously both
  sides were treated identically (inherited from kurbo), causing visible
  artifacts. Verified against Skia, tiny-skia, and Vello reference
  implementations.
  ([#168](https://github.com/gogpu/gg/issues/168),
  reported in [#159](https://github.com/gogpu/gg/issues/159) by
  [@rcarlier](https://github.com/rcarlier))

### Changed

- **Per-batch uniform buffers for MSDF text pipeline** — replace single
  uniform buffer/bind group with pooled slices that grow per batch, fixing
  resource lifecycle for multi-batch text rendering.

## [0.33.4] - 2026-03-07

### Fixed

- **Fix `DrawStringAnchored` vertical anchor (`ay`) formula** — the formula
  `y += h * ay` (inherited from fogleman/gg) did not match the documented
  semantics `(0,0)=top-left, (0.5,0.5)=center, (1,1)=bottom-right`. Replaced
  with the correct bounding-box anchor formula `y = y + ascent - ay * h` where
  `h = ascent + descent` (visual bounding box, no lineGap). Research verified
  against Cairo, Skia, and HTML Canvas baseline models.
  ([#166](https://github.com/gogpu/gg/issues/166),
  reported in [#159](https://github.com/gogpu/gg/issues/159) by
  [@rcarlier](https://github.com/rcarlier))

- **Fix `DrawStringWrapped` vertical anchor and height calculation** — same
  formula fix applied. Block height now uses
  `(n-1)*fh*lineSpacing + ascent + descent` (visual bounding box model).

- **Fix `MeasureMultilineString` height calculation** — now returns visual
  bounding box height consistent with `DrawStringWrapped`.

## [0.33.3] - 2026-03-07

### Changed

- **Update wgpu v0.19.6 → v0.19.7** — Queue.WriteTexture public API
  ([wgpu#95](https://github.com/gogpu/wgpu/pull/95) by [@Carmen-Shannon](https://github.com/Carmen-Shannon))
- **Update naga v0.14.5 → v0.14.6** — MSL pass-through globals fix
  ([naga#40](https://github.com/gogpu/naga/pull/40))

## [0.33.2] - 2026-03-05

### Fixed

- **Logger propagation to wgpu HAL** — `gg.SetLogger()` now propagates to
  `hal.SetLogger()`, enabling Metal/Vulkan backend logging with a single call.
  Previously, HAL-level logs (surface configuration, pipeline creation, command
  submission) were silently discarded even when gg logging was enabled.

### Added

- **RenderFrame debug log** — render session logs shape/text counts and surface
  mode at DEBUG level, making it visible when GPU rendering actually executes.

### Changed

- **Update wgpu v0.19.5 → v0.19.6** — Metal MSAA resolve store action fix
  ([wgpu#94](https://github.com/gogpu/wgpu/pull/94))

## [0.33.1] - 2026-03-05

### Fixed

- **Fix FDot6→FDot16 integer overflow causing black lines/artifacts** — three-layer fix:
  (1) reduce aaShift from 4 to 2 (Skia default), expanding max coordinate from 2048px to
  8191px; (2) path clipping to canvas bounds in EdgeBuilder with Skia-style sentinel
  vertical lines preserving winding; (3) saturating FDot6ToFDot16 conversion clamping to
  int32 range instead of wrapping. aaShift=4 (16x AA) was unnecessarily aggressive —
  Skia ships aaShift=2 (4x AA) on billions of devices with excellent quality.
  ([#148](https://github.com/gogpu/gg/issues/148))

### Changed

- **Update wgpu v0.19.4 → v0.19.5** — Metal vertex descriptor fix
  ([wgpu#93](https://github.com/gogpu/wgpu/pull/93))
- **Update naga v0.14.4 → v0.14.5**
- **Update goffi v0.4.1 → v0.4.2**

## [0.33.0] - 2026-03-03

### Added

- **DrawImage respects clip stack** — `DrawImageEx` refactored to route through the
  `Fill()` pipeline (image-as-shader pattern). Images now correctly clip to any path
  set via `Clip()`, `ClipRect()`, or nested `Push`/`Pop` clips. This follows the
  enterprise pattern used by Skia, Cairo, tiny-skia, and Vello.
  ([#155](https://github.com/gogpu/gg/issues/155))
- **`DrawImageRounded(img, x, y, radius)`** — convenience method for drawing images
  with rounded corners
- **`DrawImageCircular(img, cx, cy, radius)`** — convenience method for drawing
  circular avatar-style images
- **`ImagePattern.SetAnchor(x, y)`** — position image patterns at arbitrary canvas
  coordinates instead of tiling from origin (0,0)
- **`ImagePattern.SetScale(sx, sy)`** — scale image patterns
- **`ImagePattern.SetOpacity(opacity)`** — opacity multiplier for image patterns
- **`ImagePattern.SetClamp(bool)`** — clamp mode: out-of-bounds returns transparent
  instead of tiling
- **Fill() and Stroke() respect clip stack** — all software rendering paths (analytic
  filler + coverage filler) now apply clip masks via `Paint.ClipCoverage`
- **Anti-aliased clip masks** — path-based clips now use 4x Y-supersampling with
  fractional X-edge coverage for smooth clip edges (previously binary 0/255 only)

## [0.32.5] - 2026-03-02

### Changed

- **Update wgpu v0.19.3 → v0.19.4** — fix SIGSEGV on Linux/macOS for Vulkan
  functions with >6 arguments ([goffi#19](https://github.com/go-webgpu/goffi/issues/19),
  [gogpu#119](https://github.com/gogpu/gogpu/issues/119))

## [0.32.4] - 2026-03-01

### Changed

- **Update wgpu v0.19.0 → v0.19.3** — includes MSL backend fixes for Apple Silicon:
  vertex `[[stage_in]]` for struct-typed arguments, `metal::discard_fragment()` namespace
  ([naga#38](https://github.com/gogpu/naga/pull/38),
  [ui#23](https://github.com/gogpu/ui/issues/23))

## [0.32.3] - 2026-03-01

### Fixed

- **Horizontal line artifacts in rotated text (#148)** — forward differencing in
  `QuadraticEdge`/`CubicEdge` produced zero-height segments after FDot6 rounding,
  silently losing winding contribution. The residual propagated via tail accumulator
  to all pixels rightward, creating horizontal gray lines from curved glyphs (e, o,
  b, p) at small rotation angles. Fix: flatten curves to line segments (adaptive
  subdivision, 0.1px tolerance) before AnalyticFiller scanline processing —
  industry-standard approach (tiny-skia, Skia AAA).
- **Tab character rendering as tofu boxes (TEXT-008)** — tab (`\t`) rendered as
  `.notdef` rectangle across all text paths: bitmap (`font.Drawer`), outline
  (`drawStringAsOutlines`), and HarfBuzz (`GoTextShaper`). Fix: unified tab handling
  at each rendering layer — `expandTabs()` for bitmap path, space GID + tab-stop
  advance for shaper/outline paths. Configurable via `text.SetTabWidth()` (default: 8,
  matching CSS `tab-size`, Pango, and POSIX terminal conventions).
- **Text rasterizer mode propagation** — `drawStringAsOutlines()` bypassed `doFill()`,
  so `SetRasterizerMode()` had no effect on outline-rendered text.

### Added

- **Tab character API** — `text.SetTabWidth(n)` / `text.TabWidth()` for configurable
  tab stops (default: 8, matching CSS `tab-size`, Pango, POSIX).
- **Text regression test suite (TEXT-011)** — programmatic artifact detection for
  rotated text (9 angles, curved glyphs), tab rendering verification (bitmap + outline),
  and unit tests for tab configuration (`expandTabs`, `SetTabWidth`, `tabAdvance`,
  `fixTabGlyphs`). Cross-platform, no golden images.

## [0.32.2] - 2026-03-01

### Fixed

- **GPU error propagation for `WriteBuffer`** — 15+ call sites across `render_session.go`,
  `sdf_render.go`, `stencil_renderer.go`, `vello_accelerator.go`, `vello_compute.go` now
  check and propagate errors instead of silently swallowing them. Buffer upload failures
  trigger proper cleanup (destroy buffer) before returning errors.
- **GPU error propagation for `WriteTexture`** — `text_pipeline.go` and `sdf_gpu.go` now
  propagate texture upload errors with cleanup on failure.
- **`uploadPathAuxData` returns error** — `VelloAccelerator.uploadPathAuxData` now returns
  `error` instead of silently ignoring buffer upload failures.

### Changed

- Update wgpu v0.18.1 → v0.19.0 — `WriteBuffer` and `WriteTexture` breaking interface changes

## [0.32.1] - 2026-02-28

### Added

- **CPU text transform support (TEXT-002)** — `DrawString` now respects the full CTM
  (Current Transform Matrix) for CPU text rendering, not just position. Three-tier
  hybrid decision tree modeled after Skia/Cairo/Vello:
  - **Tier 0**: Translation-only → bitmap fast path (zero quality loss)
  - **Tier 1**: Uniform positive scale ≤256px → bitmap at device pixel size (Skia pattern)
  - **Tier 2**: Rotation, shear, non-uniform scale, mirror, extreme scale → glyph vector
    outlines converted to `Path`, transformed by CTM, filled via `SoftwareRenderer`
  - `DrawStringAnchored` and `DrawStringWrapped` inherit transform support automatically
  - MultiFace graceful degradation (falls back to position-only bitmap)
  - Lazy `OutlineExtractor` initialization on Context (GC-managed lifecycle)
  ([#145](https://github.com/gogpu/gg/issues/145))
- **GPU MSDF text transform support (TEXT-001)** — CTM passed to GPU MSDF
  vertex shader for correct scale, rotation, and skew of GPU-rendered text.
  ([#146](https://github.com/gogpu/gg/issues/146))
- **Text transform golden tests (TEXT-003)** — 9-scenario golden test suite
  (identity, translate, scale, rotate, shear) with cross-comparison validation.
- **`examples/text_transform`** — Visual 3×3 grid example demonstrating all
  CPU text rendering tiers with per-cell clipping.

### Fixed

- **Outline text Y-coordinate inversion** — `drawStringAsOutlines` used Y-up
  formula but `sfnt.LoadGlyph` returns Y-down (screen convention). Text rendered
  via Tier 2 (rotation, shear, non-uniform scale) was upside-down.
  ([#145](https://github.com/gogpu/gg/issues/145))
- **`scene/text.go` FlipY default** — Changed `TextRendererConfig.FlipY` default
  from `true` to `false`. Since `OutlineExtractor` preserves sfnt's Y-down
  convention, no flip is needed. Fixes inverted text in scene text rendering.

## [0.32.0] - 2026-02-28

### Added

- **Smart rasterizer selection** — Multi-factor auto-selection of rasterization
  algorithm per-path. Adaptive threshold formula `max(32, 2048/sqrt(bboxArea))`
  considers path complexity and bounding box area. BBox precheck: paths < 32px
  always use scanline. Five algorithms: AnalyticFiller (scanline), SparseStrips
  (4×4 tiles), TileCompute (16×16 tiles), SDFAccelerator (per-pixel SDF),
  Vello PTCL (GPU compute).
- **`CoverageFiller` interface** — Tile-based coverage rasterizer interface with
  `RegisterCoverageFiller()` / `GetCoverageFiller()` registration pattern
  (mirrors `GPUAccelerator`). `ForceableFiller` extension interface exposes
  `SparseFiller()` / `ComputeFiller()` for forced algorithm selection.
- **`AdaptiveFiller`** — Auto-selects between SparseStrips (4×4) and TileCompute
  (16×16) based on estimated segment count (10K threshold) and canvas area (2MP).
- **`RasterizerMode` API** — Per-context force override: `RasterizerAuto`,
  `RasterizerAnalytic`, `RasterizerSparseStrips`, `RasterizerTileCompute`,
  `RasterizerSDF`. Use `Context.SetRasterizerMode()` for debugging, benchmarking,
  or known workloads.
- **`ForceSDFAware` interface** — Optional GPU accelerator interface for forced
  SDF rendering. `SetForceSDF(true)` bypasses the 16px minimum size check.
- **`gg/raster/` package** — CPU-only tile rasterizer registration via blank
  import `import _ "github.com/gogpu/gg/raster"`. Independent of GPU packages.
- **SDF minimum size** — Shapes smaller than 16px skip SDF rendering (unless
  `RasterizerSDF` mode is forced) to avoid overhead on tiny shapes.

## [0.31.1] - 2026-02-27

### Fixed

- **Vulkan: rounded rectangle pixel corruption** — update wgpu v0.18.0 → v0.18.1 which fixes
  buffer-to-image copy row stride calculation on non-power-of-2 width textures.
  ([gogpu#96](https://github.com/gogpu/gogpu/discussions/96))

## [0.31.0] - 2026-02-27

### Breaking Changes

- **`text.Shape()` signature changed** — Removed redundant `size float64` parameter. Size is now obtained from `face.Size()`. All callers must update: `Shape(text, face, size)` → `Shape(text, face)`. This affects `Shape`, `LayoutText`, `LayoutTextWithContext`, `LayoutTextSimple`, `WrapText`, `MeasureText`, and the `Shaper` interface. ([#138](https://github.com/gogpu/gg/issues/138))

### Added

- **`DrawStringWrapped()`** — Wraps text to width and draws with alignment and anchoring. Compatible with fogleman/gg's `DrawStringWrapped`. Supports `AlignLeft`, `AlignCenter`, `AlignRight`. ([#138](https://github.com/gogpu/gg/issues/138))
- **`MeasureMultilineString()`** — Measures text containing newlines with configurable line spacing. Compatible with fogleman/gg. ([#138](https://github.com/gogpu/gg/issues/138))
- **`WordWrap()`** — Wraps text at word boundaries, returns `[]string`. Compatible with fogleman/gg. ([#138](https://github.com/gogpu/gg/issues/138))
- **`Align` type + constants** — `gg.AlignLeft`, `gg.AlignCenter`, `gg.AlignRight` re-exported from `text.Alignment` for convenience. ([#138](https://github.com/gogpu/gg/issues/138))
- **`gg.RGBA` implements `color.Color`** — Added `RGBA()` method returning premultiplied uint32 values for stdlib compatibility. ([#138](https://github.com/gogpu/gg/issues/138))
- **`Pixmap.SetPixelPremul()`** — Direct premultiplied RGBA pixel write without alpha conversion overhead. ([#114](https://github.com/gogpu/gg/issues/114))
- **Recording mirror** — `DrawStringWrapped`, `MeasureMultilineString`, `WordWrap` mirrored on `recording.Recorder` for vector export.

### GPU Pipeline

- **Tier 5 scene accumulation (GG-COMPUTE-008)** — `VelloAccelerator` now accumulates `PathDef`s during `FillPath`/`StrokePath` and dispatches via compute pipeline on `Flush`. Path conversion (gg.Path → tilecompute.PathDef) with Euler spiral curve flattening.
- **PipelineMode wiring (GG-COMPUTE-006)** — `Context.SetPipelineMode()` propagates to GPU accelerator. `SDFAccelerator` holds internal `VelloAccelerator` and routes to compute pipeline when `PipelineModeCompute` is active. `SelectPipeline()` heuristics exported.
- **Removed 2 naga workarounds from `path_tiling.wgsl`** — Inline `span()` replaced with function call, `let`-chain replaced with `var` reassignment. Validated by golden tests. 3 workarounds remain due to active naga SPIR-V bugs ([#139](https://github.com/gogpu/gg/issues/139)).

### Fixed

- **`LayoutText` wrapped line Y positions** — Lines all had Y=0 instead of cumulative vertical positions. Each line now has correct Y = previous Y + descent + line gap + current ascent. ([#138](https://github.com/gogpu/gg/issues/138))
- Resolved all golangci-lint issues (errorlint, gocognit, staticcheck, dupl).

### Dependencies

- wgpu v0.16.17 → v0.18.0

## [0.30.2] - 2026-02-27

### Fixed

- `FontSource.Face()` now panics with clear message instead of cryptic SIGSEGV when called on nil receiver ([#134](https://github.com/gogpu/gg/issues/134))
- `BuiltinShaper` now skips control characters (U+0000..U+001F) instead of rendering them as missing glyph boxes ([#134](https://github.com/gogpu/gg/issues/134))
- `WrapText` now respects hard line breaks (`\n`, `\r\n`, `\r`) — paragraphs are split before wrapping, matching `LayoutText` behavior ([#134](https://github.com/gogpu/gg/issues/134))
- **Vello compute GPU buffer overflow** — `computeBufferSizes` used `numLines * 4` heuristic for segment buffer allocation, which overflowed for scenes with long diagonal lines (e.g., a 3-line triangle needed 23 segment slots but only 12 were allocated). Replaced with DDA upper bound `numLines * (widthInTiles + heightInTiles)` ([#135](https://github.com/gogpu/gg/issues/135))

### Dependencies

- wgpu v0.16.15 → v0.16.17 (load platform Vulkan surface creation functions — [gogpu#106](https://github.com/gogpu/gogpu/issues/106))

## [0.30.1] - 2026-02-25

### Dependencies

- wgpu v0.16.14 → v0.16.15 (software backend always compiled, no build tags — [gogpu#106](https://github.com/gogpu/gogpu/issues/106))

## [0.30.0] - 2026-02-25

### Added

- **Vello compute pipeline (Tier 5)** — Port of vello's 9-stage GPU compute
  architecture for full-scene parallel rasterization. 9 WGSL compute shaders
  (pathtag_reduce, pathtag_scan, draw_reduce, draw_leaf, path_count, backdrop,
  coarse, path_tiling, fine) dispatched via wgpu HAL. 16×16 tiles, 256 threads
  per workgroup.
- **tilecompute CPU reference** — Complete CPU implementation of the 9-stage
  pipeline (`RasterizeScenePTCL`) for golden test comparison and CPU fallback.
  Includes scene encoding (`EncodeScene`/`PackScene`), Euler spiral curve
  flattening, path tag/draw monoid prefix scans, per-tile segment counting,
  backdrop accumulation, coarse PTCL generation, path_tiling segment clipping,
  and fine per-pixel rasterization.
- **PipelineMode API** — `PipelineModeAuto`, `PipelineModeRenderPass`,
  `PipelineModeCompute` for selecting between render-pass (Tiers 1–4) and
  compute (Tier 5) GPU pipelines.
- **GPU vs CPU golden tests** — 7 test scenes (triangle, square, circle,
  star nonzero/evenodd, multipath, overlapping semitransparent) comparing
  GPU compute output against CPU reference pixel-by-pixel.

### Fixed

- **DrawString not affected by Transform** ([#129](https://github.com/gogpu/gg/issues/129)) —
  `DrawString` and `DrawStringAnchored` now apply `c.matrix.TransformPoint()` to the text
  position before rendering, consistent with `MoveTo`, `LineTo`, and other drawing methods.
- **DrawImageEx missing scaling transform** ([#130](https://github.com/gogpu/gg/issues/130)) —
  `DrawImageEx` now computes a scaling transform that maps dst rect coordinates to src rect
  coordinates. Without this, images were clipped to source size when the destination was larger.
- **fine.wgsl y_edge** — select() workaround for naga SPIR-V codegen bug
  that caused incorrect edge coverage in fine rasterization stage.
- **coarse.wgsl Z-order** — per-tile iteration instead of per-draw-object
  ensures correct front-to-back ordering in PTCL generation.

### Dependencies

- naga v0.14.2 → v0.14.3 (5 SPIR-V backend bug fixes)
- wgpu v0.16.13 → v0.16.14 (Vulkan null surface handle guard)

## [0.29.5] - 2026-02-24

### Fixed

- **AdvanceX drift causing edge expansion** ([#95](https://github.com/gogpu/gg/issues/95)) —
  scanline-to-scanline AdvanceX() accumulated floating-point error, causing triangle/polygon
  edges to progressively expand toward the bottom of shapes. Replaced with direct per-scanline
  X computation from edge endpoints.
- **coverageToRuns maxValue bug** ([#95](https://github.com/gogpu/gg/issues/95)) —
  when merging adjacent alpha runs, the merged run used the sum of coverage values instead of
  the maximum, causing vertex pixels to receive incorrect partial coverage (darker than expected).
  Added 4 regression tests for vertex pixel accuracy.

### Dependencies

- wgpu v0.16.12 → v0.16.13 (VK_EXT_debug_utils fix)
- gogpu v0.20.3 → v0.20.4 (examples/gogpu_integration)

## [0.29.4] - 2026-02-23

### Fixed

- **scene.Renderer: delegate rasterization to gg.SoftwareRenderer** (#124)
  - Replaced broken internal rasterizer with delegation to `gg.SoftwareRenderer`
  - Fill/stroke now rendered with analytic anti-aliasing (Vello tile-based AA)
  - Full curve support in stroke (CubicTo, QuadTo) — circles/ellipses render correctly
  - Premultiplied source-over alpha compositing (replaces raw `copy()`)
  - Background preservation — user's `target.Clear()` is no longer destroyed
  - `sync.Pool`-based per-tile SoftwareRenderer and Pixmap reuse
  - Path conversion: `scene.Path` (float32) → `gg.Path` (float64) with tile offset
  - Brush/style conversion: `scene.Brush` → `gg.Paint` via non-deprecated `SetStroke()` API
  - Removed dead code: `fillPathOnTile`, `strokePathOnTile`, `drawLineOnTile`, `blendPixel`
  - Zero public API changes — `NewRenderer`, `Render`, `RenderDirty` unchanged
  - Orchestration preserved: TileGrid, WorkerPool, DirtyRegion, LayerCache untouched
  - 11 new pixel-level correctness tests

## [0.29.3] - 2026-02-23

### Dependencies

- wgpu v0.16.11 → v0.16.12 (Vulkan debug object naming)
- gogpu v0.20.2 → v0.20.3 (examples/gogpu_integration)

## [0.29.2] - 2026-02-23

### Dependencies

- wgpu v0.16.10 → v0.16.11 (Vulkan zero-extent swapchain fix)
- gogpu v0.20.1 → v0.20.2 (examples/gogpu_integration)

## [0.29.1] - 2026-02-22

### Dependencies

- wgpu v0.16.9 → v0.16.10
- naga v0.14.1 → v0.14.2
- gogpu v0.20.0 → v0.20.1 (examples/gogpu_integration)

## [0.29.0] - 2026-02-21

### Added
- **GPU MSDF text pipeline** — `MSDFTextPipeline` renders text entirely on GPU using
  Multi-channel Signed Distance Field technique (Tier 4). WGSL fragment shader with
  standard Chlumsky/msdfgen `screenPxRange` formula produces resolution-independent
  anti-aliased text. 48px MSDF cells, pxRange=6, pixel-snapped quads, centered glyph
  content in atlas cells for correct positioning of all glyph aspect ratios.
- **Four-tier GPU render pipeline** — GPURenderSession upgraded from three-tier to
  four-tier: SDF (Tier 1) + Convex (Tier 2a) + Stencil+Cover (Tier 2b) + MSDF Text (Tier 4).
- **ggcanvas auto-registration** — `ggcanvas.Canvas` auto-registers with `App.TrackResource()`
  via duck-typed interface detection. No manual `defer canvas.Close()` or `OnClose` wiring
  needed — shutdown cleanup is automatic (LIFO order).
- **GPU stroke rendering** — `SDFAccelerator.StrokePath()` converts stroked paths to filled
  polygon outlines via stroke-expand-then-fill, then routes through the GPU convex polygon
  renderer. Eliminates CPU fallback for line strokes (checkbox checkmarks, radio outlines).

### Fixed
- **SceneBuilder.WithTransform invisible rendering** ([#116](https://github.com/gogpu/gg/issues/116)) —
  tile-based renderer early-out used untransformed encoding bounds, causing content moved by
  transforms to be skipped. Bounds management moved from Encoding to Scene level with proper
  coordinate transforms. Clip paths no longer incorrectly expand encoding bounds.
- **GPU text pipeline resource leak** — destroy MSDFTextPipeline in SDFAccelerator.Close()
  (ShaderModule, PipelineLayout, Pipelines, DescriptorSetLayout, Sampler).
- **Surface dimension mismatch** — `GPURenderSession.RenderFrame()` uses surface dimensions
  for MSAA texture sizing and viewport uniforms in RenderDirect mode.
- **DX12 text disappearing after ~1 second** — text bind group was unconditionally destroyed
  and recreated every frame, freeing DX12 descriptor heap slots still referenced by in-flight
  GPU work. Changed to persistent bind group pattern (matching SDF) — create once, invalidate
  only when buffers are reallocated or atlas changes.

### Dependencies
- wgpu v0.16.6 → v0.16.9 (Metal presentDrawable fix, naga v0.14.1)
- naga v0.13.1 → v0.14.1 (HLSL row_major matrices for DX12, GLSL namedExpressions fix for GLES)
- gogpu v0.19.6 → v0.20.0 (ResourceTracker, automatic GPU resource cleanup)

## [0.28.6] - 2026-02-18

### Dependencies
- wgpu v0.16.5 → v0.16.6 (Metal debug logging, goffi v0.3.9)

## [0.28.5] - 2026-02-18

### Dependencies
- wgpu v0.16.4 → v0.16.5 (per-encoder command pools, fixes VkCommandBuffer crash)

## [0.28.4] - 2026-02-18

### Dependencies
- wgpu v0.16.3 → v0.16.4 (Vulkan timeline semaphore, FencePool, command buffer batch allocation, hot-path allocation optimization)
- naga v0.13.0 → v0.13.1 (SPIR-V OpArrayLength fix, −32% compiler allocations)
- gogpu v0.19.1 → v0.19.2 in examples (hot-path benchmarks)

## [0.28.3] - 2026-02-16

### Dependencies
- wgpu v0.16.1 → v0.16.2 (Metal autorelease pool LIFO fix for macOS Tahoe)

## [0.28.2] - 2026-02-15

### Changed

- **Persistent GPU buffers** — SDF/convex vertex buffers, uniform buffers, and bind
  groups survive across frames with grow-only reallocation (2x headroom). Reduces
  per-frame GPU overhead from ~14 buffer create/destroy cycles to zero in steady-state.
- **Fence-free surface submit** — surface rendering mode submits without fence wait;
  previous frame's command buffer freed at start of next frame (VSync guarantees GPU
  completion). Readback mode still uses fence. Eliminates 0.5-2ms/frame fence latency.
- **Vertex staging reuse** — CPU-side byte slices for SDF and convex vertex data reused
  across frames with grow-only strategy to reduce GC pressure.
- **Stencil buffer pooling** — pool-based approach for multi-path stencil buffer reuse.
- **GPU queue drain on shutdown** — no-op command buffer ensures GPU idle before resource
  destruction on shutdown and mode switch.
- **gogpu_integration example** — `CloseAccelerator` in `OnClose` handler with correct
  shutdown order; dependency update to gg v0.28.1.

### Fixed
- **golangci-lint config** — exclude `tmp/` directory from linting (gitignored debug files)

### Dependencies
- wgpu v0.16.0 → v0.16.1 (Vulkan framebuffer cache invalidation fix)
- gogpu v0.18.1 → v0.18.2, gg v0.28.1 → v0.28.2 (in examples)

## [0.28.1] - 2026-02-15

### Fixed

- **GPU readback compositing** — replaced `convertBGRAToRGBA` with Porter-Duff "over"
  compositing (`compositeBGRAOverRGBA`) for multi-flush correctness. GPU readback now
  correctly composites over existing canvas content instead of overwriting it.

### Changed

- **gogpu_integration example** — updated to event-driven rendering with `AnimationToken`,
  demonstrates three-state model (idle/animating/continuous) and Space key pause/resume

### Dependencies
- gogpu v0.18.0 → v0.18.1 (in examples)

## [0.28.0] - 2026-02-15

### Added

#### Three-Tier GPU Render Pipeline

Complete GPU rendering pipeline with three tiers, unified under a single render pass.

##### Tier 1: SDF Render Pipeline
- **SDF render pipeline** — Signed Distance Field rendering for smooth primitive shapes
  - GPU-accelerated SDF for circles, ellipses, rectangles, rounded rectangles
  - Convexity detection for automatic tier selection
  - WGSL SDF shaders with analytic anti-aliasing

##### Tier 2a: Convex Fast-Path Renderer
- **Convex fast-path renderer** — optimized rendering for convex polygons
  - Direct vertex emission without tessellation overhead
  - Automatic convexity detection from path geometry
  - Single draw call per convex shape

##### Tier 2b: Stencil-Then-Cover (Arbitrary Paths)
- **Stencil-then-cover pipeline** — GPU rendering for arbitrary complex paths
  - `StencilRenderer` with MSAA + stencil texture management
  - Fan tessellator for converting paths to triangle fans
  - Stencil fill + cover render pipelines with WGSL shaders
  - EvenOdd fill rule support for stencil-then-cover (GG-GPU-010)
  - Integrated into `GPUAccelerator.FillPath`

##### Unified Architecture
- **Unified render pass** — all three tiers rendered in a single `BeginRenderPass`
  - Eliminates per-tier render pass overhead
  - Shared depth/stencil state across tiers
- **`RenderDirect()`** — zero-copy GPU surface rendering (GG-GPU-019)
  - Renders directly to GPU surface without intermediate buffer copies
  - `CloseAccelerator()` and GPU flush on `Context.Close()`
  - Lazy GPU initialization with surface target persistence between frames

#### ggcanvas Enhancements
- **`Canvas.Draw()` helper** — draws with `gg.Context` and marks dirty atomically,
  replacing manual `MarkDirty()` calls
- **Deferred texture destruction** on resize for DX12 stability

#### Observability
- **Structured logging via `log/slog`** — all GPU subsystem logging uses `slog`,
  silent by default (no output unless handler configured)

#### Testing
- **Raster package coverage** increased from 42.9% to 90.8%

### Fixed

- **TextureViewDescriptor wgpu-native compatibility** — all `CreateTextureView` calls now
  set explicit `Format`, `Dimension`, `Aspect`, and `MipLevelCount` instead of relying on
  zero-value defaults. Native Go backends handle zero defaults gracefully, but wgpu-native
  panics on `MipLevelCount=0`.
- **ggcanvas: DX12 texture disappearance during resize** — deferred texture
  destruction prevents descriptor heap use-after-free. Old texture is kept alive
  until after `WriteTexture` completes (GPU idle), then destroyed safely.
  Root cause: DX12 shader-visible sampler heap has a hard 2048-slot limit;
  leaked textures exhaust it, causing `CreateBindGroup` to fail silently
- **ggcanvas: removed debug logging** — alpha pixel counting and diagnostic
  `log.Printf` calls removed from `Flush()`
- **GPU readback pitch alignment** — aligned readback buffer pitch and added
  barrier after copy for correct GPU-to-CPU data transfer
- **GPU texture layout transition** — added texture layout transition before
  `CopyTextureToBuffer` to prevent validation errors
- **Surface target persistence** — keep surface target between frames, lazy GPU
  initialization prevents crashes on early frames
- **WGSL shader syntax** — removed stray semicolons from WGSL shader struct
  declarations
- **Raster X-bounds clipping** — added X-bounds clipping to analytic AA coverage
  computation, preventing out-of-bounds writes
- **gogpu integration exit crash** — example updated to use `App.OnClose()` for canvas
  cleanup, preventing Vulkan validation errors when GPU resources were destroyed after device
- **Linter warnings** resolved in raster and ggcanvas packages

### Changed

- **GPU architecture refactored** — deleted compute pipeline legacy code, retained
  render pipeline only
- **Examples updated** — `gpu` and `gogpu_integration` examples rewritten for
  three-tier rendering architecture with GLES backend support

### Dependencies
- wgpu v0.15.0 → v0.16.0
- naga v0.12.0 → v0.13.0
- gogpu v0.17.0 → v0.18.0 (in examples)

## [0.27.1] - 2026-02-10

### Fixed

- **Text rendering over GPU shapes** — `DrawString` and `DrawStringAnchored` now flush pending GPU accelerator batch before drawing text, preventing GPU-rendered shapes (e.g., rounded rect backgrounds) from overwriting previously drawn text

## [0.27.0] - 2026-02-10

### Added

- **SDF Accelerator** — Signed Distance Field rendering for smooth shapes
  - `SDFAccelerator` — CPU SDF for circles, ellipses, rectangles, rounded rectangles
  - `DetectShape(path)` — auto-detects circle (4 cubics with kappa), rect, rrect from path elements
  - `Context.Fill()/Stroke()` tries accelerator first, falls back to `SoftwareRenderer`
  - Register via `gg.RegisterAccelerator(&gg.SDFAccelerator{})`
  - ~30% smoother edges compared to area-based rasterizer
- **GPU SDF compute pipeline** — GPU-accelerated SDF via wgpu HAL
  - `NativeSDFAccelerator` with DeviceProvider integration for GPU device sharing
  - WGSL compute shaders (`sdf_batch.wgsl`) for batch SDF rendering
  - Multi-pass dispatch workaround for naga loop iteration bug
  - GPU → CPU buffer readback via `hal.Queue.ReadBuffer`
- **GPUAccelerator interface** extended with `FillPath`, `StrokePath` rendering methods and `CanAccelerate` shape detection
- **`gpu/` public registration package** (ADR-009) — opt-in GPU acceleration via `import _ "github.com/gogpu/gg/gpu"`
- **SDF example** (`examples/sdf/`) — demonstrates SDF accelerator with filled and stroked shapes

### Changed

- **Architecture:** `internal/native` renamed to `internal/gpu` for clarity
- **Dependencies updated:**
  - gpucontext v0.8.0 → v0.9.0
  - naga v0.11.0 → v0.12.0
  - wgpu v0.13.2 → v0.15.0
  - golang.org/x/image v0.35.0 → v0.36.0
  - golang.org/x/text v0.33.0 → v0.34.0
- **Examples:** gogpu_integration updated to gogpu v0.17.0+, gg v0.27.0+

### Fixed

- Curve flattening tolerance and stroke join continuity improvements
- WGSL SDF shaders rewritten to work around naga SPIR-V codegen bugs (5 bugs documented)
- Flush pending GPU shapes before pixel readback

## [0.26.1] - 2026-02-07

### Changed

- **naga** dependency updated v0.10.0 → v0.11.0 — fixes SPIR-V `if/else` GPU hang, adds 55 new WGSL built-in functions
- **wgpu** dependency updated v0.13.1 → v0.13.2
- **gogpu_integration example** — updated minimum gogpu version to v0.15.7+

## [0.26.0] - 2026-02-06

### Added

- **GPUAccelerator interface** — optional GPU acceleration with transparent CPU fallback
  - `RegisterAccelerator()` for opt-in GPU via blank import pattern
  - `ErrFallbackToCPU` sentinel error for graceful degradation
  - `AcceleratedOp` bitfield for capability checking
  - Zero overhead (~17ns) when no GPU registered

### Changed

- **Architecture: CPU raster is core, GPU is optional accelerator**
  - CPU rasterization types extracted to `internal/raster` package
  - Native rendering pipeline moved to `internal/native` package
  - `SoftwareRenderer` uses `internal/raster` directly (no backend abstraction)
  - `cache`, `gpucore` packages moved to `internal/` (implementation details)

### Removed

- **`backend/` package** — RenderBackend interface, registry pattern, SoftwareBackend wrapper
- **`backend/rust/`** — dead Rust FFI backend code (5 files)
- **`internal/raster/` (legacy)** — old supersampled AA rasterizer (14 files, replaced by analytic AA)
- **`go-webgpu/webgpu`** dependency — no longer needed
- **`go-webgpu/goffi`** dependency — no longer needed

## [0.25.0] - 2026-02-06

### Added

- **Vello tile-based analytic anti-aliasing rasterizer**
  - Port of vello_shaders CPU fine rasterizer (`fine.rs`) to Go
  - 16x16 tile binning with DDA-based segment distribution
  - Analytic trapezoidal area coverage per pixel (no supersampling)
  - yEdge mechanism for correct winding number propagation via backdrop prefix sum
  - VelloLine float pipeline: bypasses fixed-point quantization (FDot6/FDot16) for improved accuracy
  - Bottom-of-circle artifact improved from alpha=191 to alpha=248
  - NonZero and EvenOdd fill rules
  - Golden test infrastructure with 7 test shapes and reference image comparison
  - Research documentation with detailed algorithm analysis

### Changed

- **Examples:** update `gogpu_integration` dependencies to gg v0.24.1, gogpu v0.15.5

### Planned for v1.0.0
- API Review and cleanup
- Comprehensive documentation
- Performance benchmarks

## [0.24.1] - 2026-02-05

### Fixed

- **Alpha compositing: fix dark halos around anti-aliased shapes**
  - Root cause: mixed alpha conventions — `FillSpanBlend` stored premultiplied, `BlendPixelAlpha` stored straight, causing double-premultiplication
  - Standardized on **premultiplied alpha** (industry standard: tiny-skia, Ebitengine, vello, femtovg, Cairo, SDL)
  - `Pixmap`: store premultiplied RGBA in `SetPixel`, `Clear`, `FillSpan`
  - `Pixmap`: un-premultiply in `GetPixel` for public API
  - `Pixmap.At()` returns `color.RGBA` (premultiplied), `ColorModel()` → `color.RGBAModel`
  - Software renderer: fix all 4 `BlendPixelAlpha` locations to premultiplied source-over
  - `FromColor()`: correctly un-premultiply Go's `color.Color.RGBA()` output
  - `ColorMatrixFilter`: un-premultiply before matrix transform, re-premultiply after
  - `ggcanvas`: mark textures as premultiplied via `SetPremultiplied(true)`
  - Requires gogpu v0.15.5+ for correct GPU compositing with `BlendFactorOne`
- **Examples:** fix hardcoded output paths in `clipping` and `images` examples ([#85](https://github.com/gogpu/gg/pull/85))
  - Both used `examples/*/output.png` which only worked from repo root
  - Now use `output.png` — `go run .` works from example directory
- **gogpu_integration example:** update dependency versions to gg v0.24.0 / gogpu v0.15.4
- **Cleanup:** remove stale `rect_debug/` directory (debug artifacts from rasterizer experiments)

## [0.24.0] - 2026-02-05

### Added

- **GoTextShaper: HarfBuzz-level text shaping** ([#78](https://github.com/gogpu/gg/issues/78))
  - `GoTextShaper` wraps go-text/typesetting's HarfBuzz engine
  - Supports ligatures, kerning, contextual alternates, complex scripts
  - Opt-in via `text.SetShaper(text.NewGoTextShaper())`
  - Thread-safe: `sync.Pool` for HarfBuzz shapers, cached `font.Font` (read-only)
  - Fixed concurrency bug: `font.Face` and `HarfbuzzShaper` are not goroutine-safe
  - Uses `font.Font` cache (thread-safe) + per-call `font.NewFace()` (lightweight)
  - Uses deprecated `ClusterIndex` replaced with `TextIndex()`
  - 20+ tests including concurrency, kerning, ligatures, cache management
  - 3 benchmarks (short, standard, long text)

- **WebP image format support** ([#77](https://github.com/gogpu/gg/issues/77))
  - `LoadWebP()`, `DecodeWebP()` for explicit WebP decoding
  - `LoadImage()` and `LoadImageFromBytes()` auto-detect WebP via registered decoder
  - Uses `golang.org/x/image/webp` (already in go.mod)

- **gogpu_integration example** — moved from `gogpu/examples/gg_integration/` to fix inverted dependency (gogpu no longer depends on gg)
  - Isolated Go module with own `go.mod`
  - Demonstrates gg + gogpu rendering via ggcanvas

### Fixed

- **Custom Pattern implementations always render black** ([#75](https://github.com/gogpu/gg/issues/75))
  - Root cause 1: `getColorFromPaint()` only handled `*SolidPattern`, returned Black for everything else
  - Root cause 2: `SetFillPattern()`/`SetStrokePattern()` didn't sync `paint.Brush`, breaking `ColorAt()` precedence
  - Fix: New `painterPixmapAdapter` samples `paint.ColorAt(x,y)` per-pixel for non-solid paints
  - Solid paints still use fast single-color path (no performance regression)
  - New `Painter` interface (`painter.go`) for future span-based optimizations

- **ggcanvas texture updates silently failing** ([#79](https://github.com/gogpu/gg/issues/79))
  - Root cause: local `textureUpdater` interface expected `UpdateData(data []byte)` (no error return), but `gogpu.Texture.UpdateData` returns `error` — type assertion failed silently
  - Fix: use shared `gpucontext.TextureUpdater` interface with proper error handling
  - Added auto-dirty in `RenderToEx()` — calling `RenderTo` now always uploads current content
  - Compile-time interface check for mock in tests

## [0.23.0] - 2026-02-03

### Added

#### Recording System for Vector Export (ARCH-011)

Command-based drawing recording system enabling vector export to PDF, SVG, and other formats.

**Architecture (Cairo/Skia-inspired)**
- **Command Pattern** — Typed command structs for all drawing operations
- **Resource Pooling** — PathRef, BrushRef, ImageRef for efficient storage
- **Backend Interface** — Pluggable renderers via `recording.Backend`
- **Driver Pattern** — database/sql style registration via blank imports

**Core Types (recording/)**
- **Recorder** — Captures drawing operations with full gg.Context-like API
  - Path operations: MoveTo, LineTo, QuadraticTo, CubicTo, ClosePath
  - Shape helpers: DrawRectangle, DrawRoundedRectangle, DrawCircle, DrawEllipse, DrawArc
  - Fill/stroke with solid colors and gradients
  - Line styles: width, cap, join, miter limit, dash patterns
  - Transformations: Translate, Rotate, Scale, matrix operations
  - Clipping: path-based clipping with fill rules
  - State management: Push/Pop (Save/Restore)
  - Text rendering, image drawing
- **Recording** — Immutable command sequence for playback
  - `Commands()` — Access recorded commands
  - `Resources()` — Access resource pool
  - `Playback(backend)` — Render to any backend
- **ResourcePool** — Deduplicating storage for paths, brushes, images, fonts

**Brush Types**
- **SolidBrush** — Single solid color
- **LinearGradientBrush** — Linear color gradient with spread modes
- **RadialGradientBrush** — Radial color gradient
- **SweepGradientBrush** — Angular/conic gradient

**Backend Interface**
- **Backend** — Core rendering interface
  - `Begin(width, height)`, `End()`
  - `Save()`, `Restore()`
  - `SetTransform(m Matrix)`
  - `SetClip(path, rule)`, `ClearClip()`
  - `FillPath(path, brush, rule)`
  - `StrokePath(path, brush, stroke)`
  - `FillRect(rect, brush)`
  - `DrawImage(img, src, dst, opts)`
  - `DrawText(s, x, y, face, brush)`
- **WriterBackend** — `WriteTo(w io.Writer)` for streaming
- **FileBackend** — `SaveToFile(path)` for file output
- **PixmapBackend** — `Pixmap()` for raster access

**Backend Registry**
- `Register(name, factory)` — Register backend factory
- `NewBackend(name)` — Create backend by name
- `IsRegistered(name)` — Check availability
- `Backends()` — List all registered backends

**Built-in Raster Backend (recording/backends/raster/)**
- Renders to gg.Context for PNG output
- Auto-registers as "raster" backend
- Implements Backend, WriterBackend, FileBackend, PixmapBackend

**External Export Backends**
- **github.com/gogpu/gg-pdf** — PDF export via gxpdf
- **github.com/gogpu/gg-svg** — SVG export (pure Go)

### Example

```go
import (
    "github.com/gogpu/gg/recording"
    _ "github.com/gogpu/gg/recording/backends/raster"
    _ "github.com/gogpu/gg-pdf" // Optional PDF export
    _ "github.com/gogpu/gg-svg" // Optional SVG export
)

// Record drawing
rec := recording.NewRecorder(800, 600)
rec.SetFillRGBA(1, 0, 0, 1)
rec.DrawCircle(400, 300, 100)
rec.Fill()
r := rec.FinishRecording()

// Export to multiple formats
for _, name := range []string{"raster", "pdf", "svg"} {
    if backend, err := recording.NewBackend(name); err == nil {
        r.Playback(backend)
        backend.(recording.FileBackend).SaveToFile("output." + name)
    }
}
```

### Statistics
- **~3,500 LOC** in recording/ package
- **20+ command types** for all drawing operations
- **4 brush types** with gradient support
- **3 backend interfaces** for flexible output
- **Comprehensive tests** with 90%+ coverage

## [0.22.3] - 2026-02-02

### Fixed

- **Semi-transparent color blending** ([#73](https://github.com/gogpu/gg/issues/73))
  - `BlendPixelAlpha` now correctly checks color alpha before using fast path
  - Fixes "mosaic" artifacts when filling shapes with alpha < 255
  - Thanks to @i2534 for reporting

## [0.22.2] - 2026-02-01

### Changed

- **Update naga v0.9.0 → v0.10.0** — Storage textures, switch statements
- **Update wgpu v0.12.0 → v0.13.0** — Format capabilities, array textures, render bundles

## [0.22.1] - 2026-01-30

### Fixed

- **LineJoinRound rendering** ([#62](https://github.com/gogpu/gg/issues/62))
  - Round join arc now correctly starts from previous segment's normal
  - Fixes angular/incorrect appearance when using `LineJoinRound`

## [0.22.0] - 2026-01-30

### Added

- **gpucontext.TextureDrawer integration** — Unified cross-package texture API
  - `ggcanvas.RenderTo()` now accepts `gpucontext.TextureDrawer` interface
  - Enables seamless integration with any GPU framework implementing the interface
  - No direct gogpu imports required in ggcanvas

### Changed

- **Update gpucontext v0.3.1 → v0.4.0** — Texture, Touch interfaces
- **Update wgpu v0.11.2 → v0.12.0** — BufferRowLength fix (aspect ratio)
- **Update naga v0.8.4 → v0.9.0** — Shader compiler improvements
- **Update go-webgpu/webgpu v0.1.4 → v0.2.1** — Latest FFI bindings

### Fixed

- Test mocks for new `hal.NativeHandle` interface
- ggcanvas tests for new `gpucontext.TextureDrawer` interface

## [0.21.4] - 2026-01-29

### Added

- **GGCanvas Integration Package** (INT-004)
  - New `integration/ggcanvas/` package for gogpu integration
  - `Canvas` type wrapping gg.Context with GPU texture management
  - `RenderTo(dc)` — Draw canvas to gogpu window
  - `RenderToEx(dc, opts)` — Draw with position, scale, alpha options
  - Lazy texture creation on first flush
  - Dirty tracking to avoid unnecessary GPU uploads
  - 14 unit tests, full documentation

### Changed

- **Update dependencies** for webgpu.h spec compliance
  - `github.com/gogpu/gpucontext` v0.3.0 → v0.3.1
  - `github.com/gogpu/wgpu` v0.11.1 → v0.11.2

### Usage Example

```go
canvas, _ := ggcanvas.New(app.GPUContextProvider(), 800, 600)
defer canvas.Close()

// Draw with gg API
cc := canvas.Context()
cc.SetRGB(1, 0, 0)
cc.DrawCircle(400, 300, 100)
cc.Fill()

// Render to gogpu window
canvas.RenderTo(dc)
```

## [0.21.3] - 2026-01-29

### Changed

- Migrate to unified `gputypes` package for WebGPU types
  - Replace `wgpu/types` imports with `gputypes`
  - Update `render/` package to use `gputypes.TextureFormat`
  - Update `backend/native/` for gputypes compatibility

### Dependencies

- Add `github.com/gogpu/gputypes` v0.1.0
- Update `github.com/gogpu/gpucontext` v0.2.0 → v0.3.0
- Update `github.com/gogpu/wgpu` v0.10.2 → v0.11.1

## [0.21.2] - 2026-01-28

### Added

- **Hairline rendering** (BUG-003, [#56](https://github.com/gogpu/gg/issues/56))
  - Dual-path stroke rendering following tiny-skia/Skia pattern
  - Thin strokes (width <= 1px after transform) use direct hairline rendering
  - Fixed-point arithmetic (FDot6/FDot16) for sub-pixel precision
  - +0.5 centering fix for correct pixel distribution on integer coordinates
  - Line cap support (butt, round, square) for hairlines

- **Transform-aware stroke system**
  - `Matrix.ScaleFactor()` — extracts max scale from transform matrix
  - `Paint.TransformScale` — passes transform info to renderer
  - `Dash.Scale()` — scales dash pattern by transform (Cairo/Skia convention)

### Fixed

- **Thin dashed strokes render as disconnected pixels** ([#56](https://github.com/gogpu/gg/issues/56))
  - Root cause 1: Stroke expansion creates paths too thin for proper coverage
  - Solution: Hairline rendering for strokes ≤1px (after transform)

- **Stroke expansion artifacts with scale > 1** ([#56](https://github.com/gogpu/gg/issues/56))
  - Root cause 2: `finish()` computed wrong normal for end cap from point difference
  - Solution: Save `lastNorm` in `doLine()`, use it for end cap (tiny-skia pattern)
  - Eliminates horizontal stripes inside dash segments at scale > 1

### New Files

- `internal/raster/hairline_aa.go` — Core AA hairline algorithm
- `internal/raster/hairline_blitter.go` — Hairline blitter interface
- `internal/raster/hairline_caps.go` — Line cap handling
- `internal/raster/hairline_types.go` — Fixed-point types

## [0.21.1] - 2026-01-28

### Fixed

- **Dashed strokes with scale** (BUG-002, [#54](https://github.com/gogpu/gg/issues/54))
  - Root cause: `path.Flatten()` lost subpath boundaries, causing rasterizer to create incorrect "connecting edges" between separate subpaths
  - Solution: New `path.EdgeIter` following tiny-skia pattern — iterates over edges directly without creating inter-subpath connections
  - Added `raster.FillAAFromEdges()` for correct edge-based rasterization

## [0.21.0] - 2026-01-27

### Added

- **Enterprise Architecture** for gogpu/ui integration

#### Package Restructuring
- **core/** (ARCH-003) — CPU rendering internals separated from GPU code
- **surface/** (ARCH-004) — Unified Surface interface (ImageSurface, GPUSurface)
- **render/** (INT-001) — Device integration package
  - `DeviceHandle` — alias for gpucontext.DeviceProvider
  - `RenderTarget` — interface for CPU/GPU render targets
  - `Scene` — retained-mode drawing commands
  - `Renderer` — interface for render implementations

#### UI Integration (UI-ARCH-001)
- **Damage Tracking** — `Scene.Invalidate()`, `DirtyRects()`, `NeedsFullRedraw()`
- **LayeredTarget** — Z-ordered layers for popups, dropdowns, tooltips
- **Context.Resize()** — Frame reuse without allocation

#### gpucontext Integration (ARCH-006)
- Uses `github.com/gogpu/gpucontext` v0.2.0
- DeviceProvider, EventSource interfaces
- IME support for CJK input

### Fixed

- **Dash patterns** with analytic AA (BUG-001, [#52](https://github.com/gogpu/gg/issues/52))

### Changed

- **Direct Matrix API** (FEAT-001, [#51](https://github.com/gogpu/gg/issues/51))
  - Added `Transform(m Matrix)` — apply transform
  - Added `SetTransform(m Matrix)` — replace transform
  - Added `GetTransform() Matrix` — get current transform

## [0.20.2] - 2026-01-26

### Fixed

- **Bezier curve smoothness** — Analytic anti-aliasing for smooth bezier rendering
  - Forward differencing edges for quadratic/cubic curves
  - Proper curve flattening with tight bounds computation
  - Anti-aliased strokes via stroke expansion
  - Fixes [#48](https://github.com/gogpu/gg/issues/48)

## [0.20.1] - 2026-01-24

### Changed

- **wgpu v0.10.2** — FFI build tag fix
  - Clear error message when CGO enabled: `undefined: GOFFI_REQUIRES_CGO_ENABLED_0`
  - See [wgpu v0.10.2 release](https://github.com/gogpu/wgpu/releases/tag/v0.10.2)

## [0.20.0] - 2026-01-22

### Added

#### GPU Backend Completion (Enterprise-Grade)

Complete GPU backend implementation following wgpu-rs, vello, and tiny-skia patterns.

##### Command Encoder (GPU-CMD-001)
- **CoreCommandEncoder** — State machine with deferred error handling
  - States: Recording → Locked → Finished → Consumed
  - Thread-safe with mutex protection
  - WebGPU-compliant 4-byte alignment validation
- **RenderPassEncoder** / **ComputePassEncoder** — Full pass recording
- **CommandBuffer** — Finished buffer for queue submission

##### Texture Management (GPU-TEX-001)
- **Texture** — Wraps hal.Texture with lazy default view
  - `GetDefaultView()` uses `sync.Once` for thread-safe creation
  - Automatic view dimension inference
- **TextureView** — Non-owning view with destroy tracking
- **CreateCoreTexture** / **CreateCoreTextureSimple** — Factory functions

##### Buffer Mapping (GPU-BUF-001)
- **Buffer** — Async mapping with state machine
  - States: Unmapped → Pending → Mapped
  - `MapAsync(mode, offset, size, callback)` — Non-blocking map request
  - `GetMappedRange(offset, size)` — Access mapped data
  - `Unmap()` — Release mapped memory
- **BufferMapAsyncStatus** — Success, ValidationError, etc.

##### Render/Compute Pass (GPU-PASS-001)
- **RenderPassEncoder** — Full WebGPU render pass API
  - SetPipeline, SetBindGroup, SetVertexBuffer, SetIndexBuffer
  - Draw, DrawIndexed, DrawIndirect
  - SetViewport, SetScissorRect, SetBlendConstant
  - PushDebugGroup, PopDebugGroup, InsertDebugMarker
- **ComputePassEncoder** — Compute dispatch
  - SetPipeline, SetBindGroup, DispatchWorkgroups

##### Pipeline Caching (GPU-PIP-001)
- **PipelineCacheCore** — FNV-1a descriptor hashing
  - Double-check locking pattern for thread safety
  - Atomic hit/miss statistics
  - `GetOrCreateRenderPipeline` / `GetOrCreateComputePipeline`
- Zero-allocation hash computation for descriptors

##### Stroke Expansion (GPU-STK-001)
- **internal/stroke** package — kurbo/tiny-skia algorithm
  - `StrokeExpander` — Converts stroked paths to filled outlines
  - Line caps: Butt, Round, Square (cubic Bezier arcs)
  - Line joins: Miter (with limit), Round, Bevel
  - Quadratic and cubic Bezier curve flattening
  - Adaptive tolerance-based subdivision

##### Glyph Run Builder (GPU-TXT-001)
- **GlyphRunBuilder** — Efficient glyph batching for GPU rendering
  - `AddGlyph`, `AddShapedGlyph`, `AddShapedRun`, `AddShapedGlyphs`
  - `Build(createGlyph)` — Generate draw commands
  - `BuildTransformed(createGlyph, transform)` — With user transform
- **GlyphRunBuilderPool** — sync.Pool for high-concurrency
- Float32 size bits conversion for exact key matching

##### Color Emoji Rendering (GG-EMOJI-001)
- **text/emoji** package enhancements
  - CBDT/CBLC bitmap extraction (Noto Color Emoji support)
  - COLR/CPAL color glyph support
- **CBDTExtractor** — Extract PNG bitmaps from CBDT tables
- Fixes [#45](https://github.com/gogpu/gg/issues/45) — Blank color emoji

### Changed

#### Type Consolidation (GPU-REF-001)
- **Removed HAL prefix** from all types for cleaner API
  - `HALCommandEncoder` → `CoreCommandEncoder`
  - `HALTexture` → `Texture`
  - `HALBuffer` → `Buffer`
  - `HALRenderPassEncoder` → `RenderPassEncoder`
  - `HALComputePassEncoder` → `ComputePassEncoder`
  - `HALPipelineCache` → `PipelineCacheCore`
- **File renames** (preserves git history)
  - `hal_texture.go` → `texture.go`
  - `hal_buffer.go` → `buffer.go`
  - `hal_render_pass.go` → `render_pass.go`
  - `hal_compute_pass.go` → `compute_pass.go`
  - `hal_pipeline_cache.go` → `pipeline_cache_core.go`

### Statistics
- **+8,700 LOC** across 20+ files
- **9 tasks completed** (8 features + 1 refactoring)
- **All tests pass** with comprehensive coverage
- **0 linter issues**

## [0.19.0] - 2026-01-22

### Added

#### Anti-Aliased Rendering (tiny-skia algorithm)

Professional-grade anti-aliasing using the tiny-skia algorithm (same as Chrome, Android, Flutter).

**4x Supersampling System**
- **SuperBlitter** — Coordinates 4x supersampling for sub-pixel accuracy
  - SUPERSAMPLE_SHIFT=2 (4x resolution)
  - Coverage accumulation across 4 scanlines
  - NonZero and EvenOdd fill rule support
- **AlphaRuns** — RLE-encoded alpha buffer for memory efficiency
  - O(spans) memory instead of O(width×height)
  - Efficient merge and accumulation
  - Zero-allocation hot path

**Rasterizer Integration**
- **FillAA** — Anti-aliased path filling in software renderer
- **FillPathAA** — Context-level AA fill method
- **Automatic fallback** — Graceful degradation when AA unavailable

### Fixed
- **Pixelated circles and curves** — Shapes now render with smooth edges ([#43](https://github.com/gogpu/gg/issues/43))
  - Root cause: `antiAlias` parameter was ignored in rasterizer
  - Fix: Implemented full AA pipeline with supersampling

### Statistics
- **~700 LOC added** across 5 files
- **100% backward compatible** — No breaking changes

## [0.18.1] - 2026-01-16

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.10.0 → v0.10.1
  - Non-blocking swapchain acquire (16ms timeout)
  - Window responsiveness fix during resize/drag
  - ErrNotReady for skip-frame handling

## [0.18.0] - 2026-01-15

### Added

#### Renderer Dependency Injection
- **Renderer Interface** — Pluggable renderer abstraction
  - `Fill(pixmap, path, paint)` — Fill path with paint
  - `Stroke(pixmap, path, paint)` — Stroke path with paint
- **SoftwareRenderer** — Default CPU-based implementation
  - `NewSoftwareRenderer(width, height)` — Create renderer
- **Functional Options** — Modern Go pattern for NewContext
  - `WithRenderer(r Renderer)` — Inject custom renderer
  - `WithPixmap(pm *Pixmap)` — Inject custom pixmap

#### Backend Refactoring
- **Renamed `backend/wgpu/` → `backend/native/`** — Pure Go WebGPU backend
- **Removed `backend/gogpu/`** — Unnecessary abstraction layer
- **Added `backend/rust/`** — wgpu-native FFI backend via go-webgpu/webgpu
- **Backend Constants** — `BackendNative`, `BackendRust`, `BackendSoftware`

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.9.3 → v0.10.0
  - HAL Backend Integration layer

### Example

```go
// Default software renderer
dc := gg.NewContext(800, 600)

// Custom renderer via dependency injection
customRenderer := NewCustomRenderer(800, 600)
dc := gg.NewContext(800, 600, gg.WithRenderer(customRenderer))

// Use gg's gpu GPU backend directly
import "github.com/gogpu/gg/backend/gpu"
// See backend/gpu/ for GPU-accelerated rendering
```

## [0.17.1] - 2026-01-10

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.9.2 → v0.9.3
  - Intel Vulkan compatibility: VkRenderPass, wgpu-style swapchain sync
  - Triangle rendering works on Intel Iris Xe Graphics
- Updated dependency: `github.com/gogpu/naga` v0.8.3 → v0.8.4
  - SPIR-V instruction ordering fix for Intel Vulkan

## [0.17.0] - 2026-01-05

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.9.0 → v0.9.2
  - v0.9.1: Vulkan vkDestroyDevice fix, features and limits mapping
  - v0.9.2: Metal NSString double-free fix on autorelease pool drain

## [0.16.0] - 2026-01-05

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.8.8 → v0.9.0
  - Core-HAL Bridge implementation
  - Snatchable pattern for safe resource destruction
  - TrackerIndex Allocator for state tracking
  - Buffer State Tracker for validation
  - 58 TODO comments replaced with proper documentation

### Removed
- **Deprecated tessellation code** — Removed unused `strips.go` and `tessellate.go` from wgpu backend
  - These were experimental triangle strip optimization code
  - Cleanup reduces backend/wgpu from ~2.5K to ~500 LOC

## [0.15.9] - 2026-01-04

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.8.7 → v0.8.8
  - Skip Metal tests on CI (Metal unavailable in virtualized macOS)
  - MSL `[[position]]` attribute fix via naga v0.8.3
- Updated dependency: `github.com/gogpu/naga` v0.8.2 → v0.8.3
  - Fixes MSL `[[position]]` attribute placement (now on struct member, not function)

## [0.15.8] - 2026-01-04

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.8.6 → v0.8.7
  - Metal ARM64 ObjC typed arguments
  - goffi v0.3.7 with improved ARM64 ABI support
- Updated dependency: `github.com/gogpu/naga` v0.8.1 → v0.8.2
  - MSL backend improvements for triangle shader compilation

## [0.15.7] - 2025-12-29

### Fixed
- **MultiFace and FilteredFace rendering** — `text.Draw()` now correctly renders text using composite Face types ([#34](https://github.com/gogpu/gg/issues/34))
  - Previously, `text.Draw()` silently failed when passed `MultiFace` or `FilteredFace`
  - Root cause: type assertion to `*sourceFace` returned early for composite faces
  - Fix: implemented type switch to handle all Face implementations

### Added
- **Regression tests for composite faces** — comprehensive tests for `MultiFace` and `FilteredFace` rendering
  - `TestDrawMultiFace` — verifies MultiFace renders correctly
  - `TestDrawFilteredFace` — verifies FilteredFace renders correctly
  - `TestDrawMultiFaceWithFilteredFaces` — tests nested composite faces
  - `TestMeasureMultiFace` and `TestMeasureFilteredFace` — measurement tests

## [0.15.6] - 2025-12-29

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.8.5 → v0.8.6
  - Metal double present fix
  - goffi v0.3.6 with ARM64 struct return fixes
  - Resolves macOS ARM64 blank window issue (gogpu/gogpu#24)

## [0.15.5] - 2025-12-29

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.8.4 → v0.8.5
  - DX12 backend now auto-registers on Windows
  - Windows backend priority: Vulkan → DX12 → GLES → Software

## [0.15.4] - 2025-12-29

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.8.1 → v0.8.4
  - Metal macOS blank window fix (Issue gogpu/gogpu#24)
  - Fixes missing `clamp()` WGSL built-in function (naga v0.8.1)

## [0.15.3] - 2025-12-29

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.7.2 → v0.8.1
  - DX12 backend complete
  - Intel GPU COM calling convention fix
- Updated dependency: `github.com/gogpu/naga` v0.6.0 → v0.8.0
  - HLSL backend for DirectX 11/12
  - All 4 shader backends stable

## [0.15.2] - 2025-12-26

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.7.1 → v0.7.2
  - Fixes Metal CommandEncoder state bug (wgpu Issue #24)
  - Metal backend properly tracks recording state via `cmdBuffer != 0`

## [0.15.1] - 2025-12-26

### Changed
- Updated dependency: `github.com/gogpu/wgpu` v0.6.0 → v0.7.1
  - Includes `ErrZeroArea` validation for zero-dimension surfaces
  - Fixes macOS timing issue when window initially has zero dimensions

## [0.15.0] - 2025-12-26

### Added

#### GPU Compute Shaders for Sparse Strips (Phase 6)

Implements vello-style GPU compute shader pipeline for high-performance 2D rasterization.

##### Phase 6.1: Fine Shader (GPU coverage)
- **GPUFineRasterizer** — GPU-accelerated fine rasterization
  - `gpu_fine.go` (752 LOC) — GPU rasterizer with CPU fallback
  - `shaders/fine.wgsl` (290 LOC) — WGSL compute shader
  - Per-pixel coverage calculation with analytic anti-aliasing
  - NonZero and EvenOdd fill rules support

##### Phase 6.2: Coarse Shader (tile binning)
- **GPUCoarseRasterizer** — GPU-accelerated tile binning
  - `gpu_coarse.go` (698 LOC) — GPU rasterizer with CPU fallback
  - `shaders/coarse.wgsl` (335 LOC) — WGSL compute shader with atomics
  - Efficient segment-to-tile mapping
  - Dynamic tile entry allocation

##### Phase 6.3: Flatten Shader (curves)
- **GPUFlattenRasterizer** — GPU-accelerated curve flattening
  - `gpu_flatten.go` (809 LOC) — GPU rasterizer with CPU fallback
  - `shaders/flatten.wgsl` (589 LOC) — Bezier flattening shader
  - Quadratic and cubic Bezier support
  - Affine transform integration

##### Phase 6.4: Full GPU/CPU Integration
- **HybridPipeline** — Unified GPU/CPU pipeline
  - `sparse_strips_gpu.go` (837 LOC) — Full pipeline integration
  - Automatic GPU/CPU selection based on workload
  - Per-stage threshold configuration
  - Comprehensive statistics tracking
  - `RasterizePath(path, transform, fillRule)` — Full pipeline execution

### Statistics
- **+6,470 LOC** across 15 files
- **3 WGSL compute shaders** (1,214 lines total)
- **6 new Go files** with comprehensive tests
- **87.6% coverage** maintained

## [0.14.0] - 2025-12-24

### Added

#### Alpha Mask System (TASK-118a)
- **Mask** — Alpha mask type for compositing operations
  - `NewMask(width, height)` — Create empty mask
  - `NewMaskFromAlpha(img)` — Create mask from image alpha channel
  - `At(x, y)`, `Set(x, y, value)` — Pixel access
  - `Fill(value)` — Fill entire mask with value
  - `Invert()` — Invert all mask values
  - `Clone()` — Create independent copy
  - `Width()`, `Height()`, `Bounds()` — Dimension queries
- **Context mask methods**
  - `SetMask(mask)` — Set current mask for drawing
  - `GetMask()` — Get current mask
  - `InvertMask()` — Invert current mask in-place
  - `ClearMask()` — Remove mask
  - `AsMask()` — Convert current drawing to mask
- **Push/Pop integration** — Mask state saved/restored with context stack

#### Fluent PathBuilder (TASK-118b)
- **PathBuilder** — Fluent API for path construction
  - `BuildPath()` — Start building a path
  - `MoveTo(x, y)`, `LineTo(x, y)` — Basic path commands
  - `QuadTo(cx, cy, x, y)` — Quadratic bezier
  - `CubicTo(c1x, c1y, c2x, c2y, x, y)` — Cubic bezier
  - `Close()` — Close current subpath
  - **13 shape methods:**
    - `Rect(x, y, w, h)` — Rectangle
    - `RoundRect(x, y, w, h, r)` — Rounded rectangle
    - `Circle(cx, cy, r)` — Circle
    - `Ellipse(cx, cy, rx, ry)` — Ellipse
    - `Arc(cx, cy, r, startAngle, endAngle)` — Arc
    - `Polygon(cx, cy, r, sides)` — Regular polygon
    - `Star(cx, cy, outerR, innerR, points)` — Star shape
    - `Line(x1, y1, x2, y2)` — Line segment
    - `Triangle(x1, y1, x2, y2, x3, y3)` — Triangle
    - `RegularPolygon(cx, cy, r, sides, rotation)` — Rotated polygon
    - `RoundedLine(x1, y1, x2, y2, width)` — Line with round caps
  - `Build()` — Return completed Path
- Method chaining for concise path construction

#### Resource Cleanup (TASK-118c)
- **Context.Close()** — Implements `io.Closer` interface
  - Clears all internal state (pixmap, path, font, mask, stacks)
  - Safe to call multiple times (idempotent)
  - Enables `defer ctx.Close()` pattern

#### Path Helpers (TASK-118d)
- **Context.GetCurrentPoint()** — Returns current path point and validity
- **Path.HasCurrentPoint()** — Check if path has a current point
- **Path.Clone()** — Create independent copy of path

#### Streaming I/O (TASK-118e)
- **Context.EncodePNG(w io.Writer)** — Encode to any writer
- **Context.EncodeJPEG(w io.Writer, quality)** — Encode JPEG to writer
- **Pixmap.EncodePNG(w io.Writer)** — Direct pixmap encoding
- **Pixmap.EncodeJPEG(w io.Writer, quality)** — Direct JPEG encoding

### Statistics

- **~800 LOC added** across 8 files
- **16 tests** for mask functionality
- **11 tests** for PathBuilder
- **0 linter issues**
- **Fully backward compatible** — No breaking changes

## [0.13.0] - 2025-12-24

### Added

#### Go 1.25+ Modernization

**Path Iterators (TASK-117c)**
- **Path.Elements()** — `iter.Seq[PathElement]` for path iteration
- **Path.ElementsWithCursor()** — `iter.Seq2[PathElement, Point]` with cursor position
- **PathElement** — Typed element with MoveTo, LineTo, QuadTo, CubicTo, Close
- **Zero-allocation** — 438 ns/op, 0 B/op benchmarks

**Generic Cache Package (TASK-117b)**
- **cache/** — New top-level package extracted from text/cache
- **Cache[K, V]** — Thread-safe LRU cache with soft limit eviction
- **ShardedCache[K, V]** — 16-shard cache for reduced lock contention
- **Hasher functions** — StringHasher, IntHasher, Uint64Hasher for shard selection
- **Atomic statistics** — Zero-allocation stat reads via atomic.Uint64
- **Performance** — GetHit: 23ns, Put: 34ns, 0 allocs/op

**Context Support (TASK-117a)**
- **scene/Renderer** — `RenderWithContext()`, `RenderDirtyWithContext()`
- **backend/wgpu** — `RenderSceneWithContext()`, `RenderToPixmapWithContext()`
- **text/Layout** — `LayoutTextWithContext()` with cancellation
- **Periodic checks** — Every 8 paragraphs, 32 tiles for responsive cancellation

**Unicode-Aware Text Wrapping (TASK-117d)**
- **WrapMode enum** — WrapWordChar (default), WrapNone, WrapWord, WrapChar
- **BreakClass** — UAX #14 simplified line breaking (Space, Zero, Open, Close, Hyphen, Ideographic)
- **WrapText()** — Wrap text to fit maxWidth with specified mode
- **MeasureText()** — Measure total advance width
- **LayoutOptions.WrapMode** — Integration with layout engine
- **CJK support** — Break opportunities at ideograph boundaries
- **Performance** — FindBreakOpportunities: 1,185 ns/op, ClassifyRune: 174 ns/op, 0 allocs

### Changed

- **DefaultLayoutOptions()** — WrapMode defaults to WrapWordChar for backward compatibility
- **text/cache.go** — Marked as deprecated in favor of cache/ package

### Statistics

- **~1,700 LOC added** across 15 files
- **87.6% test coverage** maintained
- **0 linter issues**
- **Fully backward compatible** — No breaking changes

## [0.12.0] - 2025-12-24

### Added

#### Brush Enum System (vello/peniko pattern)
- **Brush interface** — Sealed interface with `brushMarker()` for type safety
- **SolidBrush** — Single-color brush with `Solid()`, `SolidRGB()`, `SolidHex()`
- **CustomBrush** — Extensibility escape hatch for user-defined patterns
- **Pattern compatibility** — `BrushFromPattern()`, `PatternFromBrush()`

#### Gradient Types (tiny-skia/vello pattern)
- **LinearGradientBrush** — Linear gradient with start/end points
- **RadialGradientBrush** — Radial gradient with center, radius, optional focus
- **SweepGradientBrush** — Conic/sweep gradient with angle range
- **ExtendMode** — Pad, Repeat, Reflect for gradient extension
- **Linear sRGB interpolation** — Correct color blending

#### Stroke Struct (tiny-skia/kurbo pattern)
- **Stroke** — Unified stroke parameters (Width, Cap, Join, MiterLimit, Dash)
- **Dash** — Dash pattern support with offset
- **Fluent API** — `WithWidth()`, `WithCap()`, `WithJoin()`, `WithDash()`
- **Context integration** — `SetStroke()`, `GetStroke()`, `StrokeWithStyle()`

#### Error Handling (Go 1.13+ best practices)
- **text/errors.go** — `ErrEmptyFontData`, `ErrEmptyFaces`, `DirectionMismatchError`
- **text/msdf/errors.go** — `ErrAllocationFailed`, `ErrLengthMismatch`
- All errors support `errors.Is()` and `errors.As()`

### Statistics
- **4,337 LOC added** across 22 files
- **87.6% test coverage** maintained
- **0 linter issues**

## [0.11.0] - 2025-12-24

### Added

#### Glyph-as-Path Rendering (TASK-050b)
- **OutlineExtractor** — Extracts bezier outlines from fonts via sfnt
- **GlyphOutline** — Segments, Bounds, Advance, Clone/Scale/Translate/Transform
- **AffineTransform** — 2D affine matrix operations
- **GlyphRenderer** — Converts shaped glyphs to renderable outlines

#### Glyph Cache LRU (TASK-050c)
- **GlyphCache** — Thread-safe 16-shard LRU cache
- **OutlineCacheKey** — FontID, GlyphID, Size, Hinting
- **64-frame lifetime** — Automatic eviction via Maintain()
- **Cache hit: <50ns** — Zero-allocation hot path
- **GlyphCachePool** — Per-thread cache instances

#### MSDF Text Rendering (TASK-050f, 050g, 050h)
- **text/msdf package** — Pure Go MSDF generator
  - Edge detection: Linear, Quadratic, Cubic bezier
  - Edge coloring algorithm for corner preservation
  - Distance field computation with configurable range
  - MedianFilter and ErrorCorrection post-processing
- **AtlasManager** — Multi-atlas management with shelf packing
  - GridAllocator for uniform glyph cells
  - LRU eviction for large glyph sets
  - Dirty tracking for GPU upload
  - ConcurrentAtlasManager for high-throughput scenarios
- **WGSL Shader** — GPU text rendering
  - median3() for SDF reconstruction
  - Screen-space anti-aliasing via fwidth
  - Outline and shadow shader variants
- **TextPipeline** — GPU rendering integration
  - TextQuad/TextVertex for instanced rendering
  - TextRenderer combining pipeline with atlas

#### Emoji and Color Fonts (TASK-050i)
- **text/emoji package** — Full emoji support
  - IsEmoji, IsEmojiModifier, IsZWJ, IsRegionalIndicator
  - Segment() — Split text into emoji/non-emoji runs
  - Parse() — ZWJ sequence parsing (family, profession, etc.)
  - Flag sequences (regional indicators, subdivision tags)
  - Skin tone modifiers (U+1F3FB-U+1F3FF)
- **COLRv0/v1 support** — Color glyph parsing and rendering
- **sbix/CBDT support** — Bitmap emoji (PNG, JPEG, TIFF)

#### Subpixel Text Positioning (TASK-050j)
- **SubpixelMode** — None, Subpixel4, Subpixel10
- **Quantize()** — Fractional position to integer + subpixel
- **SubpixelCache** — Subpixel-aware glyph caching
- **~2ns overhead** — Zero-allocation quantization

### Statistics
- **16,200 LOC added** across 40+ files
- **87.6% test coverage** overall
- **0 linter issues**
- **4 new subpackages**: text/msdf, text/emoji, scene/text, backend/wgpu/text

## [0.10.1] - 2025-12-24

### Fixed
- **deps:** Update gogpu/wgpu to v0.6.0

### Changed
- **go.mod:** Clean up Go version (1.25.0 → 1.25)

## [0.10.0] - 2025-12-24

### Added

#### GPU Text Pipeline (text/)

**Pluggable Shaper Interface (TEXT-001)**
- **Shaper interface** — Converts text to positioned glyphs
  - Shape(text, face, size) → []ShapedGlyph
  - Pluggable architecture for custom shapers
- **BuiltinShaper** — Default implementation using golang.org/x/image
- **SetShaper/GetShaper** — Global shaper management (thread-safe)
- **ShapedGlyph** — GPU-ready glyph with GID, Cluster, X, Y, XAdvance, YAdvance

**Extended Shaping Types (TEXT-002)**
- **Direction** — LTR, RTL, TTB, BTT with IsHorizontal/IsVertical methods
- **GlyphType** — Simple, Ligature, Mark, Component classification
- **GlyphFlags** — Cluster boundaries, safe-to-break, whitespace markers
- **ShapedRun** — Sequence of glyphs with uniform style (direction, face, size)
  - Width(), Height(), LineHeight(), Bounds() methods

**Sharded LRU Shaping Cache (TEXT-003)**
- **ShapingCache** — Thread-safe 16-shard LRU cache
  - 1024 entries per shard (16K total)
  - FNV-64a hashing for even distribution
  - Get/Put with zero-allocation hot path
- **ShapingResult** — Cached shaped glyphs with metrics
- **93.7% test coverage**, 0 linter issues

**Bidi/Script Segmentation (TEXT-004)**
- **Script enum** — 25+ Unicode scripts (Latin, Arabic, Hebrew, Han, Cyrillic, etc.)
- **DetectScript(rune)** — Pure Go script detection from Unicode ranges
- **Segmenter interface** — Splits text into direction/script runs
- **BuiltinSegmenter** — Uses golang.org/x/text/unicode/bidi
  - Correct rune-based indexing (not byte indices)
  - Script inheritance for Common/Inherited characters
  - Numbers in RTL text: inherit script, keep LTR direction
- **Segment** — Text run with Direction, Script, Level

**Multi-line Layout Engine (TEXT-005)**
- **Alignment** — Left, Center, Right, Justify (placeholder)
- **LayoutOptions** — MaxWidth, LineSpacing, Alignment, Direction
- **Line** — Positioned line with runs, glyphs, width, ascent, descent, Y
- **Layout** — Complete layout result with lines, total width/height
- **LayoutText(text, face, size, opts)** — Full layout with options
- **LayoutTextSimple(text, face, size)** — Convenience wrapper
- **Features:**
  - Hard line break handling (\\n, \\r\\n, \\r)
  - Bidi-aware paragraph segmentation
  - Greedy line wrapping at word boundaries
  - CJK character break opportunities
  - Proper alignment with container width

### Statistics
- **5 major features** implemented (TEXT-001 through TEXT-005)
- **~2,500 LOC added** across 12 files
- **87.0% text package coverage** (93.7% cache package)
- **0 linter issues**
- **Zero new dependencies** — Uses existing golang.org/x/text

### Architecture

**GPU Text Pipeline**
```
Text → Segmenter → Shaper → Layout → GPU Renderer
         │           │        │
    Bidi/Script    Cache    Lines
```

Key design decisions:
- Pluggable Shaper allows future go-text/typesetting integration
- Sharded cache prevents lock contention
- Bidi segmentation uses Unicode standard via golang.org/x/text
- Layout engine ready for GPU rendering pipeline

## [0.9.2] - 2025-12-19

### Fixed
- **Raster winding direction** — Compute edge direction before point swap ([#15](https://github.com/gogpu/gg/pull/15))
  - Non-zero winding rule was broken because direction was computed AFTER swapping points
  - Direction must be determined from original point order before normalizing edges
  - Thanks to @cmaglie for reporting and testing

## [0.9.1] - 2025-12-19

### Fixed
- **Text rendering blank images** — Text was drawn to a copy of the pixmap instead of the actual pixmap ([#11](https://github.com/gogpu/gg/issues/11), [#12](https://github.com/gogpu/gg/pull/12))
  - Added `Set()` method to `Pixmap` to implement `draw.Image` interface
  - Added `TestTextDrawsPixels` regression test

## [0.9.0] - 2025-12-18

### Added

#### GPU Backend (backend/wgpu/)

**WGPUBackend Core**
- **WGPUBackend** — GPU-accelerated rendering backend implementing RenderBackend interface
  - Init()/Close() — GPU lifecycle management
  - NewRenderer() — Create GPU-backed immediate mode renderer
  - RenderScene() — Retained mode scene rendering via GPUSceneRenderer
- **Auto-registration** — Registered on package import with priority over software
- **GPUInfo** — GPU vendor, device name, driver info

**GPU Memory Management (memory.go)**
- **MemoryManager** — GPU resource lifecycle with LRU eviction
  - 256MB default budget (configurable 16MB-8GB)
  - Thread-safe with sync.RWMutex
  - Automatic eviction on memory pressure
- **GPUTexture** — Texture wrapper with usage tracking
- **GPUBuffer** — Buffer wrapper for vertex/uniform data
- **TextureAtlas** — Shelf-packing atlas for small textures
  - 2048x2048 default size
  - Region allocation with padding
- **RectAllocator** — Guillotine algorithm for atlas packing

**Strip Tessellation (tessellate.go)**
- **Tessellator** — Converts paths to GPU-ready sparse strips
  - Active Edge Table algorithm
  - EvenOdd and NonZero fill rules
  - Sub-pixel anti-aliasing via coverage
- **StripBuffer** — GPU buffer for strip data
- **Strip** — Single scanline coverage span (y, x1, x2, coverage)
- Handles all path operations: MoveTo, LineTo, QuadTo, CubicTo, Close

**WGSL Shaders (shaders/)**
- **blit.wgsl** (43 LOC) — Simple texture copy to screen
- **blend.wgsl** (424 LOC) — All 29 blend modes
  - 14 Porter-Duff: Clear, Src, Dst, SrcOver, DstOver, SrcIn, DstIn, SrcOut, DstOut, SrcAtop, DstAtop, Xor, Plus, Modulate
  - 11 Advanced: Multiply, Screen, Overlay, Darken, Lighten, ColorDodge, ColorBurn, HardLight, SoftLight, Difference, Exclusion
  - 4 HSL: Hue, Saturation, Color, Luminosity
- **strip.wgsl** (155 LOC) — Compute shader for strip rasterization
  - Workgroup size 64
  - Coverage-based anti-aliasing
- **composite.wgsl** (235 LOC) — Layer compositing with blend modes

**Render Pipeline (pipeline.go)**
- **PipelineCache** — Caches compiled render/compute pipelines
- **GPUPipelineConfig** — Pipeline configuration descriptors
- **ShaderLoader** — Loads and compiles WGSL shaders

**GPU Scene Renderer (renderer.go)**
- **GPUSceneRenderer** — Complete scene rendering on GPU
  - Scene traversal and command encoding
  - Layer stack management
  - Strip tessellation and rasterization
  - Blend mode compositing
- **GPUSceneRendererConfig** — Width, height, debug options

**Command Encoding (commands.go)**
- **CommandEncoder** — WebGPU command buffer building
- **RenderPass** — Render pass commands (draw, bind, viewport)
- **ComputePass** — Compute shader dispatch

### Architecture

**Sparse Strips Algorithm (vello 2025 pattern)**
```
Path → CPU Tessellation → Strips → GPU Rasterization → Compositing → Output
         (tessellate.go)    ↓         (strip.wgsl)      (composite.wgsl)
                       StripBuffer
```

Key benefits:
- CPU handles complex path math (curves, intersections)
- GPU handles parallel pixel processing
- Minimal CPU→GPU data transfer (strips are compact)
- Compatible with all existing gg features

### Statistics
- **9,930 LOC added** across 21 files
- **4 WGSL shaders** (857 LOC total)
- **29 blend modes** supported on GPU
- **All tests pass** (build + unit + integration)
- **0 linter issues**

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

[0.30.0]: https://github.com/gogpu/gg/compare/v0.29.5...v0.30.0
[0.29.5]: https://github.com/gogpu/gg/compare/v0.29.4...v0.29.5
[0.29.4]: https://github.com/gogpu/gg/compare/v0.29.3...v0.29.4
[0.29.3]: https://github.com/gogpu/gg/compare/v0.29.2...v0.29.3
[0.29.2]: https://github.com/gogpu/gg/compare/v0.29.1...v0.29.2
[0.29.1]: https://github.com/gogpu/gg/compare/v0.29.0...v0.29.1
[0.29.0]: https://github.com/gogpu/gg/compare/v0.28.6...v0.29.0
[0.28.6]: https://github.com/gogpu/gg/compare/v0.28.5...v0.28.6
[0.28.5]: https://github.com/gogpu/gg/compare/v0.28.4...v0.28.5
[0.28.4]: https://github.com/gogpu/gg/compare/v0.28.3...v0.28.4
[0.28.3]: https://github.com/gogpu/gg/compare/v0.28.2...v0.28.3
[0.28.2]: https://github.com/gogpu/gg/compare/v0.28.1...v0.28.2
[0.28.1]: https://github.com/gogpu/gg/compare/v0.28.0...v0.28.1
[0.28.0]: https://github.com/gogpu/gg/compare/v0.27.1...v0.28.0
[0.27.1]: https://github.com/gogpu/gg/compare/v0.27.0...v0.27.1
[0.27.0]: https://github.com/gogpu/gg/compare/v0.26.1...v0.27.0
[0.26.1]: https://github.com/gogpu/gg/compare/v0.26.0...v0.26.1
[0.26.0]: https://github.com/gogpu/gg/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/gogpu/gg/compare/v0.24.1...v0.25.0
[0.24.1]: https://github.com/gogpu/gg/compare/v0.24.0...v0.24.1
[0.24.0]: https://github.com/gogpu/gg/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/gogpu/gg/compare/v0.22.3...v0.23.0
[0.22.3]: https://github.com/gogpu/gg/compare/v0.22.2...v0.22.3
[0.22.2]: https://github.com/gogpu/gg/compare/v0.22.1...v0.22.2
[0.22.1]: https://github.com/gogpu/gg/compare/v0.22.0...v0.22.1
[0.22.0]: https://github.com/gogpu/gg/compare/v0.21.4...v0.22.0
[0.21.4]: https://github.com/gogpu/gg/compare/v0.21.3...v0.21.4
[0.21.3]: https://github.com/gogpu/gg/compare/v0.21.2...v0.21.3
[0.21.2]: https://github.com/gogpu/gg/compare/v0.21.1...v0.21.2
[0.21.1]: https://github.com/gogpu/gg/compare/v0.21.0...v0.21.1
[0.21.0]: https://github.com/gogpu/gg/compare/v0.20.1...v0.21.0
[0.20.1]: https://github.com/gogpu/gg/compare/v0.20.0...v0.20.1
[0.20.0]: https://github.com/gogpu/gg/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/gogpu/gg/compare/v0.18.1...v0.19.0
[0.18.1]: https://github.com/gogpu/gg/compare/v0.18.0...v0.18.1
[0.18.0]: https://github.com/gogpu/gg/compare/v0.17.1...v0.18.0
[0.17.1]: https://github.com/gogpu/gg/compare/v0.17.0...v0.17.1
[0.17.0]: https://github.com/gogpu/gg/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/gogpu/gg/compare/v0.15.9...v0.16.0
[0.15.9]: https://github.com/gogpu/gg/compare/v0.15.8...v0.15.9
[0.15.8]: https://github.com/gogpu/gg/compare/v0.15.7...v0.15.8
[0.15.7]: https://github.com/gogpu/gg/compare/v0.15.6...v0.15.7
[0.15.6]: https://github.com/gogpu/gg/compare/v0.15.5...v0.15.6
[0.15.5]: https://github.com/gogpu/gg/compare/v0.15.4...v0.15.5
[0.15.4]: https://github.com/gogpu/gg/compare/v0.15.3...v0.15.4
[0.15.3]: https://github.com/gogpu/gg/compare/v0.15.2...v0.15.3
[0.15.2]: https://github.com/gogpu/gg/compare/v0.15.1...v0.15.2
[0.15.1]: https://github.com/gogpu/gg/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/gogpu/gg/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/gogpu/gg/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/gogpu/gg/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/gogpu/gg/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/gogpu/gg/compare/v0.10.1...v0.11.0
[0.10.1]: https://github.com/gogpu/gg/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/gogpu/gg/compare/v0.9.2...v0.10.0
[0.9.2]: https://github.com/gogpu/gg/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/gogpu/gg/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/gogpu/gg/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/gogpu/gg/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/gogpu/gg/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/gogpu/gg/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/gogpu/gg/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/gogpu/gg/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/gogpu/gg/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gogpu/gg/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gogpu/gg/releases/tag/v0.1.0
