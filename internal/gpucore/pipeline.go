package gpucore

import (
	"fmt"
	"sync"
)

// PipelineConfig configures a HybridPipeline.
type PipelineConfig struct {
	// Width is the viewport width in pixels.
	Width int

	// Height is the viewport height in pixels.
	Height int

	// MaxPaths is the maximum number of path elements to process.
	// If 0, defaults to 10000.
	MaxPaths int

	// MaxSegments is the maximum number of output segments.
	// If 0, defaults to MaxPaths * MaxSegmentsPerCurve.
	MaxSegments int

	// Tolerance is the flattening tolerance in pixels.
	// If 0, defaults to DefaultTolerance.
	Tolerance float32

	// UseCPUFallback forces CPU execution of all stages.
	// Useful for debugging or when GPU compute is unreliable.
	UseCPUFallback bool
}

// HybridPipeline orchestrates the GPU rendering pipeline.
//
// The pipeline consists of three stages:
//  1. Flatten: Convert Bezier curves to line segments
//  2. Coarse: Bin segments into tiles
//  3. Fine: Calculate per-pixel coverage
//
// Each stage can run on GPU or CPU depending on hardware support
// and configuration.
type HybridPipeline struct {
	mu sync.Mutex

	adapter GPUAdapter
	config  PipelineConfig

	// Computed dimensions
	tileColumns int
	tileRows    int
	tileCount   int

	// GPU resources (if using GPU path)
	// These will be populated in Phase 2 when algorithms are extracted

	// State
	initialized bool
	useGPU      bool
}

// NewHybridPipeline creates a new rendering pipeline.
//
// Parameters:
//   - adapter: GPU adapter implementation
//   - config: pipeline configuration
//
// Returns an error if initialization fails.
func NewHybridPipeline(adapter GPUAdapter, config *PipelineConfig) (*HybridPipeline, error) {
	if adapter == nil {
		return nil, fmt.Errorf("gpucore: adapter is required")
	}
	if config == nil {
		return nil, fmt.Errorf("gpucore: config is required")
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, fmt.Errorf("gpucore: invalid viewport size: %dx%d", config.Width, config.Height)
	}

	// Apply defaults
	cfg := *config
	if cfg.MaxPaths <= 0 {
		cfg.MaxPaths = 10000
	}
	if cfg.MaxSegments <= 0 {
		cfg.MaxSegments = cfg.MaxPaths * MaxSegmentsPerCurve
	}
	if cfg.Tolerance <= 0 {
		cfg.Tolerance = DefaultTolerance
	}

	// Calculate tile dimensions
	tileColumns := (cfg.Width + TileSize - 1) / TileSize
	tileRows := (cfg.Height + TileSize - 1) / TileSize
	tileCount := tileColumns * tileRows

	// Determine if GPU path is available
	useGPU := !cfg.UseCPUFallback && adapter.SupportsCompute()

	p := &HybridPipeline{
		adapter:     adapter,
		config:      cfg,
		tileColumns: tileColumns,
		tileRows:    tileRows,
		tileCount:   tileCount,
		useGPU:      useGPU,
	}

	if err := p.init(); err != nil {
		p.Destroy()
		return nil, err
	}

	return p, nil
}

// init initializes GPU resources if using GPU path.
func (p *HybridPipeline) init() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Phase 1: Just mark as initialized
	// Phase 2 will add shader compilation and pipeline creation

	p.initialized = true
	return nil
}

// Execute runs the rendering pipeline.
//
// This method orchestrates the three pipeline stages:
//  1. Flatten paths to line segments
//  2. Bin segments into tiles (coarse rasterization)
//  3. Calculate pixel coverage (fine rasterization)
//
// Parameters:
//   - paths: path data to render (format TBD in Phase 2)
//   - transform: world-to-viewport transform
//   - fillRule: NonZero or EvenOdd
//
// Returns coverage data or an error.
func (p *HybridPipeline) Execute(paths interface{}, transform AffineTransform, fillRule FillRule) ([]uint8, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return nil, fmt.Errorf("gpucore: pipeline not initialized")
	}

	// Phase 1: Skeleton only - return empty coverage
	// Phase 2 will implement actual rendering

	_ = paths
	_ = transform
	_ = fillRule

	// Return empty coverage for now
	coverageSize := p.tileCount * TileSize * TileSize
	return make([]uint8, coverageSize), nil
}

// Resize updates the pipeline for a new viewport size.
func (p *HybridPipeline) Resize(width, height int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if width <= 0 || height <= 0 {
		return fmt.Errorf("gpucore: invalid viewport size: %dx%d", width, height)
	}

	p.config.Width = width
	p.config.Height = height
	p.tileColumns = (width + TileSize - 1) / TileSize
	p.tileRows = (height + TileSize - 1) / TileSize
	p.tileCount = p.tileColumns * p.tileRows

	// Phase 2 will handle buffer reallocation if needed

	return nil
}

// SetTolerance updates the flattening tolerance.
func (p *HybridPipeline) SetTolerance(tolerance float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if tolerance > 0 {
		p.config.Tolerance = tolerance
	}
}

// Tolerance returns the current flattening tolerance.
func (p *HybridPipeline) Tolerance() float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.config.Tolerance
}

// UseGPU returns whether the pipeline is using GPU acceleration.
func (p *HybridPipeline) UseGPU() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.useGPU
}

// SetUseCPUFallback enables or disables CPU fallback mode.
// When enabled, all stages run on CPU regardless of GPU support.
func (p *HybridPipeline) SetUseCPUFallback(useCPU bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.UseCPUFallback = useCPU
	p.useGPU = !useCPU && p.adapter.SupportsCompute()
}

// Config returns a copy of the pipeline configuration.
func (p *HybridPipeline) Config() PipelineConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.config
}

// TileColumns returns the number of tile columns.
func (p *HybridPipeline) TileColumns() int {
	return p.tileColumns
}

// TileRows returns the number of tile rows.
func (p *HybridPipeline) TileRows() int {
	return p.tileRows
}

// TileCount returns the total number of tiles.
func (p *HybridPipeline) TileCount() int {
	return p.tileCount
}

// IsInitialized returns whether the pipeline is initialized.
func (p *HybridPipeline) IsInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized
}

// Destroy releases all GPU resources.
func (p *HybridPipeline) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Phase 2 will add resource cleanup
	// For now, just mark as uninitialized

	p.initialized = false
}

// PipelineStats contains pipeline execution statistics.
type PipelineStats struct {
	// PathCount is the number of paths processed.
	PathCount int

	// SegmentCount is the number of segments generated.
	SegmentCount int

	// TileEntryCount is the number of tile entries generated.
	TileEntryCount int

	// FlattenTimeNS is the time spent in the flatten stage (nanoseconds).
	FlattenTimeNS int64

	// CoarseTimeNS is the time spent in the coarse stage (nanoseconds).
	CoarseTimeNS int64

	// FineTimeNS is the time spent in the fine stage (nanoseconds).
	FineTimeNS int64

	// TotalTimeNS is the total execution time (nanoseconds).
	TotalTimeNS int64

	// UsedGPU indicates whether GPU was used for this execution.
	UsedGPU bool
}

// ExecuteWithStats runs the pipeline and returns execution statistics.
// This is useful for performance profiling and debugging.
func (p *HybridPipeline) ExecuteWithStats(paths interface{}, transform AffineTransform, fillRule FillRule) ([]uint8, *PipelineStats, error) {
	// Phase 1: Return empty stats
	// Phase 2 will implement actual timing

	coverage, err := p.Execute(paths, transform, fillRule)
	if err != nil {
		return nil, nil, err
	}

	stats := &PipelineStats{
		UsedGPU: p.useGPU,
	}

	return coverage, stats, nil
}
