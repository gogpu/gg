//go:build !nogpu

package gpu

import "github.com/gogpu/gg"

const (
	// extremeSegmentThreshold is the estimated segment count above which
	// TileCompute (16x16) is preferred over SparseStrips (4x4).
	// At 10K+ segments, hashmap pressure for 4x4 tiles becomes significant
	// because the number of active tiles grows quadratically with path complexity,
	// while 16x16 tiles reduce tile count by 16x.
	extremeSegmentThreshold = 10000

	// largeBBoxThreshold is the bounding box area (px^2) above which
	// TileCompute is considered. Approximately 1920x1080 = 2 megapixels.
	// Below this, 4x4 tiles provide better cache locality and lower overhead.
	largeBBoxThreshold = 2_000_000

	// segmentMultiplier estimates flattened segments from path elements.
	// Average: 1x for lines, ~4x for quadratics, ~8x for cubics.
	// Typical mixed paths average ~3x after curve flattening.
	segmentMultiplier = 3
)

// AdaptiveFiller selects between SparseStrips (4x4) and TileCompute (16x16)
// based on path complexity and canvas size.
//
// For most paths, SparseStrips is faster due to smaller tile granularity and
// SIMD-friendly 4x4 layout. TileCompute becomes advantageous for extremely
// complex paths (10K+ segments) on large canvases, where 16x16 tiles reduce
// the overhead of tile management and backdrop propagation.
type AdaptiveFiller struct {
	sparse  SparseStripsFiller
	compute TileComputeFiller
}

// SparseFiller returns the SparseStrips (4x4 tiles) filler component.
func (f *AdaptiveFiller) SparseFiller() gg.CoverageFiller {
	return &f.sparse
}

// ComputeFiller returns the TileCompute (16x16 tiles) filler component.
func (f *AdaptiveFiller) ComputeFiller() gg.CoverageFiller {
	return &f.compute
}

// FillCoverage rasterizes the path using the appropriate tile-based filler.
// It estimates segment count from path element count and selects TileCompute
// only for very complex paths on large canvases.
func (f *AdaptiveFiller) FillCoverage(
	path *gg.Path, width, height int, fillRule gg.FillRule,
	callback func(x, y int, coverage uint8),
) {
	elements := len(path.Elements())
	estimatedSegments := elements * segmentMultiplier
	canvasArea := width * height

	if estimatedSegments > extremeSegmentThreshold && canvasArea > largeBBoxThreshold {
		f.compute.FillCoverage(path, width, height, fillRule, callback)
		return
	}
	f.sparse.FillCoverage(path, width, height, fillRule, callback)
}
