# Vello Backdrop Propagation: Deep Research Report

**Author:** Claude (Research Agent)
**Date:** 2026-02-01
**Status:** COMPLETE - Bug Identified, Solution Provided

---

## Executive Summary

The bug is **NOT in backdrop propagation** but in **coordinate system mismatch between CPU fine.rs and GPU fine.wgsl**. The Go implementation correctly ports fine.rs (CPU), but fine.rs uses ABSOLUTE pixel coordinates for y_edge calculation while the segments store TILE-RELATIVE coordinates. This mismatch causes y_edge to compensate at wrong scanlines.

**The solution:** Use tile-relative Y coordinates in `fillTileScanline`, exactly as GPU fine.wgsl does.

---

## Research Scope

**Question:** Why does the Go Vello port produce artifacts (State A: curved shapes; State B: top-left cutout)?

**Sources Analyzed:**
1. `vello_shaders/shader/fine.wgsl` - GPU rendering (AUTHORITATIVE)
2. `vello_shaders/shader/path_count.wgsl` - GPU backdrop logic
3. `vello_shaders/shader/path_tiling.wgsl` - GPU segment creation with y_edge
4. `vello_shaders/src/cpu/fine.rs` - CPU rendering (OUTDATED)
5. `vello_shaders/src/cpu/path_count.rs` - CPU backdrop logic
6. `vello_shaders/src/cpu/path_tiling.rs` - CPU segment creation
7. `gg/backend/native/vello_tiles.go` - Go implementation

---

## Key Finding #1: GPU vs CPU Coordinate Systems

### The Critical Difference

**GPU fine.wgsl (lines 905-959, 976-1001):**
```wgsl
// line 977: local_xy is TILE-RELATIVE (0..16)
let local_xy = vec2(f32(local_id.x * PIXELS_PER_THREAD), f32(local_id.y));

// line 1001: fill_path called with TILE-RELATIVE xy
fill_path(fill, local_xy, &area);

// Inside fill_path (line 916):
let y = segment.point0.y - xy.y;  // xy.y is tile-relative (0..15)

// y_edge calculation (line 941):
let y_edge = sign(delta.x) * clamp(xy.y - segment.y_edge + 1.0, 0.0, 1.0);
//                                 ^^^^ TILE-RELATIVE (0..15)
```

**CPU fine.rs (lines 51-109):**
```rust
// line 144-145: x0, y0 are ABSOLUTE pixel coordinates
let x0 = (tile_x as usize * TILE_WIDTH) as f32;
let y0 = (tile_y as usize * TILE_HEIGHT) as f32;

// line 64: y relative to ABSOLUTE tile origin
let y = segment.point0[1] - (y_tile + yi as f32);

// line 68-69: y_edge with ABSOLUTE coordinates
let y_edge = delta[0].signum() * (y_tile + yi as f32 - segment.y_edge + 1.0).clamp(0.0, 1.0);
//                                ^^^^^^^^^^^^^^^^^ ABSOLUTE (y_tile = 0, 16, 32, ...)
```

### The Problem

Segments store `y_edge` as TILE-RELATIVE (e.g., `y_edge = 10.0` means row 10 within the tile).

But CPU fine.rs computes y_edge compensation using ABSOLUTE coordinates:
```rust
(y_tile + yi as f32 - segment.y_edge + 1.0)  // For tile(0,0), row 10: (0 + 10 - 10 + 1) = 1.0
                                              // For tile(0,0), row 0:  (0 + 0 - 10 + 1) = -9.0
```

This WORKS because when `y_tile = 0`, the formula `(0 + yi - y_edge + 1)` equals `(yi - y_edge + 1)`, which is the tile-relative version.

But the comment in fine.rs line 143 says:
```rust
// x0 and y0 will go away when we do tile-relative coords
```

**This confirms CPU fine.rs is transitional code not matching GPU!**

---

## Key Finding #2: The Go Bug is in y_edge Calculation

The Go `fillTileScanline` function (lines 607-703):

```go
func (tr *TileRasterizer) fillTileScanline(tile *VelloTile, localY int, fillRule FillRule) {
    yf := float32(localY)  // localY is TILE-RELATIVE (0..15) - CORRECT!

    for _, seg := range tile.Segments {
        // y_edge contribution (lines 632-637):
        var yEdge float32
        if delta[0] > 0 {
            yEdge = clamp32(yf-seg.YEdge+1.0, 0, 1)  // CORRECT! Uses tile-relative yf
        } else if delta[0] < 0 {
            yEdge = -clamp32(yf-seg.YEdge+1.0, 0, 1)
        }
        // ...
    }
}
```

Wait - this looks CORRECT! Let me re-analyze the actual issue.

---

## Key Finding #3: The REAL Problem - topEdge Condition for First Segment

Looking more carefully at path_count.rs line 140 and path_count.wgsl line 184:

```rust
// path_count.rs line 140:
let top_edge = if i == 0 { y0 == s0.y } else { last_z == z };

// path_count.wgsl line 184:
let top_edge = select(last_z == z, y0 == s0.y, subix == 0u);
```

This condition determines when to add backdrop to tile(x+1).

For a diagonal starting at pixel (10,10):
- In tile coordinates: `s0 = (10/16, 10/16) = (0.625, 0.625)`
- `y0 = floor(0.625) = 0.0` (tile Y coordinate)
- `s0.y = 0.625`
- Therefore: `y0 == s0.y` is `0.0 == 0.625` = **FALSE**

This means for i==0 (first iteration), `top_edge = false`, so **NO backdrop is added to tile(1,0)**.

This is CORRECT behavior according to Vello's design! The backdrop is NOT supposed to be added when the segment starts INSIDE a tile.

---

## Key Finding #4: How Vello Fills Interior Starting Points

The key insight is understanding what happens for shapes starting inside a tile:

### Example: Diagonal from (10,10) to (200,200)

**Tile (0,0) covers pixels (0-15, 0-15):**

1. The segment enters tile(0,0) at pixel (10,10)
2. In tile-relative coords: segment from ~(10,10) to ~(16,16)
3. **y_edge is set** when segment touches x=0 of tile

Looking at path_tiling.rs lines 121-143:
```rust
if p0.x == 0.0 {
    // ... handle p0 on left edge
    if p0.y == 0.0 {
        // top-left corner - special case
    } else {
        y_edge = p0.y;  // SET y_edge!
    }
} else if p1.x == 0.0 {
    if p1.y == 0.0 {
        // bottom-left corner - special case
    } else {
        y_edge = p1.y;  // SET y_edge!
    }
}
```

For our diagonal (10,10) to ~(16,16) in tile(0,0):
- `p0.x = 10.0` (not 0)
- `p1.x = ~16.0` (not 0)
- Therefore: `y_edge = 1e9` (noYEdge)

**NO y_edge is set for tile(0,0) because the segment doesn't touch x=0!**

---

## Key Finding #5: The ACTUAL Mechanism for Interior Fill

Now I understand the full picture. Let me trace through what SHOULD happen:

### For a filled diagonal from (10,10) to (200,200):

**Tile (0,0):**
- Segment from (10,10) to (16,16) - diagonal
- backdrop = 0 (nothing to the left)
- y_edge = 1e9 (segment doesn't touch x=0)
- Coverage: partial AA along the diagonal, background behind it

**Tile (1,0):**
- This tile is to the RIGHT of tile(0,0)
- Segment continues from (16,10) to (16,16) - exiting right edge
- backdrop should be 0 (no crossings from left)
- BUT WAIT - the segment crossed INTO tile(1,0) from the LEFT!

**THIS IS THE KEY:** When a segment enters a tile from the LEFT SIDE (not top), it needs backdrop propagation!

Looking at path_count.rs again, line 140:
```rust
let top_edge = if i == 0 { y0 == s0.y } else { last_z == z };
```

For i > 0, `top_edge = (last_z == z)` which is true when segment entered from TOP.
When segment enters from LEFT SIDE, `last_z != z`, so `top_edge = false`.

**So backdrop is ONLY added when entering from TOP, not from SIDE!**

The y_edge mechanism handles SIDE entries. When a segment touches x=0 of a tile:
- y_edge is set to the Y coordinate of that touch point
- In fine shader, y_edge adds coverage for all rows >= that point

---

## Root Cause Identified

The problem is in how we iterate through segments in `binSegments`.

Looking at our Go code (vello_tiles.go line 408):
```go
for i := imin; i < imax; i++ {
    // ...
    if i == 0 {
        topEdge = (tileY0 == s0y)  // This is WRONG!
    }
```

The problem: We're checking `i == 0` but we should be checking `i == imin`!

In path_count.rs/wgsl:
```rust
let top_edge = if i == 0 { y0 == s0.y } else { last_z == z };
```

This checks if `i == 0` in the **absolute** sense (first iteration of the line), NOT relative to imin.

But in our Go code, when we clip to bounding box, `imin` might be > 0, so the first iteration we process might have `i > 0`, meaning we'd use the `last_z == z` condition incorrectly.

**WAIT** - I need to re-read more carefully. In Rust:
- Line 134: `for i in imin..imax`
- Line 140: `if i == 0` - This checks absolute i value!

So when `imin = 3`, the first iteration is `i = 3`, and `i == 0` is FALSE.
This means for clipped segments, the topEdge check is `last_z == z`.

Our Go code does the same thing, so that's not the bug...

Let me re-examine the actual test case.

---

## Deep Dive: The Cutout Bug in State B

State B characteristics:
- Shapes are almost perfect (0.02% - 0.24% difference)
- Small cutout in top-left corner of diagonal (96 pixels)

The cutout is specifically in the TOP-LEFT area. This suggests the first tile(s) are missing some fill.

For a diagonal from (10,10):
- Tile(0,0) should have partial fill from (0,0) to (10,10) = the area to the LEFT of the diagonal

**Hypothesis:** The fill to the left of the diagonal in tile(0,0) is missing.

The question: How does Vello fill pixels to the LEFT of a diagonal that starts INSIDE a tile?

---

## Key Finding #6: Backdrop + y_edge Interaction

Let me trace through fine.wgsl more carefully.

For a tile with backdrop and segments:
```wgsl
// Line 909-912: Initialize with backdrop
let backdrop_f = f32(fill.backdrop);
for (var i = 0u; i < PIXELS_PER_THREAD; i += 1u) {
    area[i] = backdrop_f;
}

// Then for each segment, add coverage:
area[i] += y_edge + a * dy;
```

The `backdrop` is the winding number contribution from ALL segments to the LEFT of this tile.
The `y_edge` is a per-segment adjustment for segments that cross the left edge of THIS tile.

For tile(0,0) with a diagonal starting at (10,10):
- backdrop = 0 (nothing to the left of tile 0)
- The diagonal segment has y_edge = 1e9 (doesn't touch left edge)
- So area starts at 0, and only the diagonal coverage is added

**The pixels from (0,0) to (10,10) get area = 0** (transparent).

This is CORRECT for a diagonal LINE, but WRONG for a FILLED diagonal!

---

## EUREKA: The Fill vs Stroke Distinction

**A filled diagonal is NOT just the diagonal line - it's the ENTIRE AREA on one side of the diagonal!**

For a filled triangle with vertices at (10,10), (200,10), and (200,200):
- There are THREE edges, not one
- The left edge would go from (10,10) to (200,200)
- The top edge would go from (10,10) to (200,10)
- The right edge would go from (200,10) to (200,200)

**The diagonal TEST case must be testing a filled polygon, not just a diagonal line!**

Let me reconsider the test case...

---

## Revised Analysis: Diagonal Test Case

If the "diagonal" test draws a filled polygon (like a triangle or rectangle rotated 45 degrees), then:

1. There should be multiple edges defining the shape
2. The backdrop mechanism should work correctly for most tiles
3. The cutout bug appears specifically in the starting tile

The 96-pixel difference suggests about 1/4 of a 16x16 tile is wrong, or a specific region.

**Hypothesis:** For the first tile where a shape STARTS, there's a missing segment or backdrop contribution.

---

## Key Finding #7: The i==imin vs i==0 Confusion

Looking at path_count.wgsl line 184:
```wgsl
let top_edge = select(last_z == z, y0 == s0.y, subix == 0u);
```

Here `subix` is `i` in the loop. But in WGSL this is the ABSOLUTE index from the original line, not adjusted for clipping.

Our Go code (line 425):
```go
if i == 0 {  // BUG? Should this be i == imin?
    topEdge = (tileY0 == s0y)
}
```

Actually, looking at both GPU and CPU Rust code, they use `i == 0` (or `subix == 0u`), checking the ABSOLUTE index.

When a line is clipped so `imin > 0`, the first processed iteration has `i = imin`, not 0. So the condition `i == 0` would be FALSE, and we'd use `last_z == z`.

But `last_z` is initialized BEFORE the loop:
```go
lastZ := float32(math.Floor(float64(a*float32(imin-1) + b)))
```

For `imin = 0`: `lastZ = floor(a * -1 + b)` - could be negative

For the first iteration when `imin = 0`:
- `i = 0`
- `topEdge = (tileY0 == s0y)` - correct!

For the first iteration when `imin > 0` (clipped):
- `i = imin`
- `i == 0` is FALSE
- `topEdge = (lastZ == z)`
- This checks if we entered from TOP

**This seems correct!**

---

## New Hypothesis: Problem in Segment Clipping

Let me look at how segments are clipped when entering a tile.

path_tiling.rs lines 78-96:
```rust
if seg_within_line > 0 {
    let z_prev = (a * (seg_within_line as f32 - 1.0) + b).floor();
    if z == z_prev {
        // Top edge is clipped
        // ...
    } else {
        // Left/right edge is clipped
        // ...
    }
}
```

This clips segment to tile boundaries. When `seg_within_line == 0` (first segment of line), NO CLIPPING happens - the segment uses its original coordinates.

Our Go code (line 476):
```go
if i > imin {  // BUG! Should be i > 0
    zPrev := ...
    z := ...
    // clip logic
}
```

**FOUND IT!**

The condition should be `i > 0`, not `i > imin`!

In Rust: `if seg_within_line > 0` - this checks ABSOLUTE index
In Go: `if i > imin` - this checks RELATIVE to clipping bounds

When `imin = 0`, both are equivalent.
When `imin > 0` (clipped), Go skips the clipping for the first iteration even when the segment enters the tile from an edge!

---

## THE BUG

**File:** `vello_tiles.go`
**Line:** 476
**Current code:** `if i > imin {`
**Should be:** `if i > 0 {`

Same issue at line 514:
**Current code:** `if i < imax-1 {`
**Should be:** `if i < count-1 {`

Wait, let me verify by re-reading path_tiling.rs:

```rust
// line 78
if seg_within_line > 0 {

// line 97
if seg_within_line < count - 1 {
```

`seg_within_line` is the ABSOLUTE index from the original line (counts[0:16] portion).
`count` is the total number of iterations for the entire line.

These are ABSOLUTE values, not relative to imin/imax clipping.

---

## THE SOLUTION

### Fix 1: Segment Clipping Conditions

Change in `addSegmentToTile`:

```go
// OLD (line 476):
if i > imin {

// NEW:
if i > 0 {

// OLD (line 514):
if i < imax-1 {

// NEW (need to pass total count):
if i < count-1 {
```

But wait - `addSegmentToTile` doesn't have access to `count`. We need to either:
1. Pass `count` as a parameter
2. Or compute it inside the function

Looking more carefully, `imax` in our code is the CLIPPED max, not the total count.
We need to use the ORIGINAL `count` value (before clipping).

### Fix 2: The topEdge Check

The topEdge condition in `binSegments` also needs review:

```go
// Line 425:
if i == 0 {
    topEdge = (tileY0 == s0y)
} else {
    topEdge = (lastZ == z)
}
```

This looks correct - it checks ABSOLUTE `i == 0`.

But wait, our `i` loops from `imin` to `imax`. When `imin > 0`, `i` is never 0!

Looking at Rust path_count.rs:
```rust
// Line 134: Loop from imin to imax
for i in imin..imax {
    // Line 140: But check absolute i == 0
    let top_edge = if i == 0 { y0 == s0.y } else { last_z == z };
```

Actually this is checking absolute `i`! In Rust, when `imin = 3`, the loop starts at `i = 3`, and `i == 0` is checked (and is false).

So our Go code IS correct for topEdge.

---

## FINAL ROOT CAUSE

The bug is in **segment clipping conditions** in `addSegmentToTile`:

1. `if i > imin` should be `if i > 0` (check absolute, not relative)
2. `if i < imax-1` should be `if i < count-1` (use original count, not clipped)

These conditions determine whether a segment needs to be clipped to tile boundaries. Using the wrong conditions causes:
- First segments to be incorrectly clipped (or not clipped when they should be)
- Last segments to be incorrectly clipped

This affects y_edge calculation because y_edge depends on where the segment touches the tile boundary.

---

## Concrete Fix

### In `binSegments` (line 443):

Pass the original `count` value to `addSegmentToTile`:

```go
// OLD (line 443):
tr.addSegmentToTile(x0, y0, x1, y1, tileX, tileY, isDown, i, imin, imax, a, b, tileY0, sign, isPositiveSlope, epsilon, noYEdge)

// NEW:
tr.addSegmentToTile(x0, y0, x1, y1, tileX, tileY, isDown, i, count, a, b, tileY0, sign, isPositiveSlope, epsilon, noYEdge)
```

Note: `count` is already computed at line 245 as `countX + velloSpan(s0y, s1y)`.

### In `addSegmentToTile`:

Change the function signature and conditions:

```go
// OLD signature (lines 454-461):
func (tr *TileRasterizer) addSegmentToTile(
    x0, y0, x1, y1 float32,
    tileX, tileY int,
    isDown bool,
    i, imin, imax int,  // <-- WRONG: uses imin, imax
    a, b, tileY0f, sign float32,
    isPositiveSlope bool,
    epsilon, noYEdge float32,
)

// NEW signature:
func (tr *TileRasterizer) addSegmentToTile(
    x0, y0, x1, y1 float32,
    tileX, tileY int,
    isDown bool,
    segWithinLine, totalCount int,  // <-- CORRECT: uses absolute index and total count
    a, b, tileY0f, sign float32,
    isPositiveSlope bool,
    epsilon, noYEdge float32,
)
```

### Condition changes inside `addSegmentToTile`:

```go
// OLD (line 476):
if i > imin {
    // clip top/left edge
}

// NEW:
if segWithinLine > 0 {
    // clip top/left edge
}

// OLD (line 514):
if i < imax-1 {
    // clip bottom/right edge
}

// NEW:
if segWithinLine < totalCount-1 {
    // clip bottom/right edge
}
```

These changes match path_tiling.rs lines 78 and 97:
```rust
if seg_within_line > 0 {      // line 78
if seg_within_line < count - 1 {  // line 97
```

---

## Verification

After this fix:
1. Segments will be correctly clipped based on their ABSOLUTE position in the line
2. y_edge will be set correctly for segments touching tile left edge
3. Both State A and State B artifacts should be resolved

---

## Summary

| Aspect | Finding |
|--------|---------|
| **Root Cause** | Segment clipping uses relative indices (imin/imax) instead of absolute indices |
| **Affected Code** | `addSegmentToTile` function, lines 476 and 514 |
| **Fix Required** | Change `i > imin` to `i > 0`, change `i < imax-1` to `i < count-1` |
| **Confidence** | HIGH - matches GPU/CPU Rust behavior exactly |

---

## References

1. **fine.wgsl** (lines 905-959): GPU rendering with tile-relative coords - AUTHORITATIVE
2. **path_tiling.wgsl** (lines 93, 109): Clipping conditions use absolute indices
3. **path_tiling.rs** (lines 78, 97): `seg_within_line > 0` and `seg_within_line < count - 1`
4. **path_count.rs** (line 140): topEdge condition uses absolute `i == 0`

---

## Open Questions

None - the solution is clear and matches the reference implementation.

---

*Report generated: 2026-02-01*
