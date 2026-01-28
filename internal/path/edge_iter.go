// Package path provides internal path processing utilities.
package path

// Edge represents a line segment from P0 to P1.
type Edge struct {
	P0, P1 Point
}

// EdgeIter iterates over edges in a path, correctly handling subpath boundaries.
// Unlike Flatten which returns a flat []Point list, EdgeIter never creates
// edges between separate subpaths - it properly closes each subpath to its
// own start point before moving to the next.
//
// This follows the same pattern as tiny-skia's PathEdgeIter.
type EdgeIter struct {
	elements       []PathElement
	index          int
	current        Point
	moveTo         Point // Start of current subpath
	needsCloseLine bool
}

// NewEdgeIter creates a new edge iterator for the given path elements.
func NewEdgeIter(elements []PathElement) *EdgeIter {
	return &EdgeIter{
		elements: elements,
	}
}

// Next returns the next edge in the path, or nil when iteration is complete.
// Curves (QuadTo, CubicTo) are flattened to line segments automatically.
func (iter *EdgeIter) Next() *Edge {
	for iter.index < len(iter.elements) {
		elem := iter.elements[iter.index]
		iter.index++

		switch e := elem.(type) {
		case MoveTo:
			if edge := iter.handleMoveTo(e); edge != nil {
				return edge
			}

		case LineTo:
			if edge := iter.handleLineTo(e); edge != nil {
				return edge
			}

		case QuadTo:
			if edge := iter.handleQuadTo(e); edge != nil {
				return edge
			}

		case CubicTo:
			if edge := iter.handleCubicTo(e); edge != nil {
				return edge
			}

		case Close:
			if iter.needsCloseLine {
				return iter.closeLine()
			}
		}
	}

	// End of elements - close final subpath if needed
	if iter.needsCloseLine {
		return iter.closeLine()
	}

	return nil
}

// handleMoveTo processes a MoveTo element.
func (iter *EdgeIter) handleMoveTo(e MoveTo) *Edge {
	// If we need to close the previous subpath, do it first
	if iter.needsCloseLine {
		iter.index-- // Reprocess this MoveTo next time
		return iter.closeLine()
	}
	// Start new subpath
	iter.moveTo = e.Point
	iter.current = e.Point
	return nil
}

// handleLineTo processes a LineTo element and returns an edge if valid.
func (iter *EdgeIter) handleLineTo(e LineTo) *Edge {
	iter.needsCloseLine = true
	p0 := iter.current
	iter.current = e.Point
	// Skip zero-length edges
	if p0.X == iter.current.X && p0.Y == iter.current.Y {
		return nil
	}
	return &Edge{P0: p0, P1: iter.current}
}

// handleQuadTo flattens a quadratic curve and returns the first edge.
func (iter *EdgeIter) handleQuadTo(e QuadTo) *Edge {
	iter.needsCloseLine = true
	segments := flattenQuadratic(iter.current, e.Control, e.Point, Tolerance)
	return iter.processFlattened(segments)
}

// handleCubicTo flattens a cubic curve and returns the first edge.
func (iter *EdgeIter) handleCubicTo(e CubicTo) *Edge {
	iter.needsCloseLine = true
	segments := flattenCubic(iter.current, e.Control1, e.Control2, e.Point, Tolerance)
	return iter.processFlattened(segments)
}

// processFlattened handles flattened curve segments.
func (iter *EdgeIter) processFlattened(segments []Point) *Edge {
	if len(segments) == 0 {
		return nil
	}

	p0 := iter.current
	iter.current = segments[0]

	// Insert remaining segments as LineTo elements
	if len(segments) > 1 {
		remaining := make([]PathElement, len(segments)-1)
		for i := 1; i < len(segments); i++ {
			remaining[i-1] = LineTo{Point: segments[i]}
		}
		iter.insertElements(remaining)
	}

	// Skip zero-length edges
	if p0.X == iter.current.X && p0.Y == iter.current.Y {
		return nil
	}
	return &Edge{P0: p0, P1: iter.current}
}

// closeLine returns an edge from current position back to subpath start.
func (iter *EdgeIter) closeLine() *Edge {
	iter.needsCloseLine = false
	p0 := iter.current
	iter.current = iter.moveTo
	// Skip zero-length close edges
	if p0.X == iter.moveTo.X && p0.Y == iter.moveTo.Y {
		return iter.Next() // Continue to next edge
	}
	return &Edge{P0: p0, P1: iter.moveTo}
}

// insertElements inserts elements at the current position for processing.
func (iter *EdgeIter) insertElements(elems []PathElement) {
	if len(elems) == 0 {
		return
	}
	// Insert at current index position
	newElements := make([]PathElement, 0, len(iter.elements)+len(elems))
	newElements = append(newElements, iter.elements[:iter.index]...)
	newElements = append(newElements, elems...)
	newElements = append(newElements, iter.elements[iter.index:]...)
	iter.elements = newElements
}

// CollectEdges returns all edges from the path elements.
// This is a convenience function for cases where all edges are needed at once.
func CollectEdges(elements []PathElement) []Edge {
	var edges []Edge
	iter := NewEdgeIter(elements)
	for {
		edge := iter.Next()
		if edge == nil {
			break
		}
		edges = append(edges, *edge)
	}
	return edges
}
