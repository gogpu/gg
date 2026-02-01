# Vello Bottom Circle Artifact

> Known issue: 3 pixels at bottom of circle have incorrect alpha.

**Status:** ⚠️ KNOWN ISSUE — Help wanted!

## Problem Description

Circle r=80 at center (100,100) has 3 pixels with incorrect alpha at bottom:
- **Location:** Tile (6,11), pixels (99-101, 179)
- **Expected:** alpha = 255 (fully inside circle)
- **Actual:** alpha = 191 (75% coverage)
- **Difference:** -64 per pixel

## Visual Impact

At 1:1 scale, the 25% alpha difference on 3 edge pixels is barely visible. This is why it's documented as a known issue rather than blocking release.

## Root Cause Analysis

### Backdrop After Prefix Sum

After `computeBackdropPrefixSum()`, tile (6,11) has:
- `backdrop = -1` (inside circle, counter-clockwise winding)

This is different from the top-right artifact where `backdrop = 0`.

### Segment Analysis

Tile (6,11) segments for row 3 (localY=3):

```
Seg[0]: P0=(11.92, 3.50) → P1=(16.00, 2.88)
        dx=+4.08, dy=-0.62, YEdge=1e9 (no left edge)

Seg[1]: P0=(0.00, 3.67) → P1=(6.47, 4.00)
        dx=+6.47, dy=+0.33, YEdge=3.67 (touches left edge!)

Seg[2]: P0=(1.53, 4.00) → P1=(11.40, 3.50)
        dx=+9.87, dy=-0.50, YEdge=1e9 (no left edge)
```

### Area Calculation

For pixel x=99 (tile-relative x=3) at row 3:

```
backdrop = -1.0

Seg[1] contribution:
  yEdge = +clamp(3.0 - 3.67 + 1.0, 0, 1) = +0.33
  a*dy = -0.177
  net = +0.15

Seg[2] contribution:
  yEdge = 0 (no left edge)
  a*dy = +0.10
  net = +0.10

Total: area = -1.0 + 0.15 + 0.10 = -0.75
Alpha: abs(-0.75) * 255 = 191
```

**Expected:** area should be -1.0 → alpha = 255

### Why Contributions Don't Sum to Zero

For pixels fully inside the shape, segment contributions should cancel out (entering and exiting cancel). But here:

- yEdge = +0.33 (segment enters from left)
- Coverage contributions = -0.08 (net)
- **Total = +0.25** instead of 0

This +0.25 reduces the magnitude of area from -1.0 to -0.75.

## Why Fix is Difficult

### Attempt 1: Pattern Detection

Tried to detect "bottom problem tiles" similar to top-right:

```go
// Condition: backdrop<0, 1 segment with YEdge, dx>0, dy>0
if tile.Backdrop < 0 && leftEdgeCount == 1 && allPositiveDx && allNonNegativeDy {
    tile.IsBottomProblem = true
}
```

**Result:** 41+ false positives on other shapes (circles, diagonals)

### Attempt 2: More Specific Conditions

Added checks for segment characteristics:

```go
if dy > 0 && dy < 1.0 && seg.Point0[0] < 0.1 {
    tile.IsBottomProblem = true
}
```

**Result:** Still catches unrelated tiles, breaks diagonal stripe

### Key Difference from Top-Right

| Property | Top-Right (7,1) | Bottom (6,11) |
|----------|-----------------|---------------|
| Backdrop | 0 (outside) | -1 (inside) |
| Segment direction | EXIT left (dx<0) | ENTER from left (dx>0) |
| Pattern | All segments go same way | Mixed directions |

The top-right pattern is very specific (all segments exiting left). The bottom pattern is less distinctive, making detection hard.

## Proposed Solutions (Help Wanted!)

### Idea 1: Y-axis Mirror

Similar to X-mirror for top-right, but for bottom tiles:
- Mirror Y coordinates: `y' = TileHeight - y`
- Recompute yEdge
- Run standard algorithm
- Mirror rows back

**Challenge:** We process scanline-by-scanline, so Y-mirror would need to process entire tile at once.

### Idea 2: Backdrop Correction

Detect when backdrop and segment contributions should sum to integer but don't:

```go
// If tile is clearly inside shape, ensure full coverage
expectedArea := float32(tile.Backdrop)  // -1.0
actualContrib := computeContributions(tile, row)
if abs(expectedArea + actualContrib) < 0.5 {
    // Contributions nearly cancel, use backdrop directly
    area = expectedArea
}
```

**Challenge:** Determining "clearly inside" without false positives.

### Idea 3: Reference Implementation Comparison

Add runtime comparison with AnalyticFiller for edge tiles:

```go
if tile.IsEdgeTile() {
    velloAlpha := fillWithVello(tile, row)
    analyticAlpha := fillWithAnalytic(tile, row)
    if diff(velloAlpha, analyticAlpha) > threshold {
        return analyticAlpha  // Use reference
    }
}
```

**Challenge:** Performance cost, determining which tiles are "edge tiles".

## How to Reproduce

```bash
cd backend/native
go test -v -run TestHuntArtifactPixels
```

Output shows exact pixels and their analysis:
```
=== TILE (6, 11) — pixels (96-111, 176-191) ===
Artifact pixels in this tile: 3
  Pixel ( 99, 179): Analytic=255, Vello=191, diff= -64
  Pixel (100, 179): Analytic=255, Vello=191, diff= -64
  Pixel (101, 179): Analytic=255, Vello=191, diff= -64
```

## Code References

- Detection attempt: `vello_tiles.go:218` (commented out)
- Fill function: `vello_tiles.go:885` — `fillBottomProblemTileScanline()` (disabled)
- Test: `vello_artifact_hunt_test.go:241` — `TestHuntArtifactPixels`

## Contributing

If you have ideas for fixing this artifact without causing false positives, please:

1. Fork the repository
2. Implement your fix
3. Run `go test ./backend/native/...` — all tests must pass
4. Submit a PR with explanation

Key tests that must pass:
- `TestVelloAgainstGolden` — all shapes
- `TestVelloCompareDiagonal` — 0% diff required
- `TestVelloCompareWithOriginal` — circle comparison

---

*Last updated: 2026-02-02*
