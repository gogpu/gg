package native

import (
	"fmt"
	"github.com/gogpu/gg/internal/raster"
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestBackdropDebug traces backdrop computation for circle r=80
// to understand why tile (6,11) has backdrop=0 instead of expected -1.
func TestBackdropDebug(t *testing.T) {
	const width, height = 200, 200

	cx, cy := float32(100), float32(100)
	radius := float32(80)
	const k = 0.5522847498

	tr := NewTileRasterizer(width, height)
	eb := raster.NewEdgeBuilder(2)

	path := scene.NewPath()
	path.MoveTo(cx+radius, cy)
	path.CubicTo(cx+radius, cy-radius*k, cx+radius*k, cy-radius, cx, cy-radius)
	path.CubicTo(cx-radius*k, cy-radius, cx-radius, cy-radius*k, cx-radius, cy)
	path.CubicTo(cx-radius, cy+radius*k, cx-radius*k, cy+radius, cx, cy+radius)
	path.CubicTo(cx+radius*k, cy+radius, cx+radius, cy+radius*k, cx+radius, cy)
	path.Close()
	eb.SetFlattenCurves(true)
	BuildEdgesFromScenePath(eb, path, scene.IdentityAffine())

	aaShift := eb.AAShift()
	aaScale := float32(int32(1) << uint(aaShift))
	tr.binSegments(eb, aaScale)

	// Print backdrop BEFORE prefix sum for row 11
	fmt.Printf("=== BACKDROP BEFORE PREFIX SUM (row 11) ===\n")
	for tx := 0; tx < tr.tilesX; tx++ {
		idx := 11*tr.tilesX + tx
		bd := tr.tiles[idx].Backdrop
		if bd != 0 {
			fmt.Printf("  Tile (%d, 11): backdrop=%d, segs=%d\n", tx, bd, len(tr.tiles[idx].Segments))
		}
	}

	// Print backdrop for ALL rows to understand pattern
	fmt.Printf("\n=== BACKDROP BEFORE PREFIX SUM (all rows with non-zero) ===\n")
	for ty := 0; ty < tr.tilesY; ty++ {
		for tx := 0; tx < tr.tilesX; tx++ {
			idx := ty*tr.tilesX + tx
			bd := tr.tiles[idx].Backdrop
			if bd != 0 {
				fmt.Printf("  Tile (%d, %d): backdrop=%d\n", tx, ty, bd)
			}
		}
	}

	// Now do prefix sum
	tr.computeBackdropPrefixSum()

	// Print backdrop AFTER prefix sum for row 11
	fmt.Printf("\n=== BACKDROP AFTER PREFIX SUM (row 11) ===\n")
	for tx := 0; tx < tr.tilesX; tx++ {
		idx := 11*tr.tilesX + tx
		bd := tr.tiles[idx].Backdrop
		segs := len(tr.tiles[idx].Segments)
		if bd != 0 || segs > 0 {
			fmt.Printf("  Tile (%d, 11): backdrop=%d, segs=%d\n", tx, bd, segs)
		}
	}

	// Detail segments for tile (6, 11)
	fmt.Printf("\n=== TILE (6, 11) SEGMENTS DETAIL ===\n")
	idx := 11*tr.tilesX + 6
	tile := &tr.tiles[idx]
	fmt.Printf("Backdrop (after prefix sum): %d\n", tile.Backdrop)
	fmt.Printf("IsProblemTile: %v\n", tile.IsProblemTile)
	for i, seg := range tile.Segments {
		dx := seg.Point1[0] - seg.Point0[0]
		dy := seg.Point1[1] - seg.Point0[1]
		fmt.Printf("  Seg[%d]: P0=(%.3f, %.3f) P1=(%.3f, %.3f) dx=%.3f dy=%.3f YEdge=%.4f\n",
			i, seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1], dx, dy, seg.YEdge)
	}

	// Also check tiles around the bottom of the circle (y≈180, tileY=11)
	fmt.Printf("\n=== ROW 11 TILES WITH SEGMENTS ===\n")
	for tx := 0; tx < tr.tilesX; tx++ {
		idx := 11*tr.tilesX + tx
		tile := &tr.tiles[idx]
		if len(tile.Segments) > 0 || tile.Backdrop != 0 {
			fmt.Printf("  Tile (%d, 11): backdrop=%d, segs=%d, problem=%v\n",
				tx, tile.Backdrop, len(tile.Segments), tile.IsProblemTile)
		}
	}

	// Manually trace fillTileScanline for tile (6,11) at localY=3 (pixelY=179)
	fmt.Printf("\n=== MANUAL TRACE: tile (6,11), localY=3 (pixelY=179) ===\n")
	tile = &tr.tiles[idx]
	localY := 3
	yf := float32(localY)
	backdropF := float32(tile.Backdrop)
	fmt.Printf("backdrop = %.1f\n", backdropF)

	for si, seg := range tile.Segments {
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

		fmt.Printf("\nSeg[%d]: P0=(%.3f, %.3f) P1=(%.3f, %.3f)\n", si,
			seg.Point0[0], seg.Point0[1], seg.Point1[0], seg.Point1[1])
		fmt.Printf("  y=%.4f, y0=%.4f, y1=%.4f, dy=%.4f\n", y, y0, y1, dy)
		fmt.Printf("  delta.x=%.4f, YEdge=%.4f, yEdge=%.4f\n", delta[0], seg.YEdge, yEdge)

		if dy != 0 {
			vecYRecip := 1.0 / delta[1]
			t0 := (y0 - y) * vecYRecip
			t1 := (y1 - y) * vecYRecip
			startX := seg.Point0[0]
			segX0 := startX + t0*delta[0]
			segX1 := startX + t1*delta[0]
			xmin0 := min32f(segX0, segX1)
			xmax0 := max32f(segX0, segX1)

			// Trace pixel x=3 (global x=99)
			i := 3
			iF := float32(i)
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
			fmt.Printf("  pixel[3]: a=%.4f, a*dy=%.4f, yEdge=%.4f, total_contrib=%.4f\n",
				a, a*dy, yEdge, a*dy+yEdge)
		} else {
			fmt.Printf("  dy=0, only yEdge contribution: %.4f\n", yEdge)
		}
	}

	// Compute final area for pixel[3]
	area := backdropF
	for si, seg := range tile.Segments {
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
			vecYRecip := 1.0 / delta[1]
			t0 := (y0 - y) * vecYRecip
			t1 := (y1 - y) * vecYRecip
			startX := seg.Point0[0]
			segX0 := startX + t0*delta[0]
			segX1 := startX + t1*delta[0]
			xmin0 := min32f(segX0, segX1)
			xmax0 := max32f(segX0, segX1)

			i := 3
			iF := float32(i)
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
			area += a * dy
			fmt.Printf("\nAfter Seg[%d] a*dy: area=%.4f\n", si, area)
		}
		area += yEdge
		fmt.Printf("After Seg[%d] +yEdge(%.4f): area=%.4f\n", si, yEdge, area)
	}

	fmt.Printf("\nFINAL area=%.4f, abs=%.4f, alpha=%d\n",
		area, abs32(area), uint8(clamp32(abs32(area), 0, 1)*255.0))
}
