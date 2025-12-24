package msdf

// ShelfAllocator implements shelf-based rectangle packing.
// Simple and fast algorithm suitable for uniform-sized glyphs.
//
// The algorithm organizes rectangles in horizontal "shelves".
// Each shelf has a fixed height (determined by the tallest item placed so far).
// New items are placed left-to-right on the current shelf until no space remains,
// then a new shelf is started below.
type ShelfAllocator struct {
	width   int     // Total width of the atlas
	height  int     // Total height of the atlas
	padding int     // Padding between glyphs
	shelves []shelf // List of shelves

	// Tracking for utilization
	usedArea int
}

// shelf represents a horizontal strip in the atlas.
type shelf struct {
	y      int // Y position of shelf top
	height int // Height of the shelf (tallest item so far)
	x      int // Current X position (next free slot)
}

// NewShelfAllocator creates a new allocator for the given dimensions.
func NewShelfAllocator(width, height, padding int) *ShelfAllocator {
	return &ShelfAllocator{
		width:   width,
		height:  height,
		padding: padding,
		shelves: make([]shelf, 0, 16), // Preallocate for typical use
	}
}

// Allocate finds space for a rectangle of the given size.
// Returns x, y position and true if space was found, or -1, -1, false if not.
//
// The algorithm:
// 1. Try to fit on an existing shelf with enough height
// 2. If no shelf fits, create a new shelf
// 3. If no space for new shelf, allocation fails
func (a *ShelfAllocator) Allocate(w, h int) (x, y int, ok bool) {
	// Add padding to requested size
	paddedW := w + a.padding
	paddedH := h + a.padding

	// Try to find an existing shelf with enough space and height
	for i := range a.shelves {
		shelf := &a.shelves[i]

		// Check if item fits horizontally
		if shelf.x+paddedW > a.width {
			continue
		}

		// Check if item fits vertically in this shelf
		if h > shelf.height {
			// Item is taller than shelf - check if we can extend the shelf
			// Only possible if this is the last shelf and there's room below
			if i == len(a.shelves)-1 {
				newBottom := shelf.y + paddedH
				if newBottom <= a.height {
					// Extend shelf height
					shelf.height = h
					x, y = shelf.x, shelf.y
					shelf.x += paddedW
					a.usedArea += w * h
					return x, y, true
				}
			}
			continue
		}

		// Item fits on this shelf
		x, y = shelf.x, shelf.y
		shelf.x += paddedW
		a.usedArea += w * h
		return x, y, true
	}

	// No existing shelf works - try to create a new one
	newY := 0
	if len(a.shelves) > 0 {
		last := a.shelves[len(a.shelves)-1]
		newY = last.y + last.height + a.padding
	}

	// Check if new shelf fits
	if newY+paddedH > a.height {
		return -1, -1, false
	}

	// Create new shelf
	newShelf := shelf{
		y:      newY,
		height: h,
		x:      paddedW,
	}
	a.shelves = append(a.shelves, newShelf)
	a.usedArea += w * h

	return 0, newY, true
}

// AllocateFixed allocates a fixed-size cell, optimized for uniform glyph sizes.
// This is more efficient when all cells are the same size.
func (a *ShelfAllocator) AllocateFixed(cellSize int) (x, y int, ok bool) {
	return a.Allocate(cellSize, cellSize)
}

// Reset clears all allocations, allowing the allocator to be reused.
func (a *ShelfAllocator) Reset() {
	a.shelves = a.shelves[:0] // Keep capacity
	a.usedArea = 0
}

// Utilization returns the percentage of atlas space used (0.0 to 1.0).
func (a *ShelfAllocator) Utilization() float64 {
	if a.width <= 0 || a.height <= 0 {
		return 0
	}
	totalArea := a.width * a.height
	return float64(a.usedArea) / float64(totalArea)
}

// UsedArea returns the total area used by allocations.
func (a *ShelfAllocator) UsedArea() int {
	return a.usedArea
}

// TotalArea returns the total area of the atlas.
func (a *ShelfAllocator) TotalArea() int {
	return a.width * a.height
}

// ShelfCount returns the number of shelves currently in use.
func (a *ShelfAllocator) ShelfCount() int {
	return len(a.shelves)
}

// CanFit returns true if an item of the given size could possibly fit.
// This is a quick check without actually allocating.
func (a *ShelfAllocator) CanFit(w, h int) bool {
	paddedW := w + a.padding
	paddedH := h + a.padding

	// Items wider than the allocator can never fit
	if paddedW > a.width {
		return false
	}

	// Items taller than the allocator can never fit
	if paddedH > a.height {
		return false
	}

	// Check existing shelves
	for i := range a.shelves {
		shelf := &a.shelves[i]

		// Check if item fits horizontally
		if shelf.x+paddedW > a.width {
			continue
		}

		// Check if item fits in shelf height
		if h <= shelf.height {
			return true
		}

		// Check if we can extend last shelf
		if i == len(a.shelves)-1 {
			if shelf.y+paddedH <= a.height {
				return true
			}
		}
	}

	// Check if we can create a new shelf
	newY := 0
	if len(a.shelves) > 0 {
		last := a.shelves[len(a.shelves)-1]
		newY = last.y + last.height + a.padding
	}

	return newY+paddedH <= a.height
}

// RemainingHeight returns the vertical space remaining for new shelves.
func (a *ShelfAllocator) RemainingHeight() int {
	if len(a.shelves) == 0 {
		return a.height
	}

	last := a.shelves[len(a.shelves)-1]
	used := last.y + last.height + a.padding
	if used >= a.height {
		return 0
	}
	return a.height - used
}

// CurrentShelfRemainingWidth returns the remaining width on the current (last) shelf.
func (a *ShelfAllocator) CurrentShelfRemainingWidth() int {
	if len(a.shelves) == 0 {
		return a.width
	}
	last := a.shelves[len(a.shelves)-1]
	if last.x >= a.width {
		return 0
	}
	return a.width - last.x
}

// GridAllocator is a specialized allocator for uniform grid-based layouts.
// More efficient than ShelfAllocator when all cells are exactly the same size.
type GridAllocator struct {
	width    int // Atlas width
	height   int // Atlas height
	cellSize int // Size of each cell (square)
	padding  int // Padding between cells
	cols     int // Number of columns
	rows     int // Number of rows
	next     int // Next cell index
}

// NewGridAllocator creates a grid allocator for uniform cells.
func NewGridAllocator(width, height, cellSize, padding int) *GridAllocator {
	cellWithPad := cellSize + padding
	cols := width / cellWithPad
	rows := height / cellWithPad

	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}

	return &GridAllocator{
		width:    width,
		height:   height,
		cellSize: cellSize,
		padding:  padding,
		cols:     cols,
		rows:     rows,
		next:     0,
	}
}

// Allocate returns the position of the next available cell.
// Returns -1, -1, false if the grid is full.
func (g *GridAllocator) Allocate() (x, y int, ok bool) {
	if g.next >= g.cols*g.rows {
		return -1, -1, false
	}

	col := g.next % g.cols
	row := g.next / g.cols

	cellWithPad := g.cellSize + g.padding
	x = col * cellWithPad
	y = row * cellWithPad

	g.next++
	return x, y, true
}

// Reset clears all allocations.
func (g *GridAllocator) Reset() {
	g.next = 0
}

// Capacity returns the maximum number of cells that can be allocated.
func (g *GridAllocator) Capacity() int {
	return g.cols * g.rows
}

// Allocated returns the number of cells currently allocated.
func (g *GridAllocator) Allocated() int {
	return g.next
}

// Remaining returns the number of cells still available.
func (g *GridAllocator) Remaining() int {
	return g.Capacity() - g.next
}

// IsFull returns true if no more cells can be allocated.
func (g *GridAllocator) IsFull() bool {
	return g.next >= g.cols*g.rows
}

// Utilization returns the percentage of cells used (0.0 to 1.0).
func (g *GridAllocator) Utilization() float64 {
	capacity := g.Capacity()
	if capacity <= 0 {
		return 0
	}
	return float64(g.next) / float64(capacity)
}

// CellSize returns the size of each cell.
func (g *GridAllocator) CellSize() int {
	return g.cellSize
}

// GridDimensions returns the number of columns and rows.
func (g *GridAllocator) GridDimensions() (cols, rows int) {
	return g.cols, g.rows
}
