//go:build !nogpu

// Package wgpu provides GPU-accelerated rendering using WebGPU.
package wgpu

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

// FillRule returns the current fill rule.
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
