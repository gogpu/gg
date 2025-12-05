# gg

[![Go Reference](https://pkg.go.dev/badge/github.com/gogpu/gg.svg)](https://pkg.go.dev/github.com/gogpu/gg)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Simple 2D Graphics for Go** — GPU-accelerated, inspired by Processing.

> 📋 **Planned** — Coming after gogpu v1.0

---

## ✨ Vision

A simple, intuitive 2D graphics library built on [gogpu](https://github.com/gogpu/gogpu):

```go
package main

import "github.com/gogpu/gg"

func main() {
    ctx := gg.NewContext(800, 600)

    // Draw shapes
    ctx.SetColor(gg.Red)
    ctx.DrawCircle(400, 300, 100)
    ctx.Fill()

    ctx.SetColor(gg.Blue)
    ctx.DrawRectangle(100, 100, 200, 150)
    ctx.Stroke()

    // Save to file
    ctx.SavePNG("output.png")
}
```

## 🎯 Goals

- **Simple API** — Like [fogleman/gg](https://github.com/fogleman/gg) but GPU-accelerated
- **Processing-style** — Familiar to creative coders
- **GPU Backend** — Fast rendering via gogpu
- **Export** — PNG, JPEG, SVG output

## 🗺️ Planned Features

- Basic shapes (rect, circle, ellipse, line, polygon)
- Paths and curves (Bezier, arc)
- Text rendering (TTF fonts)
- Image loading and drawing
- Transformations (translate, rotate, scale)
- Gradients and patterns
- Anti-aliasing

## 🔗 Related Projects

| Project | Description |
|---------|-------------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | Graphics framework (backend) |
| [fogleman/gg](https://github.com/fogleman/gg) | CPU-based 2D graphics (inspiration) |

## 📄 License

MIT License

---

<p align="center">
  <b>gg</b> — 2D Graphics Made Easy
</p>
