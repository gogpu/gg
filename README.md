# gg - Pure Go 2D Graphics Library

[![Go Reference](https://pkg.go.dev/badge/github.com/gogpu/gg.svg)](https://pkg.go.dev/github.com/gogpu/gg)
[![Go Report Card](https://goreportcard.com/badge/github.com/gogpu/gg)](https://goreportcard.com/report/github.com/gogpu/gg)
[![CI](https://github.com/gogpu/gg/workflows/CI/badge.svg)](https://github.com/gogpu/gg/actions)

A simple, elegant 2D graphics library for Go, inspired by [fogleman/gg](https://github.com/fogleman/gg) and designed to integrate with the [GoGPU ecosystem](https://github.com/gogpu).

## Features

- **Simple API** - Immediate-mode drawing API similar to HTML Canvas
- **Pure Go** - No C dependencies (v0.1 software renderer)
- **fogleman/gg compatible** - Easy migration from existing code
- **Dual Renderer** - Software (v0.1) + GPU-accelerated (v0.3+)
- **Rich Shapes** - Rectangles, circles, ellipses, polygons, arcs, Bezier curves
- **Transformations** - Translate, rotate, scale, shear with matrix stack
- **Zero Dependencies** - Software renderer has no external dependencies

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
    "log"
)

func main() {
    // Create a 512x512 context
    ctx := gg.NewContext(512, 512)

    // Clear with white background
    ctx.ClearWithColor(gg.White)

    // Draw a red circle
    ctx.SetRGB(1, 0, 0)
    ctx.DrawCircle(256, 256, 100)
    ctx.Fill()

    // Save to PNG
    if err := ctx.SavePNG("output.png"); err != nil {
        log.Fatal(err)
    }
}
```

## Part of GoGPU Ecosystem

This library is part of the [GoGPU project](https://github.com/gogpu).

---

**Status:** v0.1.0-alpha - API unstable, production use not recommended yet.
