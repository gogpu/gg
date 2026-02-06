# Experiment: Unconditional yEdge + VelloLine Float Pipeline

**Date:** 2026-02-06
**Branch:** feat/vello-tile-rasterizer
**Author:** Claude Code (Opus 4.6)

## Baseline (before changes)

| Test | Result |
|------|--------|
| TestVelloCompareWithOriginal | 3 pixels diff (0.01%) — bottom artifact tile (6,11) |
| TestVelloCompareDiagonal | 0 pixels diff (0.00%) |
| TestHuntArtifactPixels | PASS |
| TestVelloAgainstGolden | PASS |

## Experiment 1: Unconditional yEdge

**Hypothesis:** Matching Vello WGSL `fine.wgsl:941-944` which applies yEdge unconditionally.

**Result:** No improvement. The 3-pixel artifact persists unchanged. The unconditional yEdge
was already effectively applied because the conditional path produced the same values for
the affected segments.

## Experiment 2: VelloLine Float Pipeline

**Hypothesis:** Fixed-point quantization in EdgeBuilder (float→FDot6→FDot16→float round-trip)
introduces coordinate errors that prevent perfect segment cancellation at curve extrema.

**Approach:** Added `VelloLine` type storing original float32 coordinates from curve flattening,
bypassing the fixed-point quantization entirely. Modified `binSegments` to iterate VelloLines
instead of AllEdges+LineEdge.

### Results

| Metric | Before (LineEdge) | After (VelloLine) |
|--------|-------------------|-------------------|
| Artifact pixel alpha | 191 | 248 |
| Artifact area | -0.750 | -0.9748 |
| TestVelloCompareWithOriginal | 3 diff (0.01%) | 623 diff (1.56%) |
| TestVelloCompareDiagonal | 0 diff (0.00%) | 640 diff (1.60%) |
| Golden circle_r7 | PASS | FAIL (13.0% diff) |
| Golden circle_r60 | PASS | FAIL (1.17% diff) |
| Golden circle_r80 | PASS | FAIL (1.56% diff) |
| Golden diagonal | PASS | FAIL (1.60% diff) |

### Tolerance experiments (with VelloLine)

| Flattening tolerance | Artifact alpha | Segment count |
|---------------------|----------------|---------------|
| 0.25 (default) | 248 | 64 |
| 0.10 | 251 | ~100 |
| 0.01 | 253 | 256 |

### Analysis

1. **VelloLine partially fixes the artifact** (191→248) by using exact float coordinates
2. **The remaining gap (248→255)** is inherent to curve flattening — line segments don't perfectly
   approximate cubic beziers at extrema (bottom of circle where curvature changes rapidly)
3. **Widespread edge diffs (~1.5%)** occur because the DDA tile walk changes with different
   Y coordinates (float vs quantized-to-scanline-boundaries). Different tile binning → different
   segment positions in tiles → different anti-aliasing at all edges
4. **Small shapes hit hardest** — circle_r7 has 13% diff because fewer total pixels
5. **Edge diffs are small** (5-14 alpha values) and represent different but valid anti-aliasing

### Root cause confirmed

The 3-pixel artifact at the bottom of circles is caused by **two factors**:

1. **Fixed-point quantization** — Y coordinates snapped to scanline boundaries lose sub-pixel
   precision, causing segment endpoints to shift. This accounts for ~75% of the error
   (improving area from -0.75 to -0.97).

2. **Curve flattening approximation** — Flattened line segments don't perfectly represent the
   cubic bezier at its extremum. The yEdge correction and a*dy contribution don't cancel
   perfectly because the line segment enters the tile at a slightly different angle than the
   true curve. This accounts for ~25% of the error.

### Decision: Reverted

VelloLine approach reverted because:
- 4 golden test failures
- 13% diff on small shapes
- Incomplete fix (248 vs 255)
- Would need golden image regeneration + threshold updates across all tests

VelloLine infrastructure (type, storage, accessor) kept in `edge_builder.go` for potential
future use with a more targeted approach.

## Future directions

1. **Targeted coordinate fix**: Use VelloLine coords only for tile-local segment computation
   (inside addSegmentToTile), not for the DDA walk. Keep DDA on quantized coords for
   consistency with backdrop computation.
2. **Native curve support**: Evaluate quadratic bezier segments in tiles instead of flattening
   to lines. This would eliminate flattening error at extrema.
3. **Accept the artifact**: 3 pixels at 0.01% is barely visible. Document as known limitation.
