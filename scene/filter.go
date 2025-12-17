package scene

import "github.com/gogpu/gg"

// Filter applies visual effects to rendered layers.
// Filters are applied during layer pop when LayerFiltered is used.
//
// The filter interface supports both in-place and copy operations:
// - If src == dst, the filter operates in-place when possible
// - Some filters (like blur) may require temporary buffers internally
//
// Bounds handling:
// - Input bounds specify the region to process
// - ExpandBounds returns how much the output grows (e.g., blur radius)
type Filter interface {
	// Apply processes src pixmap and writes result to dst.
	// bounds specifies the affected region in pixel coordinates.
	// The filter may read pixels outside bounds but only writes within bounds.
	Apply(src, dst *gg.Pixmap, bounds Rect)

	// ExpandBounds returns the expanded bounds after filter application.
	// This is used for buffer allocation:
	// - Blur expands by radius in all directions
	// - Shadow expands by offset + blur radius
	// - Color matrix does not expand
	ExpandBounds(input Rect) Rect
}

// FilterType identifies the type of filter for serialization and debugging.
type FilterType uint8

// Filter type constants.
const (
	// FilterNone represents no filter (identity).
	FilterNone FilterType = iota

	// FilterBlur represents Gaussian blur filter.
	FilterBlur

	// FilterDropShadow represents drop shadow filter.
	FilterDropShadow

	// FilterColorMatrix represents color matrix transformation.
	FilterColorMatrix
)

// String returns a human-readable name for the filter type.
func (ft FilterType) String() string {
	switch ft {
	case FilterNone:
		return "None"
	case FilterBlur:
		return "Blur"
	case FilterDropShadow:
		return "DropShadow"
	case FilterColorMatrix:
		return "ColorMatrix"
	default:
		return unknownStr
	}
}

// ExpandsOutput returns true if this filter type typically expands output bounds.
func (ft FilterType) ExpandsOutput() bool {
	return ft == FilterBlur || ft == FilterDropShadow
}

// FilterChain represents multiple filters applied in sequence.
// Filters are applied in order from first to last.
type FilterChain struct {
	filters []Filter
}

// NewFilterChain creates a new filter chain from the given filters.
func NewFilterChain(filters ...Filter) *FilterChain {
	chain := &FilterChain{
		filters: make([]Filter, 0, len(filters)),
	}
	for _, f := range filters {
		if f != nil {
			chain.filters = append(chain.filters, f)
		}
	}
	return chain
}

// Add appends a filter to the chain.
func (fc *FilterChain) Add(f Filter) {
	if f != nil {
		fc.filters = append(fc.filters, f)
	}
}

// Apply processes src through all filters in sequence.
// For chains with more than one filter, temporary buffers are used.
func (fc *FilterChain) Apply(src, dst *gg.Pixmap, bounds Rect) {
	if len(fc.filters) == 0 {
		// No filters - copy src to dst if different
		if src != dst {
			copyPixmap(src, dst, bounds)
		}
		return
	}

	if len(fc.filters) == 1 {
		// Single filter - direct application
		fc.filters[0].Apply(src, dst, bounds)
		return
	}

	// Multiple filters - need temporary buffer
	// Calculate maximum expanded bounds
	maxBounds := bounds
	for _, f := range fc.filters {
		maxBounds = f.ExpandBounds(maxBounds)
	}

	// Create temporary buffer
	w := int(maxBounds.Width()) + 1
	h := int(maxBounds.Height()) + 1
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	temp1 := gg.NewPixmap(w, h)
	temp2 := gg.NewPixmap(w, h)

	// Apply first filter: src -> temp1
	fc.filters[0].Apply(src, temp1, bounds)
	currentBounds := fc.filters[0].ExpandBounds(bounds)

	// Apply middle filters: alternate between temp1 and temp2
	current := temp1
	next := temp2
	for i := 1; i < len(fc.filters)-1; i++ {
		fc.filters[i].Apply(current, next, currentBounds)
		currentBounds = fc.filters[i].ExpandBounds(currentBounds)
		current, next = next, current
	}

	// Apply last filter: current -> dst
	fc.filters[len(fc.filters)-1].Apply(current, dst, currentBounds)
}

// ExpandBounds returns the combined expansion of all filters.
func (fc *FilterChain) ExpandBounds(input Rect) Rect {
	result := input
	for _, f := range fc.filters {
		result = f.ExpandBounds(result)
	}
	return result
}

// Len returns the number of filters in the chain.
func (fc *FilterChain) Len() int {
	return len(fc.filters)
}

// IsEmpty returns true if the chain has no filters.
func (fc *FilterChain) IsEmpty() bool {
	return len(fc.filters) == 0
}

// copyPixmap copies pixels from src to dst within bounds.
func copyPixmap(src, dst *gg.Pixmap, bounds Rect) {
	minX := int(bounds.MinX)
	maxX := int(bounds.MaxX)
	minY := int(bounds.MinY)
	maxY := int(bounds.MaxY)

	// Clamp to pixmap dimensions
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > src.Width() {
		maxX = src.Width()
	}
	if maxY > src.Height() {
		maxY = src.Height()
	}
	if maxX > dst.Width() {
		maxX = dst.Width()
	}
	if maxY > dst.Height() {
		maxY = dst.Height()
	}

	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			dst.SetPixel(x, y, src.GetPixel(x, y))
		}
	}
}
