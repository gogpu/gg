# Vello Mirror Algorithm for Top-Right Artifacts

> Solution for over-fill artifacts at right edges of curved shapes.

## Problem Description

Circle with center (100,100) and radius 80 shows over-fill artifact at top-right:
- **Location:** Tile (7,1), pixels x=112-127, y=20-24
- **Symptom:** Vello fills pixels that should be empty (alpha=127 instead of 0)
- **Impact:** 53 pixels (0.13% of circle area)

## Root Cause Analysis

### The Pattern

```
Tile (7,1): backdrop=0, segments exit LEFT (dx < 0)

Standard Vello algorithm (left-to-right):
1. yEdge compensates for LEFT edge crossings
2. Segments exit left → yEdge contribution is negative (-1.0)
3. Applied to ALL pixels: area = 0 + (-1.0) = -1.0
4. abs(-1.0) = 1.0 → full fill → ARTIFACT!
```

### Why Standard Algorithm Fails

Vello's algorithm is optimized for shapes where:
- Backdrop propagates from LEFT to RIGHT
- yEdge compensates for LEFT edge partial coverage
- Fill accumulates as you scan right

But for tiles at the RIGHT edge of shapes:
- Backdrop = 0 (nothing comes from left, we're outside shape)
- Segments EXIT through left edge (dx < 0)
- yEdge is negative, applied to all pixels
- Results in spurious fill

## Solution: Mathematical Mirror

### Key Insight

Instead of modifying yEdge logic (20+ failed attempts), we transform the coordinate space:

1. **Mirror X coordinates:** `x' = TileWidth - x`
2. **Recompute yEdge** for mirrored segments (right edge becomes "left")
3. **Run standard algorithm** on mirrored data
4. **Mirror result back:** swap `area[i]` with `area[TileWidth-1-i]`

### Implementation

```go
func fillProblemTileScanline(tile *VelloTile, localY int, fillRule FillRule) {
    // Step 1: Create mirrored segments
    mirroredSegs := make([]PathSegment, len(tile.Segments))
    tileW := float32(TileWidth)

    for i, seg := range tile.Segments {
        // Mirror X coordinates: x' = TileWidth - x
        mirroredSegs[i] = PathSegment{
            Point0: [2]float32{tileW - seg.Point0[0], seg.Point0[1]},
            Point1: [2]float32{tileW - seg.Point1[0], seg.Point1[1]},
            YEdge:  1e9, // Will be recomputed
        }

        // Step 2: Compute YEdge for mirrored segment
        // Right edge (x=16) becomes left edge (x=0) in mirror space
        mx0, mx1 := mirroredSegs[i].Point0[0], mirroredSegs[i].Point1[0]
        my0, my1 := mirroredSegs[i].Point0[1], mirroredSegs[i].Point1[1]

        if (mx0 <= 0 && mx1 > 0) || (mx1 <= 0 && mx0 > 0) {
            dx := mx1 - mx0
            if dx != 0 {
                t := (0 - mx0) / dx
                yEdge := my0 + t*(my1-my0)
                mirroredSegs[i].YEdge = yEdge
            }
        }
    }

    // Step 3: Run standard Vello on mirrored segments
    // ... standard area calculation ...

    // Step 4: Mirror result back
    for i := 0; i < TileWidth/2; i++ {
        j := TileWidth - 1 - i
        area[i], area[j] = area[j], area[i]
    }
}
```

### Why It Works

```
Original tile (7,1):
- Segment (0.45, 4.5) → (0, 4.48) exits LEFT
- yEdge computed for left edge crossing
- Standard algorithm: INCORRECT fill

After mirror:
- Segment (15.55, 4.5) → (16, 4.48) exits RIGHT
- In mirror space, right edge is the "left edge"
- yEdge computed for right edge crossing
- Standard algorithm: CORRECT fill
- Mirror result back: pixels in right position
```

## Detection Criteria

Problem tiles are detected in `markProblemTiles()`:

```go
tile.IsProblemTile = (
    tile.Backdrop == 0 &&           // Outside shape from left
    leftEdgeCount >= 2 &&           // ≥2 segments touch left edge
    allNegativeDx &&                // All such segments go LEFT (dx < 0)
    allNonPositiveDy                // All such segments go UP (dy ≤ 0)
)
```

This pattern occurs ONLY at top-right corners where the shape "retreats" left-and-up.

## Results

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Artifact pixels | 53 | 1 | 98% |
| Difference % | 0.13% | 0.01% | 95% |

The remaining 1 pixel has diff=+1 (alpha 254 vs 255) which is invisible.

## Failed Approaches

Before discovering the mirror solution, we tried 20+ approaches:

| Attempt | Change | Result |
|---------|--------|--------|
| Remove yEdge for p1x==0 | Broke circles (2.85%) |
| Limit yEdge to \|dy\| | Broke diagonal (0.24%) |
| Skip yEdge if \|dy\| < threshold | Too restrictive |
| Use segment minX for backdrop | No effect |
| Supersampling | 60% improvement only |
| Modulate yEdge | Partial improvement |

The mirror algorithm is the only approach that achieves near-perfect results without side effects.

## Code Location

- **Detection:** `vello_tiles.go:174` — `markProblemTiles()`
- **Algorithm:** `vello_tiles.go:709` — `fillProblemTileScanline()`

---

*Last updated: 2026-02-02*
