# Clipping Example

This example demonstrates the clipping API in gogpu/gg.

## Features Demonstrated

1. **Circular Clip** - Using `Clip()` to set a circular clipping region
2. **ClipRect** - Fast rectangular clipping with `ClipRect()`
3. **ClipPreserve** - Using `ClipPreserve()` to clip and then stroke the same path
4. **Nested Clips** - Multiple intersecting clips with `Push()`/`Pop()`
5. **Complex Path** - Clipping with curved Bezier paths

## API Methods

- `Clip()` - Set current path as clip region, clear path
- `ClipPreserve()` - Set current path as clip region, keep path
- `ClipRect(x, y, w, h)` - Fast rectangular clip
- `ResetClip()` - Remove all clipping regions
- `Push()`/`Pop()` - Save/restore clip state

## Running

```bash
go run main.go
```

This will generate `output.png` showing all clipping examples.

## Integration with Push/Pop

Clipping state is saved and restored with `Push()`/`Pop()`:

```go
dc.Push()
dc.ClipRect(10, 10, 100, 100)
// Drawing is clipped
dc.Pop() // Clip is restored
```

## Performance Notes

- `ClipRect()` is faster than creating a rectangular path and calling `Clip()`
- Anti-aliased clipping is enabled by default for smooth edges
- Multiple clips are intersected (AND operation)
