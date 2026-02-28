package gg

// RasterizerMode controls which rasterization algorithm is used for path rendering.
//
// The default is RasterizerAuto, which uses intelligent multi-factor auto-selection
// based on path complexity, bounding box area, and shape type. Force modes bypass
// auto-selection and always use a specific algorithm.
//
// The mode is per-Context, not global. Different contexts can use different strategies.
//
// Use cases for force modes:
//   - Debugging: force AnalyticFiller to isolate tile rasterizer bugs
//   - Benchmarking: compare algorithms on the same workload
//   - Known workload: user knows their data better than heuristics
//   - Quality control: force SDF for maximum circle/rrect quality
//   - Regression testing: ensure each algorithm produces correct output
type RasterizerMode int

const (
	// RasterizerAuto uses intelligent multi-factor auto-selection (default).
	// It considers path complexity, bounding box area, shape type, and registered
	// fillers to choose the optimal algorithm per-path.
	RasterizerAuto RasterizerMode = iota

	// RasterizerAnalytic forces the AnalyticFiller (scanline) for all paths.
	// This bypasses CoverageFiller entirely, even for complex paths.
	// Best for: debugging, simple UIs, guaranteed minimal overhead.
	RasterizerAnalytic

	// RasterizerSparseStrips forces SparseStrips (4x4 tiles) for all paths.
	// Bypasses the adaptive threshold and always uses SparseStrips regardless
	// of path complexity.
	// Requires a CoverageFiller with ForceableFiller support to be registered.
	// Best for: CPU-optimized rendering, SIMD-friendly workloads.
	RasterizerSparseStrips

	// RasterizerTileCompute forces TileCompute (16x16 tiles) for all paths.
	// Bypasses the adaptive threshold and always uses TileCompute regardless
	// of path complexity.
	// Requires a CoverageFiller with ForceableFiller support to be registered.
	// Best for: GPU workgroup-ready workloads, extreme complexity.
	RasterizerTileCompute

	// RasterizerSDF forces SDF rendering for detected shapes, bypassing the
	// minimum size check. Non-SDF shapes fall back to auto-selection.
	// Best for: maximum visual quality on geometric shapes (circles, rrects).
	RasterizerSDF
)

// String returns the rasterizer mode name.
func (m RasterizerMode) String() string {
	switch m {
	case RasterizerAuto:
		return "Auto"
	case RasterizerAnalytic:
		return "Analytic"
	case RasterizerSparseStrips:
		return "SparseStrips"
	case RasterizerTileCompute:
		return "TileCompute"
	case RasterizerSDF:
		return "SDF"
	default:
		return "Unknown"
	}
}
