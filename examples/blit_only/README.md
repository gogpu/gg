# Blit-Only Compositor Path (ADR-016)

Demonstrates the non-MSAA blit-only fast path in isolation.

## What it does

- CPU-drawn content (background, animated circles, grid) via `FillRectCPU` and `SetPixelPremul`
- Uploaded to GPU via `FlushPixmap`
- Composited via `DrawGPUTextureBase` + `FlushGPUWithView`
- NO SDF shapes, NO GPU text — `isBlitOnly()` returns true
- Triggers the 1x render pass (no 4x MSAA overhead, 93% bandwidth reduction)

## Architecture

```
CPU pixmap (FillRectCPU, SetPixelPremul)
    │
    ▼
FlushPixmap()           ← upload to GPU texture
EnsureGPUTexture()      ← promote pending → real texture
PixmapTextureView()     ← get view handle
    │
    ▼
DrawGPUTextureBase()    ← queue as base layer (Tier 0)
FlushGPUWithView()      ← encode + submit blit-only pass (1x, non-MSAA)
```

This is the path `ui/desktop.go` uses when all animated content lives in
RepaintBoundary GPU textures and the main compositor only blits the CPU pixmap
plus overlay quads.

## Running

```bash
go run .
```

Press **Escape** to quit.

## See also

- [`zero_readback/`](../zero_readback/) — GPU-direct rendering with SDF shapes + text (MSAA path)
- [`zero_readback_manual/`](../zero_readback_manual/) — manual step-by-step pipeline
- ADR-015: Compositor Base Layer
- ADR-016: Non-MSAA Compositor Fast Path
