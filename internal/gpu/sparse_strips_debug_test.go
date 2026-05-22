//go:build !nogpu

package gpu

import (
	"fmt"
	"math"
	"testing"

	"github.com/gogpu/gg/scene"

	"github.com/gogpu/gg/internal/stroke"
)

// pixelWinding holds the winding decomposition for a single pixel column.
type pixelWinding struct {
	backdrop     float32
	segmentArea  float32  // sum of (area*sign + acc) from all segments
	totalWinding float32  // backdrop + segmentArea
	coverage     uint8    // final coverage after clamp + quantize
	segDetails   []string // per-segment contribution details
}

// TestSparseStripsDebug_StrokeSineWave is a diagnostic test that traces exactly
// what happens inside SparseStripsFiller when rasterizing a self-intersecting
// stroke-expanded sine wave. It logs segment contributions, winding accumulation,
// and final coverage at 3 specific pixel columns (x=55, x=100, x=200) to show
// WHERE the extra coverage comes from compared to AnalyticFiller.
//
// The stroke-expanded path is a closed outline around a 2px-wide sine wave.
// Where the sine wave doubles back, the outline self-intersects. With NonZero
// fill rule, the winding number at these intersections can exceed +/-1, but
// coverage is clamped to 1.0. The key question: do tiles OUTSIDE the 2px stroke
// band incorrectly accumulate non-zero backdrop winding from these intersections?
func TestSparseStripsDebug_StrokeSineWave(t *testing.T) {
	const (
		canvasW     = 400
		canvasH     = 500
		lineWidth   = 2.0
		numSegments = 100
	)

	// Phase 1: Build the exact same stroke-expanded sine wave as filler_comparison_test.go.
	sinePath := buildDebugSinePath(numSegments)
	t.Logf("Source sine wave: %d verbs", sinePath.VerbCount())

	// Stroke-expand to get the outline path.
	strokePath := expandSineStroke(sinePath, lineWidth)
	t.Logf("Stroke-expanded path: %d verbs, %d coords",
		len(strokePath.Verbs()), len(strokePath.Points()))

	// Phase 2: Run the SparseStrips pipeline manually so we can inspect internals.
	config := DefaultConfig(canvasW, canvasH)
	config.FillRule = scene.FillNonZero
	ssr := NewSparseStripsRasterizer(config)

	// Flatten.
	ssr.flattenCtx.Reset()
	ssr.flattenCtx.FlattenPathTo(strokePath, scene.IdentityAffine(), FlattenTolerance)
	segments := ssr.flattenCtx.Segments()
	t.Logf("Flattened segments: %d", segments.Len())

	// Coarse rasterize.
	ssr.coarse.Rasterize(segments)
	ssr.coarse.SortEntries()
	backdrop := ssr.coarse.CalculateBackdrop()
	t.Logf("Coarse entries: %d, backdrop size: %d", len(ssr.coarse.Entries()), len(backdrop))

	// Phase 3: For 3 diagnostic columns, trace segments, winding, and coverage.
	diagnosticColumns := []int{55, 100, 200}

	for _, diagX := range diagnosticColumns {
		t.Logf("")
		t.Logf("========================================")
		t.Logf("DIAGNOSTIC COLUMN: x=%d", diagX)
		t.Logf("========================================")

		analyzeColumn(t, diagX, canvasW, canvasH, segments, ssr.coarse, backdrop)
	}

	// Phase 4: Run fine rasterization and report actual coverage map at these columns.
	ssr.fine.SetFillRule(scene.FillNonZero)
	ssr.fine.Rasterize(ssr.coarse, segments, backdrop)
	grid := ssr.fine.Grid()

	t.Logf("")
	t.Logf("========================================")
	t.Logf("FINAL COVERAGE FROM TILEGRID")
	t.Logf("========================================")

	for _, diagX := range diagnosticColumns {
		t.Logf("")
		t.Logf("--- Column x=%d ---", diagX)

		// Find the expected Y center of the sine wave at this X.
		tt := (float64(diagX) - 50.0) / 70.0
		expectedY := 250.0 - math.Sin(tt)*math.Exp(-tt*0.1)*200.0
		t.Logf("Expected sine wave center Y: %.2f (stroke band: %.2f to %.2f)",
			expectedY, expectedY-lineWidth/2, expectedY+lineWidth/2)

		// Collect all non-zero coverage pixels in this column.
		type covEntry struct {
			y   int
			cov uint8
		}
		var entries []covEntry
		minY, maxY := canvasH, 0

		// Walk all tiles that could contain this column.
		tileX := int32(diagX) >> TileShift
		localX := diagX % TileSize

		for tileY := int32(0); tileY < int32((canvasH+TileSize-1)/TileSize); tileY++ {
			tile := grid.Get(tileX, tileY)
			if tile == nil {
				continue
			}
			for py := 0; py < TileSize; py++ {
				cov := tile.GetCoverage(localX, py)
				if cov > 0 {
					pixelY := int(tileY)*TileSize + py
					entries = append(entries, covEntry{pixelY, cov})
					if pixelY < minY {
						minY = pixelY
					}
					if pixelY > maxY {
						maxY = pixelY
					}
				}
			}
		}

		t.Logf("Non-zero coverage pixels: %d (y range: %d to %d, spread: %d)",
			len(entries), minY, maxY, maxY-minY+1)

		// Print every pixel with coverage.
		for _, e := range entries {
			inBand := ""
			if float64(e.y) >= expectedY-lineWidth/2-1 && float64(e.y) <= expectedY+lineWidth/2+1 {
				inBand = " (INSIDE stroke band)"
			} else {
				inBand = " *** OUTSIDE stroke band ***"
			}
			t.Logf("  y=%d: coverage=%d%s", e.y, e.cov, inBand)
		}
	}
}

// analyzeColumn traces the full pipeline for a single pixel column:
// which segments cross tiles in this column, what backdrop they produce,
// and what winding values accumulate at each pixel.
func analyzeColumn(
	t *testing.T,
	pixelX, _, canvasH int,
	segments *SegmentList,
	coarse *CoarseRasterizer,
	backdrop []int32,
) {
	t.Helper()

	tileX := int32(pixelX) >> TileShift
	localX := pixelX % TileSize
	tileColumns := int(coarse.TileColumns())
	allLines := segments.Segments()

	t.Logf("Pixel x=%d -> tile column %d, local offset %d", pixelX, tileX, localX)

	// 1. Find all segments that cross this pixel column's X range.
	t.Logf("")
	t.Logf("--- Segments crossing pixel column x=%d ---", pixelX)
	pxLeft := float32(pixelX)
	pxRight := pxLeft + 1.0

	type crossingSegment struct {
		idx     int
		seg     LineSegment
		yEntry  float32 // Y where segment enters pixel column
		yExit   float32 // Y where segment exits pixel column
		winding int8
	}
	var crossings []crossingSegment

	for i, seg := range allLines {
		// Does this segment's X range overlap [pxLeft, pxRight]?
		segMinX := seg.X0
		segMaxX := seg.X1
		if segMinX > segMaxX {
			segMinX, segMaxX = segMaxX, segMinX
		}

		if segMaxX < pxLeft || segMinX > pxRight {
			continue // No X overlap.
		}

		// Segment crosses this column. Find Y range.
		yEntry := seg.Y0
		yExit := seg.Y1
		if seg.Y0 > seg.Y1 {
			yEntry, yExit = seg.Y1, seg.Y0
		}

		crossings = append(crossings, crossingSegment{
			idx: i, seg: seg,
			yEntry: yEntry, yExit: yExit,
			winding: seg.Winding,
		})
	}

	t.Logf("Total segments crossing x=%d: %d", pixelX, len(crossings))
	for i, cs := range crossings {
		if i >= 30 { // Limit output
			t.Logf("  ... and %d more", len(crossings)-30)
			break
		}
		t.Logf("  seg[%d]: (%.2f,%.2f)->(%.2f,%.2f) winding=%+d Y_range=[%.2f, %.2f]",
			cs.idx, cs.seg.X0, cs.seg.Y0, cs.seg.X1, cs.seg.Y1,
			cs.winding, cs.yEntry, cs.yExit)
	}

	// 2. Report coarse entries for tiles in this column.
	t.Logf("")
	t.Logf("--- Coarse entries for tile column %d ---", tileX)
	entries := coarse.Entries()
	var tileEntryCount int
	for _, entry := range entries {
		if int32(entry.X) == tileX {
			tileEntryCount++
			if tileEntryCount <= 40 {
				lineStr := "N/A"
				if int(entry.LineIdx) < len(allLines) {
					seg := allLines[entry.LineIdx]
					lineStr = fmt.Sprintf("(%.2f,%.2f)->(%.2f,%.2f) w=%+d",
						seg.X0, seg.Y0, seg.X1, seg.Y1, seg.Winding)
				}
				t.Logf("  tile(%d,%d) lineIdx=%d winding=%v seg=%s",
					entry.X, entry.Y, entry.LineIdx, entry.Winding, lineStr)
			}
		}
	}
	if tileEntryCount > 40 {
		t.Logf("  ... %d total coarse entries for tile column %d", tileEntryCount, tileX)
	}

	// 3. Report backdrop values for tiles in this column.
	t.Logf("")
	t.Logf("--- Backdrop values for tile column %d ---", tileX)
	tileRows := (canvasH + TileSize - 1) / TileSize
	for ty := 0; ty < tileRows; ty++ {
		idx := ty*tileColumns + int(tileX)
		if idx < len(backdrop) && backdrop[idx] != 0 {
			pixelYStart := ty * TileSize
			pixelYEnd := pixelYStart + TileSize - 1
			t.Logf("  tile(%d,%d) [pixels y=%d..%d]: backdrop=%d",
				tileX, ty, pixelYStart, pixelYEnd, backdrop[idx])
		}
	}

	// 4. Manually compute per-pixel winding at this column using the exact
	//    same algorithm as FineRasterizer.processSegment + initTileWinding.
	t.Logf("")
	t.Logf("--- Per-pixel winding reconstruction for x=%d ---", pixelX)

	// For each tile row, reproduce the fine rasterization for the specific
	// pixel column, tracking winding contributions from each segment.
	pixelWindings := make([]pixelWinding, canvasH)

	for ty := 0; ty < tileRows; ty++ {
		// Get backdrop for this tile.
		bdIdx := ty*tileColumns + int(tileX)
		var bdVal int32
		if bdIdx >= 0 && bdIdx < len(backdrop) {
			bdVal = backdrop[bdIdx]
		}
		bdF := float32(bdVal)

		// Initialize winding array for this tile (same as initTileWinding).
		var tileWinding [TileSize][TileSize]float32
		var accWinding [TileSize]float32
		for y := 0; y < TileSize; y++ {
			accWinding[y] = bdF
			for x := 0; x < TileSize; x++ {
				tileWinding[y][x] = bdF
			}
		}

		// Find coarse entries for this tile and process them.
		for _, entry := range entries {
			if int32(entry.X) != tileX || int32(entry.Y) != int32(ty) {
				continue
			}
			if int(entry.LineIdx) >= len(allLines) {
				continue
			}
			seg := allLines[entry.LineIdx]

			// Reproduce processSegment for this specific tile.
			tileLeftX := float32(tileX) * float32(TileWidth)
			tileTopY := float32(ty) * float32(TileHeight)

			p0x := seg.X0 - tileLeftX
			p0y := seg.Y0 - tileTopY
			p1x := seg.X1 - tileLeftX
			p1y := seg.Y1 - tileTopY

			if p0y == p1y {
				continue
			}

			sign := float32(seg.Winding)
			lineTopY := p0y
			lineTopX := p0x
			lineBottomY := p1y
			lineBottomX := p1x

			dy := lineBottomY - lineTopY
			dx := lineBottomX - lineTopX

			var ySlope float32
			if dx == 0 {
				if lineBottomY > lineTopY {
					ySlope = 1e10
				} else {
					ySlope = -1e10
				}
			} else {
				ySlope = dy / dx
			}
			xSlope := 1.0 / ySlope

			for yIdx := 0; yIdx < TileHeight; yIdx++ {
				pxTopY := float32(yIdx)
				pxBottomY := pxTopY + 1.0

				yMin := maxf32(lineTopY, pxTopY)
				yMax := minf32(lineBottomY, pxBottomY)

				if yMin >= yMax {
					continue
				}

				h := yMax - yMin

				// Walk columns up to localX to compute acc.
				acc := float32(0)
				for xIdx := 0; xIdx <= localX; xIdx++ {
					pxLeftXL := float32(xIdx)
					pxRightXL := pxLeftXL + 1.0

					linePxLeftY := lineTopY + (pxLeftXL-lineTopX)*ySlope
					linePxRightY := lineTopY + (pxRightXL-lineTopX)*ySlope

					linePxLeftY = clampf32(linePxLeftY, yMin, yMax)
					linePxRightY = clampf32(linePxRightY, yMin, yMax)

					linePxLeftYX := lineTopX + (linePxLeftY-lineTopY)*xSlope
					linePxRightYX := lineTopX + (linePxRightY-lineTopY)*xSlope

					pixelH := absf32(linePxRightY - linePxLeftY)
					area := 0.5 * pixelH * (2*pxRightXL - linePxRightYX - linePxLeftYX)

					if xIdx == localX {
						// This is our target pixel.
						pixelY := ty*TileSize + yIdx
						if pixelY < canvasH {
							contribution := area*sign + acc
							pixelWindings[pixelY].segmentArea += contribution
							pixelWindings[pixelY].segDetails = append(
								pixelWindings[pixelY].segDetails,
								fmt.Sprintf("seg[%d](w=%+d): area=%.4f, acc=%.4f, contrib=%.4f (h=%.4f)",
									entry.LineIdx, seg.Winding, area*sign, acc, contribution, h),
							)
						}
					}

					acc += pixelH * sign
				}

				_ = h // h is used for accWinding update in real code but we focus on tileWinding
			}
		}

		// Record backdrop + compute total winding for pixels in this tile row.
		for yIdx := 0; yIdx < TileSize; yIdx++ {
			pixelY := ty*TileSize + yIdx
			if pixelY >= canvasH {
				continue
			}
			pw := &pixelWindings[pixelY]
			pw.backdrop = bdF
			pw.totalWinding = pw.backdrop + pw.segmentArea

			// Compute coverage (NonZero).
			cov := absf32(pw.totalWinding)
			if cov > 1.0 {
				cov = 1.0
			}
			pw.coverage = uint8(cov*255.0 + 0.5)
		}
	}

	// Print the analysis for pixels with non-zero winding or coverage.
	var nonZeroCount int
	for y := 0; y < canvasH; y++ {
		pw := pixelWindings[y]
		if pw.coverage > 0 || pw.backdrop != 0 || len(pw.segDetails) > 0 {
			nonZeroCount++
		}
	}
	t.Logf("Pixels with non-zero winding/coverage: %d", nonZeroCount)

	// Find contiguous ranges of non-zero coverage and print them.
	inRange := false
	rangeStart := 0
	for y := 0; y <= canvasH; y++ {
		hasData := y < canvasH && pixelHasData(&pixelWindings[y])

		switch {
		case hasData && !inRange:
			inRange = true
			rangeStart = y
		case !hasData && inRange:
			inRange = false
			// Print the range.
			t.Logf("")
			t.Logf("  Non-zero range: y=%d..%d (span=%d)", rangeStart, y-1, y-rangeStart)
			for py := rangeStart; py < y; py++ {
				logPixelWinding(t, py, &pixelWindings[py])
			}
		}
	}
}

// pixelHasData returns true if a pixel has any non-zero winding data.
func pixelHasData(pw *pixelWinding) bool {
	return pw.coverage > 0 || pw.backdrop != 0 || pw.totalWinding != 0
}

// logPixelWinding logs detailed winding info for a single pixel.
func logPixelWinding(t *testing.T, y int, pw *pixelWinding) {
	t.Helper()
	marker := ""
	if pw.backdrop != 0 && len(pw.segDetails) == 0 {
		marker = " *** BACKDROP-ONLY (no local segments) ***"
	}
	t.Logf("    y=%d: backdrop=%.2f + segArea=%.4f = totalWinding=%.4f -> coverage=%d%s",
		y, pw.backdrop, pw.segmentArea, pw.totalWinding, pw.coverage, marker)
	if len(pw.segDetails) > 0 && len(pw.segDetails) <= 10 {
		for _, d := range pw.segDetails {
			t.Logf("        %s", d)
		}
	} else if len(pw.segDetails) > 10 {
		for _, d := range pw.segDetails[:5] {
			t.Logf("        %s", d)
		}
		t.Logf("        ... and %d more segment contributions", len(pw.segDetails)-5)
	}
}

// buildDebugSinePath creates the 100-segment damped sine wave as a scene.Path
// (float32, for direct use with SparseStripsRasterizer).
func buildDebugSinePath(numSegments int) *scene.Path {
	p := scene.NewPath()
	for i := 0; i <= numSegments; i++ {
		tt := float64(i) * 0.1
		x := 50 + tt*70
		y := 250 - math.Sin(tt)*math.Exp(-tt*0.1)*200
		if i == 0 {
			p.MoveTo(float32(x), float32(y))
		} else {
			p.LineTo(float32(x), float32(y))
		}
	}
	return p
}

// expandSineStroke takes the centerline sine wave (scene.Path) and produces
// the stroke-expanded outline using the same stroke expander as software.go.
// Returns a scene.Path (float32) ready for SparseStripsRasterizer.
func expandSineStroke(centerline *scene.Path, width float64) *scene.Path {
	// Convert scene.Path verbs/points to stroke expander format.
	verbs := centerline.Verbs()
	points := centerline.Points()

	strokeVerbs := make([]stroke.PathVerb, len(verbs))
	for i, v := range verbs {
		switch v {
		case scene.MoveTo:
			strokeVerbs[i] = stroke.VerbMoveTo
		case scene.LineTo:
			strokeVerbs[i] = stroke.VerbLineTo
		case scene.QuadTo:
			strokeVerbs[i] = stroke.VerbQuadTo
		case scene.CubicTo:
			strokeVerbs[i] = stroke.VerbCubicTo
		case scene.Close:
			strokeVerbs[i] = stroke.VerbClose
		}
	}

	// Convert float32 points to float64 coords for stroke expander.
	coords := make([]float64, len(points))
	for i, p := range points {
		coords[i] = float64(p)
	}

	// Create stroke style matching filler_comparison_test.go.
	style := stroke.Stroke{
		Width:      width,
		Cap:        stroke.LineCapButt,
		Join:       stroke.LineJoinMiter,
		MiterLimit: 10.0,
	}

	expander := stroke.NewStrokeExpander(style)
	expander.SetTolerance(0.1)
	outVerbs, outCoords := expander.Expand(strokeVerbs, coords)

	// Convert back to scene.Path.
	result := scene.NewPath()
	ci := 0
	for _, v := range outVerbs {
		switch v {
		case stroke.VerbMoveTo:
			result.MoveTo(float32(outCoords[ci]), float32(outCoords[ci+1]))
			ci += 2
		case stroke.VerbLineTo:
			result.LineTo(float32(outCoords[ci]), float32(outCoords[ci+1]))
			ci += 2
		case stroke.VerbQuadTo:
			result.QuadTo(
				float32(outCoords[ci]), float32(outCoords[ci+1]),
				float32(outCoords[ci+2]), float32(outCoords[ci+3]),
			)
			ci += 4
		case stroke.VerbCubicTo:
			result.CubicTo(
				float32(outCoords[ci]), float32(outCoords[ci+1]),
				float32(outCoords[ci+2]), float32(outCoords[ci+3]),
				float32(outCoords[ci+4]), float32(outCoords[ci+5]),
			)
			ci += 6
		case stroke.VerbClose:
			result.Close()
		}
	}

	return result
}

// TestSparseStripsDebug_BackdropSpill is a focused test that checks whether
// the CalculateBackdrop function produces non-zero values in tile rows that
// are far from any actual segment. A non-zero backdrop in a row where no
// segments actually cross means the fill will "spill" winding from distant
// self-intersections into tiles that should be empty.
func TestSparseStripsDebug_BackdropSpill(t *testing.T) {
	const (
		canvasW     = 400
		canvasH     = 500
		lineWidth   = 2.0
		numSegments = 100
	)

	sinePath := buildDebugSinePath(numSegments)
	strokePath := expandSineStroke(sinePath, lineWidth)

	config := DefaultConfig(canvasW, canvasH)
	config.FillRule = scene.FillNonZero
	ssr := NewSparseStripsRasterizer(config)

	ssr.flattenCtx.Reset()
	ssr.flattenCtx.FlattenPathTo(strokePath, scene.IdentityAffine(), FlattenTolerance)
	segments := ssr.flattenCtx.Segments()

	ssr.coarse.Rasterize(segments)
	ssr.coarse.SortEntries()
	backdrop := ssr.coarse.CalculateBackdrop()

	tileColumns := int(ssr.coarse.TileColumns())
	tileRows := (canvasH + TileSize - 1) / TileSize

	// For each tile, check if it has a non-zero backdrop but NO coarse entries
	// pointing segments that actually cross it. This is the "spill" condition.
	entries := ssr.coarse.Entries()

	// Build a set of tiles that have actual segment entries.
	type tileKey struct{ x, y int }
	tilesWithSegments := make(map[tileKey]int) // count of segment entries

	for _, e := range entries {
		k := tileKey{int(e.X), int(e.Y)}
		tilesWithSegments[k]++
	}

	// Report tiles with non-zero backdrop.
	t.Logf("=== BACKDROP ANALYSIS ===")
	t.Logf("Canvas: %dx%d, Tiles: %dx%d (4x4 px each)", canvasW, canvasH, tileColumns, tileRows)

	var backdropOnlyTiles int
	var backdropWithSegTiles int

	for ty := 0; ty < tileRows; ty++ {
		for tx := 0; tx < tileColumns; tx++ {
			idx := ty*tileColumns + tx
			if idx >= len(backdrop) || backdrop[idx] == 0 {
				continue
			}

			k := tileKey{tx, ty}
			segCount := tilesWithSegments[k]

			if segCount == 0 {
				backdropOnlyTiles++
				t.Logf("  BACKDROP-ONLY tile(%d,%d) [px y=%d..%d, x=%d..%d]: backdrop=%d (no segments!)",
					tx, ty, ty*TileSize, ty*TileSize+TileSize-1,
					tx*TileSize, tx*TileSize+TileSize-1, backdrop[idx])
			} else {
				backdropWithSegTiles++
			}
		}
	}

	t.Logf("")
	t.Logf("Summary:")
	t.Logf("  Tiles with non-zero backdrop + segments: %d", backdropWithSegTiles)
	t.Logf("  Tiles with non-zero backdrop + NO segments (BACKDROP-ONLY): %d", backdropOnlyTiles)
	t.Logf("  Total tiles with non-zero backdrop: %d", backdropOnlyTiles+backdropWithSegTiles)

	if backdropOnlyTiles > 0 {
		t.Logf("")
		t.Logf("FINDING: %d tiles have non-zero backdrop winding but no local segments.", backdropOnlyTiles)
		t.Logf("These tiles will render with solid coverage (backdrop becomes fill).")
		t.Logf("For a thin 2px stroke, NO tile should be backdrop-only filled.")
		t.Logf("This is the source of the 2.2x thicker strokes.")
	}

	// Also check: how many tile ROWS have backdrop-only tiles?
	rowsWithSpill := make(map[int]int)
	for ty := 0; ty < tileRows; ty++ {
		for tx := 0; tx < tileColumns; tx++ {
			idx := ty*tileColumns + tx
			if idx >= len(backdrop) && backdrop[idx] == 0 {
				continue
			}
			if idx < len(backdrop) && backdrop[idx] != 0 {
				k := tileKey{tx, ty}
				if tilesWithSegments[k] == 0 {
					rowsWithSpill[ty]++
				}
			}
		}
	}
	if len(rowsWithSpill) > 0 {
		t.Logf("")
		t.Logf("Tile rows with backdrop spill:")
		for ty, count := range rowsWithSpill {
			t.Logf("  row %d (y=%d..%d): %d backdrop-only tiles",
				ty, ty*TileSize, ty*TileSize+TileSize-1, count)
		}
	}
}

// TestSparseStripsDebug_WindingAccumulation traces the winding number at a specific
// pixel position across all segments to show exactly how winding builds up.
// Uses x=100 which is in the middle of the sine wave where self-intersection is likely.
func TestSparseStripsDebug_WindingAccumulation(t *testing.T) {
	const (
		canvasW     = 400
		canvasH     = 500
		lineWidth   = 2.0
		numSegments = 100
	)

	sinePath := buildDebugSinePath(numSegments)
	strokePath := expandSineStroke(sinePath, lineWidth)

	config := DefaultConfig(canvasW, canvasH)
	config.FillRule = scene.FillNonZero
	ssr := NewSparseStripsRasterizer(config)

	ssr.flattenCtx.Reset()
	ssr.flattenCtx.FlattenPathTo(strokePath, scene.IdentityAffine(), FlattenTolerance)
	segments := ssr.flattenCtx.Segments()
	allLines := segments.Segments()

	t.Logf("Total flattened segments: %d", len(allLines))

	// Find all segments that actually cross x=100.
	pixelX := float32(100)
	t.Logf("")
	t.Logf("=== Segments crossing x=%.0f ===", pixelX)

	type segCrossing struct {
		idx int
		seg LineSegment
	}
	var crossings []segCrossing

	for i, seg := range allLines {
		segMinX := seg.X0
		segMaxX := seg.X1
		if segMinX > segMaxX {
			segMinX, segMaxX = segMaxX, segMinX
		}
		if segMaxX < pixelX || segMinX > pixelX+1 {
			continue
		}
		crossings = append(crossings, segCrossing{
			idx: i, seg: seg,
		})
	}

	t.Logf("Segments crossing x=100: %d", len(crossings))

	// For each Y value, count how many segments with winding +1 vs -1 pass
	// to the LEFT of x=100. This is what determines the backdrop.
	//
	// In a correctly-wound stroke outline, at any Y inside the stroke band,
	// there should be exactly 2 edges: one going left-to-right (+1) and one
	// right-to-left (-1), with the stroke interior between them.
	// Net winding inside stroke = +1, outside = 0.
	//
	// If the stroke outline self-intersects, some Y values may see extra edges,
	// causing net winding > 1 or unexpected non-zero winding at positions
	// outside the 2px band.

	// Sample a few Y values inside and outside the expected stroke band.
	tt := (float64(100) - 50.0) / 70.0
	expectedCenterY := 250.0 - math.Sin(tt)*math.Exp(-tt*0.1)*200.0
	t.Logf("Expected center Y at x=100: %.2f", expectedCenterY)

	sampleYs := []float64{
		expectedCenterY - 10, // well outside
		expectedCenterY - 5,  // outside
		expectedCenterY - 2,  // edge of stroke
		expectedCenterY - 1,  // AA zone
		expectedCenterY,      // center
		expectedCenterY + 1,  // AA zone
		expectedCenterY + 2,  // edge of stroke
		expectedCenterY + 5,  // outside
		expectedCenterY + 10, // well outside
	}

	for _, sy := range sampleYs {
		// Count winding from segments passing LEFT of x=100 at y=sy.
		windingLeft := 0
		var details []string

		for _, seg := range allLines {
			// Does this segment's Y range include sy?
			if float32(sy) < seg.Y0 || float32(sy) >= seg.Y1 {
				continue
			}
			// What is the X of this segment at y=sy?
			segX := seg.XAtY(float32(sy))
			if segX < pixelX {
				windingLeft += int(seg.Winding)
				details = append(details,
					fmt.Sprintf("seg (%.1f,%.1f)->(%.1f,%.1f) w=%+d x_at_y=%.2f",
						seg.X0, seg.Y0, seg.X1, seg.Y1, seg.Winding, segX))
			}
		}

		label := "OUTSIDE"
		if math.Abs(sy-expectedCenterY) <= lineWidth/2+0.5 {
			label = "STROKE BAND"
		}

		t.Logf("")
		t.Logf("Y=%.1f [%s]: winding_left_of_x100 = %d", sy, label, windingLeft)
		if len(details) <= 8 {
			for _, d := range details {
				t.Logf("  %s", d)
			}
		} else {
			for _, d := range details[:4] {
				t.Logf("  %s", d)
			}
			t.Logf("  ... and %d more segments to the left", len(details)-4)
		}
	}
}

// absf32 is an alias for the package-level absf32 (already defined in fine.go).
// If this causes a compile error, remove it.
// We don't redefine it here because it's in the same package.
