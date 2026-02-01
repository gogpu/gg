# Vello Tile-Based Rasterization Algorithm

> Pure Go implementation of Linebender's Vello rasterization algorithm for CPU rendering.

## Overview

Vello is a modern vector graphics rasterization algorithm developed by Google/Linebender for high-performance GPU rendering. This document describes our Go implementation for CPU, which can serve as a reference or fallback when GPU is unavailable.

**Repository:** [github.com/gogpu/gg](https://github.com/gogpu/gg)
**Status:** Working with known limitations (see [Known Issues](#known-issues))

## Architecture

### Tile-Based Approach

The image is divided into 16×16 pixel tiles. Each tile is processed independently, enabling parallelism on GPU and cache locality on CPU.

```
┌────────┬────────┬────────┬────────┐
│ Tile   │ Tile   │ Tile   │ Tile   │
│ (0,0)  │ (1,0)  │ (2,0)  │ (3,0)  │
├────────┼────────┼────────┼────────┤
│ Tile   │ Tile   │ Tile   │ Tile   │
│ (0,1)  │ (1,1)  │ (2,1)  │ (3,1)  │
└────────┴────────┴────────┴────────┘
```

### Key Concepts

| Concept | Description |
|---------|-------------|
| **Backdrop** | Accumulated winding number passed from left tiles to right |
| **Segment** | Line segment of path contour within a tile (tile-relative coords 0-16) |
| **YEdge** | Y coordinate where segment touches left edge (x=0), used for partial fill |
| **Winding Rule** | Non-zero or even-odd fill rule for determining inside/outside |

### Algorithm Pipeline

```
1. Path Flattening    → Convert Bezier curves to line segments
2. Segment Binning    → Distribute segments to tiles (DDA walk)
3. Backdrop Prefix Sum → Propagate winding numbers left-to-right
4. Tile Rendering     → Fill each tile scanline by scanline
```

## Implementation Details

### Segment Binning (binSegments)

Uses Digital Differential Analyzer (DDA) to walk through tiles:

```go
// For each segment, determine all tiles it passes through
for i := imin; i < imax; i++ {
    tileY := int(tileY0 + float32(i) - z)
    tileX := int(tileX0 + sign*z)

    // Add segment to tile
    tr.tiles[tileY*tr.tilesX+tileX].Segments = append(...)

    // Update backdrop for adjacent tile
    if topEdge && tileX+1 < tilesX {
        tr.tiles[tileY*tr.tilesX+tileX+1].Backdrop += delta
    }
}
```

### Backdrop Prefix Sum

```go
// For each row, sum backdrops from left to right
for ty := 0; ty < tilesY; ty++ {
    sum := 0
    for tx := 0; tx < tilesX; tx++ {
        sum += tiles[ty*tilesX+tx].Backdrop
        tiles[ty*tilesX+tx].Backdrop = sum
    }
}
```

### Tile Rendering (fillTileScanline)

Port of Vello's `fine.wgsl` GPU shader to CPU:

```go
func fillTileScanline(tile *VelloTile, localY int, fillRule FillRule) {
    // Initialize with backdrop
    for i := 0; i < TileWidth; i++ {
        area[i] = float32(tile.Backdrop)
    }

    // Process each segment
    for _, seg := range tile.Segments {
        // Compute coverage using trapezoid formula
        // Add yEdge contribution for left-edge crossings
        // Accumulate area
    }

    // Apply fill rule (non-zero or even-odd)
}
```

## Key Files

| File | Purpose |
|------|---------|
| `backend/native/vello_tiles.go` | Main algorithm implementation |
| `backend/native/analytic_filler.go` | Reference implementation for comparison |
| `backend/native/vello_visual_test.go` | Visual regression tests |
| `backend/native/golden_test.go` | Golden image comparison tests |

## Known Issues

### 1. Top-Right Circle Artifact (SOLVED)

**Problem:** Circle r=80 showed over-fill artifact at top-right (53 pixels)

**Root Cause:** For tiles at the right edge of shapes (backdrop=0), segments exiting left caused yEdge to fill all pixels incorrectly.

**Solution:** Mirror algorithm (Fix #21) — mathematically mirror X coordinates, run standard algorithm, mirror result back. Reduces artifact from 53 to 1 pixel (95% improvement).

See: [VELLO_MIRROR_ALGORITHM.md](VELLO_MIRROR_ALGORITHM.md)

### 2. Bottom Circle Artifact (Known Issue)

**Problem:** 3 pixels at bottom of circle r=80 have alpha=191 instead of 255

**Root Cause:** After prefix sum, backdrop=-1 (inside shape). Segment contributions (+0.25) don't fully compensate, giving area=-0.75 instead of -1.0.

**Status:** Documented as known issue. Visual impact is minimal (25% alpha difference on 3 pixels at edge). Detection criteria that catch this pattern also cause 41+ false positives on other shapes.

See: [VELLO_BOTTOM_ARTIFACT.md](VELLO_BOTTOM_ARTIFACT.md)

### 3. DDA First Row Skip (SOLVED)

**Problem:** DDA loop starts at `imin = ceil(s0y)`, skipping first tile row for shapes starting inside tiles.

**Solution:** Synthetic y_edge segment for perfectly vertical edges (dx=0). Adds segment to adjacent tile with YEdge indicating fill start row.

See: [VELLO_DDA_FIX.md](VELLO_DDA_FIX.md)

## Test Results

| Shape | Difference vs Reference | Status |
|-------|------------------------|--------|
| Square 6×6 | 0.00% | ✅ Perfect |
| Circle r=7 | 0.50% (2 px) | ✅ Acceptable |
| Circle r=60 | 0.02% (7 px) | ✅ Acceptable |
| Circle r=80 | 0.01% (4 px) | ✅ Acceptable |
| Diagonal stripe | 0.00% | ✅ Perfect |
| Rectangle aligned | 12.50% | ⚠️ Tile boundary |
| Rectangle unaligned | 4.69% | ⚠️ Tile crossing |

## References

- [Vello (Linebender)](https://github.com/linebender/vello) — Original GPU implementation
- [Vello fine.wgsl](https://github.com/linebender/vello/blob/main/vello_shaders/shader/fine.wgsl) — GPU shader we ported
- [Vello path_tiling.wgsl](https://github.com/linebender/vello/blob/main/vello_shaders/shader/path_tiling.wgsl) — Segment binning

## Contributing

We welcome contributions! Areas where help is needed:

1. **Bottom artifact fix** — Find detection criteria that don't cause false positives
2. **Rectangle tile boundary** — Investigate why aligned rectangles differ
3. **GPU implementation** — Port to compute shaders using gogpu/wgpu

---

*Last updated: 2026-02-02*
