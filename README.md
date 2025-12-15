<h1 align="center">gg</h1>

<p align="center">
  <strong>Pure Go 2D Graphics Library</strong><br>
  Simple API, zero dependencies. Part of GoGPU ecosystem.
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

## Status: v0.2.0 — Text Rendering

> **Pure Go 2D graphics with TrueType font support!** Inspired by [fogleman/gg](https://github.com/fogleman/gg).
>
> **Star the repo to follow progress!**

---

## Features

- **Simple API** — Immediate-mode drawing API similar to HTML Canvas
- **Pure Go** — No C dependencies, software renderer
- **Text Rendering** — TrueType fonts, font fallback, Unicode support
- **Rich Shapes** — Rectangles, circles, ellipses, arcs, Bezier curves
- **Path Operations** — MoveTo, LineTo, QuadraticTo, CubicTo
- **Transformations** — Translate, rotate, scale with matrix stack
- **Colors** — RGBA, hex parsing, named colors
- **Font Composition** — MultiFace for fallback, FilteredFace for Unicode ranges

## Installation

```bash
go get github.com/gogpu/gg
```

**Requirements:** Go 1.25+

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

## Roadmap

| Version | Focus | Status |
|---------|-------|--------|
| v0.1.0 | Core shapes, software renderer | ✅ Done |
| v0.2.0 | Text rendering | **Current** |
| v0.3.0 | Image loading, clipping | Planned |
| v0.4.0 | Gradients, patterns | Planned |
| v0.5.0 | GPU acceleration (optional) | Planned |

## Part of GoGPU Ecosystem

| Component | Description | Version |
|-----------|-------------|---------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | GPU framework | v0.3.0 |
| [gogpu/wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU | v0.4.0 |
| [gogpu/naga](https://github.com/gogpu/naga) | Shader compiler | v0.4.0 |
| **gogpu/gg** | **2D graphics** | **v0.2.0** |

---

## License

MIT License — see [LICENSE](LICENSE) for details.
