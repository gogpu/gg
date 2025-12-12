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

## Status: v0.1.0 — Initial Release

> **Pure Go 2D graphics with software renderer!** Inspired by [fogleman/gg](https://github.com/fogleman/gg).
>
> **Star the repo to follow progress!**

---

## Features

- **Simple API** — Immediate-mode drawing API similar to HTML Canvas
- **Pure Go** — No C dependencies, software renderer
- **Rich Shapes** — Rectangles, circles, ellipses, arcs, Bezier curves
- **Path Operations** — MoveTo, LineTo, QuadraticTo, CubicTo
- **Transformations** — Translate, rotate, scale with matrix stack
- **Colors** — RGBA, hex parsing, named colors
- **Zero Dependencies** — Pure Go implementation

## Installation

```bash
go get github.com/gogpu/gg
```

**Requirements:** Go 1.25+

## Quick Start

```go
package main

import "github.com/gogpu/gg"

func main() {
    // Create a 512x512 context
    ctx := gg.NewContext(512, 512)

    // Clear with white background
    ctx.SetColor(gg.White)
    ctx.Clear()

    // Draw a blue circle
    ctx.SetColor(gg.Hex("#3498db"))
    ctx.DrawCircle(256, 256, 100)
    ctx.Fill()

    // Draw a red rectangle
    ctx.SetColor(gg.Hex("#e74c3c"))
    ctx.DrawRectangle(50, 50, 150, 100)
    ctx.Fill()

    // Save to PNG
    ctx.SavePNG("output.png")
}
```

## Roadmap

| Version | Focus | Status |
|---------|-------|--------|
| v0.1.0 | Core shapes, software renderer | **Current** |
| v0.2.0 | Text rendering, image loading | Planned |
| v0.3.0 | Gradients, patterns, clipping | Planned |
| v0.4.0 | GPU acceleration (optional) | Planned |

## Part of GoGPU Ecosystem

| Component | Description | Version |
|-----------|-------------|---------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | GPU framework | v0.3.0 |
| [gogpu/wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU | v0.4.0 |
| [gogpu/naga](https://github.com/gogpu/naga) | Shader compiler | v0.4.0 |
| **gogpu/gg** | **2D graphics** | **v0.1.0** |

---

## License

MIT License — see [LICENSE](LICENSE) for details.
