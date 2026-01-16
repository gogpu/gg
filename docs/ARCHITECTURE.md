# gg Architecture

This document describes the architecture of the gg 2D graphics library.

## Overview

gg is a 2D graphics library for Go, inspired by HTML5 Canvas API.

```
                        ┌───────────────────┐
                        │ User Application  │
                        └─────────┬─────────┘
                                  │
                           ┌──────▼──────┐
                           │     gg      │
                           │  2D Canvas  │
                           └──────┬──────┘
                                  │
         ┌────────────────────────┼────────────────────────┐
         │                        │                        │
  ┌──────▼──────┐          ┌──────▼──────┐          ┌──────▼──────┐
  │   backend   │          │   backend   │          │   backend   │
  │    rust     │          │   native    │          │  software   │
  └──────┬──────┘          └──────┬──────┘          └──────┬──────┘
         │                        │                        │
         │                 ┌──────▼──────┐                 │
         │                 │    wgpu     │                 │
         │                 │    core     │                 │
         │                 └──────┬──────┘                 │
         │                        │                        │
         │              ┌─────────┼─────────┐              │
         │              │         │         │              │
         │           ┌──▼──┐  ┌───▼───┐  ┌──▼──┐           │
         │           │ Vk  │  │ Metal │  │Soft │           │
         │           └─────┘  └───────┘  └─────┘           │
         │                   wgpu/hal                      │
         │                                                 │
  ┌──────▼──────┐                                   ┌──────▼──────┐
  │ wgpu-native │                                   │    CPU      │
  │ (Rust FFI)  │                                   │ Rasterizer  │
  └─────────────┘                                   └─────────────┘
         │                                                 │
      GPU API                                      Direct 2D render
  (Vulkan/Metal/DX12)                               (no GPU needed)
```

## Backend System

gg supports three rendering backends:

| Backend      | Constant           | Description               | GPU Required |
|--------------|--------------------|---------------------------|--------------|
| **Rust**     | `BackendRust`      | wgpu-native via FFI       | Yes          |
| **Native**   | `BackendNative`    | Pure Go via gogpu/wgpu    | Yes          |
| **Software** | `BackendSoftware`  | CPU 2D rasterizer         | No           |

Aliases for convenience:
- `BackendGo` = `BackendNative`

### Backend Priority

When multiple backends are available, gg selects automatically:

```
Rust → Native → Software
 (1)     (2)       (3)
```

1. **Rust** — Maximum performance (if compiled with `-tags rust`)
2. **Native** — Good performance, zero dependencies (default)
3. **Software** — Always available fallback

### Build Tags

```bash
# Default: Native + Software backends
go build ./...

# With Rust backend
go build -tags rust ./...
```

## Backend Selection

```go
import "github.com/gogpu/gg/backend"

// Auto-select best available
b := backend.Default()

// Get specific backend by name
b := backend.Get(backend.BackendNative)   // Pure Go GPU
b := backend.Get(backend.BackendGo)       // Alias for Native
b := backend.Get(backend.BackendRust)     // Rust FFI GPU
b := backend.Get(backend.BackendSoftware) // CPU fallback

// Initialize default backend
b, err := backend.InitDefault()
```

## Software Rendering: Two Levels

There are **two different** software rendering options in the ecosystem:

| Component              | Level   | Purpose                              |
|------------------------|---------|--------------------------------------|
| `wgpu/hal/software`    | HAL     | Full WebGPU emulation on CPU         |
| `gg/backend/software`  | Backend | Lightweight 2D rasterizer (no wgpu)  |

- **wgpu/hal/software** — Used when Native backend needs CPU fallback
- **gg/backend/software** — Direct 2D rendering without WebGPU overhead

## RenderBackend Interface

gg uses a simple 6-method interface:

```go
type RenderBackend interface {
    // Identification
    Name() string

    // Lifecycle
    Init() error
    Close()

    // Rendering
    NewRenderer(width, height int) gg.Renderer
    RenderScene(target *gg.Pixmap, scene *scene.Scene) error
}
```

This is intentionally simpler than gogpu's 120+ method interface.

## Package Structure

```
gg/
├── context.go          # Canvas-like drawing context
├── path.go             # Vector path operations
├── paint.go            # Fill and stroke styles
├── pixmap.go           # Pixel buffer operations
├── text.go             # Text rendering
│
├── backend/            # Backend abstraction
│   ├── backend.go      # RenderBackend interface
│   ├── registry.go     # Auto-registration
│   ├── software.go     # CPU rasterizer + constants
│   ├── native/         # Pure Go GPU backend
│   └── rust/           # Rust FFI backend
│
├── scene/              # Retained mode rendering
│   ├── scene.go        # Scene graph
│   └── renderer.go     # Parallel tile renderer
│
├── font/               # Font loading
├── text/               # Text layout
└── image/              # Image loading
```

## Rendering Modes

### Immediate Mode

Traditional draw-as-you-go approach:

```go
dc := gg.NewContext(800, 600)
dc.SetRGB(1, 0, 0)
dc.DrawCircle(400, 300, 100)
dc.Fill()
dc.SavePNG("output.png")
```

### Retained Mode

Scene graph approach for complex scenes:

```go
s := scene.New()
s.PushLayer(scene.LayerConfig{})
s.Fill(path, paint)
s.PopLayer()

renderer.RenderScene(pixmap, s)
```

## Relationship to gogpu Ecosystem

```
naga (shader compiler)
  │
  └──► wgpu (Pure Go WebGPU)
         │
         ├──► gogpu (framework)
         │
         └──► gg (2D graphics) ◄── this project
```

gg and gogpu are **independent libraries**:

| Aspect                | gg                    | gogpu                |
|-----------------------|-----------------------|----------------------|
| **Purpose**           | 2D graphics library   | GPU framework        |
| **Dependencies**      | wgpu, naga            | wgpu                 |
| **Backend interface** | 6 methods             | 120+ methods         |
| **Software fallback** | Yes                   | No                   |

Both use **gogpu/wgpu** as the shared WebGPU implementation.

## See Also

- [README.md](../README.md) — Quick start guide
- [CHANGELOG.md](../CHANGELOG.md) — Version history
- [Examples](../examples/) — Code examples
