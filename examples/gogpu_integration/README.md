# GoGPU + gg Integration Example

Renders animated 2D graphics with [gg](https://github.com/gogpu/gg) inside a
[gogpu](https://github.com/gogpu/gogpu) GPU-accelerated window using the
`ggcanvas` integration package.

Uses **event-driven rendering** with `AnimationToken` — 0% CPU when paused,
VSync rendering while animating. Press **Space** to pause/resume.

## Architecture

```
gg.Context (2D draw calls)
    ↓
ggcanvas.Canvas (dirty tracking, deferred texture destruction)
    ↓  RenderDirect (zero-copy to surface)
gogpu.Context (GPU rendering)
    ↓
Window (Vulkan / Metal / DX12)
```

### Three-Tier GPU Rendering

The example showcases all three GPU rendering tiers:

| Tier | Shapes | Technique |
|------|--------|-----------|
| **SDF** | Circles, rounded rect | Signed Distance Field per-pixel |
| **Convex** | Triangle, pentagon, hexagon | Fan tessellation, single draw call |
| **Stencil+Cover** | Star, curved paths | Stencil buffer winding + cover fill |

## Run

```bash
go run .
```

The example opens an 800×600 window with animated shapes across all three
GPU tiers. Press **Space** to pause (0% CPU idle) and resume (VSync ~60fps).

## Requirements

| Dependency | Minimum version |
|------------|-----------------|
| Go | 1.25+ |
| gogpu | v0.18.1+ |
| gg | v0.28.1+ |
