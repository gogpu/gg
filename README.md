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

## Current: v0.9.2

> **GPU Backend — Hardware acceleration via gogpu/wgpu with Sparse Strips!**
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

### Images (v0.3.0)
- **7 Pixel Formats** — Gray8, Gray16, RGB8, RGBA8, RGBAPremul, BGRA8, BGRAPremul
- **DrawImage** — Draw images with position, transforms, opacity
- **Interpolation** — Nearest, Bilinear, Bicubic sampling
- **Patterns** — Image patterns for fills with repeat modes
- **Mipmaps** — Automatic mipmap chain generation

### Clipping (v0.3.0)
- **Clip()** — Clip to current path
- **ClipRect()** — Fast rectangular clipping
- **ClipPreserve()** — Clip keeping path for stroke
- **Hierarchical** — Push/Pop state preserves clip regions

### Compositing (v0.3.0)
- **Porter-Duff** — 14 blend modes (SrcOver, DstIn, Xor, etc.)
- **Advanced Blends** — Screen, Overlay, Darken, Lighten, ColorDodge, etc.

### Color Pipeline & Layers (v0.4.0)
- **Layer API** — PushLayer/PopLayer for isolated drawing with blend modes
- **HSL Blend Modes** — Hue, Saturation, Color, Luminosity (W3C spec)
- **Linear Blending** — Correct sRGB ↔ Linear color space pipeline
- **ColorSpace Package** — ColorF32/ColorU8 types with conversions

### SIMD Optimization (v0.5.0)
- **Fast div255** — Shift approximation, no division (2.4x faster)
- **sRGB LUTs** — Pre-computed lookup tables (260x faster than math.Pow)
- **Wide Types** — U16x16/F32x8 for batch processing (16 pixels at once)
- **Batch Blending** — All 14 Porter-Duff modes + 7 advanced modes
- **Auto-vectorization** — Fixed-size arrays trigger Go compiler SIMD
- **FillSpan** — Optimized span filling for rasterizer integration

### Parallel Rendering (v0.6.0)
- **TileGrid** — Canvas divided into 64x64 pixel tiles for independent rendering
- **WorkerPool** — Goroutine pool with work stealing for load balancing
- **ParallelRasterizer** — Clear, FillRect, Composite operations across tiles
- **Lock-Free DirtyRegion** — Atomic bitmap tracking for changed tiles (10.9ns/mark)
- **Visual Regression Tests** — Pixel-perfect comparison of parallel vs serial
- **Scaling Benchmarks** — Linear scaling with 1, 2, 4, 8+ cores

### Scene Graph (v0.7.0)
- **Dual-Stream Encoding** — GPU-ready command buffer (vello pattern)
- **Scene API** — Fill, Stroke, PushLayer, PopLayer, PushClip, transforms
- **13 Shape Types** — Rect, Circle, Ellipse, Line, Polygon, RoundedRect...
- **29 Blend Modes** — 14 Porter-Duff + 11 Advanced + 4 HSL
- **Filter Effects** — Gaussian blur, drop shadow, color matrix (10 presets)
- **LRU Layer Cache** — 64MB default, configurable, 90ns Get / 393ns Put
- **SceneBuilder** — Fluent API for ergonomic scene construction
- **Parallel Renderer** — TileGrid + WorkerPool integration

### Backend Abstraction (v0.8.0)
- **RenderBackend Interface** — Pluggable rendering backends
- **SoftwareBackend** — CPU-based rendering (default)
- **Backend Registry** — Auto-selection with priority (wgpu > software)
- **Fallback Mechanism** — Graceful degradation when GPU unavailable

### GPU Backend (v0.9.0)
- **WGPUBackend** — Hardware acceleration via gogpu/wgpu integration
- **Sparse Strips Algorithm** — CPU tessellates paths to strips, GPU rasterizes (vello pattern)
- **GPU Memory Management** — LRU eviction, 256MB+ budget, texture atlas with shelf packing
- **WGSL Shaders** — 4 compute/fragment shaders (blit, blend, strip, composite)
- **29 Blend Modes** — All Porter-Duff + Advanced + HSL modes on GPU
- **Render Pipeline** — PipelineCache, GPUSceneRenderer, CommandEncoder pattern
- **9,930 LOC** — Complete GPU rendering infrastructure

### Coming Soon (v0.10.0+)
- **Text Rendering on GPU** — Glyph atlas and SDF fonts

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

## Image Drawing

```go
// Load image
img, _ := gg.LoadImage("photo.png")

// Draw at position
ctx.DrawImage(img, 100, 100)

// Draw with options
ctx.DrawImageEx(img, gg.DrawImageOptions{
    X:       200,
    Y:       200,
    ScaleX:  0.5,
    ScaleY:  0.5,
    Opacity: 0.8,
})
```

---

## Clipping

```go
// Clip to circle
ctx.DrawCircle(256, 256, 100)
ctx.Clip()

// Everything drawn now is clipped
ctx.SetColor(gg.Red)
ctx.DrawRectangle(0, 0, 512, 512)
ctx.Fill()

// Reset clip
ctx.ResetClip()

// Or use Push/Pop for scoped clipping
ctx.Push()
ctx.ClipRect(100, 100, 200, 200)
// ... draw clipped content ...
ctx.Pop()
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
ctx.DrawString("Hello! :)", 50, 150)
```

---

## Roadmap to v1.0.0

| Version | Focus | Status |
|---------|-------|--------|
| v0.1.0 | Core shapes, software renderer | Released |
| v0.2.0 | Text rendering | Released |
| v0.3.0 | Images, clipping, compositing | Released |
| v0.4.0 | Color pipeline, layer API | Released |
| v0.5.0 | SIMD optimization | Released |
| v0.6.0 | Parallel rendering | Released |
| v0.7.0 | Scene graph (retained mode) | Released |
| v0.8.0 | Backend abstraction | Released |
| **v0.9.0** | **GPU acceleration (Sparse Strips)** | **Released** |
| v0.10.0 | GPU text rendering | Planned |
| **v1.0.0** | **Production release** | **Target** |

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
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | GPU framework | **v0.4.0** |
| [gogpu/wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU | **v0.5.0** |
| [gogpu/naga](https://github.com/gogpu/naga) | Shader compiler | v0.4.0 |
| **gogpu/gg** | **2D graphics** | **v0.9.2** |

---

## License

MIT License — see [LICENSE](LICENSE) for details.
