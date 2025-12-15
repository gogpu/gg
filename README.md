<h1 align="center">gg</h1>

<p align="center">
  <strong>Enterprise-Grade 2D Graphics Library for Go</strong><br>
  Professional rendering • Pure Go • Part of GoGPU Ecosystem
</p>

<p align="center">
  <a href="https://github.com/gogpu/gg/actions/workflows/ci.yml"><img src="https://github.com/gogpu/gg/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/gogpu/gg"><img src="https://codecov.io/gh/gogpu/gg/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="https://pkg.go.dev/github.com/gogpu/gg"><img src="https://pkg.go.dev/badge/github.com/gogpu/gg.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/gogpu/gg"><img src="https://goreportcard.com/badge/github.com/gogpu/gg" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/gogpu/gg"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go Version"></a>
</p>

---

## Vision

**gg** is designed to become the **reference 2D graphics library** for the Go ecosystem — capable of powering:

- **IDEs** (GoLAND, VS Code level)
- **Browsers** (Chrome level)
- **Professional graphics applications**

Inspired by [fogleman/gg](https://github.com/fogleman/gg), [tiny-skia](https://github.com/nicotine-scx/tiny-skia), and [vello](https://github.com/linebender/vello).

---

## Current: v0.2.0

> **Pure Go 2D graphics with TrueType font support!**
>
> **Star the repo to follow progress!**

---

## Features

### Core Graphics
- **Simple API** — Immediate-mode drawing API similar to HTML Canvas
- **Pure Go** — No C dependencies, cross-platform
- **Rich Shapes** — Rectangles, circles, ellipses, arcs, Bezier curves
- **Path Operations** — MoveTo, LineTo, QuadraticTo, CubicTo
- **Transformations** — Translate, rotate, scale with matrix stack
- **Colors** — RGBA, hex parsing, named colors

### Text Rendering (v0.2.0)
- **TrueType Fonts** — Full TTF support via golang.org/x/image
- **Font Composition** — MultiFace for fallback chains
- **Unicode Support** — FilteredFace for emoji and special ranges
- **Zero-Allocation Iterators** — Go 1.25+ iter.Seq[Glyph]

### Coming Soon (v0.3.0+)
- **Images** — Loading, drawing, patterns, mipmaps
- **Clipping** — Edge clipper, mask clipper, clip stack
- **Compositing** — Porter-Duff, blend modes, layers
- **SIMD Optimization** — 3-5x faster blending
- **Parallel Rendering** — Multi-core tile-based rasterization
- **GPU Acceleration** — via gogpu/wgpu

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
    ctx.SetColor(gg.Hex("#3498db"))
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

## Text Rendering

```go
// Load font (heavyweight, share across app)
source, err := text.NewFontSourceFromFile("Roboto.ttf")
defer source.Close()

// Create face (lightweight, per size)
face := source.Face(24)

// Draw text
ctx.SetFont(face)
ctx.DrawString("Hello World!", 50, 100)
ctx.DrawStringAnchored("Centered", 256, 256, 0.5, 0.5)

// Measure text
w, h := ctx.MeasureString("Hello")

// Font fallback for emoji
emoji, _ := text.NewFontSourceFromFile("NotoEmoji.ttf")
multiFace, _ := text.NewMultiFace(
    source.Face(24),
    text.NewFilteredFace(emoji.Face(24), text.RangeEmoji),
)
ctx.SetFont(multiFace)
ctx.DrawString("Hello! 🎉", 50, 150)
```

---

## Roadmap to v1.0.0

| Version | Focus | Status |
|---------|-------|--------|
| v0.1.0 | Core shapes, software renderer | ✅ Released |
| v0.2.0 | Text rendering | ✅ **Released** |
| v0.3.0 | Images, clipping, compositing | 🔵 In Progress |
| v0.4.0 | Color pipeline (sRGB/Linear) | Planned |
| v0.5.0 | SIMD optimization | Planned |
| v0.6.0 | Parallel rendering | Planned |
| v0.7.0 | Scene graph (retained mode) | Planned |
| v0.8.0 | Backend abstraction | Planned |
| v0.9.0 | GPU acceleration | Planned |
| **v1.0.0** | **Production release** | **Target** |

**Timeline to v1.0.0:** ~7 months

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
         Software           SIMD             GPU
         (current)        (v0.5.0)       (gogpu/wgpu)
```

---

## Part of GoGPU Ecosystem

**gogpu** is a Pure Go GPU Computing Ecosystem — professional graphics libraries for Go.

| Component | Description | Version |
|-----------|-------------|---------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | GPU framework | v0.3.0 |
| [gogpu/wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU | v0.4.0 |
| [gogpu/naga](https://github.com/gogpu/naga) | Shader compiler | v0.4.0 |
| **gogpu/gg** | **2D graphics** | **v0.2.0** |

---

## License

MIT License — see [LICENSE](LICENSE) for details.
