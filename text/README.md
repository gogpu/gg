# text — GPU Text Pipeline for gg

**Status:** v0.10.0 (Released)

This package implements a modern GPU-ready text pipeline for gogpu/gg, inspired by Ebitengine text/v2 and go-text/typesetting.

## Architecture

```
FontSource (heavyweight, shared)
    ↓
Face (lightweight, per-size)
    ↓
Segmenter → Shaper → Layout → GPU Renderer
    │           │        │
Bidi/Script  Cache    Lines
```

## Features

### Pluggable Shaper (v0.10.0)
- **Shaper interface** — Converts text to positioned glyphs
- **BuiltinShaper** — Default using golang.org/x/image
- **Custom shapers** — Plug in go-text/typesetting or HarfBuzz

### Bidi/Script Segmentation (v0.10.0)
- **25+ Unicode scripts** — Latin, Arabic, Hebrew, Han, Cyrillic, Thai, etc.
- **Full Unicode Bidi Algorithm** — Via golang.org/x/text/unicode/bidi
- **Script inheritance** — Common/Inherited characters resolved from context

### Multi-line Layout (v0.10.0)
- **Alignment** — Left, Center, Right, Justify (placeholder)
- **Line wrapping** — At MaxWidth with word boundaries
- **Line spacing** — Configurable multiplier
- **Bidi-aware** — Proper RTL/LTR segment ordering

### Shaping Cache (v0.10.0)
- **16-shard LRU** — Concurrent access without lock contention
- **16K total entries** — 1024 per shard
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

// Use with gg.Context
ctx := gg.NewContext(800, 600)
ctx.SetFont(face)
ctx.DrawString("Hello, GoGPU!", 100, 100)
```

### Text Shaping

```go
// Shape text to positioned glyphs
glyphs := text.Shape("Hello", face, 24)
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
}
layout := text.LayoutText(longText, face, 16, opts)

// Access lines
for _, line := range layout.Lines {
    fmt.Printf("Y=%.1f Width=%.1f Glyphs=%d\n",
        line.Y, line.Width, len(line.Glyphs))
}

// Simple layout (no wrapping)
layout = text.LayoutTextSimple("Hello\nWorld", face, 16)
```

### Custom Shaper

```go
// Implement custom shaper (e.g., go-text/typesetting)
type MyShaper struct {
    // ...
}

func (s *MyShaper) Shape(text string, face text.Face, size float64) []text.ShapedGlyph {
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

## Test Coverage

- **text package**: 87.0%
- **text/cache package**: 93.7%
- **0 linter issues**

## Roadmap

### v0.10.0 (Current)
- [x] Pluggable Shaper interface
- [x] Extended shaping types
- [x] Sharded LRU shaping cache
- [x] Bidi/Script segmentation
- [x] Multi-line Layout Engine

### v0.11.0 (Planned)
- [ ] go-text/typesetting integration
- [ ] Glyph-as-Path rendering
- [ ] MSDF atlas for GPU
- [ ] Emoji support (COLRv1)
