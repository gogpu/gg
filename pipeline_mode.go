package gg

// PipelineMode selects the GPU rendering pipeline.
type PipelineMode int

const (
	// PipelineModeAuto lets the framework select the best pipeline
	// based on scene complexity and GPU capabilities.
	PipelineModeAuto PipelineMode = iota

	// PipelineModeRenderPass forces the traditional multi-tier render pass
	// pipeline (SDF, Convex, Stencil-then-Cover, MSDF Text).
	PipelineModeRenderPass

	// PipelineModeCompute forces the Vello-style compute pipeline
	// (scene encoding, tile binning, PTCL fine rasterization).
	PipelineModeCompute
)

// String returns the pipeline mode name.
func (m PipelineMode) String() string {
	switch m {
	case PipelineModeAuto:
		return "Auto"
	case PipelineModeRenderPass:
		return "RenderPass"
	case PipelineModeCompute:
		return "Compute"
	default:
		return "Unknown"
	}
}

// SceneStats holds metrics for pipeline auto-selection.
// These are computed by analyzing the current frame's draw operations.
type SceneStats struct {
	ShapeCount    int     // Total number of shapes
	PathCount     int     // Complex paths (not simple SDF shapes)
	TextCount     int     // Text elements
	ClipDepth     int     // Maximum clip nesting depth
	OverlapFactor float64 // Estimated overlap ratio [0, 1]
}

// SelectPipeline chooses the best pipeline based on scene statistics
// and GPU capabilities.
//
// Heuristics:
//   - Simple scenes (< 10 shapes, shallow clips): RenderPass is faster
//     (no encoding overhead, direct GPU draw calls)
//   - Complex scenes (> 50 shapes, deep clips, high overlap): Compute excels
//     (massively parallel tile-based processing)
//   - Text-heavy: RenderPass (MSDF Text tier is specialized)
//   - Default for medium complexity: Compute
func SelectPipeline(stats SceneStats, hasComputeSupport bool) PipelineMode {
	if !hasComputeSupport {
		return PipelineModeRenderPass
	}

	// Simple scenes: render pass is faster (no encoding overhead)
	if stats.ShapeCount < 10 && stats.ClipDepth < 2 {
		return PipelineModeRenderPass
	}

	// Complex scenes: compute excels
	if stats.ShapeCount > 50 || stats.ClipDepth > 3 || stats.OverlapFactor > 0.5 {
		return PipelineModeCompute
	}

	// Text-heavy: render pass (MSDF Text tier is specialized)
	if stats.TextCount > 0 {
		total := stats.ShapeCount + stats.TextCount
		if total > 0 {
			textRatio := float64(stats.TextCount) / float64(total)
			if textRatio > 0.6 {
				return PipelineModeRenderPass
			}
		}
	}

	// Default: compute for medium complexity
	return PipelineModeCompute
}
