# Clip Demo

GPU-accelerated clipping with `ClipRect` and `ClipRoundRect` APIs.

## What it shows

- **Left half:** rotating ring of colored circles — no clip (fully visible)
- **Right half:** animated shapes inside a pulsing clip region (clipped at edges)
- Hardware scissor rect (GPU-CLIP-001) + analytic SDF RRect clip (GPU-CLIP-002)

## Run

```bash
go run .
```

Press **Escape** to quit.
