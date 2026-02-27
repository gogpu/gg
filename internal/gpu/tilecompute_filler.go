//go:build !nogpu

package gpu

import (
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/gpu/tilecompute"
	"github.com/gogpu/gg/scene"
)

// TileComputeFiller implements gg.CoverageFiller using the tilecompute
// rasterizer (16x16 tiles). This is an alternative filler optimized for
// GPU workgroup-sized tiles.
type TileComputeFiller struct{}

// FillCoverage rasterizes the path using tilecompute and calls callback
// for each pixel with non-zero coverage.
func (f *TileComputeFiller) FillCoverage(
	path *gg.Path, width, height int, fillRule gg.FillRule,
	callback func(x, y int, coverage uint8),
) {
	if path == nil || len(path.Elements()) == 0 {
		return
	}

	// 1. Convert gg.Path → scene.Path → flatten → LineSoup
	scenePath := convertGGToScenePath(path)
	if scenePath.IsEmpty() {
		return
	}

	flatCtx := NewFlattenContext()
	flatCtx.FlattenPathTo(scenePath, scene.IdentityAffine(), FlattenTolerance)
	segs := flatCtx.Segments()
	if segs.Len() == 0 {
		return
	}

	lines := segmentsToLineSoup(segs)
	if len(lines) == 0 {
		return
	}

	// 2. Convert fill rule
	tcFillRule := tilecompute.FillRuleNonZero
	if fillRule == gg.FillRuleEvenOdd {
		tcFillRule = tilecompute.FillRuleEvenOdd
	}

	// 3. Rasterize with tilecompute (16x16 tiles)
	rast := tilecompute.NewRasterizer(width, height)
	alphas := rast.Rasterize(lines, tcFillRule)

	// 4. Convert float32 alphas → uint8 → callback
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			a := alphas[y*width+x]
			if a > 0 {
				c := uint8(a*255 + 0.5) //nolint:gosec // Intentional float-to-uint8 with rounding
				if c > 0 {
					callback(x, y, c)
				}
			}
		}
	}
}

// segmentsToLineSoup converts SparseStrips SegmentList to tilecompute LineSoup.
// LineSegments store monotonic segments (Y0 <= Y1) with a Winding field.
// LineSoup expects original (unsorted) direction — we restore it using Winding.
func segmentsToLineSoup(segs *SegmentList) []tilecompute.LineSoup {
	lines := make([]tilecompute.LineSoup, 0, segs.Len())

	for _, seg := range segs.Segments() {
		var p0, p1 [2]float32
		if seg.Winding < 0 {
			// Original direction was upward — swap back to restore direction
			p0 = [2]float32{seg.X1, seg.Y1}
			p1 = [2]float32{seg.X0, seg.Y0}
		} else {
			// Original direction was downward — keep as is
			p0 = [2]float32{seg.X0, seg.Y0}
			p1 = [2]float32{seg.X1, seg.Y1}
		}
		lines = append(lines, tilecompute.LineSoup{
			PathIx: 0,
			P0:     p0,
			P1:     p1,
		})
	}

	return lines
}
