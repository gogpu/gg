# Recording System Example

This example demonstrates the recording system for vector export.

## What it does

1. Creates a `Recorder` to capture drawing operations
2. Draws various shapes: gradient background, circle, rounded rectangle, dashed line, rotated square
3. Exports to multiple formats: PNG (raster), PDF, SVG

## Running

### Raster only (no additional dependencies)

```bash
go run main.go
```

Output: `output.png`

### With PDF export

```bash
go get github.com/gogpu/gg-pdf
```

Then uncomment the import in `main.go`:
```go
_ "github.com/gogpu/gg-pdf"
```

Output: `output.png`, `output.pdf`

### With SVG export

```bash
go get github.com/gogpu/gg-svg
```

Then uncomment the import in `main.go`:
```go
_ "github.com/gogpu/gg-svg"
```

Output: `output.png`, `output.svg`

## Key Concepts

### Recording
```go
rec := recording.NewRecorder(width, height)
// Draw using familiar API...
r := rec.FinishRecording()
```

### Playback
```go
backend, _ := recording.NewBackend("pdf")
r.Playback(backend)
backend.(recording.FileBackend).SaveToFile("output.pdf")
```

### Backend Registration

Backends are registered via blank imports (like database/sql drivers):
```go
import _ "github.com/gogpu/gg-pdf"  // Registers "pdf"
import _ "github.com/gogpu/gg-svg"  // Registers "svg"
```

## Output

The example produces a 400x300 image with:
- Gradient background (dark blue to light blue)
- Red circle
- White rounded rectangle (stroked)
- Yellow dashed line
- Green rotated square with transparency
