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
  <a href="https://github.com/gogpu/gg/releases"><img src="https://img.shields.io/github/v/release/gogpu/gg" alt="Latest Release"></a>
</p>

<p align="center">
  <strong>Go 1.25+ Required</strong>
</p>

---

## Overview

**gg** is a professional 2D graphics library for Go, designed to power IDEs, browsers, and graphics-intensive applications. Built with modern Rust-inspired patterns from [vello](https://github.com/linebender/vello) and [tiny-skia](https://github.com/RazrFalcon/tiny-skia), it delivers enterprise-grade rendering with zero CGO dependencies.

### Key Features

| Category | Capabilities |
|----------|--------------|
| **Rendering** | Immediate & retained mode, GPU acceleration via WebGPU, CPU fallback |
| **Shapes** | Rectangles, circles, ellipses, arcs, bezier curves, polygons, stars |
| **Text** | TrueType fonts, MSDF rendering, emoji/COLRv1, bidirectional text, CJK |
| **Compositing** | 29 blend modes (Porter-Duff + Advanced + HSL), layer isolation |
| **Images** | 7 pixel formats, PNG/JPEG I/O, mipmaps, affine transforms |
| **Performance** | Sparse strips GPU, tile-based parallel rendering, LRU caching |

### Architecture Highlights

```
                         gg (Public API)
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
    Immediate Mode       Retained Mode        Resources
    (Context API)        (Scene Graph)     (Images, Fonts)
          │                   │                   │
          └───────────────────┼───────────────────┘
                              │
                     RenderBackend Interface
                              │
             ┌────────────────┼────────────────┐
             │                                 │
        Software                             GPU
        (CPU SIMD)                    (gogpu/wgpu WebGPU)
```

---

## Installation

```bash
go get github.com/gogpu/gg
```

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
    defer ctx.Close() // Clean resource release

    ctx.ClearWithColor(gg.White)

    // Draw a gradient circle
    ctx.SetHexColor("#3498db")
    ctx.DrawCircle(256, 256, 100)
    ctx.Fill()

    // Text rendering with font fallback
    source, _ := text.NewFontSourceFromFile("arial.ttf")
    defer source.Close()

    ctx.SetFont(source.Face(32))
    ctx.SetColor(gg.Black)
    ctx.DrawString("Hello, GoGPU!", 180, 260)

    ctx.SavePNG("output.png")
}
```

---

## Core APIs

### Immediate Mode (Context)

Classic canvas-style drawing with transformation stack:

```go
ctx := gg.NewContext(800, 600)
defer ctx.Close()

// Shapes with transforms
ctx.Push()
ctx.Translate(400, 300)
ctx.Rotate(math.Pi / 4)
ctx.DrawRectangle(-50, -50, 100, 100)
ctx.SetRGB(0.2, 0.5, 0.8)
ctx.Fill()
ctx.Pop()

// Bezier paths
ctx.MoveTo(100, 100)
ctx.QuadraticTo(200, 50, 300, 100)
ctx.CubicTo(350, 150, 350, 250, 300, 300)
ctx.SetLineWidth(3)
ctx.Stroke()
```

### Fluent Path Builder

Type-safe path construction with method chaining:

```go
path := gg.BuildPath().
    MoveTo(100, 100).
    LineTo(200, 100).
    QuadTo(250, 150, 200, 200).
    CubicTo(150, 250, 100, 250, 50, 200).
    Close().
    Circle(300, 150, 50).
    Star(400, 150, 40, 20, 5).
    Build()

ctx.SetPath(path)
ctx.Fill()
```

### Retained Mode (Scene Graph)

GPU-optimized scene graph for complex applications:

```go
scene := gg.NewScene()

// Build scene with layers
scene.PushLayer(gg.BlendMultiply, 0.8)
scene.Fill(style, transform, gg.Solid(gg.Red), gg.Circle(150, 200, 100))
scene.Fill(style, transform, gg.Solid(gg.Blue), gg.Circle(250, 200, 100))
scene.PopLayer()

// Render to pixmap
renderer := scene.NewRenderer()
renderer.Render(target, scene)
```

### Text Rendering

Full Unicode support with bidirectional text:

```go
// Font composition with fallback
mainFont, _ := text.NewFontSourceFromFile("Roboto.ttf")
emojiFont, _ := text.NewFontSourceFromFile("NotoEmoji.ttf")
defer mainFont.Close()
defer emojiFont.Close()

multiFace, _ := text.NewMultiFace(
    mainFont.Face(24),
    text.NewFilteredFace(emojiFont.Face(24), text.RangeEmoji),
)

ctx.SetFont(multiFace)
ctx.DrawString("Hello World! Nice day!", 50, 100)

// Layout with wrapping
opts := text.LayoutOptions{
    MaxWidth: 400,
    WrapMode: text.WrapWordChar,
    Alignment: text.AlignCenter,
}
layout := text.LayoutText("Long text...", face, 16, opts)
```

### Alpha Masks

Sophisticated compositing with alpha masks:

```go
// Create mask from current drawing
ctx.DrawCircle(200, 200, 100)
ctx.Fill()
mask := ctx.AsMask()

// Apply mask to new context
ctx2 := gg.NewContext(400, 400)
ctx2.SetMask(mask)
ctx2.DrawRectangle(0, 0, 400, 400)
ctx2.Fill() // Only visible through mask

// Mask operations
ctx2.InvertMask()
ctx2.ClearMask()
```

### Layer Compositing

29 blend modes with isolated layers:

```go
ctx.PushLayer(gg.BlendOverlay, 0.7)

ctx.SetRGB(1, 0, 0)
ctx.DrawCircle(150, 200, 100)
ctx.Fill()

ctx.SetRGB(0, 0, 1)
ctx.DrawCircle(250, 200, 100)
ctx.Fill()

ctx.PopLayer() // Composite with overlay blend
```

---

## Ecosystem

**gg** is part of the [GoGPU](https://github.com/gogpu) ecosystem — Pure Go GPU computing libraries.

| Component | Description |
|-----------|-------------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | GPU framework with WebGPU, windowing, input |
| [gogpu/wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU implementation (Vulkan/Metal/DX12) |
| [gogpu/naga](https://github.com/gogpu/naga) | WGSL shader compiler (WGSL → SPIR-V) |
| **gogpu/gg** | **2D graphics library (this repo)** |
| [gogpu/ui](https://github.com/gogpu/ui) | GUI toolkit (coming soon) |

---

## Performance

| Operation | Time | Notes |
|-----------|------|-------|
| sRGB → Linear | 0.16ns | 260x faster than math.Pow |
| LayerCache.Get | 90ns | Thread-safe LRU |
| DirtyRegion.Mark | 10.9ns | Lock-free atomic |
| MSDF lookup | <10ns | Zero-allocation |
| Path iteration | 438ns | iter.Seq, 0 allocs |

---

## Documentation

- **[ROADMAP.md](ROADMAP.md)** — Development roadmap and version history
- **[CHANGELOG.md](CHANGELOG.md)** — Detailed release notes
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — Contribution guidelines
- **[pkg.go.dev](https://pkg.go.dev/github.com/gogpu/gg)** — API reference

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
