# text — GPU Text Pipeline for gg

**Status:** v0.14.0 (Released as part of gg v0.31.0)

This package implements a modern GPU-ready text pipeline for gogpu/gg, inspired by Ebitengine text/v2, go-text/typesetting, and vello.

## Architecture

```
FontSource (heavyweight, shared)
    ↓
Face (lightweight, per-size)
    ↓
Segmenter → Shaper → Layout → GlyphRenderer
    │           │        │          │
Bidi/Script  Cache    Lines    ┌────┴────┐
                               │         │
                          Vector Path  MSDF
                          (quality)   (perf)
                               │         │
                               └────┬────┘
                                    ↓
                              GPU Rendering
```

## Features

### Pluggable Shaper (v0.10.0)
- **Shaper interface** — Converts text to positioned glyphs
- **BuiltinShaper** — Default using golang.org/x/image
- **GoTextShaper** — HarfBuzz-level shaping via go-text/typesetting (opt-in)
- **Custom shapers** — Plug in any implementation via SetShaper()

### Bidi/Script Segmentation (v0.10.0)
- **25+ Unicode scripts** — Latin, Arabic, Hebrew, Han, Cyrillic, Thai, etc.
- **Full Unicode Bidi Algorithm** — Via golang.org/x/text/unicode/bidi
- **Script inheritance** — Common/Inherited characters resolved from context

### Multi-line Layout (v0.10.0+)
- **Alignment** — Left, Center, Right, Justify (placeholder)
- **Line wrapping** — At MaxWidth with word boundaries
- **Line spacing** — Configurable multiplier
- **Bidi-aware** — Proper RTL/LTR segment ordering

### Unicode Text Wrapping (v0.13.0)
- **WrapMode enum** — WrapWordChar (default), WrapNone, WrapWord, WrapChar
- **UAX #14 simplified** — Break classification (Space, Zero, Open, Close, Hyphen, Ideographic)
- **CJK support** — Break opportunities at ideograph boundaries
- **Performance** — 1,185 ns/op FindBreakOpportunities, 0 allocs

### Shaping Cache (v0.10.0)
- **16-shard LRU** — Concurrent access without lock contention
- **4K total entries** — 256 per shard
- **Zero-allocation hot path** — Pre-allocated result storage

## Usage

### Basic Text Drawing

```go
// Load font (heavyweight, do once)
source, err := text.NewFontSourceFromFile("Roboto-Regular.ttf")
if err != nil {
    log.Fatal(err)
}
defer source.Close()

// Create face at specific size (lightweight)
face := source.Face(24)

// Use with gg.Context (dc = drawing context)
dc := gg.NewContext(800, 600)
dc.SetFont(face)
dc.DrawString("Hello, GoGPU!", 100, 100)
```

### Text Shaping

```go
// Shape text to positioned glyphs
glyphs := text.Shape("Hello", face)
for _, g := range glyphs {
    fmt.Printf("GID=%d X=%.1f Y=%.1f\n", g.GID, g.X, g.Y)
}
```

### Bidi/Script Segmentation

```go
// Segment mixed-direction text
segments := text.SegmentText("Hello שלום مرحبا")
for _, seg := range segments {
    fmt.Printf("'%s' Dir=%s Script=%s\n",
        seg.Text, seg.Direction, seg.Script)
}

// RTL base direction
segments = text.SegmentTextRTL("مرحبا Hello")
```

### Multi-line Layout

```go
// Layout with options
opts := text.LayoutOptions{
    MaxWidth:    400,
    LineSpacing: 1.2,
    Alignment:   text.AlignCenter,
    Direction:   text.DirectionLTR,
    WrapMode:    text.WrapWordChar, // Word-first, char fallback (default)
}
layout := text.LayoutText(longText, face, opts)

// Access lines
for _, line := range layout.Lines {
    fmt.Printf("Y=%.1f Width=%.1f Glyphs=%d\n",
        line.Y, line.Width, len(line.Glyphs))
}

// Simple layout (no wrapping)
layout = text.LayoutTextSimple("Hello\nWorld", face)
```

### Text Wrapping Modes

```go
// WrapWordChar (default) — Word boundaries first, character fallback for long words
opts := text.LayoutOptions{
    MaxWidth: 200,
    WrapMode: text.WrapWordChar,
}

// WrapWord — Word boundaries only, long words overflow
opts.WrapMode = text.WrapWord

// WrapChar — Character boundaries, any character can break
opts.WrapMode = text.WrapChar

// WrapNone — No wrapping, text may exceed MaxWidth
opts.WrapMode = text.WrapNone

// Standalone wrapping API
results := text.WrapText("Hello World", face, 100, text.WrapWordChar)
for _, r := range results {
    fmt.Printf("Line: '%s' (%d-%d)\n", r.Text, r.Start, r.End)
}

// Measure text width
width := text.MeasureText("Hello World", face)
```

### GoTextShaper (HarfBuzz-level shaping)

```go
// Enable HarfBuzz-level shaping for ligatures, kerning, and complex scripts
shaper := text.NewGoTextShaper()
text.SetShaper(shaper)
defer text.SetShaper(nil) // Reset to BuiltinShaper

// Shape text — now uses go-text/typesetting HarfBuzz engine
glyphs := text.Shape("Hello", face)
```

GoTextShaper is safe for concurrent use and caches parsed font data internally.

### Custom Shaper

```go
// Implement custom shaper
type MyShaper struct {
    // ...
}

func (s *MyShaper) Shape(text string, face text.Face) []text.ShapedGlyph {
    // Custom shaping logic
}

// Set as global shaper
text.SetShaper(&MyShaper{})
defer text.SetShaper(nil) // Reset to default
```

## Types

### ShapedGlyph
```go
type ShapedGlyph struct {
    GID      GlyphID  // Glyph index in font
    Cluster  int      // Source character index
    X, Y     float64  // Position relative to origin
    XAdvance float64  // Horizontal advance
    YAdvance float64  // Vertical advance (for TTB)
}
```

### Segment
```go
type Segment struct {
    Text      string    // Segment text
    Start     int       // Byte offset in original text
    End       int       // End byte offset
    Direction Direction // LTR or RTL
    Script    Script    // Unicode script
    Level     int       // Bidi embedding level
}
```

### Layout
```go
type Layout struct {
    Lines  []Line   // Positioned lines
    Width  float64  // Maximum line width
    Height float64  // Total height
}

type Line struct {
    Runs    []ShapedRun   // Runs with uniform style
    Glyphs  []ShapedGlyph // All positioned glyphs
    Width   float64       // Line width
    Ascent  float64       // Max ascent
    Descent float64       // Max descent
    Y       float64       // Baseline Y position
}
```

## Dependencies

- `golang.org/x/image/font/opentype` — TTF/OTF parsing
- `golang.org/x/text/unicode/bidi` — Unicode Bidirectional Algorithm
- `github.com/go-text/typesetting` — HarfBuzz shaping engine (used by GoTextShaper)

## Subpackages

### text/cache
LRU caching infrastructure for shaping results.

### text/msdf (v0.11.0)
Multi-channel Signed Distance Field generation for GPU text rendering.

- **Generator** — Pure Go MSDF with edge coloring algorithm
- **AtlasManager** — Multi-atlas management with shelf packing
- **ConcurrentAtlasManager** — High-throughput sharded variant

### text/emoji (v0.11.0)
Emoji and color font support.

- **Detection** — IsEmoji, IsZWJ, IsRegionalIndicator
- **Sequences** — ZWJ, flags, skin tones, keycaps
- **COLRv0/v1** — Color glyph parsing and rendering
- **sbix/CBDT** — Bitmap emoji (PNG, JPEG, TIFF)

## Test Coverage

- **text package**: 87.6%
- **text/cache package**: 93.7%
- **text/msdf package**: 87.0%
- **text/emoji package**: 85.0%
- **0 linter issues**

## Versions

### v0.14.0 (Current — part of gg v0.31.0)
- [x] **BREAKING: Removed `size` parameter** — `Shape()`, `LayoutText()`, `WrapText()`, `MeasureText()` now derive size from `face.Size()`
- [x] **Shaper interface simplified** — `Shape(text, face)` instead of `Shape(text, face, size)`
- [x] **LayoutText Y positions fixed** — Lines now have correct cumulative Y positions
- [x] **Hard line breaks in WrapText** — `\n`, `\r\n`, `\r` respected in wrapping

### v0.13.0
- [x] **WrapMode enum** — WrapWordChar, WrapNone, WrapWord, WrapChar
- [x] **BreakClass** — UAX #14 simplified line breaking
- [x] **WrapText()** — Standalone wrapping API
- [x] **MeasureText()** — Measure text advance width
- [x] **CJK support** — Ideograph break opportunities
- [x] **context.Context** — LayoutTextWithContext() cancellation

### v0.11.0
- [x] Glyph-as-Path rendering (OutlineExtractor)
- [x] GlyphCache LRU (16-shard, <50ns hit)
- [x] MSDF generator (Pure Go)
- [x] MSDF atlas with shelf packing
- [x] Emoji support (COLRv1, sbix, ZWJ)
- [x] Subpixel positioning (4/10 levels)

### v0.10.0
- [x] Pluggable Shaper interface
- [x] Extended shaping types
- [x] Sharded LRU shaping cache
- [x] Bidi/Script segmentation
- [x] Multi-line Layout Engine
