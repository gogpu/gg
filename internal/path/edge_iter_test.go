package path

import (
	"testing"
)

func TestEdgeIterSingleSubpath(t *testing.T) {
	// Simple triangle - single subpath
	elements := []PathElement{
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		LineTo{Point{50, 100}},
		Close{},
	}

	edges := CollectEdges(elements)

	// Should have 3 edges (including close edge back to start)
	if len(edges) != 3 {
		t.Errorf("Expected 3 edges, got %d", len(edges))
	}

	// Verify edges
	expected := []Edge{
		{Point{0, 0}, Point{100, 0}},
		{Point{100, 0}, Point{50, 100}},
		{Point{50, 100}, Point{0, 0}}, // Close edge
	}

	for i, e := range edges {
		if i >= len(expected) {
			break
		}
		if e.P0 != expected[i].P0 || e.P1 != expected[i].P1 {
			t.Errorf("Edge %d: expected (%v,%v)->(%v,%v), got (%v,%v)->(%v,%v)",
				i, expected[i].P0.X, expected[i].P0.Y, expected[i].P1.X, expected[i].P1.Y,
				e.P0.X, e.P0.Y, e.P1.X, e.P1.Y)
		}
	}
}

func TestEdgeIterMultipleSubpaths(t *testing.T) {
	// Two separate rectangles (like stroke expansion creates)
	elements := []PathElement{
		// First rectangle
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		LineTo{Point{100, 50}},
		LineTo{Point{0, 50}},
		Close{},
		// Second rectangle (separate subpath)
		MoveTo{Point{10, 10}},
		LineTo{Point{90, 10}},
		LineTo{Point{90, 40}},
		LineTo{Point{10, 40}},
		Close{},
	}

	edges := CollectEdges(elements)

	// Should have 8 edges (4 per rectangle), NO connecting edge between subpaths
	if len(edges) != 8 {
		t.Errorf("Expected 8 edges, got %d", len(edges))
	}

	// Check that there is NO edge connecting (0,0) area to (10,10) area
	// This is the key test for BUG-002 fix
	for i, e := range edges {
		// Check for "connecting" edge between subpaths
		if (e.P0.Y <= 0 && e.P1.Y >= 10) || (e.P0.Y >= 10 && e.P1.Y <= 0) {
			// Allow vertical edges within a subpath, but not between them
			if e.P0.X != e.P1.X { // Not a pure vertical edge
				t.Errorf("Found connecting edge between subpaths at index %d: (%v,%v)->(%v,%v)",
					i, e.P0.X, e.P0.Y, e.P1.X, e.P1.Y)
			}
		}
	}

	// Verify first rectangle closes to (0,0)
	foundFirstClose := false
	for _, e := range edges {
		if e.P1.X == 0 && e.P1.Y == 0 && e.P0.X == 0 && e.P0.Y == 50 {
			foundFirstClose = true
			break
		}
	}
	if !foundFirstClose {
		t.Error("First rectangle should close back to (0,0)")
	}

	// Verify second rectangle closes to (10,10)
	foundSecondClose := false
	for _, e := range edges {
		if e.P1.X == 10 && e.P1.Y == 10 && e.P0.X == 10 && e.P0.Y == 40 {
			foundSecondClose = true
			break
		}
	}
	if !foundSecondClose {
		t.Error("Second rectangle should close back to (10,10)")
	}
}

func TestEdgeIterNoConnectingEdge(t *testing.T) {
	// Simulates stroke expansion output - outer and inner perimeters
	// The key is that Close should return to each subpath's own start
	elements := []PathElement{
		// Outer perimeter (starts at 0,0)
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		LineTo{Point{100, 100}},
		LineTo{Point{0, 100}},
		Close{},
		// Inner perimeter (starts at 10,10)
		MoveTo{Point{10, 10}},
		LineTo{Point{10, 90}},
		LineTo{Point{90, 90}},
		LineTo{Point{90, 10}},
		Close{},
	}

	edges := CollectEdges(elements)

	// Count edges that would connect the two subpaths
	connectingEdges := 0
	for _, e := range edges {
		// An edge from outer perimeter boundary to inner perimeter boundary
		isFromOuter := e.P0.X == 0 || e.P0.Y == 0 || e.P0.X == 100 || e.P0.Y == 100
		isToInner := e.P1.X == 10 || e.P1.Y == 10 || e.P1.X == 90 || e.P1.Y == 90

		isFromInner := e.P0.X == 10 || e.P0.Y == 10 || e.P0.X == 90 || e.P0.Y == 90
		isToOuter := e.P1.X == 0 || e.P1.Y == 0 || e.P1.X == 100 || e.P1.Y == 100

		if (isFromOuter && isToInner) || (isFromInner && isToOuter) {
			// Check if this is actually a connecting edge (crosses boundary)
			if (e.P0.X < 10 && e.P1.X >= 10) || (e.P0.X > 90 && e.P1.X <= 90) ||
				(e.P0.Y < 10 && e.P1.Y >= 10) || (e.P0.Y > 90 && e.P1.Y <= 90) {
				connectingEdges++
			}
		}
	}

	if connectingEdges > 0 {
		t.Errorf("Found %d connecting edges between subpaths (should be 0)", connectingEdges)
	}
}

func TestEdgeIterZeroLengthEdges(t *testing.T) {
	// Path where close point equals start point (zero-length close edge)
	elements := []PathElement{
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		LineTo{Point{100, 100}},
		LineTo{Point{0, 100}},
		LineTo{Point{0, 0}}, // Explicitly returns to start
		Close{},             // Close should skip zero-length edge
	}

	edges := CollectEdges(elements)

	// Zero-length edges should be skipped
	for i, e := range edges {
		if e.P0 == e.P1 {
			t.Errorf("Found zero-length edge at index %d: (%v,%v)->(%v,%v)",
				i, e.P0.X, e.P0.Y, e.P1.X, e.P1.Y)
		}
	}
}

func TestEdgeIterImplicitClose(t *testing.T) {
	// Path without explicit Close - should still close automatically
	elements := []PathElement{
		MoveTo{Point{0, 0}},
		LineTo{Point{100, 0}},
		LineTo{Point{50, 100}},
		// No Close - should auto-close
	}

	edges := CollectEdges(elements)

	// Should have 3 edges including implicit close
	if len(edges) != 3 {
		t.Errorf("Expected 3 edges (with implicit close), got %d", len(edges))
	}

	// Last edge should close back to start
	lastEdge := edges[len(edges)-1]
	if lastEdge.P1.X != 0 || lastEdge.P1.Y != 0 {
		t.Errorf("Last edge should close to (0,0), got (%v,%v)", lastEdge.P1.X, lastEdge.P1.Y)
	}
}
