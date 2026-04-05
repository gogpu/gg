//go:build !nogpu

package gpu

import (
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// SparseStripsFiller implements gg.CoverageFiller using the SparseStrips
// rasterizer (4x4 tiles). This is the default filler, optimized for CPU
// with SIMD-friendly tile sizes.
type SparseStripsFiller struct{}

// FillCoverage rasterizes the path using SparseStrips and calls callback
// for each pixel with non-zero coverage.
func (f *SparseStripsFiller) FillCoverage(
	path *gg.Path, width, height int, fillRule gg.FillRule,
	callback func(x, y int, coverage uint8),
) {
	if path == nil || path.NumVerbs() == 0 {
		return
	}

	// 1. Convert gg.Path → scene.Path
	scenePath := convertGGToScenePath(path)
	if scenePath.IsEmpty() {
		return
	}

	// 2. Get SparseStripsRasterizer from pool
	config := DefaultConfig(uint16(width), uint16(height)) //nolint:gosec // width/height bounded by viewport
	config.FillRule = convertToSceneFillRule(fillRule)
	ssr := globalSparseStripsPool.Get(config)
	defer globalSparseStripsPool.Put(ssr)

	// 3. Rasterize
	ssr.RasterizePath(scenePath, scene.IdentityAffine(), FlattenTolerance)

	// 4. Walk TileGrid → callback
	grid := ssr.Grid()
	grid.ForEach(func(tile *Tile) {
		baseX := int(tile.PixelX())
		baseY := int(tile.PixelY())
		for py := 0; py < TileSize; py++ {
			y := baseY + py
			if y < 0 || y >= height {
				continue
			}
			for px := 0; px < TileSize; px++ {
				x := baseX + px
				if x < 0 || x >= width {
					continue
				}
				c := tile.GetCoverage(px, py)
				if c > 0 {
					callback(x, y, c)
				}
			}
		}
	})
}

// convertGGToScenePath converts a gg.Path (float64) to a scene.Path (float32).
func convertGGToScenePath(p *gg.Path) *scene.Path {
	sp := scene.NewPath()
	p.Iterate(func(verb gg.PathVerb, coords []float64) {
		switch verb {
		case gg.MoveTo:
			sp.MoveTo(float32(coords[0]), float32(coords[1]))
		case gg.LineTo:
			sp.LineTo(float32(coords[0]), float32(coords[1]))
		case gg.QuadTo:
			sp.QuadTo(
				float32(coords[0]), float32(coords[1]),
				float32(coords[2]), float32(coords[3]),
			)
		case gg.CubicTo:
			sp.CubicTo(
				float32(coords[0]), float32(coords[1]),
				float32(coords[2]), float32(coords[3]),
				float32(coords[4]), float32(coords[5]),
			)
		case gg.Close:
			sp.Close()
		}
	})
	return sp
}

// convertToSceneFillRule converts gg.FillRule to scene.FillStyle.
func convertToSceneFillRule(rule gg.FillRule) scene.FillStyle {
	if rule == gg.FillRuleEvenOdd {
		return scene.FillEvenOdd
	}
	return scene.FillNonZero
}
