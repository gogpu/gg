package clip

import "math"

// RRectClip describes a rounded rectangle clip region.
// The Rect field holds the bounding rectangle in device coordinates,
// and Radius is the corner radius (uniform for all four corners).
type RRectClip struct {
	Rect   Rect
	Radius float64
}

// ClipStack manages hierarchical clip regions with push/pop operations.
// It maintains a stack of clip entries, where each entry can be either a
// rectangular clip, a rounded rectangle clip, or a path-based mask clip.
type ClipStack struct {
	entries []clipEntry
	bounds  Rect
}

// clipEntry represents a single clip operation in the stack.
type clipEntry struct {
	prevBounds Rect
	mask       *MaskClipper
	rrect      *RRectClip // non-nil for rounded rectangle clips
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

// PushRRect pushes a rounded rectangle clip region onto the stack.
// The new clip bounds are the intersection of the current bounds and the
// given rectangle. The radius is the corner radius (clamped to half the
// minimum dimension). If radius is zero, this is equivalent to PushRect.
func (cs *ClipStack) PushRRect(r Rect, radius float64) {
	// Clamp radius to half the smaller dimension.
	maxRadius := math.Min(r.W, r.H) / 2
	if radius > maxRadius {
		radius = maxRadius
	}
	if radius <= 0 {
		cs.PushRect(r)
		return
	}

	// Compute intersection with current bounds.
	newBounds := cs.bounds.Intersect(r)

	// Push entry with rrect data.
	cs.entries = append(cs.entries, clipEntry{
		prevBounds: cs.bounds,
		rrect:      &RRectClip{Rect: r, Radius: radius},
	})

	// Update current bounds.
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
// For RRect clips, this checks the SDF distance.
// For mask clips, this checks if the coverage is non-zero.
func (cs *ClipStack) IsVisible(x, y float64) bool {
	// First check if point is within current bounds
	if !cs.bounds.Contains(Pt(x, y)) {
		return false
	}

	// Check all clip entries in the stack
	for i := range cs.entries {
		entry := &cs.entries[i]
		if entry.rrect != nil {
			if rrectSDF(x, y, entry.rrect) > 0 {
				return false
			}
		}
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
// This multiplies the coverage from all clip entries (RRect and mask) in the stack.
// Returns 0 if the point is outside the current bounds.
func (cs *ClipStack) Coverage(x, y float64) byte {
	// First check if point is within current bounds
	if !cs.bounds.Contains(Pt(x, y)) {
		return 0
	}

	// Start with full coverage
	coverage := uint16(255)

	// Multiply coverage from all clip entries
	for i := range cs.entries {
		entry := &cs.entries[i]

		// RRect SDF coverage
		if entry.rrect != nil {
			rrCov := rrectCoverage(x, y, entry.rrect)
			if rrCov == 0 {
				return 0
			}
			coverage = (coverage * uint16(rrCov)) / 255
			if coverage == 0 {
				return 0
			}
		}

		// Mask coverage
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

// rrectSDF computes the signed distance from point (px, py) to the rounded
// rectangle defined by rr. Negative values are inside, positive outside.
// Uses the same formula as gg.sdfRRect (Inigo Quilez box SDF).
func rrectSDF(px, py float64, rr *RRectClip) float64 {
	cx := rr.Rect.X + rr.Rect.W/2
	cy := rr.Rect.Y + rr.Rect.H/2
	halfW := rr.Rect.W / 2
	halfH := rr.Rect.H / 2
	r := rr.Radius

	dx := math.Abs(px-cx) - halfW + r
	dy := math.Abs(py-cy) - halfH + r

	outside := math.Sqrt(math.Max(dx, 0)*math.Max(dx, 0) + math.Max(dy, 0)*math.Max(dy, 0))
	inside := math.Min(math.Max(dx, dy), 0)

	return outside + inside - r
}

// rrectCoverage computes anti-aliased coverage (0-255) for a point against
// a rounded rectangle clip. Uses a smoothstep transition over 0.7 pixels
// matching the SDF anti-alias width in gg.sdf.go.
func rrectCoverage(px, py float64, rr *RRectClip) byte {
	const aaWidth = 0.7
	d := rrectSDF(px, py, rr)
	if d >= aaWidth {
		return 0
	}
	if d <= -aaWidth {
		return 255
	}
	t := (d + aaWidth) / (2 * aaWidth)
	// Hermite smoothstep: 1 - (3t^2 - 2t^3)
	cov := 1 - (t * t * (3 - 2*t))
	return byte(cov * 255) //nolint:gosec // cov is in [0,1], result fits byte
}

// IsRectOnly reports whether the clip stack contains only rectangular clips
// (no path-based masks and no rounded rectangle clips). When true, clipping
// can be applied by restricting the destination bounds rather than per-pixel
// coverage, which preserves bitmap text quality and is significantly faster.
func (cs *ClipStack) IsRectOnly() bool {
	for i := range cs.entries {
		if cs.entries[i].mask != nil || cs.entries[i].rrect != nil {
			return false
		}
	}
	return true
}

// IsRRectOnly reports whether the clip stack contains only rectangular and/or
// rounded rectangle clips (no path-based masks). When true, clipping can be
// handled via hardware scissor rect (for the bounding box) plus analytic SDF
// in the fragment shader (for rounded corners).
func (cs *ClipStack) IsRRectOnly() bool {
	for i := range cs.entries {
		if cs.entries[i].mask != nil {
			return false
		}
	}
	return true
}

// RRectBounds returns the bounds and radius of the innermost (most recently
// pushed) rounded rectangle clip entry, or false if no RRect clip is active.
// When multiple RRect clips are nested, only the innermost is returned —
// the scissor rect already handles the outer bounds intersection.
func (cs *ClipStack) RRectBounds() (Rect, float64, bool) {
	// Scan from top of stack to find the most recent RRect.
	for i := len(cs.entries) - 1; i >= 0; i-- {
		if cs.entries[i].rrect != nil {
			rr := cs.entries[i].rrect
			return rr.Rect, rr.Radius, true
		}
	}
	return Rect{}, 0, false
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
