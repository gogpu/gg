# Vello Rust vs Go Implementation Comparison

**Date:** 2026-02-01
**Issue:** Circle r=80 artifact - extra fill at top-right (53 pixels, 0.13% diff)
**Severity correlation:** Proximity to bbox top edge (top Y=20 -> 56px diff, top Y=120 -> 3px diff)

---

## Executive Summary

Found the EXACT Rust reference code that uses 16x16 tiles and y_edge mechanism:
- `vello_shaders/src/cpu/path_tiling.rs` - Segment binning with y_edge calculation
- `vello_shaders/src/cpu/fine.rs` - Fill path coverage computation

**The Go port is a correct port of this code.** The artifact is likely caused by a subtle difference in how we handle the first segment in a tile (i == 0) when determining entry points.

---

## 1. Reference Architecture (16x16 tiles with y_edge)

### 1.1 Rust Vello has TWO CPU renderers:

| Renderer | Tile Size | y_edge | Files |
|----------|-----------|--------|-------|
| **sparse_strips** | 4x4 | NO | `vello_common/src/{tile,strip}.rs` |
| **cpu shaders** | 16x16 | YES | `vello_shaders/src/cpu/{path_tiling,fine}.rs` |

Our Go port is based on the **cpu shaders** architecture (16x16 tiles, y_edge).

### 1.2 Data Flow (matching our port)

```
LineSoup -> path_tiling_main() -> PathSegment[] with y_edge
                                       |
                                       v
                               fill_path() in fine.rs
                                       |
                                       v
                               area[] coverage buffer
```

---

## 2. y_edge Calculation: Side-by-Side Comparison

### 2.1 Rust (path_tiling.rs lines 116-144)

```rust
let mut y_edge = 1e9;
// Apply numerical robustness logic
let mut p0 = xy0 - tile_xy;
let mut p1 = xy1 - tile_xy;
const EPSILON: f32 = 1e-6;
if p0.x == 0.0 {
    if p1.x == 0.0 {
        p0.x = EPSILON;
        if p0.y == 0.0 {
            // Entire tile
            p1.x = EPSILON;
            p1.y = TILE_HEIGHT as f32;
        } else {
            // Make segment disappear
            p1.x = 2.0 * EPSILON;
            p1.y = p0.y;
        }
    } else if p0.y == 0.0 {
        p0.x = EPSILON;
    } else {
        y_edge = p0.y;  // <-- SET y_edge when p0 at left edge (not top-left corner)
    }
} else if p1.x == 0.0 {
    if p1.y == 0.0 {
        p1.x = EPSILON;
    } else {
        y_edge = p1.y;  // <-- SET y_edge when p1 at left edge (not top-left corner)
    }
}
```

### 2.2 Go Port (vello_tiles.go lines 600-622)

```go
if p0x == 0.0 {
    if p1x == 0.0 {
        p0x = epsilon
        if p0y == 0.0 {
            p1x = epsilon
            p1y = tileH
        } else {
            p1x = 2.0 * epsilon
            p1y = p0y
        }
    } else if p0y == 0.0 {
        p0x = epsilon
    } else {
        yEdge = p0y  // <-- MATCHES RUST
    }
} else if p1x == 0.0 {
    if p1y == 0.0 {
        p1x = epsilon
    } else {
        yEdge = p1y  // <-- MATCHES RUST
    }
}
```

**VERDICT:** The y_edge setting logic is IDENTICAL.

---

## 3. Segment Clipping: Side-by-Side Comparison

### 3.1 Rust Clipping (path_tiling.rs lines 78-115)

```rust
if seg_within_line > 0 {
    let z_prev = (a * (seg_within_line as f32 - 1.0) + b).floor();
    if z == z_prev {
        // Top edge is clipped
        let mut xt = xy0.x + (xy1.x - xy0.x) * (tile_xy.y - xy0.y) / (xy1.y - xy0.y);
        xt = xt.clamp(tile_xy.x + 1e-3, tile_xy1.x);
        xy0 = Vec2::new(xt, tile_xy.y);
    } else {
        // Left/right edge is clipped
        let x_clip = if is_positive_slope { tile_xy.x } else { tile_xy1.x };
        let mut yt = xy0.y + (xy1.y - xy0.y) * (x_clip - xy0.x) / (xy1.x - xy0.x);
        yt = yt.clamp(tile_xy.y + 1e-3, tile_xy1.y);
        xy0 = Vec2::new(x_clip, yt);
    }
}
```

### 3.2 Go Port Clipping (vello_tiles.go lines 503-541)

```go
if i > imin {
    zPrev := float32(math.Floor(float64(a*float32(i-1) + b)))
    z := float32(math.Floor(float64(a*float32(i) + b)))
    if z == zPrev {
        // Top edge is clipped
        if y1 != y0 {
            t := (tileTopY - y0) / (y1 - y0)
            xt := x0 + (x1-x0)*t
            if xt < tileLeftX+1e-3 { xt = tileLeftX + 1e-3 }
            if xt > tileRightX { xt = tileRightX }
            xy0x, xy0y = xt, tileTopY
        }
    } else {
        // Left or right edge is clipped
        var xClip float32
        if isPositiveSlope { xClip = tileLeftX } else { xClip = tileRightX }
        if x1 != x0 {
            t := (xClip - x0) / (x1 - x0)
            yt := y0 + (y1-y0)*t
            if yt < tileTopY+1e-3 { yt = tileTopY + 1e-3 }
            if yt > tileBotY { yt = tileBotY }
            xy0x, xy0y = xClip, yt
        }
    }
}
```

**VERDICT:** Logic matches. BUT there's a subtle difference:

| Aspect | Rust | Go |
|--------|------|-----|
| Condition | `seg_within_line > 0` | `i > imin` |
| Clamp min | `tile_xy.x + 1e-3` | `tileLeftX + 1e-3` |

The `i > imin` vs `seg_within_line > 0` should be equivalent since:
- Rust: `seg_within_line` is the index within the line's tile traversal
- Go: `i` is the DDA iteration index, `imin` is the starting iteration

---

## 4. Fill Path Coverage: Side-by-Side Comparison

### 4.1 Rust (fine.rs lines 51-109)

```rust
fn fill_path(area: &mut [f32], segments: &[PathSegment], fill: &CmdFill, x_tile: f32, y_tile: f32) {
    let backdrop_f = fill.backdrop as f32;
    for a in area.iter_mut() { *a = backdrop_f; }

    for segment in &segments[..] {
        let delta = [
            segment.point1[0] - segment.point0[0],
            segment.point1[1] - segment.point0[1],
        ];
        for yi in 0..TILE_HEIGHT {
            let y = segment.point0[1] - (y_tile + yi as f32);
            let y0 = y.clamp(0.0, 1.0);
            let y1 = (y + delta[1]).clamp(0.0, 1.0);
            let dy = y0 - y1;
            let y_edge =
                delta[0].signum() * (y_tile + yi as f32 - segment.y_edge + 1.0).clamp(0.0, 1.0);
            if dy != 0.0 {
                // ... trapezoidal area calculation
                for i in 0..TILE_WIDTH {
                    area[yi * TILE_WIDTH + i] += y_edge + a * dy;
                }
            } else if y_edge != 0.0 {
                for i in 0..TILE_WIDTH {
                    area[yi * TILE_WIDTH + i] += y_edge;
                }
            }
        }
    }
    // Apply fill rule...
}
```

### 4.2 Go Port (vello_tiles.go lines 647-743)

```go
func (tr *TileRasterizer) fillTileScanline(tile *VelloTile, localY int, fillRule FillRule) {
    backdropF := float32(tile.Backdrop)
    for i := 0; i < VelloTileWidth; i++ { tr.area[i] = backdropF }

    yf := float32(localY)
    for _, seg := range tile.Segments {
        delta := [2]float32{
            seg.Point1[0] - seg.Point0[0],
            seg.Point1[1] - seg.Point0[1],
        }
        y := seg.Point0[1] - yf
        y0 := clamp32(y, 0, 1)
        y1 := clamp32(y+delta[1], 0, 1)
        dy := y0 - y1

        var yEdge float32
        if delta[0] > 0 {
            yEdge = clamp32(yf-seg.YEdge+1.0, 0, 1)
        } else if delta[0] < 0 {
            yEdge = -clamp32(yf-seg.YEdge+1.0, 0, 1)
        }

        if dy != 0 {
            // ... trapezoidal area calculation
            for i := 0; i < VelloTileWidth; i++ {
                tr.area[i] += yEdge + a*dy
            }
        } else if yEdge != 0 {
            for i := 0; i < VelloTileWidth; i++ {
                tr.area[i] += yEdge
            }
        }
    }
    // Apply fill rule...
}
```

**CRITICAL DIFFERENCE FOUND!**

### Rust y_edge calculation:
```rust
let y_edge = delta[0].signum() * (y_tile + yi as f32 - segment.y_edge + 1.0).clamp(0.0, 1.0);
```

### Go y_edge calculation:
```go
if delta[0] > 0 {
    yEdge = clamp32(yf-seg.YEdge+1.0, 0, 1)
} else if delta[0] < 0 {
    yEdge = -clamp32(yf-seg.YEdge+1.0, 0, 1)
}
```

The difference:
- **Rust:** Uses `y_tile + yi` as the current Y coordinate (y_tile is the tile's top-left Y, yi is 0..15)
- **Go:** Uses `yf` which is `localY` (0..15) WITHOUT adding the tile's Y offset

**But wait** - in the Rust code, `y_tile` is passed in as `y0 = (tile_y as usize * TILE_HEIGHT) as f32`, which is the PIXEL coordinate of the tile's top edge. Then `y_tile + yi` gives the absolute pixel Y.

In Go, we process scanline by scanline, where `yf = float32(localY)` is 0..15 (relative to tile).

**HYPOTHESIS:** The Rust code uses ABSOLUTE pixel coordinates for y_edge comparison, while Go uses TILE-RELATIVE coordinates.

Let me verify by tracing through an example...

Actually, looking more carefully at the Rust code:
```rust
let y_edge = delta[0].signum() * (y_tile + yi as f32 - segment.y_edge + 1.0).clamp(0.0, 1.0);
```

And in Rust `fill_path`, `y_tile` is passed as the tile's top-left Y in PIXEL coordinates.
But `segment.y_edge` is in TILE-RELATIVE coordinates (0..16).

So the calculation `y_tile + yi - segment.y_edge + 1.0` is mixing absolute and relative coordinates!

Wait, let me re-read path_tiling.rs:
```rust
let mut y_edge = 1e9;
// ...
y_edge = p0.y;  // p0 is relative to tile_xy
```

Yes, `y_edge` is TILE-RELATIVE (0..16).

And in fine.rs:
```rust
let y_edge = delta[0].signum() * (y_tile + yi as f32 - segment.y_edge + 1.0).clamp(0.0, 1.0);
```

Where `y_tile` is absolute pixel Y, `yi` is 0..15, and `segment.y_edge` is tile-relative.

So: `y_tile + yi - segment.y_edge + 1.0` = `(absolute Y of current pixel) - (tile-relative y_edge) + 1.0`

This is WRONG in the Rust code! It's mixing coordinate systems.

**BUT** - if `y_tile` is always 0.0 (as in our Go code where we process scanline 0..15 within a tile), then:
- Rust: `0 + yi - y_edge + 1.0`
- Go: `yi - y_edge + 1.0`

These MATCH when `y_tile = 0`.

Looking at fine.rs again:
```rust
fn fill_path(area: &mut [f32], segments: &[PathSegment], fill: &CmdFill, x_tile: f32, y_tile: f32)
// Called with:
let x0 = (tile_x as usize * TILE_WIDTH) as f32;
let y0 = (tile_y as usize * TILE_HEIGHT) as f32;
fill_path(&mut area, segments, &fill, x0, y0);
```

So Rust passes the ABSOLUTE pixel coordinates. But then:
```rust
let y = segment.point0[1] - (y_tile + yi as f32);
```

Wait, `segment.point0` is TILE-RELATIVE (0..16 range). So subtracting `(y_tile + yi)` where `y_tile` is absolute would give negative values!

No wait - let me trace through more carefully. In path_tiling.rs:
```rust
let tile_xy = Vec2::new(x as f32 * TILE_WIDTH as f32, y as f32 * TILE_HEIGHT as f32);
// ...
let mut p0 = xy0 - tile_xy;  // Convert to tile-relative
```

Yes, `p0` and `p1` are tile-relative. So `segment.point0[1]` is in range 0..16.

Then in fine.rs:
```rust
let y = segment.point0[1] - (y_tile + yi as f32);
```

If `y_tile` = 160.0 (absolute pixel Y for tile row 10), and `segment.point0[1]` = 5.0 (tile-relative):
`y = 5.0 - (160.0 + 0) = -155.0`

That would be WRONG!

**CONCLUSION:** Either I'm misreading the Rust code, or there's a bug in Vello's reference implementation.

Let me check if fine.rs is even used...

---

## 5. INVESTIGATION: Is fine.rs Used?

Looking at fine.rs:
```rust
#[expect(unused, reason = "Draft code as textures not wired up")]
fn fine_main(...)
```

**The fine_main function is UNUSED draft code!**

This means the `fill_path` function in fine.rs is reference/draft code, NOT the actual production implementation.

The ACTUAL production code is in `sparse_strips/vello_cpu/` which uses 4x4 tiles.

---

## 6. REAL Reference: sparse_strips

The sparse_strips implementation in `vello_common/src/strip.rs` does NOT use y_edge at all!

It uses:
1. **Tiles (4x4)** with intersection masks (T/B/L/R flags)
2. **Winding accumulation** via `winding_delta` and `accumulated_winding`
3. **Trapezoidal area** calculation per pixel

Our Go port uses a HYBRID approach:
- 16x16 tiles (from the unused fine.rs)
- y_edge mechanism (from the unused fine.rs)
- But different segment clipping logic

---

## 7. ROOT CAUSE HYPOTHESIS

The artifact is caused by our use of a DRAFT algorithm (fine.rs y_edge) that was never production-tested in Rust Vello.

The production Vello (sparse_strips) uses a completely different approach:
1. Smaller 4x4 tiles for finer granularity
2. No y_edge - uses per-pixel trapezoidal area calculation
3. Winding delta tracked via tile intersection flags

---

## 8. RECOMMENDED FIX OPTIONS

### Option A: Fix y_edge Logic (incremental)

Review the y_edge calculation coordinate systems:
- Ensure segment.YEdge is compared against tile-relative Y, not absolute Y
- Add bounds checking for edge cases near tile boundaries

### Option B: Switch to strip.rs Algorithm (major refactor)

Port the production sparse_strips algorithm:
1. Use 4x4 tiles
2. Track intersection masks per tile
3. Use winding_delta + accumulated_winding
4. Trapezoidal area per pixel

### Option C: Hybrid Fix (recommended)

Keep 16x16 tiles but fix the area calculation:
1. Remove y_edge mechanism entirely
2. Use the per-pixel trapezoidal area from strip.rs
3. Track winding via backdrop prefix sum (already implemented)

---

## 9. Key Files for Further Investigation

### Rust Reference (PRODUCTION):
- `D:\projects\gogpu\reference\rust-2d\vello\sparse_strips\vello_common\src\strip.rs` - Production fill algorithm
- `D:\projects\gogpu\reference\rust-2d\vello\sparse_strips\vello_common\src\tile.rs` - Tile generation

### Rust Reference (DRAFT, basis for our port):
- `D:\projects\gogpu\reference\rust-2d\vello\vello_shaders\src\cpu\fine.rs` - UNUSED draft code
- `D:\projects\gogpu\reference\rust-2d\vello\vello_shaders\src\cpu\path_tiling.rs` - Segment tiling

### Go Port:
- `D:\projects\gogpu\gg\backend\native\vello_tiles.go` - Our implementation

---

## 10. Immediate Action Items

1. **Debug the exact segment causing the artifact:**
   - Add logging to track segments in tile containing (160, 20)
   - Print y_edge values and their contribution to area[]

2. **Test removing y_edge entirely:**
   - Set yEdge = 0 always in fillTileScanline
   - See if artifact disappears (and what breaks)

3. **Consider porting strip.rs algorithm:**
   - The production algorithm is battle-tested
   - Uses fundamentally different (and correct) approach

---

## 11. DEFINITIVE PROOF: fine.rs is BROKEN

### The Bug in fine.rs

From path_tiling.rs line 160:
```rust
assert!(p0.x >= 0.0 && p0.x <= TILE_WIDTH as f32);
```

This PROVES `segment.point0` and `segment.point1` are TILE-RELATIVE (0..16 range).

But in fine.rs line 64:
```rust
let y = segment.point0[1] - (y_tile + yi as f32);
```

If `segment.point0[1] = 5.0` (tile-relative) and `y_tile = 160.0` (absolute pixel Y):
```
y = 5.0 - 160.0 = -155.0  // WRONG!
```

**The fine.rs code mixes tile-relative and absolute coordinates. It's BROKEN.**

### Why Our Port Appears to Work

Our Go port inadvertently "fixed" this by using only tile-relative coordinates:
- We use `yf = float32(localY)` where localY is 0..15
- Segment points are stored tile-relative

So our calculation `y = seg.Point0[1] - yf` is CORRECT.

But we inherited other problematic patterns from the draft code.

---

## 12. FINAL RECOMMENDATION

### Port the PRODUCTION Algorithm (strip.rs)

The sparse_strips implementation is:
1. Actually used in production
2. Battle-tested
3. Uses sound coordinate handling

### Key Changes Required:

1. **Use 4x4 tiles** (or adapt algorithm for 16x16)
2. **Remove y_edge** - use trapezoidal area accumulation instead
3. **Track winding via intersection flags** (T/B/L/R bits)
4. **Use accumulated_winding** propagation

### Immediate Workaround:

For the current artifact, try:
1. Disable y_edge entirely (`yEdge = 0` always)
2. Rely on backdrop + trapezoidal area only
3. See what breaks and iterate

---

## 13. GitHub Issues Search Results

### No direct issues found for this artifact

Searched linebender/vello issues for:
- Circle fill artifacts
- y_edge bugs
- backdrop propagation issues

**Key findings from issues:**

1. **Issue #303 (Stroke Rework) - CLOSED**: Mentions replacing y_edge with "test that x0 or x1 = 0" as part of numerical robustness work. This confirms y_edge was known to be problematic.

2. **Issue #49 (Conflation Artifacts) - OPEN**: Different problem (boundary artifacts between shapes), not related to our issue.

3. **Issue #592 (Anti-aliasing Quality)**: About jagged edges, not fill artifacts.

4. **Issue #670 (Sparse Strips)**: Describes the NEW algorithm that replaces fine.rs. Uses completely different approach.

**Conclusion:** The y_edge problems in fine.rs were never fixed - instead, Vello team rewrote the entire algorithm (sparse_strips) without y_edge!

---

## 14. DEFINITIVE FINDING: sparse_strips Algorithm

### How sparse_strips replaces y_edge

In `strip.rs`:

```rust
// Line 322 - Winding tracked via TOP edge intersection, not left edge!
winding_delta += sign as i32 * i32::from(tile.winding());

// tile.winding() = bit 5 of intersection_mask (TOP edge crossing)
```

### Key differences:

| Aspect | fine.rs (our port) | sparse_strips (production) |
|--------|-------------------|---------------------------|
| Winding source | Left edge (y_edge) | **Top edge** (winding bit) |
| Mechanism | Float compensation | **Integer delta counter** |
| Tile size | 16x16 | **4x4** |
| Area calc | Per-row y_edge add | **Per-pixel trapezoid** |

### Why sparse_strips works:

1. **Winding from TOP edge**: Each tile tracks if line crosses TOP - this is used for winding_delta propagation across the row.

2. **No y_edge needed**: Because winding is tracked via top-edge crossings, there's no need for left-edge compensation.

3. **Smaller tiles (4x4)**: Reduces error accumulation, more precise intersections.

---

## 15. RECOMMENDED FIX

### Option 1: Port winding from top edge (recommended)

Instead of y_edge (left edge), track winding via TOP edge intersection:

```go
// In binSegments, instead of setting yEdge for left-edge segments,
// track which segments cross the TOP edge of each tile.
// Use this for winding_delta in fillTileScanline.
```

### Option 2: Port sparse_strips algorithm

Full rewrite to use 4x4 tiles + boundary fragment merge. High effort but battle-tested.

### Option 3: Fix y_edge coordinate handling

The y_edge logic may have subtle coordinate bugs. Debug the specific case of circle r=80 at top=20 to find the exact error.

---

## 16. DEEP DEBUG SESSION (2026-02-01)

### Artifact Location

Circle r=80 at (100,100). Top of circle at y=20.
Artifact pixels: x=112-127, y=20-24 (Tile 7,1)

### Key Debug Finding: Backdrop Distribution

```
=== Backdrop BEFORE Prefix Sum (All Rows) ===
Row 1 (y=16-31): EMPTY!  <-- No backdrop at all for artifact row!
Row 2 (y=32-47): T4=-1 T9=+1
Row 3 (y=48-63): T3=-1 T11=+1
...
```

**Row 1 has NO backdrop** because no segments cross the TOP edge of row 1 (y=16).
The circle starts at y=20, which is INSIDE row 1.

### Root Cause Chain

1. **Circle top (y=20) is INSIDE row 1 (y=16-31)**
2. **No segments cross y=16** → no backdrop delta for row 1
3. **But segments DO touch left edge of tiles** at y≈20 → yEdge is set
4. **yEdge is negative** (dx < 0 for right side of circle)
5. **backdrop=0 + yEdge=-1** = area=-1
6. **abs(-1) = 1** → alpha=255 → **ARTIFACT!**

### Tile (7,1) Analysis

```
Tile (7,1) pixels=(112-127, 16-31) backdrop=0 segs=4
  Seg[0]: (0.45,4.50)->(0.00,4.48) dx=-0.455 dy=-0.023
         YEdge=4.477 (contributes to rows >= 4)
         Row 4 (y=20): yEdge=-0.523
         Row 5 (y=21): yEdge=-1.000  <-- segment doesn't cross row 5, but yEdge still applied!
         ...
```

The segment touches left edge at y=4.48 (tile-relative) = pixel y=20.48.
But yEdge contribution is applied to ALL rows below, even though segment doesn't cross them!

### Attempted Fix #1: Only apply yEdge when backdrop != 0

```go
} else if yEdge != 0 && backdropF != 0 {
```

**Result:** Made circles WORSE!
- circle_r7: 0.5% → 6.50%
- circle_r80: 0.13% → 2.81%

**Reason:** This breaks cases where yEdge is legitimately needed inside the shape.

### The Real Problem

The backdrop computation is fundamentally flawed for shapes that START inside a tile row:

1. Backdrop deltas go to `bboxMinX` (leftmost tile of path bbox)
2. Left and right segments CANCEL each other out at bboxMinX
3. For rows where shape STARTS (not enters from top), backdrop=0
4. But yEdge still applies → creates artifacts

### Potential Solutions

| Solution | Effort | Risk | Notes |
|----------|--------|------|-------|
| Fix backdrop to use segment's own tile | Medium | Medium | May break other cases |
| Limit yEdge to segment's actual Y range | Medium | Low | Need per-segment Y bounds |
| Port sparse_strips (no yEdge) | High | Low | Production-tested algorithm |
| Add "shape start row" tracking | Medium | Medium | Skip yEdge for first row of shape |

### Experiment Results Log

| Fix | square | circle_r7 | circle_r80 | diagonal | rect_unaligned |
|-----|--------|-----------|------------|----------|----------------|
| **Original** | 0% | 0.5% | **0.13%** | 0% | 4.69% |
| Fix #1: yEdge only if backdrop!=0 | 0% | 6.50% | 2.81% | 0.24% | 7.03% |
| Fix #2: yEdge only if row in segment Y range | 0% | 6.50% | 4.15% | 0.20% | 6.64% |
| Fix #3: backdrop for i==0 && !topEdge | **6%** | 14.50% | 12.63% | 8.77% | **0%!** |
| Fix #4: only if nearTop/nearBottom | 6% | 14.50% | 6.79% | 8.77% | **0%** |
| Fix #5: only if tileY==bboxMinY | 6% | 13.50% | 2.53% | 2.51% | **0%** |
| Fix #6: only if segY near bounds.MinY | 6% | 13.50% | 2.53% | 2.51% | **0%** |
| Fix #7: segMinY >= yf (wrong condition) | 0% | 6.50% | 4.15% | 0.20% | 6.64% |
| **Fix #8: yf >= floor(seg.YEdge)** | **0%** | **0.50%** | **0.13%** | **0%** | **4.69%** ✓ |

### Fix #8: yf >= floor(seg.YEdge) (APPLIED)

**The fix:** In `fillTileScanline`, only apply yEdge when `yf >= floor(seg.YEdge)`.

```go
} else if yEdge != 0 {
    // Only apply yEdge when current row is at or below segment's YEdge position
    if yf >= float32(int(seg.YEdge)) {
        for i := 0; i < VelloTileWidth; i++ {
            tr.area[i] += yEdge
        }
    }
}
```

**Result:** All golden tests pass, but 53 pixel artifact remains at tile (7,1).

---

## 17. DEEP DEBUG SESSION #2 (2026-02-01)

### Pixel-Level Analysis (TestHuntArtifactPixels)

Created test to trace exact artifact pixels. Key findings:

**Tile (7,1) — 53 artifact pixels at top-right of circle:**
```
Seg[0]: covers X from 0.00 to 0.45 at row 4
Seg[1]: covers X from 0.00 to 0.85 at row 4

Pixel 1 (x=113) simulation:
  Seg[0]: yEdge=-0.5228, area_contrib=-0.5000  ← yEdge APPLIED to pixel OUTSIDE segment range!
  Seg[1]: yEdge=-0.1297, area_contrib=0.0000
  TOTAL: area=-0.5000, abs=0.5000, alpha=127 ← ARTIFACT!
```

**Root Cause:** yEdge is applied to ALL 16 pixels in a row, but segments only cover x=0 to ~0.85.
Pixels at x > 0.85 should NOT get yEdge contribution.

### Additional Fix Attempts

| Fix | Idea | Result |
|-----|------|--------|
| **#9** | Skip yEdge if segmentArea ≈ 0 | ❌ 1125 diff — broke interior fill |
| **#10** | Limit yEdge to segment X range: `iF > xmax0 → skip` | ❌ 229 diff — broke edge correction |
| **#11** | Skip yEdge if backdrop=0 | ❌ 1331 diff — broke edge tiles |
| **#12** | More targeted X range check | ❌ 1740 diff — broke multiple tiles |
| **#13** | Don't apply abs() when backdrop=0 && area<0 | ❌ 2280 diff — broke left edge fill |

### Why All Fixes Failed

The problem is **contextual** — what works for tile (7,1) breaks other tiles:

| Tile | Position | backdrop | yEdge sign | Fill needed? |
|------|----------|----------|------------|--------------|
| (7,1) | Top-RIGHT | 0 | negative | NO (outside circle) |
| (6,1) | Top-CENTER | 0 | negative | YES (apex of circle) |
| (1,6) | LEFT edge | 0 | negative | YES (inside circle) |

For tile (7,1): Negative yEdge + abs() creates spurious fill.
For tile (1,6): Negative yEdge + abs() creates VALID fill!

The difference is that tile (7,1) pixels are OUTSIDE the circle, while tile (1,6) pixels are INSIDE.
But there's no way to know inside/outside without computing the fill first!

### Fundamental Limitation

The Vello fine.rs algorithm applies yEdge to ALL pixels in a row. This assumes the shape spans
the full tile width. For shapes with partial width (like circle edges), this creates artifacts.

**Production Vello (sparse_strips) solves this by:**
1. Using 4x4 tiles (finer granularity)
2. NOT using yEdge at all
3. Tracking winding via TOP edge crossings (not left edge)
4. Per-pixel trapezoidal area calculation

### Current Status

- Fix #8 is applied, all golden tests pass
- 53 pixel artifact (0.13%) remains at tile (7,1)
- Further fixes break other tiles worse than the original artifact
- Artifact is visible but localized to circle top-right corner

### Options

1. **Accept 0.13% limitation** — Visual artifact is small
2. **Port sparse_strips algorithm** — Significant rewrite, but production-tested
3. **Track per-row segment X extent** — Complex, may have other edge cases

---

*Report updated: 2026-02-01*
*Fix #8 applied, 53 pixel artifact remains.*
*Fixes #9-13 all made things worse by breaking interior/edge fill.*
*Root cause: yEdge applied to ALL pixels, but shape doesn't span full tile width.*
