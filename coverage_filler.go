package gg

import "sync"

// CoverageFiller is a tile-based coverage rasterizer for complex paths.
//
// When registered via RegisterCoverageFiller, the SoftwareRenderer auto-selects
// it for paths above a complexity threshold (coverageFillerThreshold elements).
// For simpler paths, the AnalyticFiller (scanline) is used directly.
//
// The callback receives per-pixel coverage values (0-255) that the caller
// composites onto the target pixmap using source-over blending.
//
// Implementations:
//   - SparseStripsFiller (4x4 tiles, CPU/SIMD-optimized) — default
//   - TileComputeFiller (16x16 tiles, GPU workgroup-ready) — alternative
type CoverageFiller interface {
	// FillCoverage rasterizes the path and calls callback for each pixel
	// with non-zero coverage. The coverage value is 0-255 (anti-aliased alpha).
	FillCoverage(path *Path, width, height int, fillRule FillRule,
		callback func(x, y int, coverage uint8))
}

var (
	coverageMu     sync.RWMutex
	coverageFiller CoverageFiller
)

// RegisterCoverageFiller registers a tile-based coverage filler for complex paths.
//
// Only one filler can be registered. Subsequent calls replace the previous one.
// Typical usage via blank import in the gpu package:
//
//	func init() {
//	    gg.RegisterCoverageFiller(&gpuimpl.SparseStripsFiller{})
//	}
func RegisterCoverageFiller(f CoverageFiller) {
	coverageMu.Lock()
	coverageFiller = f
	coverageMu.Unlock()
}

// GetCoverageFiller returns the registered CoverageFiller, or nil if none.
func GetCoverageFiller() CoverageFiller {
	coverageMu.RLock()
	f := coverageFiller
	coverageMu.RUnlock()
	return f
}

// ForceableFiller is an optional interface for CoverageFillers that support
// forced algorithm selection. When the user sets RasterizerSparseStrips or
// RasterizerTileCompute mode, the renderer uses SparseFiller() or ComputeFiller()
// instead of the adaptive auto-selection.
type ForceableFiller interface {
	CoverageFiller
	// SparseFiller returns the SparseStrips (4x4 tiles) filler component.
	SparseFiller() CoverageFiller
	// ComputeFiller returns the TileCompute (16x16 tiles) filler component.
	ComputeFiller() CoverageFiller
}
