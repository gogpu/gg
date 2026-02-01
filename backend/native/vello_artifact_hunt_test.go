// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestIdentifyProblematicTiles analyzes ALL tiles to find common characteristics
// of tiles that produce artifacts.
func TestIdentifyProblematicTiles(t *testing.T) {
	const width, height = 200, 200

	cx, cy := float32(100), float32(100)
	radius := float32(80)
	const k = 0.5522847498

	buildCircle := func(eb *EdgeBuilder) {
		path := scene.NewPath()
		path.MoveTo(cx+radius, cy)
		path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
		path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
		path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
		path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
		path.Close()
		eb.SetFlattenCurves(true)
		eb.BuildFromScenePath(path, scene.IdentityAffine())
	}

	// Render with AnalyticFiller (reference)
	analyticAlpha := make(map[[2]int]uint8)
	af := NewAnalyticFiller(width, height)
	eb1 := NewEdgeBuilder(2)
	buildCircle(eb1)
	af.Fill(eb1, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				analyticAlpha[[2]int{x, y}] = alpha
			}
		}
	})

	// Render with Vello
	velloAlpha := make(map[[2]int]uint8)
	tr := NewTileRasterizer(width, height)
	eb2 := NewEdgeBuilder(2)
	buildCircle(eb2)
	tr.Fill(eb2, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				velloAlpha[[2]int{x, y}] = alpha
			}
		}
	})

	// Get tile data
	tr2 := NewTileRasterizer(width, height)
	eb3 := NewEdgeBuilder(2)
	buildCircle(eb3)
	tr2.binSegments(eb3, 4.0)
	tr2.computeBackdropPrefixSum() // Must match Fill() order!
	tr2.markProblemTiles()         // Mark problem tiles for analysis

	fmt.Println("=== TILE ANALYSIS: Finding problematic tile characteristics ===")
	fmt.Println()

	type tileStats struct {
		tileX, tileY     int
		backdrop         int
		numSegments      int
		hasNegativeYEdge bool
		hasPositiveYEdge bool
		maxSegmentX      float32 // Max X position of segments with yEdge
		diffPixels       int
		totalPixels      int
		isProblematic    bool
	}

	var allTiles []tileStats

	for tileY := 0; tileY < tr2.tilesY; tileY++ {
		for tileX := 0; tileX < tr2.tilesX; tileX++ {
			idx := tileY*tr2.tilesX + tileX
			tile := tr2.tiles[idx]

			stats := tileStats{
				tileX:       tileX,
				tileY:       tileY,
				backdrop:    tile.Backdrop,
				numSegments: len(tile.Segments),
			}

			// Analyze segments
			for _, seg := range tile.Segments {
				dx := seg.Point1[0] - seg.Point0[0]
				if seg.YEdge < 1e8 { // Has valid yEdge
					if dx < 0 {
						stats.hasNegativeYEdge = true
					} else if dx > 0 {
						stats.hasPositiveYEdge = true
					}
					// Track max X of segment
					maxX := max32f(seg.Point0[0], seg.Point1[0])
					if maxX > stats.maxSegmentX {
						stats.maxSegmentX = maxX
					}
				}
			}

			// Count diff pixels in this tile
			startX := tileX * VelloTileWidth
			startY := tileY * VelloTileHeight
			for y := startY; y < startY+VelloTileHeight && y < height; y++ {
				for x := startX; x < startX+VelloTileWidth && x < width; x++ {
					stats.totalPixels++
					key := [2]int{x, y}
					if analyticAlpha[key] != velloAlpha[key] {
						stats.diffPixels++
					}
				}
			}

			stats.isProblematic = stats.diffPixels > 0

			allTiles = append(allTiles, stats)
		}
	}

	// Analyze problematic tiles
	fmt.Println("=== PROBLEMATIC TILES ===")
	fmt.Println()

	var problematicCount int
	for _, ts := range allTiles {
		if ts.isProblematic {
			problematicCount++
			fmt.Printf("Tile (%d, %d): backdrop=%d, segs=%d, negYEdge=%v, posYEdge=%v, maxSegX=%.2f, diff=%d/%d\n",
				ts.tileX, ts.tileY, ts.backdrop, ts.numSegments,
				ts.hasNegativeYEdge, ts.hasPositiveYEdge, ts.maxSegmentX,
				ts.diffPixels, ts.totalPixels)
		}
	}

	fmt.Printf("\nTotal problematic tiles: %d out of %d\n", problematicCount, len(allTiles))

	// Find common pattern
	fmt.Println("\n=== PATTERN ANALYSIS ===")
	fmt.Println()

	var pattern1, pattern2, pattern3 int
	for _, ts := range allTiles {
		if ts.isProblematic {
			// Pattern 1: backdrop=0 AND hasNegativeYEdge AND maxSegmentX < 2
			if ts.backdrop == 0 && ts.hasNegativeYEdge && ts.maxSegmentX < 2.0 {
				pattern1++
			}
			// Pattern 2: backdrop=0 AND hasNegativeYEdge AND !hasPositiveYEdge
			if ts.backdrop == 0 && ts.hasNegativeYEdge && !ts.hasPositiveYEdge {
				pattern2++
			}
			// Pattern 3: backdrop=0 AND hasNegativeYEdge
			if ts.backdrop == 0 && ts.hasNegativeYEdge {
				pattern3++
			}
		}
	}

	fmt.Printf("Pattern 1 (backdrop=0 && negYEdge && maxSegX<2): %d matches\n", pattern1)
	fmt.Printf("Pattern 2 (backdrop=0 && negYEdge && !posYEdge): %d matches\n", pattern2)
	fmt.Printf("Pattern 3 (backdrop=0 && negYEdge): %d matches\n", pattern3)

	// Check if pattern would catch false positives
	fmt.Println("\n=== FALSE POSITIVE CHECK ===")
	fmt.Println()

	var falsePos1, falsePos2, falsePos3 int
	for _, ts := range allTiles {
		if !ts.isProblematic {
			if ts.backdrop == 0 && ts.hasNegativeYEdge && ts.maxSegmentX < 2.0 {
				falsePos1++
			}
			if ts.backdrop == 0 && ts.hasNegativeYEdge && !ts.hasPositiveYEdge {
				falsePos2++
			}
			if ts.backdrop == 0 && ts.hasNegativeYEdge {
				falsePos3++
			}
		}
	}

	fmt.Printf("Pattern 1 false positives: %d\n", falsePos1)
	fmt.Printf("Pattern 2 false positives: %d\n", falsePos2)
	fmt.Printf("Pattern 3 false positives: %d\n", falsePos3)

	// Deep analysis of problematic vs non-problematic with same pattern
	fmt.Println("\n=== DETAILED COMPARISON ===")
	fmt.Println()

	fmt.Println("Problematic tiles:")
	for _, ts := range allTiles {
		if ts.isProblematic {
			idx := ts.tileY*tr2.tilesX + ts.tileX
			tile := tr2.tiles[idx]

			fmt.Printf("  Tile (%d,%d): backdrop=%d, segs=%d, IsProblemTile=%v, IsBottomProblem=%v\n",
				ts.tileX, ts.tileY, ts.backdrop, ts.numSegments,
				tile.IsProblemTile, tile.IsBottomProblem)
			for i, seg := range tile.Segments {
				dx := seg.Point1[0] - seg.Point0[0]
				dy := seg.Point1[1] - seg.Point0[1]
				fmt.Printf("    Seg[%d]: P0=(%.2f,%.2f) P1=(%.2f,%.2f) dx=%.2f dy=%.2f YEdge=%.2f\n",
					i, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1], dx, dy, seg.YEdge)
			}
		}
	}

	fmt.Println("\nNon-problematic tiles with backdrop=0 and negYEdge:")
	for _, ts := range allTiles {
		if !ts.isProblematic && ts.backdrop == 0 && ts.hasNegativeYEdge {
			idx := ts.tileY*tr2.tilesX + ts.tileX
			tile := tr2.tiles[idx]

			fmt.Printf("  Tile (%d,%d): backdrop=%d, segs=%d\n", ts.tileX, ts.tileY, ts.backdrop, ts.numSegments)
			for i, seg := range tile.Segments {
				dx := seg.Point1[0] - seg.Point0[0]
				fmt.Printf("    Seg[%d]: P0=(%.2f,%.2f) P1=(%.2f,%.2f) dx=%.2f YEdge=%.2f\n",
					i, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1], dx, seg.YEdge)
			}
		}
	}
}

// TestHuntArtifactPixels finds exact pixels where Vello differs from AnalyticFiller
// and traces back to understand WHY.
func TestHuntArtifactPixels(t *testing.T) {
	const width, height = 200, 200

	// Circle r=80 at (100, 100)
	cx, cy := float32(100), float32(100)
	radius := float32(80)
	const k = 0.5522847498

	buildCircle := func(eb *EdgeBuilder) {
		path := scene.NewPath()
		path.MoveTo(cx+radius, cy)
		path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
		path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
		path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
		path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
		path.Close()
		eb.SetFlattenCurves(true)
		eb.BuildFromScenePath(path, scene.IdentityAffine())
	}

	// Render with AnalyticFiller (reference)
	analyticAlpha := make(map[[2]int]uint8)
	af := NewAnalyticFiller(width, height)
	eb1 := NewEdgeBuilder(2)
	buildCircle(eb1)
	af.Fill(eb1, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				analyticAlpha[[2]int{x, y}] = alpha
			}
		}
	})

	// Render with Vello
	velloAlpha := make(map[[2]int]uint8)
	tr := NewTileRasterizer(width, height)
	eb2 := NewEdgeBuilder(2)
	buildCircle(eb2)
	tr.Fill(eb2, FillRuleNonZero, func(y int, runs *AlphaRuns) {
		for x, alpha := range runs.Iter() {
			if alpha > 0 {
				velloAlpha[[2]int{x, y}] = alpha
			}
		}
	})

	// Find differences
	fmt.Println("=== ARTIFACT PIXELS (Vello != Analytic) ===")
	fmt.Println()

	type diffPixel struct {
		x, y          int
		analyticAlpha uint8
		velloAlpha    uint8
		tileX, tileY  int
	}
	var diffs []diffPixel

	// Check all pixels
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			key := [2]int{x, y}
			aAlpha := analyticAlpha[key]
			vAlpha := velloAlpha[key]

			if aAlpha != vAlpha {
				diffs = append(diffs, diffPixel{
					x: x, y: y,
					analyticAlpha: aAlpha,
					velloAlpha:    vAlpha,
					tileX:         x / VelloTileWidth,
					tileY:         y / VelloTileHeight,
				})
			}
		}
	}

	fmt.Printf("Total diff pixels: %d\n\n", len(diffs))

	// Group by tile
	tileGroups := make(map[[2]int][]diffPixel)
	for _, d := range diffs {
		key := [2]int{d.tileX, d.tileY}
		tileGroups[key] = append(tileGroups[key], d)
	}

	fmt.Printf("Tiles with artifacts: %d\n\n", len(tileGroups))

	// Show each tile's artifacts
	for tileKey, pixels := range tileGroups {
		tileX, tileY := tileKey[0], tileKey[1]
		fmt.Printf("=== TILE (%d, %d) — pixels (%d-%d, %d-%d) ===\n",
			tileX, tileY,
			tileX*VelloTileWidth, (tileX+1)*VelloTileWidth-1,
			tileY*VelloTileHeight, (tileY+1)*VelloTileHeight-1)
		fmt.Printf("Artifact pixels in this tile: %d\n", len(pixels))

		for _, p := range pixels {
			// Check if pixel should be inside circle
			dx := float32(p.x) - cx
			dy := float32(p.y) - cy
			distSq := dx*dx + dy*dy
			inside := distSq < radius*radius
			onEdge := distSq >= (radius-1)*(radius-1) && distSq <= (radius+1)*(radius+1)

			status := "OUTSIDE"
			if inside {
				status = "INSIDE"
			}
			if onEdge {
				status += " (edge)"
			}

			fmt.Printf("  Pixel (%3d, %3d): Analytic=%3d, Vello=%3d, diff=%+4d  [%s]\n",
				p.x, p.y, p.analyticAlpha, p.velloAlpha,
				int(p.velloAlpha)-int(p.analyticAlpha), status)
		}
		fmt.Println()
	}

	// Now deep dive into the problematic tiles
	fmt.Println("=== DEEP DIVE: Segment analysis for artifact tiles ===")
	fmt.Println()

	// Re-bin segments to get tile data
	tr2 := NewTileRasterizer(width, height)
	eb3 := NewEdgeBuilder(2)
	buildCircle(eb3)
	tr2.binSegments(eb3, 4.0)

	for tileKey := range tileGroups {
		tileX, tileY := tileKey[0], tileKey[1]
		idx := tileY*tr2.tilesX + tileX
		if idx >= len(tr2.tiles) {
			continue
		}
		tile := tr2.tiles[idx]

		fmt.Printf("=== TILE (%d, %d) SEGMENTS ===\n", tileX, tileY)
		fmt.Printf("Backdrop: %d\n", tile.Backdrop)
		fmt.Printf("Segments: %d\n", len(tile.Segments))

		for i, seg := range tile.Segments {
			dx := seg.Point1[0] - seg.Point0[0]
			dy := seg.Point1[1] - seg.Point0[1]

			fmt.Printf("\nSeg[%d]: P0=(%.3f, %.3f) P1=(%.3f, %.3f)\n",
				i, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1])
			fmt.Printf("        dx=%.4f, dy=%.4f\n", dx, dy)
			fmt.Printf("        YEdge=%.4f\n", seg.YEdge)

			if seg.YEdge != 1e9 {
				fmt.Printf("        YEdge is SET (segment touches left edge at y=%.3f)\n", seg.YEdge)
			}
		}

		// Trace row 4 (y=20) in detail for tile (7,1)
		if tileX == 7 && tileY == 1 {
			fmt.Println("\n=== DETAILED ROW 4 (y=20) TRACE ===")
			yf := float32(4)
			fmt.Printf("Initial area[0..15] = %.2f (backdrop)\n", float32(tile.Backdrop))

			for i, seg := range tile.Segments {
				dx := seg.Point1[0] - seg.Point0[0]
				dy := seg.Point1[1] - seg.Point0[1]

				// Compute yEdge
				var yEdge float32
				if dx > 0 {
					yEdge = clamp32(yf-seg.YEdge+1.0, 0, 1)
				} else if dx < 0 {
					yEdge = -clamp32(yf-seg.YEdge+1.0, 0, 1)
				}

				// Check if segment crosses row 4
				y := seg.Point0[1] - yf
				y0 := clamp32(y, 0, 1)
				y1 := clamp32(y+dy, 0, 1)
				rowDy := y0 - y1

				fmt.Printf("\nSeg[%d] at row 4:\n", i)
				fmt.Printf("  y_relative = %.4f (seg.Point0[1] - yf)\n", y)
				fmt.Printf("  y0 = %.4f, y1 = %.4f, rowDy = %.4f\n", y0, y1, rowDy)
				fmt.Printf("  yEdge = %.4f\n", yEdge)

				if rowDy != 0 {
					// Segment crosses this row - compute X positions
					vecYRecip := 1.0 / dy
					t0 := (y0 - y) * vecYRecip
					t1 := (y1 - y) * vecYRecip

					segX0 := seg.Point0[0] + t0*dx
					segX1 := seg.Point0[0] + t1*dx

					xmin0 := min32f(segX0, segX1)
					xmax0 := max32f(segX0, segX1)

					fmt.Printf("  t0 = %.4f, t1 = %.4f\n", t0, t1)
					fmt.Printf("  segX0 = %.4f, segX1 = %.4f\n", segX0, segX1)
					fmt.Printf("  xmin0 = %.4f, xmax0 = %.4f\n", xmin0, xmax0)
					fmt.Printf("  => Segment covers X from %.2f to %.2f at row 4\n", xmin0, xmax0)
				} else {
					fmt.Printf("  => Segment does NOT cross row 4 (rowDy=0)\n")
				}
			}

			// Show final area values for pixels 0, 1, 8, 15
			fmt.Println("\n=== SIMULATED AREA FOR SELECTED PIXELS ===")
			for _, pixelIdx := range []int{0, 1, 8, 15} {
				totalArea := float32(tile.Backdrop)
				fmt.Printf("Pixel %d (x=%d):\n", pixelIdx, tileX*16+pixelIdx)

				for i, seg := range tile.Segments {
					dx := seg.Point1[0] - seg.Point0[0]
					dy := seg.Point1[1] - seg.Point0[1]

					var yEdge float32
					if dx > 0 {
						yEdge = clamp32(yf-seg.YEdge+1.0, 0, 1)
					} else if dx < 0 {
						yEdge = -clamp32(yf-seg.YEdge+1.0, 0, 1)
					}

					y := seg.Point0[1] - yf
					y0 := clamp32(y, 0, 1)
					y1 := clamp32(y+dy, 0, 1)
					rowDy := y0 - y1

					var contribution float32
					if rowDy != 0 {
						vecYRecip := 1.0 / dy
						t0 := (y0 - y) * vecYRecip
						t1 := (y1 - y) * vecYRecip

						segX0 := seg.Point0[0] + t0*dx
						segX1 := seg.Point0[0] + t1*dx

						xmin0 := min32f(segX0, segX1)
						xmax0 := max32f(segX0, segX1)

						iF := float32(pixelIdx)
						xmin := min32f(xmin0-iF, 1.0) - 1.0e-6
						xmax := xmax0 - iF

						b := min32f(xmax, 1.0)
						c := max32f(b, 0.0)
						d := max32f(xmin, 0.0)

						denom := xmax - xmin
						var a float32
						if denom != 0 {
							a = (b + 0.5*(d*d-c*c) - xmin) / denom
						}

						contribution = yEdge + a*rowDy
					} else if yEdge != 0 {
						contribution = yEdge
					}

					fmt.Printf("  Seg[%d]: yEdge=%.4f, area_contrib=%.4f\n", i, yEdge, contribution)
					totalArea += contribution
				}

				absArea := totalArea
				if absArea < 0 {
					absArea = -absArea
				}
				if absArea > 1 {
					absArea = 1
				}
				alpha := uint8(absArea * 255)
				fmt.Printf("  TOTAL: area=%.4f, abs=%.4f, alpha=%d\n\n", totalArea, absArea, alpha)
			}
		}
		fmt.Println()
	}

	// Save visual diff
	tmpDir := filepath.Join("..", "..", "tmp")
	_ = os.MkdirAll(tmpDir, 0o755)

	diffImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			key := [2]int{x, y}
			aAlpha := analyticAlpha[key]
			vAlpha := velloAlpha[key]

			if aAlpha != vAlpha {
				// Red for Vello has MORE alpha than should
				// Green for Vello has LESS alpha than should
				if vAlpha > aAlpha {
					diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
				} else {
					diffImg.Set(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255})
				}
			} else if vAlpha > 0 {
				// Gray for matching filled pixels
				diffImg.Set(x, y, color.RGBA{R: 100, G: 100, B: 100, A: 255})
			} else {
				// White for empty
				diffImg.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}

	diffPath := filepath.Join(tmpDir, "artifact_hunt_r80.png")
	f, _ := os.Create(diffPath)
	_ = png.Encode(f, diffImg)
	f.Close()

	fmt.Printf("\nDiff image saved: %s\n", diffPath)
	fmt.Println("RED = Vello has MORE alpha (over-fill)")
	fmt.Println("GREEN = Vello has LESS alpha (under-fill)")
}
