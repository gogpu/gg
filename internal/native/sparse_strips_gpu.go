//go:build !nogpu

// Package wgpu provides GPU-accelerated rendering using WebGPU.
package native

import (
	"log"
	"sync"

	"github.com/gogpu/gg/scene"
	"github.com/gogpu/wgpu/hal"
)

// DefaultGPUSegmentThreshold is the minimum number of segments to use GPU.
// Below this threshold, CPU is typically faster due to GPU dispatch overhead.
const DefaultGPUSegmentThreshold = 100

// GPURasterizer is the interface for GPU-accelerated rasterization.
type GPURasterizer interface {
	// Rasterize performs fine rasterization and returns coverage values.
	Rasterize(
		coarse *CoarseRasterizer,
		segments *SegmentList,
		backdrop []int32,
		fillRule scene.FillStyle,
	) ([]uint8, error)

	// Destroy releases GPU resources.
	Destroy()
}

// HybridFineRasterizer automatically selects between GPU and CPU rasterization
// based on workload size and GPU availability.
type HybridFineRasterizer struct {
	mu sync.RWMutex

	// GPU rasterizer (nil if not available)
	gpu *GPUFineRasterizer

	// CPU fallback
	cpu *FineRasterizer

	// Configuration
	segmentThreshold int  // Minimum segments for GPU
	gpuAvailable     bool // Whether GPU is available

	// Viewport dimensions
	width  uint16
	height uint16
}

// HybridFineRasterizerConfig configures the hybrid rasterizer.
type HybridFineRasterizerConfig struct {
	// Device and Queue for GPU operations (nil to use CPU only)
	Device hal.Device
	Queue  hal.Queue

	// SegmentThreshold is the minimum segments to use GPU (0 = use default)
	SegmentThreshold int

	// ForceGPU forces GPU even for small workloads (for testing)
	ForceGPU bool

	// ForceCPU disables GPU entirely (for testing/fallback)
	ForceCPU bool
}

// NewHybridFineRasterizer creates a hybrid rasterizer that automatically
// selects between GPU and CPU based on workload.
func NewHybridFineRasterizer(width, height uint16, config HybridFineRasterizerConfig) *HybridFineRasterizer {
	h := &HybridFineRasterizer{
		cpu:              NewFineRasterizer(width, height),
		width:            width,
		height:           height,
		segmentThreshold: config.SegmentThreshold,
	}

	if h.segmentThreshold <= 0 {
		h.segmentThreshold = DefaultGPUSegmentThreshold
	}

	// Try to create GPU rasterizer if not forced to CPU
	if !config.ForceCPU && config.Device != nil && config.Queue != nil {
		gpu, err := NewGPUFineRasterizer(config.Device, config.Queue, width, height)
		if err != nil {
			log.Printf("wgpu: GPU fine rasterizer unavailable, using CPU: %v", err)
		} else {
			h.gpu = gpu
			h.gpuAvailable = true
			log.Println("wgpu: GPU fine rasterizer enabled")
		}
	}

	return h
}

// Rasterize performs fine rasterization, automatically selecting GPU or CPU.
func (h *HybridFineRasterizer) Rasterize(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if coarse == nil || segments == nil || len(coarse.Entries()) == 0 {
		h.cpu.Reset()
		return
	}

	// Decide whether to use GPU
	useGPU := h.shouldUseGPU(segments.Len())

	if useGPU {
		coverage, err := h.gpu.Rasterize(coarse, segments, backdrop, h.cpu.FillRule())
		if err != nil {
			log.Printf("wgpu: GPU rasterization failed, falling back to CPU: %v", err)
			// Fall through to CPU fallback below
		} else {
			// Convert GPU coverage to tile grid
			h.coverageToGrid(coverage, coarse)
			return
		}
	}

	// CPU fallback
	h.cpu.Rasterize(coarse, segments, backdrop)
}

// shouldUseGPU determines if GPU should be used for this workload.
func (h *HybridFineRasterizer) shouldUseGPU(segmentCount int) bool {
	if !h.gpuAvailable || h.gpu == nil {
		return false
	}
	return segmentCount >= h.segmentThreshold
}

// coverageToGrid converts GPU coverage output to CPU tile grid.
func (h *HybridFineRasterizer) coverageToGrid(coverage []uint8, coarse *CoarseRasterizer) {
	if len(coverage) == 0 {
		return
	}

	h.cpu.Reset()

	// Group entries by tile to build the grid
	entries := coarse.Entries()
	type tileKey struct {
		x, y uint16
	}
	tileSet := make(map[tileKey]bool)

	for _, e := range entries {
		key := tileKey{e.X, e.Y}
		if !tileSet[key] {
			tileSet[key] = true
		}
	}

	// Populate tile grid from coverage data
	tileIdx := 0
	for key := range tileSet {
		tile := h.cpu.grid.GetOrCreate(int32(key.x), int32(key.y))

		// Copy coverage for this tile
		baseOffset := tileIdx * TileSize * TileSize
		if baseOffset+TileSize*TileSize <= len(coverage) {
			for y := 0; y < TileSize; y++ {
				for x := 0; x < TileSize; x++ {
					idx := baseOffset + y*TileSize + x
					tile.SetCoverage(x, y, coverage[idx])
				}
			}
		}
		tileIdx++
	}
}

// SetFillRule sets the fill rule for coverage calculation.
func (h *HybridFineRasterizer) SetFillRule(rule scene.FillStyle) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cpu.SetFillRule(rule)
}

// raster.FillRule returns the current fill rule.
func (h *HybridFineRasterizer) FillRule() scene.FillStyle {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cpu.FillRule()
}

// Grid returns the tile grid with computed coverage.
func (h *HybridFineRasterizer) Grid() *TileGrid {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cpu.Grid()
}

// Reset clears the rasterizer state for reuse.
func (h *HybridFineRasterizer) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cpu.Reset()
}

// IsGPUAvailable returns whether GPU rasterization is available.
func (h *HybridFineRasterizer) IsGPUAvailable() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.gpuAvailable
}

// Destroy releases all resources.
func (h *HybridFineRasterizer) Destroy() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.gpu != nil {
		h.gpu.Destroy()
		h.gpu = nil
	}
	h.gpuAvailable = false
}

// GPURasterizerStats contains statistics about GPU rasterization.
type GPURasterizerStats struct {
	// GPUAvailable indicates if GPU is available
	GPUAvailable bool

	// TotalCalls is the total number of rasterization calls
	TotalCalls uint64

	// GPUCalls is the number of calls that used GPU
	GPUCalls uint64

	// CPUCalls is the number of calls that used CPU
	CPUCalls uint64

	// SegmentThreshold is the threshold for GPU usage
	SegmentThreshold int
}

// Stats returns statistics about rasterization calls.
// Note: Currently returns static info; could track actual call counts.
func (h *HybridFineRasterizer) Stats() GPURasterizerStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return GPURasterizerStats{
		GPUAvailable:     h.gpuAvailable,
		SegmentThreshold: h.segmentThreshold,
	}
}

// CheckGPUComputeSupport checks if GPU compute shaders are supported.
// This can be used to determine if GPU rasterization is viable before
// creating a rasterizer.
func CheckGPUComputeSupport(device hal.Device) bool {
	if device == nil {
		return false
	}

	// TODO: Query device capabilities for compute shader support
	// For now, assume compute is supported if we have a device
	// The actual check would query DownlevelCapabilities.Flags

	return true
}

// -----------------------------------------------------------------------------
// HybridPipeline: Full GPU/CPU Integration
// -----------------------------------------------------------------------------

// DefaultFlattenThreshold is the minimum path elements for GPU flattening.
const DefaultFlattenThreshold = 50

// DefaultCoarseThreshold is the minimum segments for GPU coarse rasterization.
const DefaultCoarseThreshold = 100

// DefaultFineThreshold is the minimum tile entries for GPU fine rasterization.
const DefaultFineThreshold = 100

// PipelineStage represents which stage of the pipeline is being executed.
type PipelineStage uint8

const (
	// StageFlatten is the path flattening stage (path -> segments).
	StageFlatten PipelineStage = iota
	// StageCoarse is the coarse rasterization stage (segments -> tile bins).
	StageCoarse
	// StageFine is the fine rasterization stage (tile bins -> coverage).
	StageFine
)

// String returns a human-readable name for the pipeline stage.
func (s PipelineStage) String() string {
	switch s {
	case StageFlatten:
		return "Flatten"
	case StageCoarse:
		return "Coarse"
	case StageFine:
		return "Fine"
	default:
		return "Unknown"
	}
}

// HybridPipelineConfig configures the hybrid pipeline.
type HybridPipelineConfig struct {
	// Device and Queue for GPU operations (nil to use CPU only)
	Device hal.Device
	Queue  hal.Queue

	// Stage-specific thresholds (0 = use defaults)
	FlattenThreshold int // Min path elements for GPU flatten
	CoarseThreshold  int // Min segments for GPU coarse
	FineThreshold    int // Min tile entries for GPU fine

	// MaxPaths is the maximum path elements for flatten (0 = default 1024)
	MaxPaths int

	// MaxSegments is the maximum segments for flatten (0 = default MaxPaths * 64)
	MaxSegments int

	// Force flags for testing
	ForceGPU bool // Force GPU for all stages (ignores thresholds)
	ForceCPU bool // Force CPU for all stages (disables GPU)

	// Tolerance for curve flattening (0 = use default)
	Tolerance float32
}

// HybridPipelineStats contains statistics about pipeline execution.
type HybridPipelineStats struct {
	// GPU availability per stage
	FlattenGPUAvailable bool
	CoarseGPUAvailable  bool
	FineGPUAvailable    bool

	// Call counts per stage
	FlattenTotalCalls uint64
	FlattenGPUCalls   uint64
	FlattenCPUCalls   uint64

	CoarseTotalCalls uint64
	CoarseGPUCalls   uint64
	CoarseCPUCalls   uint64

	FineTotalCalls uint64
	FineGPUCalls   uint64
	FineCPUCalls   uint64

	// Thresholds
	FlattenThreshold int
	CoarseThreshold  int
	FineThreshold    int

	// Last operation details
	LastPathElements   int
	LastSegmentCount   int
	LastTileEntryCount int
	LastFlattenUsedGPU bool
	LastCoarseUsedGPU  bool
	LastFineUsedGPU    bool
}

// HybridPipeline integrates all three GPU shaders into a unified pipeline:
// Flatten (path -> segments) -> Coarse (segments -> tile bins) -> Fine (tile bins -> coverage)
//
// The pipeline automatically selects GPU or CPU for each stage based on workload
// size and GPU availability.
type HybridPipeline struct {
	mu sync.RWMutex

	// GPU rasterizers (nil if not available)
	flatten *GPUFlattenRasterizer
	coarse  *GPUCoarseRasterizer
	fine    *GPUFineRasterizer

	// CPU fallbacks
	cpuFlatten *FlattenContext
	cpuCoarse  *CoarseRasterizer
	cpuFine    *FineRasterizer

	// Configuration
	flattenThreshold int
	coarseThreshold  int
	fineThreshold    int
	forceGPU         bool
	forceCPU         bool
	tolerance        float32

	// GPU availability per stage
	flattenGPUAvailable bool
	coarseGPUAvailable  bool
	fineGPUAvailable    bool

	// Statistics
	stats HybridPipelineStats

	// Viewport dimensions
	width  uint16
	height uint16

	// Reusable intermediate storage
	segments *SegmentList
	backdrop []int32
}

// NewHybridPipeline creates a new hybrid pipeline that integrates all GPU stages.
func NewHybridPipeline(width, height uint16, config HybridPipelineConfig) *HybridPipeline {
	p := &HybridPipeline{
		width:            width,
		height:           height,
		flattenThreshold: config.FlattenThreshold,
		coarseThreshold:  config.CoarseThreshold,
		fineThreshold:    config.FineThreshold,
		forceGPU:         config.ForceGPU,
		forceCPU:         config.ForceCPU,
		tolerance:        config.Tolerance,
		segments:         NewSegmentList(),
	}

	// Apply defaults
	if p.flattenThreshold <= 0 {
		p.flattenThreshold = DefaultFlattenThreshold
	}
	if p.coarseThreshold <= 0 {
		p.coarseThreshold = DefaultCoarseThreshold
	}
	if p.fineThreshold <= 0 {
		p.fineThreshold = DefaultFineThreshold
	}
	if p.tolerance <= 0 {
		p.tolerance = FlattenTolerance
	}

	// Create CPU fallbacks (always available)
	p.cpuFlatten = NewFlattenContext()
	p.cpuCoarse = NewCoarseRasterizer(width, height)
	p.cpuFine = NewFineRasterizer(width, height)

	// Store thresholds in stats
	p.stats.FlattenThreshold = p.flattenThreshold
	p.stats.CoarseThreshold = p.coarseThreshold
	p.stats.FineThreshold = p.fineThreshold

	// Try to create GPU rasterizers if not forced to CPU
	if !config.ForceCPU && config.Device != nil && config.Queue != nil {
		p.initGPURasterizers(config)
	}

	return p
}

// initGPURasterizers attempts to create GPU rasterizers for each stage.
func (p *HybridPipeline) initGPURasterizers(config HybridPipelineConfig) {
	maxPaths := config.MaxPaths
	if maxPaths <= 0 {
		maxPaths = 1024
	}
	maxSegments := config.MaxSegments
	if maxSegments <= 0 {
		maxSegments = maxPaths * FlattenMaxSegmentsPerCurve
	}

	// Flatten rasterizer
	flatten, err := NewGPUFlattenRasterizer(config.Device, config.Queue, maxPaths, maxSegments)
	if err != nil {
		log.Printf("wgpu: GPU flatten rasterizer unavailable: %v", err)
	} else {
		p.flatten = flatten
		p.flattenGPUAvailable = true
		log.Println("wgpu: GPU flatten rasterizer enabled")
	}

	// Coarse rasterizer
	coarse, err := NewGPUCoarseRasterizer(config.Device, config.Queue, p.width, p.height)
	if err != nil {
		log.Printf("wgpu: GPU coarse rasterizer unavailable: %v", err)
	} else {
		p.coarse = coarse
		p.coarseGPUAvailable = true
		log.Println("wgpu: GPU coarse rasterizer enabled")
	}

	// Fine rasterizer
	fine, err := NewGPUFineRasterizer(config.Device, config.Queue, p.width, p.height)
	if err != nil {
		log.Printf("wgpu: GPU fine rasterizer unavailable: %v", err)
	} else {
		p.fine = fine
		p.fineGPUAvailable = true
		log.Println("wgpu: GPU fine rasterizer enabled")
	}

	// Update stats
	p.stats.FlattenGPUAvailable = p.flattenGPUAvailable
	p.stats.CoarseGPUAvailable = p.coarseGPUAvailable
	p.stats.FineGPUAvailable = p.fineGPUAvailable
}

// RasterizePath runs the full pipeline: path -> segments -> tile bins -> coverage.
//
// Parameters:
//   - path: The input path to rasterize
//   - transform: Affine transformation to apply
//   - fillRule: Fill rule for coverage calculation
//
// Returns the tile grid with computed coverage.
func (p *HybridPipeline) RasterizePath(
	path *scene.Path,
	transform scene.Affine,
	fillRule scene.FillStyle,
) *TileGrid {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Handle empty path
	if path == nil || path.IsEmpty() {
		p.cpuFine.Reset()
		return p.cpuFine.Grid()
	}

	// Stage 1: Flatten (path -> segments)
	segments := p.runFlattenStage(path, transform)

	// Handle empty segments
	if segments == nil || segments.Len() == 0 {
		p.cpuFine.Reset()
		return p.cpuFine.Grid()
	}

	// Stage 2: Coarse (segments -> tile bins)
	p.runCoarseStage(segments)

	// Handle empty coarse entries
	if len(p.cpuCoarse.Entries()) == 0 {
		p.cpuFine.Reset()
		return p.cpuFine.Grid()
	}

	// Calculate backdrop for winding
	p.backdrop = p.cpuCoarse.CalculateBackdrop()

	// Stage 3: Fine (tile bins -> coverage)
	p.runFineStage(segments, fillRule)

	return p.cpuFine.Grid()
}

// runFlattenStage executes the flatten stage (path -> segments).
func (p *HybridPipeline) runFlattenStage(path *scene.Path, transform scene.Affine) *SegmentList {
	pathElements := path.VerbCount()
	p.stats.LastPathElements = pathElements
	p.stats.FlattenTotalCalls++

	useGPU := p.shouldUseGPU(StageFlatten, pathElements)
	p.stats.LastFlattenUsedGPU = useGPU

	if useGPU && p.flatten != nil {
		// Try GPU flatten
		segments, err := p.flatten.Flatten(path, transform, p.tolerance)
		if err == nil && segments != nil {
			p.stats.FlattenGPUCalls++
			p.stats.LastSegmentCount = segments.Len()
			return segments
		}
		log.Printf("wgpu: GPU flatten failed, falling back to CPU: %v", err)
	}

	// CPU fallback
	p.stats.FlattenCPUCalls++
	p.cpuFlatten.Reset()
	p.cpuFlatten.FlattenPathTo(path, transform, p.tolerance)
	segments := p.cpuFlatten.Segments()
	p.stats.LastSegmentCount = segments.Len()
	return segments
}

// runCoarseStage executes the coarse stage (segments -> tile bins).
func (p *HybridPipeline) runCoarseStage(segments *SegmentList) {
	segmentCount := segments.Len()
	p.stats.CoarseTotalCalls++

	useGPU := p.shouldUseGPU(StageCoarse, segmentCount)
	p.stats.LastCoarseUsedGPU = useGPU

	if useGPU && p.coarse != nil {
		// Try GPU coarse
		entries, err := p.coarse.Rasterize(segments)
		if err == nil && entries != nil {
			p.stats.CoarseGPUCalls++
			// Convert GPU entries back to CPU coarse format
			p.gpuEntriesToCPUCoarse(entries)
			p.stats.LastTileEntryCount = len(p.cpuCoarse.Entries())
			return
		}
		log.Printf("wgpu: GPU coarse failed, falling back to CPU: %v", err)
	}

	// CPU fallback
	p.stats.CoarseCPUCalls++
	p.cpuCoarse.Reset()
	p.cpuCoarse.Rasterize(segments)
	p.stats.LastTileEntryCount = len(p.cpuCoarse.Entries())
}

// gpuEntriesToCPUCoarse converts GPU tile entries to CPU coarse format.
func (p *HybridPipeline) gpuEntriesToCPUCoarse(gpuEntries []GPUTileSegmentRef) {
	p.cpuCoarse.Reset()

	for _, e := range gpuEntries {
		//nolint:gosec // GPU entries have uint32 but tile coords fit in uint16
		p.cpuCoarse.addEntry(uint16(e.TileX), uint16(e.TileY), e.SegmentIdx, e.WindingFlag == 1)
	}
}

// runFineStage executes the fine stage (tile bins -> coverage).
func (p *HybridPipeline) runFineStage(segments *SegmentList, fillRule scene.FillStyle) {
	tileEntryCount := len(p.cpuCoarse.Entries())
	p.stats.FineTotalCalls++

	useGPU := p.shouldUseGPU(StageFine, tileEntryCount)
	p.stats.LastFineUsedGPU = useGPU

	if useGPU && p.fine != nil {
		// Try GPU fine
		coverage, err := p.fine.Rasterize(p.cpuCoarse, segments, p.backdrop, fillRule)
		if err == nil && coverage != nil {
			p.stats.FineGPUCalls++
			p.coverageToFineGrid(coverage)
			return
		}
		log.Printf("wgpu: GPU fine failed, falling back to CPU: %v", err)
	}

	// CPU fallback
	p.stats.FineCPUCalls++
	p.cpuFine.Reset()
	p.cpuFine.SetFillRule(fillRule)
	p.cpuFine.Rasterize(p.cpuCoarse, segments, p.backdrop)
}

// coverageToFineGrid converts GPU coverage output to the fine rasterizer grid.
func (p *HybridPipeline) coverageToFineGrid(coverage []uint8) {
	if len(coverage) == 0 {
		p.cpuFine.Reset()
		return
	}

	p.cpuFine.Reset()

	// Group entries by tile to build the grid
	entries := p.cpuCoarse.Entries()
	type tileKey struct {
		x, y uint16
	}
	tileSet := make(map[tileKey]bool)

	for _, e := range entries {
		key := tileKey{e.X, e.Y}
		if !tileSet[key] {
			tileSet[key] = true
		}
	}

	// Populate tile grid from coverage data
	tileIdx := 0
	for key := range tileSet {
		tile := p.cpuFine.grid.GetOrCreate(int32(key.x), int32(key.y))

		// Copy coverage for this tile
		baseOffset := tileIdx * TileSize * TileSize
		if baseOffset+TileSize*TileSize <= len(coverage) {
			for y := 0; y < TileSize; y++ {
				for x := 0; x < TileSize; x++ {
					idx := baseOffset + y*TileSize + x
					tile.SetCoverage(x, y, coverage[idx])
				}
			}
		}
		tileIdx++
	}
}

// shouldUseGPU determines if GPU should be used for the given stage and workload.
func (p *HybridPipeline) shouldUseGPU(stage PipelineStage, workloadSize int) bool {
	// Force flags override thresholds
	if p.forceCPU {
		return false
	}
	if p.forceGPU {
		switch stage {
		case StageFlatten:
			return p.flattenGPUAvailable
		case StageCoarse:
			return p.coarseGPUAvailable
		case StageFine:
			return p.fineGPUAvailable
		}
		return false
	}

	// Check GPU availability and threshold
	switch stage {
	case StageFlatten:
		return p.flattenGPUAvailable && workloadSize >= p.flattenThreshold
	case StageCoarse:
		return p.coarseGPUAvailable && workloadSize >= p.coarseThreshold
	case StageFine:
		return p.fineGPUAvailable && workloadSize >= p.fineThreshold
	}

	return false
}

// SetFillRule sets the fill rule for coverage calculation.
func (p *HybridPipeline) SetFillRule(rule scene.FillStyle) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cpuFine.SetFillRule(rule)
}

// SetTolerance sets the flattening tolerance.
func (p *HybridPipeline) SetTolerance(tolerance float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if tolerance > 0 {
		p.tolerance = tolerance
		if p.flatten != nil {
			p.flatten.SetTolerance(tolerance)
		}
	}
}

// Grid returns the tile grid with computed coverage.
func (p *HybridPipeline) Grid() *TileGrid {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cpuFine.Grid()
}

// Reset clears all rasterizer state for reuse.
func (p *HybridPipeline) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cpuFlatten.Reset()
	p.cpuCoarse.Reset()
	p.cpuFine.Reset()
	p.segments.Reset()
	p.backdrop = p.backdrop[:0]
}

// IsGPUAvailable returns whether any GPU stage is available.
func (p *HybridPipeline) IsGPUAvailable() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.flattenGPUAvailable || p.coarseGPUAvailable || p.fineGPUAvailable
}

// IsStageGPUAvailable returns whether a specific stage has GPU available.
func (p *HybridPipeline) IsStageGPUAvailable(stage PipelineStage) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch stage {
	case StageFlatten:
		return p.flattenGPUAvailable
	case StageCoarse:
		return p.coarseGPUAvailable
	case StageFine:
		return p.fineGPUAvailable
	}
	return false
}

// Stats returns statistics about pipeline execution.
func (p *HybridPipeline) Stats() HybridPipelineStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// ResetStats resets all statistics counters.
func (p *HybridPipeline) ResetStats() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Preserve configuration
	thresholds := struct {
		flatten, coarse, fine          int
		flattenGPU, coarseGPU, fineGPU bool
	}{
		p.stats.FlattenThreshold,
		p.stats.CoarseThreshold,
		p.stats.FineThreshold,
		p.stats.FlattenGPUAvailable,
		p.stats.CoarseGPUAvailable,
		p.stats.FineGPUAvailable,
	}

	p.stats = HybridPipelineStats{
		FlattenThreshold:    thresholds.flatten,
		CoarseThreshold:     thresholds.coarse,
		FineThreshold:       thresholds.fine,
		FlattenGPUAvailable: thresholds.flattenGPU,
		CoarseGPUAvailable:  thresholds.coarseGPU,
		FineGPUAvailable:    thresholds.fineGPU,
	}
}

// Destroy releases all GPU resources.
func (p *HybridPipeline) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.flatten != nil {
		p.flatten.Destroy()
		p.flatten = nil
	}
	if p.coarse != nil {
		p.coarse.Destroy()
		p.coarse = nil
	}
	if p.fine != nil {
		p.fine.Destroy()
		p.fine = nil
	}

	p.flattenGPUAvailable = false
	p.coarseGPUAvailable = false
	p.fineGPUAvailable = false
}
