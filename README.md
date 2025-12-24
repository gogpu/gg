<h1 align="center">gg</h1>

<p align="center">
  <strong>Enterprise-Grade 2D Graphics Library for Go</strong><br>
  Pure Go | GPU Accelerated | Rust-Inspired Architecture
</p>

<p align="center">
  <a href="https://github.com/gogpu/gg/actions/workflows/ci.yml"><img src="https://github.com/gogpu/gg/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/gogpu/gg"><img src="https://codecov.io/gh/gogpu/gg/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="https://pkg.go.dev/github.com/gogpu/gg"><img src="https://pkg.go.dev/badge/github.com/gogpu/gg.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/gogpu/gg"><img src="https://goreportcard.com/badge/github.com/gogpu/gg" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/gogpu/gg"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go Version"></a>
</p>

<p align="center">
  <strong>47,000+ LOC</strong> | <strong>87.6% Coverage</strong> | <strong>0 Linter Issues</strong>
</p>

---

## Why gg?

**gg** is designed to become the **reference 2D graphics library** for Go — capable of powering:

- **IDEs** (GoLand, VS Code level)
- **Browsers** (Chrome rendering quality)
- **Professional graphics applications**

### Architecture Inspired by Best-in-Class Rust Libraries

| Feature | Inspiration | Status |
|---------|-------------|--------|
| **Dual-Stream Encoding** | [vello](https://github.com/linebender/vello) | Implemented |
| **Sparse Strips GPU** | vello 2025 | Implemented |
| **29 Blend Modes** | vello, W3C | Implemented |
| **Layer Compositing** | Skia, vello | Implemented |
| **MSDF Text** | Industry standard | Implemented |
| **Brush/Pattern System** | [tiny-skia](https://github.com/nicotine-scx/tiny-skia), [peniko](https://github.com/linebender/peniko) | Implemented |

---

## Installation

```bash
go get github.com/gogpu/gg
```

**Requirements:** Go 1.25+

---

## Quick Start

```go
package main

import (
    "github.com/gogpu/gg"
    "github.com/gogpu/gg/text"
)

func main() {
    // Create a 512x512 context
    ctx := gg.NewContext(512, 512)
    ctx.ClearWithColor(gg.White)

    // Draw shapes
    ctx.SetHexColor("#3498db")
    ctx.DrawCircle(256, 256, 100)
    ctx.Fill()

    // Load font and draw text
    source, _ := text.NewFontSourceFromFile("arial.ttf")
    defer source.Close()

    ctx.SetFont(source.Face(32))
    ctx.SetColor(gg.Black)
    ctx.DrawString("Hello, GoGPU!", 180, 260)

    ctx.SavePNG("output.png")
}
```

---

## Features

### Core Graphics (v0.1.0+)
- **Simple API** — Immediate-mode drawing similar to HTML Canvas
- **Pure Go** — No CGO, cross-platform, single binary
- **Rich Shapes** — Rectangles, circles, ellipses, arcs, Bezier curves
- **Transformations** — Translate, rotate, scale with matrix stack

### Text Rendering (v0.2.0+)
- **TrueType Fonts** — Full TTF support
- **Font Composition** — MultiFace for fallback chains
- **Unicode Support** — FilteredFace for emoji and special ranges
- **Zero-Allocation Iterators** — Go 1.25+ `iter.Seq[Glyph]`

### GPU Text Pipeline (v0.10.0+)
- **Pluggable Shaper** — BuiltinShaper or custom HarfBuzz-compatible
- **Bidi/Script Segmentation** — Full Unicode Bidirectional Algorithm
- **25+ Scripts** — Latin, Arabic, Hebrew, Han, Devanagari, Thai, etc.
- **MSDF Text Rendering** — Sharp text at any scale (v0.11.0)
- **Emoji Support** — COLRv1, sbix, ZWJ sequences (v0.11.0)
- **Unicode Text Wrapping** — WrapWord, WrapChar, WrapWordChar modes (v0.13.0)

### Images & Clipping (v0.3.0+)
- **7 Pixel Formats** — Gray8, Gray16, RGB8, RGBA8, RGBAPremul, BGRA8, BGRAPremul
- **Interpolation** — Nearest, Bilinear, Bicubic sampling
- **Clipping** — Path-based and rectangular clipping
- **Mipmaps** — Automatic mipmap chain generation

### Compositing (v0.3.0+)
- **Porter-Duff** — 14 blend modes (SrcOver, DstIn, Xor, etc.)
- **Advanced Blends** — Screen, Overlay, ColorDodge, etc.
- **HSL Blend Modes** — Hue, Saturation, Color, Luminosity (W3C spec)

### Scene Graph (v0.7.0+)
- **Dual-Stream Encoding** — GPU-ready command buffer (vello pattern)
- **13 Shape Types** — Rect, Circle, Ellipse, Polygon, Star, etc.
- **Filter Effects** — Gaussian blur, drop shadow, color matrix
- **LRU Layer Cache** — 64MB default, 90ns Get / 393ns Put

### Go 1.25+ Features (v0.13.0+)
- **Path Iterators** — `iter.Seq[PathElement]`, zero-allocation (438 ns/op)
- **Generic Cache** — `Cache[K,V]` and `ShardedCache[K,V]` in `cache/` package
- **Context Support** — Cancellable rendering via `context.Context`
- **Unicode Wrapping** — UAX #14 simplified line breaking with CJK support

### GPU Acceleration (v0.9.0+)
- **WGPUBackend** — Hardware acceleration via gogpu/wgpu
- **Sparse Strips Algorithm** — CPU tessellates, GPU rasterizes (vello pattern)
- **WGSL Shaders** — blit, blend, strip, composite
- **GPU Memory Management** — LRU eviction, 256MB+ budget

---

## Examples

### Drawing Shapes

```go
ctx := gg.NewContext(400, 400)
ctx.ClearWithColor(gg.White)

// Rectangle
ctx.SetRGB(0.2, 0.5, 0.8)
ctx.DrawRectangle(50, 50, 100, 80)
ctx.Fill()

// Circle with stroke
ctx.SetRGB(0.8, 0.2, 0.2)
ctx.SetLineWidth(3)
ctx.DrawCircle(250, 150, 60)
ctx.Stroke()

// Rounded rectangle
ctx.SetRGBA(0.2, 0.8, 0.2, 0.7)
ctx.DrawRoundedRectangle(150, 250, 120, 80, 15)
ctx.Fill()

ctx.SavePNG("shapes.png")
```

### Text with Fallback

```go
// Load fonts
mainFont, _ := text.NewFontSourceFromFile("Roboto.ttf")
emojiFont, _ := text.NewFontSourceFromFile("NotoEmoji.ttf")
defer mainFont.Close()
defer emojiFont.Close()

// Create fallback chain
multiFace, _ := text.NewMultiFace(
    mainFont.Face(24),
    text.NewFilteredFace(emojiFont.Face(24), text.RangeEmoji),
)

ctx.SetFont(multiFace)
ctx.SetColor(gg.Black)
ctx.DrawString("Hello! Nice to meet you!", 50, 100)
```

### Layer Compositing

```go
ctx := gg.NewContext(400, 400)
ctx.ClearWithColor(gg.White)

// Draw background
ctx.SetRGB(0.9, 0.9, 0.9)
ctx.DrawRectangle(0, 0, 400, 400)
ctx.Fill()

// Create layer with blend mode
ctx.PushLayer(gg.BlendMultiply, 0.8)

ctx.SetRGB(1, 0, 0)
ctx.DrawCircle(150, 200, 100)
ctx.Fill()

ctx.SetRGB(0, 0, 1)
ctx.DrawCircle(250, 200, 100)
ctx.Fill()

ctx.PopLayer()

ctx.SavePNG("layers.png")
```

---

## Performance

| Operation | Time | Notes |
|-----------|------|-------|
| sRGB → Linear | 0.16ns | 260x faster than math.Pow |
| LayerCache.Get | 90ns | Thread-safe LRU |
| DirtyRegion.Mark | 10.9ns | Lock-free atomic |
| MSDF lookup | <10ns | Atomic + HashMap |
| Cache hit | <30ns | Lock-free read path |

---

## Roadmap

| Version | Focus | Status |
|---------|-------|--------|
| v0.1.0 - v0.13.0 | Core features | Released |
| v0.14.0 | Advanced Features (Masks, PathBuilder) | Planned |
| v0.15.0 | Documentation & RC | Planned |
| v1.0.0 | Production Release | Target |

See [ROADMAP.md](ROADMAP.md) for detailed plans.

---

## Part of GoGPU Ecosystem

**gogpu** is a Pure Go GPU Computing Ecosystem — professional graphics libraries for Go.

| Component | Description | Version |
|-----------|-------------|---------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | GPU framework | v0.7.0 |
| [gogpu/wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU | v0.6.0 |
| [gogpu/naga](https://github.com/gogpu/naga) | Shader compiler | v0.5.0 |
| **gogpu/gg** | **2D graphics** | **v0.13.0** |

---

## Articles

- [GoGPU: From Idea to 100K Lines in Two Weeks](https://dev.to/kolkov/gogpu-from-idea-to-100k-lines-in-two-weeks-building-gos-gpu-ecosystem-3b2)
- [Pure Go 2D Graphics Library with GPU Acceleration](https://dev.to/kolkov/pure-go-2d-graphics-library-with-gpu-acceleration-introducing-gogpugg-538h)
- [GoGPU Announcement](https://dev.to/kolkov/gogpu-a-pure-go-graphics-library-for-gpu-programming-2j5d)

---

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Priority areas:
- API feedback and testing
- Examples and documentation
- Performance benchmarks
- Cross-platform testing

---

## License

MIT License — see [LICENSE](LICENSE) for details.
