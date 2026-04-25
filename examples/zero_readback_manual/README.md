# Zero-Readback: Manual Compositor Pipeline

Manual zero-readback pipeline — CPU content uploaded via `FlushPixmap` (no GPU
readback), GPU shapes rendered as overlay via `DrawGPUTextureBase` + `FlushGPUWithView`.

This is the rendering path used by `ui/desktop.go` for retained-mode compositing
(ADR-006, ADR-015, ADR-016).

## APIs Demonstrated

- **FillRectCPU** — CPU-only background (no GPU SDF interference)
- **FlushPixmap** — upload pixmap to GPU texture without FlushGPU readback
- **EnsureGPUTexture** — promote pending texture to real GPU texture (once)
- **PixmapTextureView** — get GPU texture view via duck typing
- **DrawGPUTextureBase** — pixmap as compositor base layer (drawn BEFORE GPU shapes)
- **FlushGPUWithView** — single pass: base layer + GPU SDF overlay → surface

## Architecture

```
Phase 1: CPU content → pixmap
  FillRectCPU(bg)              → direct pixmap write (no SDF)
  DrawLine + Stroke            → CPU analytic AA rasterizer
  DrawString                   → GPU GlyphMask (queued, not flushed)

Phase 2: Upload pixmap (NO readback)
  FlushPixmap()                → WriteTexture to GPU (no FlushGPU!)
  EnsureGPUTexture()           → promote pendingTexture (first frame only)

Phase 3: Single-pass compositor
  DrawGPUTextureBase(pixmap)   → base layer (drawn first)
  DrawCircle + Fill            → GPU SDF overlay (drawn on top)
  FlushGPUWithView(surface)    → one render pass → swapchain
```

## When This Path Wins

The manual pipeline benefits over RenderDirect when:
- CPU pixmap is **mostly static** (dirty region upload, not full pixmap)
- Compositor pass is **blit-only** (no vector shapes → non-MSAA 1x path)
- Animated widgets use **RepaintBoundary** (small GPU textures, not full surface)

This matches `ui/desktop.go` where widgets rarely change and only small
animated regions (spinner, progress bar) trigger per-frame GPU work.

## Run

```bash
go build -o manual.exe . && ./manual.exe
```

## Compare

See `../zero_readback/` for the standard RenderDirect path where all content
is GPU-rendered in one pass (simpler, but no CPU/GPU content separation).
