# GoGPU + gg Integration Example

Renders animated 2D graphics with [gg](https://github.com/gogpu/gg) inside a
[gogpu](https://github.com/gogpu/gogpu) GPU-accelerated window using the
`ggcanvas` integration package.

## Architecture

```
gg.Context (2D draw calls)
    ↓
ggcanvas.Canvas (CPU → GPU texture upload)
    ↓
gogpu.Context (GPU rendering)
    ↓
Window (Vulkan / Metal / DX12)
```

## Run

```bash
go run .
```

The example opens an 800×600 window with 12 animated, color-cycling circles.
Resize the window to see automatic canvas reallocation.

## Requirements

| Dependency | Minimum version |
|------------|-----------------|
| Go | 1.25+ |
| gogpu | v0.15.7+ |
| gg | v0.26.0+ |
