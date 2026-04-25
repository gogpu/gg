# Zero-Readback: Standard RenderDirect Path

GPU-direct rendering — all content rendered in a single GPU pass via `RenderDirect`.
Zero CPU readback, zero pixmap upload.

## APIs Demonstrated

- **FillRectCPU** — CPU-only background fill (bypasses GPU SDF accelerator)
- **RenderDirect** — single FlushGPUWithView pass to swapchain surface
- **GPU SDF circles** (Tier 1) — rendered directly to surface
- **GPU GlyphMask text** (Tier 6) — pixel-perfect hinted text

## Architecture

```
canvas.Draw(fn)                  → BeginAcceleratorFrame + draw all content
canvas.Render(dc.RenderTarget()) → RenderDirect → FlushGPUWithView → surface
```

All visible content is GPU-rendered. CPU pixmap is not uploaded.

## Run

```bash
go build -o zero_readback.exe . && ./zero_readback.exe
```

## Compare

See `../zero_readback_manual/` for the manual pipeline that separates
CPU and GPU content — the path used by `ui/desktop.go` for retained-mode
compositing (ADR-006).
