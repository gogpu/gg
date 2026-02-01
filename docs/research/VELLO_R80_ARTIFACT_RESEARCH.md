# VELLO R80 Circle Artifact Research

**Date:** 2026-02-01
**Status:** ✅ SOLVED
**Problem:** Circle r=80 has artifact at top-right (53 pixels, 0.13%)
**Solution:** Mirror algorithm (Fix #21) — 53 → 4 pixels (95% improvement)

---

## Problem Description

Circle with center (100,100) and radius 80 shows over-fill artifact at top-right area.
- Affected area: x=112-127, y=20-24 (Tile 7,1)
- Symptom: Vello fills pixels that should be empty (alpha=127 instead of 0)

## Root Cause Analysis

### Observation 1: Backdrop = 0 Everywhere in Row 1

Debug output shows:
```
Row 1: Backdrop = 0 for ALL tiles (0-12)
```

This is **WRONG** for tiles inside the circle!

For y=25 (inside row 1), circle covers x≈20-180, so tiles 2-11 should have backdrop=1.

### Observation 2: Backdrop Added to Wrong Tile

Current code (line 400-402):
```go
for y := ymin; y < ymax; y++ {
    tr.tiles[y*tr.tilesX+bboxMinX].Backdrop += delta
}
```

`bboxMinX` is the bbox of the **ENTIRE PATH**, not each segment!

For circle:
- bboxMinX = 1 (tile covering x=16-31, left edge of circle at x=20)
- ALL segments add their delta to tile 1
- Left segments: +1, Right segments: -1
- Sum = 0 (they cancel out!)

### Observation 3: Correct Behavior

Each segment should add delta to its **OWN minimum X tile**:
- Left segments (x≈20) → add +1 to tile 1
- Right segments (x≈180) → add -1 to tile 11

After prefix sum:
- Tile 1: +1
- Tiles 2-10: +1 (propagated from tile 1)
- Tile 11: +1 - 1 = 0
- Tiles 12+: 0

This would give backdrop=1 for tiles INSIDE the circle!

### Observation 4: yEdge Over-contribution

In Tile(7,1), segments have:
- Seg[0]: dy=0.023 (tiny), yEdge=-0.523 (large!)
- Seg[1]: dy=0.130, yEdge=-0.130

For nearly horizontal segments at top of circle (dy ≈ 0), yEdge contribution dominates.
When backdrop=0 and only negative yEdge is applied, result becomes negative.
After abs() in non-zero fill rule: negative becomes positive → artifact!

## Failed Fix Attempts

### Attempt 1: Don't set yEdge for p1x==0 segments
**Result:** FAILED - broke circles completely (2.85% diff vs 0.13%)
**Reason:** yEdge is needed for correct fill; removing it breaks more than fixes

### Attempt 2: Limit yEdge to |dy|
**Result:** FAILED - broke diagonal stripe (0.24% diff)
**Reason:** Too restrictive; yEdge serves valid purpose in other cases

### Attempt 3: Don't apply yEdge when |dy| < threshold
**Result:** FAILED - same issues as attempt 2

## Key Insight: Two Problems, Not One

1. **Backdrop Problem:** All deltas go to bboxMinX, canceling out
2. **yEdge Problem:** Works incorrectly when backdrop=0 (becomes artifact after abs())

Fix #1 would make backdrop correct → yEdge would work properly → no artifact!

## Next Steps

1. **Fix backdrop calculation:** Each segment should add delta to its own minX tile, not path's bboxMinX
2. Look at how Vello computes segment bbox vs path bbox
3. The loop at line 397-404 needs to use segment's minX, not path's bboxMinX

## Code Location

- Backdrop increment: `vello_tiles.go:397-404`
- yEdge application: `vello_tiles.go:671-677`
- Prefix sum: `vello_tiles.go:154-165`

## KEY FINDING: Top Y Position Matters!

**Tested same circle (r=80) at different positions:**

| Top Y | Diff Pixels | Quality |
|-------|-------------|---------|
| 20 | 56 | Artifact |
| 70 | 47 | Better |
| 120 | 3 | Almost perfect |

**Conclusion:** The closer the top of the circle to the top edge of the image/bbox, the worse the artifact.

This explains why:
- **radius=80, center=(100,100):** top Y=20 → artifact
- **radius=60, center=(100,100):** top Y=40 → better
- **centered in 400x400:** top Y=120 → almost perfect

The issue is NOT related to tile boundaries or horizontal segments alone.
It's related to how segments are processed when the shape is close to the bbox edge.

## Failed Fix Attempts Summary

| Attempt | Change | Result | Reason |
|---------|--------|--------|--------|
| 1 | Remove yEdge for p1x==0 | Broke circles (2.85%) | yEdge needed for fill |
| 2 | Limit yEdge to |dy| | Broke diagonal (0.24%) | Too restrictive |
| 3 | Skip yEdge if |dy| < threshold | Same issues | Too restrictive |
| 4 | Use segment minX for backdrop | No effect | Coordinates wrong |

## ✅ SOLUTION: Mirror Algorithm (Fix #21)

**Date:** 2026-02-01
**Commit:** `19161d1`
**Result:** 53 pixels → 4 pixels (95% improvement)

### The Breakthrough

После 20+ неудачных попыток модифицировать yEdge, решение оказалось в **полном отзеркаливании координат**.

### Почему стандартный алгоритм не работает для tile (7,1)

```
Tile (7,1): backdrop=0, segments exit LEFT (dx < 0)

Standard Vello (left-to-right):
- yEdge compensates for LEFT edge crossings
- Segments exit left → yEdge negative (-1.0)
- Applied to ALL pixels: area = 0 + (-1.0) = -1.0
- abs(-1.0) = 1.0 → full fill → ARTIFACT!
```

### Mirror Algorithm

**100% математическое отзеркаливание:**

```go
// 1. Mirror X coordinates
mirroredSeg.Point0[0] = TileWidth - seg.Point0[0]
mirroredSeg.Point1[0] = TileWidth - seg.Point1[0]

// 2. Recompute YEdge for mirrored segments
// Right edge (x=16) becomes "left edge" (x=0) in mirror space

// 3. Run standard Vello on mirrored data

// 4. Mirror result back
for i := 0; i < TileWidth/2; i++ {
    j := TileWidth - 1 - i
    area[i], area[j] = area[j], area[i]
}
```

### Почему это работает

```
Mirror transformation:
- Original: segment (0.45, 4.5) → (0, 4.48) exits LEFT
- Mirrored: segment (15.55, 4.5) → (16, 4.48) exits RIGHT

In mirror space:
- "Left edge" is actually right edge of original
- Segment that crossed RIGHT edge now crosses "left" edge
- yEdge computed for RIGHT edge crossing
- Standard algorithm works correctly!
```

### Detection Pattern

```go
// Tile is "problem" if:
// - backdrop = 0
// - ≥2 segments with YEdge (touching left edge)
// - ALL such segments have dx < 0 (going left)
// - ALL such segments have dy ≤ 0 (going up or horizontal)
tile.IsProblemTile = leftEdgeCount >= 2 && allNegativeDx && allNonPositiveDy
```

### Remaining Artifacts

| Location | Pixels | Diff | Cause |
|----------|--------|------|-------|
| Tile (7,1) | 1 | +1 | Edge AA difference (invisible) |
| Tile (6,11) | 3 | -64 | Bottom of circle (see below) |

**Total: 4 pixels (0.02%)** — acceptable.

### Key Files

- `backend/native/vello_tiles.go:691` — `fillProblemTileScanline()` mirror algorithm
- `backend/native/vello_tiles.go:174` — `markProblemTiles()` detection

---

## Bottom Artifact Analysis (Tile 6,11)

**Date:** 2026-02-01
**Status:** ⚠️ KNOWN ISSUE (minor visual impact)

### Problem Description

Pixels (99-101, 179) in tile (6,11) have:
- Analytic: alpha=255 (fully inside circle)
- Vello: alpha=191 (75% coverage)
- Diff: -64

### Root Cause

After `computeBackdropPrefixSum()`, tile (6,11) has `backdrop=-1` (inside circle with CCW winding).

Segment contributions for row 3:
- Seg[1]: yEdge=+0.327, a*dy=-0.177 → net +0.15
- Seg[2]: a*dy=+0.10 → net +0.10
- Total contribution: +0.25

Final calculation:
```
area = backdrop + contributions = -1 + 0.25 = -0.75
alpha = abs(-0.75) * 255 = 191
```

### Why Fix is Difficult

1. **Detection Challenge**: After prefix sum, `backdrop=-1` for tiles inside circle.
   The condition `backdrop < 0` catches too many tiles (41+ false positives).

2. **Pattern Difference**: Top-right pattern has `backdrop=0` (outside shape).
   Bottom pattern has `backdrop=-1` (inside shape).

3. **No Simple Fix**: Tried multiple detection criteria:
   - `leftEdgeCount == 1 && allPositiveDx && allNonNegativeDy` — too broad
   - Adding `backdrop < 0` — still catches unrelated tiles
   - Specific segment checks — diagonal stripe breaks

### Visual Impact

At 1:1 scale, the 25% alpha difference on 3 pixels at circle edge is barely visible.
User confirmed: "глазу не сильно заметно и так при 1:1!"

### Decision

Documented as known issue. Minor visual impact does not justify complex detection logic.

---

## Debug Tests Created

- `vello_r80_debug_test.go` - tile/segment analysis
- `vello_r80_area_debug_test.go` - detailed area[] calculation
- `vello_backdrop_debug_test.go` - backdrop distribution analysis
- `vello_artifact_hunt_test.go` - artifact pixel hunting
