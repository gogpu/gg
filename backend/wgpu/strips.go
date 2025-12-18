package wgpu

import (
	"github.com/gogpu/gg/scene"
)

// Strip represents one horizontal span of coverage.
// A strip stores the coverage values for a contiguous horizontal range of pixels
// at a specific row. Only non-zero coverage regions are stored (sparse).
type Strip struct {
	// Y is the row index (scanline number)
	Y int32

	// X is the starting X coordinate of the strip
	X int32

	// Width is the number of pixels in this strip
	Width int32

	// Coverage holds anti-aliased coverage values (0-255) for each pixel
	Coverage []uint8
}

// Clone creates a deep copy of the strip.
func (s *Strip) Clone() Strip {
	cov := make([]uint8, len(s.Coverage))
	copy(cov, s.Coverage)
	return Strip{
		Y:        s.Y,
		X:        s.X,
		Width:    s.Width,
		Coverage: cov,
	}
}

// End returns the X coordinate of the pixel just past the strip.
func (s *Strip) End() int32 {
	return s.X + s.Width
}

// StripBuffer holds all strips for a tessellated path.
// It provides methods to accumulate strips during tessellation and
// pack them for GPU upload.
type StripBuffer struct {
	// strips holds all the coverage strips
	strips []Strip

	// bounds tracks the bounding rectangle of all strips
	bounds scene.Rect

	// fillRule indicates how to interpret winding numbers
	fillRule scene.FillStyle

	// dirty indicates whether GPU data needs regeneration
	dirty bool

	// stripPool is a pool for reusing coverage slices
	coveragePool [][]uint8
}

// NewStripBuffer creates a new empty strip buffer.
func NewStripBuffer() *StripBuffer {
	return &StripBuffer{
		strips:       make([]Strip, 0, 256),
		bounds:       scene.EmptyRect(),
		fillRule:     scene.FillNonZero,
		dirty:        true,
		coveragePool: make([][]uint8, 0, 32),
	}
}

// Reset clears the buffer for reuse without deallocating memory.
func (sb *StripBuffer) Reset() {
	// Return coverage slices to pool
	for i := range sb.strips {
		if cap(sb.strips[i].Coverage) > 0 {
			sb.coveragePool = append(sb.coveragePool, sb.strips[i].Coverage[:0])
		}
	}
	sb.strips = sb.strips[:0]
	sb.bounds = scene.EmptyRect()
	sb.dirty = true
}

// SetFillRule sets the fill rule for coverage interpretation.
func (sb *StripBuffer) SetFillRule(rule scene.FillStyle) {
	sb.fillRule = rule
}

// FillRule returns the current fill rule.
func (sb *StripBuffer) FillRule() scene.FillStyle {
	return sb.fillRule
}

// acquireCoverage gets a coverage slice from the pool or creates a new one.
func (sb *StripBuffer) acquireCoverage(size int) []uint8 {
	// Look for a slice with sufficient capacity
	for i := len(sb.coveragePool) - 1; i >= 0; i-- {
		if cap(sb.coveragePool[i]) >= size {
			cov := sb.coveragePool[i][:size]
			sb.coveragePool = sb.coveragePool[:i]
			return cov
		}
	}
	return make([]uint8, size)
}

// AddStrip adds a strip to the buffer.
// The coverage slice is copied, so the caller can reuse it.
func (sb *StripBuffer) AddStrip(y, x int, coverage []uint8) {
	if len(coverage) == 0 {
		return
	}

	// Get a coverage slice from pool
	cov := sb.acquireCoverage(len(coverage))
	copy(cov, coverage)

	//nolint:gosec // y and x are bounded by image dimensions
	strip := Strip{
		Y:        int32(y),
		X:        int32(x),
		Width:    int32(len(coverage)),
		Coverage: cov,
	}

	sb.strips = append(sb.strips, strip)

	// Update bounds
	sb.bounds = sb.bounds.UnionPoint(float32(x), float32(y))
	sb.bounds = sb.bounds.UnionPoint(float32(x+len(coverage)), float32(y+1))

	sb.dirty = true
}

// AddStripDirect adds a strip without copying the coverage slice.
// The caller must not modify the coverage slice after this call.
func (sb *StripBuffer) AddStripDirect(y, x int32, coverage []uint8) {
	if len(coverage) == 0 {
		return
	}

	//nolint:gosec // coverage length is bounded by image dimensions
	strip := Strip{
		Y:        y,
		X:        x,
		Width:    int32(len(coverage)),
		Coverage: coverage,
	}

	sb.strips = append(sb.strips, strip)

	// Update bounds
	sb.bounds = sb.bounds.UnionPoint(float32(x), float32(y))
	sb.bounds = sb.bounds.UnionPoint(float32(int(x)+len(coverage)), float32(y+1))

	sb.dirty = true
}

// Strips returns the slice of strips.
func (sb *StripBuffer) Strips() []Strip {
	return sb.strips
}

// StripCount returns the number of strips in the buffer.
func (sb *StripBuffer) StripCount() int {
	return len(sb.strips)
}

// Bounds returns the bounding rectangle of all strips.
func (sb *StripBuffer) Bounds() scene.Rect {
	return sb.bounds
}

// IsEmpty returns true if the buffer contains no strips.
func (sb *StripBuffer) IsEmpty() bool {
	return len(sb.strips) == 0
}

// TotalCoverage returns the total number of coverage values across all strips.
func (sb *StripBuffer) TotalCoverage() int {
	total := 0
	for i := range sb.strips {
		total += len(sb.strips[i].Coverage)
	}
	return total
}

// GPUStripHeader is the header format for GPU strip data.
// Each strip has a 16-byte header for efficient GPU access.
type GPUStripHeader struct {
	Y      int32 // Row index
	X      int32 // Start X coordinate
	Width  int32 // Number of pixels
	Offset int32 // Offset into coverage array
}

// PackForGPU returns GPU-ready data: headers and packed coverage.
// The headers array contains one GPUStripHeader per strip.
// The coverage array contains all coverage values packed contiguously.
func (sb *StripBuffer) PackForGPU() (headers []GPUStripHeader, coverage []uint8) {
	if len(sb.strips) == 0 {
		return nil, nil
	}

	headers = make([]GPUStripHeader, len(sb.strips))
	totalCoverage := sb.TotalCoverage()
	coverage = make([]uint8, totalCoverage)

	offset := int32(0)
	for i := range sb.strips {
		strip := &sb.strips[i]
		headers[i] = GPUStripHeader{
			Y:      strip.Y,
			X:      strip.X,
			Width:  strip.Width,
			Offset: offset,
		}
		copy(coverage[offset:], strip.Coverage)
		offset += strip.Width
	}

	return headers, coverage
}

// PackForGPUInto packs GPU data into provided slices to avoid allocation.
// Returns the number of strips packed, or -1 if buffers are too small.
func (sb *StripBuffer) PackForGPUInto(headers []GPUStripHeader, coverage []uint8) int {
	if len(headers) < len(sb.strips) {
		return -1
	}

	totalCov := sb.TotalCoverage()
	if len(coverage) < totalCov {
		return -1
	}

	offset := int32(0)
	for i := range sb.strips {
		strip := &sb.strips[i]
		headers[i] = GPUStripHeader{
			Y:      strip.Y,
			X:      strip.X,
			Width:  strip.Width,
			Offset: offset,
		}
		copy(coverage[offset:], strip.Coverage)
		offset += strip.Width
	}

	return len(sb.strips)
}

// MergeAdjacent merges horizontally adjacent strips on the same row.
// This reduces the number of strips and improves GPU efficiency.
func (sb *StripBuffer) MergeAdjacent() {
	if len(sb.strips) < 2 {
		return
	}

	// Sort strips by Y then X
	sortStripsByPosition(sb.strips)

	// Merge adjacent strips
	merged := sb.strips[:0]
	current := sb.strips[0]

	for i := 1; i < len(sb.strips); i++ {
		next := sb.strips[i]

		// Check if strips are adjacent on same row
		if current.Y == next.Y && current.End() == next.X {
			// Merge: extend current strip
			newCov := sb.acquireCoverage(len(current.Coverage) + len(next.Coverage))
			copy(newCov, current.Coverage)
			copy(newCov[len(current.Coverage):], next.Coverage)

			// Return old coverage to pool
			if cap(current.Coverage) > 0 {
				sb.coveragePool = append(sb.coveragePool, current.Coverage[:0])
			}

			current.Coverage = newCov
			//nolint:gosec // coverage length is bounded by image dimensions
			current.Width = int32(len(newCov))
		} else {
			// Not adjacent, emit current and start new
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	sb.strips = merged
	sb.dirty = true
}

// sortStripsByPosition sorts strips by Y coordinate, then by X coordinate.
func sortStripsByPosition(strips []Strip) {
	// Simple insertion sort (usually already mostly sorted)
	for i := 1; i < len(strips); i++ {
		j := i
		for j > 0 && stripLess(strips[j], strips[j-1]) {
			strips[j], strips[j-1] = strips[j-1], strips[j]
			j--
		}
	}
}

// stripLess returns true if a should come before b.
func stripLess(a, b Strip) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}

// Clone creates a deep copy of the strip buffer.
func (sb *StripBuffer) Clone() *StripBuffer {
	clone := NewStripBuffer()
	clone.fillRule = sb.fillRule
	clone.bounds = sb.bounds

	for i := range sb.strips {
		clone.strips = append(clone.strips, sb.strips[i].Clone())
	}

	return clone
}

// MemorySize returns the approximate memory usage in bytes.
func (sb *StripBuffer) MemorySize() int {
	size := len(sb.strips) * 24 // Strip struct overhead (Y, X, Width, slice header)
	for i := range sb.strips {
		size += cap(sb.strips[i].Coverage)
	}
	// Pool memory
	for i := range sb.coveragePool {
		size += cap(sb.coveragePool[i])
	}
	return size
}
