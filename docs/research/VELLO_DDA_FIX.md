# Vello DDA First Row Fix

> Solution for backdrop propagation when shapes start inside tiles.

## Problem Description

When a shape's vertical edge starts inside a tile (not at the boundary), the DDA algorithm skips the first tile row, causing missing fill in adjacent tiles.

### Example: Diagonal Starting at (10, 10)

```
Tile(0,0)     Tile(1,0)     Tile(2,0)
┌──────────┐  ┌──────────┐  ┌──────────┐
│          │  │          │  │          │
│    │     │  │ ???????? │  │ ???????? │  ← rows 0-9 not filled!
│    │█████│  │ no       │  │ no       │
│    │█████│  │ backdrop │  │ backdrop │
└──────────┘  └──────────┘  └──────────┘
```

Tiles (1,0) and (2,0) should be filled for rows 10-15, but they receive no backdrop information.

## Root Cause

### DDA Index Calculation

```go
s0y := (y0 - tileY0*tileH) / tileH  // Start Y in tile coordinates
imin := int(math.Ceil(float64(s0y))) // First row to process
```

For segment starting at Y=10 in tile covering Y=0-15:
- `s0y = 10/16 = 0.625`
- `imin = ceil(0.625) = 1`
- **Row 0 (tileY=0) is skipped!**

### Backdrop Only Added in DDA Loop

Backdrop is added to adjacent tiles only during the DDA walk:

```go
for i := imin; i < imax; i++ {
    if topEdge && tileX+1 < tilesX {
        tiles[tileY*tilesX+tileX+1].Backdrop += delta
    }
}
```

Since `imin=1`, the loop never processes `i=0`, and tiles in row 0 never receive backdrop.

## Solution: Synthetic Y-Edge Segment

For **perfectly vertical edges** (dx=0), add a synthetic segment to the adjacent tile with YEdge set to indicate where fill should start.

### Conditions

The fix applies only when ALL conditions are met:

```go
if i == 0 &&                           // First DDA iteration
   !topEdge &&                         // Segment starts INSIDE tile
   tileX == bboxMinX &&                // Leftmost column of path
   isVertical &&                       // dx == 0 (perfectly vertical)
   shapeExtendsRightOfTile &&          // Shape extends beyond this tile
   tileX+1 < tr.tilesX {               // Adjacent tile exists
```

### Why Only dx == 0?

| Edge Type | dx | Needs Fix? |
|-----------|-----|-----------|
| Vertical edge of rectangle | 0 | ✅ Yes |
| Circle segment (Bezier approximation) | -0.20 | ❌ No |
| Diagonal segment | 5.5 | ❌ No |

Circles are approximated by Bezier curves that produce short segments with small but **non-zero** dx. Applying the fix to these causes artifacts.

### Implementation

```go
// In binSegments DDA loop (vello_tiles.go:480)

edgeDx := x1 - x0
isVertical := edgeDx == 0
tileRightEdge := float32((tileX + 1) * VelloTileWidth)
shapeExtendsRightOfTile := bounds.MaxX > tileRightEdge

if i == 0 && !topEdge && tileX == bboxMinX && isVertical &&
   shapeExtendsRightOfTile && tileX+1 < tr.tilesX {

    // Compute segment start Y in tile coordinates
    segStartY := (s0y - float32(tileY)) * VelloTileHeight
    if segStartY < 0 {
        segStartY = 0
    }

    // Add synthetic segment to adjacent tile
    xBump := tileX + 1
    tileIdx := tileY*tr.tilesX + xBump

    tr.tiles[tileIdx].Segments = append(tr.tiles[tileIdx].Segments, PathSegment{
        Point0: [2]float32{0, segStartY},
        Point1: [2]float32{epsilon, segStartY},
        YEdge:  segStartY,  // Fill rows >= startY
    })
}
```

### How YEdge Works

The synthetic segment tells the adjacent tile:
- "There's a segment at the left edge at Y = segStartY"
- During fill: apply yEdge contribution only for rows >= YEdge
- Result: rows below segStartY stay unfilled, rows >= segStartY get filled

## Important: shapeExtendsRightOfTile Check

Initial implementation used `bboxMaxX > bboxMinX` to check if shape spans multiple tiles. This was incorrect because bbox uses `ceil()`.

**Example of the bug:**
- Square at (7,7)-(13,13)
- bboxMinX = floor(7/16) = 0
- bboxMaxX = ceil(13/16) = 1
- `bboxMaxX > bboxMinX` = true (WRONG!)

The square fits entirely within tile (0,0), but the condition suggests it spans tiles.

**Fix:** Check actual shape bounds against tile edge:
```go
tileRightEdge := float32((tileX + 1) * VelloTileWidth)
shapeExtendsRightOfTile := bounds.MaxX > tileRightEdge
```

## Results

| Shape | Before Fix | After Fix |
|-------|------------|-----------|
| Diagonal stripe | 0.24% (96 px) | **0.00%** |
| Circle r=60 | 0.02% (7 px) | **0.02%** |
| Square 6×6 | 6.00% (24 px) | **0.00%** |

Circle is unchanged because it has no perfectly vertical edges.

## Commits

```
e0f9137 fix(vello): backdrop propagation for vertical edges starting inside tiles
497bda0 fix(vello): prevent synthetic segment for shapes within single tile
```

## Code Location

- `vello_tiles.go:480-506` — Synthetic segment logic in binSegments

---

*Last updated: 2026-02-02*
