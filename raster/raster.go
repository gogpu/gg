//go:build !nogpu

// Package raster registers the CPU tile-based coverage filler for complex paths.
//
// Import this package to enable adaptive tile rasterization (4x4 or 16x16 tiles)
// for paths above the complexity threshold, without requiring GPU acceleration.
// The filler auto-selects SparseStrips (4x4 tiles) for typical paths and
// TileCompute (16x16 tiles) for extremely complex paths on large canvases.
//
// If GPU acceleration is also needed, use import _ "github.com/gogpu/gg/gpu"
// instead, which registers both the GPU accelerator and the coverage filler.
//
// Usage:
//
//	import _ "github.com/gogpu/gg/raster" // enable CPU tile rasterization
package raster

import (
	"github.com/gogpu/gg"
	gpuimpl "github.com/gogpu/gg/internal/gpu"
)

func init() {
	gg.RegisterCoverageFiller(&gpuimpl.AdaptiveFiller{})
}
