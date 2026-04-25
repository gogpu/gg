# Scene GPU Visual

Animated scene rendering with GPU acceleration in a gogpu window.

## What it shows

- **GPUSceneRenderer** — scene commands decoded into GPU draw calls
- Retained-mode scene encoding (built each frame, rendered via canvas)
- SDF shapes through per-context GPURenderContext
- Automatic CPU fallback for unsupported operations

## Architecture

```
scene.Scene (encoded commands)
    → scene.Renderer (orchestration)
    → gg.Context GPU accelerator (SDF pipeline)
    → ggcanvas.Canvas → gogpu window
```

## Run

```bash
go run .
```

Press **Escape** to quit.
