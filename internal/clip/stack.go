package clip

// ClipStack manages hierarchical clip regions with push/pop operations.
// It maintains a stack of clip entries, where each entry can be either a
// rectangular clip or a path-based mask clip.
type ClipStack struct {
	entries []clipEntry
	bounds  Rect
}

// clipEntry represents a single clip operation in the stack.
type clipEntry struct {
	prevBounds Rect
	mask       *MaskClipper
	antiAlias  bool
}

// NewClipStack creates a new clip stack with the given bounds.
// The bounds represent the maximum clipping area (typically the canvas size).
func NewClipStack(bounds Rect) *ClipStack {
	return &ClipStack{
		entries: make([]clipEntry, 0, 8), // Pre-allocate for common case
		bounds:  bounds,
	}
}

// PushRect pushes a rectangular clip region onto the stack.
// The new clip bounds are the intersection of the current bounds and the given rectangle.
func (cs *ClipStack) PushRect(r Rect) {
	// Compute intersection with current bounds
	newBounds := cs.bounds.Intersect(r)

	// Push entry onto stack
	cs.entries = append(cs.entries, clipEntry{
		prevBounds: cs.bounds,
		mask:       nil, // No mask for rectangular clips
		antiAlias:  false,
	})

	// Update current bounds
	cs.bounds = newBounds
}

// PushPath pushes a path-based clip region onto the stack.
// The path is rasterized into a mask using the current bounds.
// If antiAlias is true, the mask will use anti-aliased rendering.
func (cs *ClipStack) PushPath(path []PathElement, antiAlias bool) error {
	// Create mask clipper from path
	mask, err := NewMaskClipper(path, cs.bounds, antiAlias)
	if err != nil {
		return err
	}

	// Compute new bounds (intersection with mask bounds)
	newBounds := cs.bounds.Intersect(mask.Bounds())

	// Push entry onto stack
	cs.entries = append(cs.entries, clipEntry{
		prevBounds: cs.bounds,
		mask:       mask,
		antiAlias:  antiAlias,
	})

	// Update current bounds
	cs.bounds = newBounds

	return nil
}

// Pop removes the most recent clip region from the stack.
// If the stack is empty, this is a no-op.
func (cs *ClipStack) Pop() {
	if len(cs.entries) == 0 {
		return
	}

	// Get last entry
	lastIdx := len(cs.entries) - 1
	entry := cs.entries[lastIdx]

	// Restore previous bounds
	cs.bounds = entry.prevBounds

	// Remove entry from stack
	cs.entries = cs.entries[:lastIdx]
}

// Bounds returns the current effective clip bounds.
// This is the intersection of all pushed clip regions.
func (cs *ClipStack) Bounds() Rect {
	return cs.bounds
}

// IsVisible returns true if the point (x, y) is within the current clip region.
// For rectangular clips, this is a simple bounds check.
// For mask clips, this checks if the coverage is non-zero.
func (cs *ClipStack) IsVisible(x, y float64) bool {
	// First check if point is within current bounds
	if !cs.bounds.Contains(Pt(x, y)) {
		return false
	}

	// Check all mask clips in the stack
	for i := range cs.entries {
		entry := &cs.entries[i]
		if entry.mask != nil {
			coverage := entry.mask.Coverage(x, y)
			if coverage == 0 {
				return false
			}
		}
	}

	return true
}

// Coverage returns the combined coverage value (0-255) at the given point.
// This multiplies the coverage from all mask clips in the stack.
// Returns 0 if the point is outside the current bounds.
func (cs *ClipStack) Coverage(x, y float64) byte {
	// First check if point is within current bounds
	if !cs.bounds.Contains(Pt(x, y)) {
		return 0
	}

	// Start with full coverage
	coverage := uint16(255)

	// Multiply coverage from all mask clips
	for i := range cs.entries {
		entry := &cs.entries[i]
		if entry.mask != nil {
			maskCoverage := entry.mask.Coverage(x, y)
			if maskCoverage == 0 {
				return 0 // Early exit if any mask has zero coverage
			}

			// Multiply coverage: result = (coverage * maskCoverage) / 255
			coverage = (coverage * uint16(maskCoverage)) / 255

			if coverage == 0 {
				return 0 // Early exit if coverage becomes zero
			}
		}
	}

	return byte(coverage)
}

// Depth returns the current depth of the clip stack.
// This is primarily useful for debugging and testing.
func (cs *ClipStack) Depth() int {
	return len(cs.entries)
}

// Reset clears all clip entries and restores the original bounds.
func (cs *ClipStack) Reset(bounds Rect) {
	cs.entries = cs.entries[:0]
	cs.bounds = bounds
}
