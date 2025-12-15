# text — Text Rendering for gg

**Status:** In Development (v0.2.0+)

This package implements modern text rendering for gogpu/gg, inspired by Ebitengine text/v2.

## Architecture

```
FontSource (heavyweight, shared)
    ↓
Face (lightweight, per-size)
    ↓
Shaper → Layout → Rasterizer → Draw
```

## Current Implementation (TASK-042)

✅ **Completed:**
- `FontSource` — TTF/OTF loading and management
- Copy protection (Ebitengine pattern)
- Options pattern (SourceOption, FaceOption)
- Basic types (Direction, Hinting, Rect)
- Face interface (stub)
- Comprehensive tests

🚧 **In Progress:**
- Face implementation (TASK-043)
- Multi-level caching (TASK-044)
- Text layout (TASK-045)

## Usage

```go
// Load font (heavyweight, do once)
source, err := text.NewFontSourceFromFile("Roboto-Regular.ttf")
if err != nil {
    log.Fatal(err)
}
defer source.Close()

// Create face at specific size (lightweight)
face := source.Face(24)

// Use with gg.Context (when implemented)
ctx := gg.NewContext(800, 600)
ctx.SetFont(face)
ctx.DrawString("Hello, GoGPU!", 100, 100)
```

## Design

See: `docs/dev/research/gg-text-api-v2-design.md` for full design document.

## Dependencies

- `golang.org/x/image/font/opentype` — TTF/OTF parsing
- `golang.org/x/text` — (transitive) Character encoding

Future goal: Pure Go implementation with zero dependencies.
