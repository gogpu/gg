// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// TestNewEdge tests creating edges from two points.
func TestNewEdge(t *testing.T) {
	tests := []struct {
		name        string
		x0, y0      float32
		x1, y1      float32
		wantNil     bool
		wantYMin    float32
		wantYMax    float32
		wantXAtYMin float32
	}{
		{
			name: "simple downward edge",
			x0:   0, y0: 0, x1: 10, y1: 10,
			wantNil: false, wantYMin: 0, wantYMax: 10, wantXAtYMin: 0,
		},
		{
			name: "upward edge normalized",
			x0:   10, y0: 10, x1: 0, y1: 0,
			wantNil: false, wantYMin: 0, wantYMax: 10, wantXAtYMin: 0,
		},
		{
			name: "horizontal edge returns nil",
			x0:   0, y0: 5, x1: 10, y1: 5,
			wantNil: true,
		},
		{
			name: "very small dy returns nil",
			x0:   0, y0: 5, x1: 10, y1: 5.0000001,
			wantNil: true,
		},
		{
			name: "vertical edge",
			x0:   5, y0: 0, x1: 5, y1: 20,
			wantNil: false, wantYMin: 0, wantYMax: 20, wantXAtYMin: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewEdge(tt.x0, tt.y0, tt.x1, tt.y1)
			if tt.wantNil {
				if edge != nil {
					t.Errorf("expected nil edge for horizontal line, got %+v", edge)
				}
				return
			}
			if edge == nil {
				t.Fatal("expected non-nil edge")
			}
			if edge.YMin != tt.wantYMin {
				t.Errorf("YMin = %f, want %f", edge.YMin, tt.wantYMin)
			}
			if edge.YMax != tt.wantYMax {
				t.Errorf("YMax = %f, want %f", edge.YMax, tt.wantYMax)
			}
			if edge.XAtYMin != tt.wantXAtYMin {
				t.Errorf("XAtYMin = %f, want %f", edge.XAtYMin, tt.wantXAtYMin)
			}
		})
	}
}

// TestNewEdgeWithWinding tests edge creation with explicit winding.
func TestNewEdgeWithWinding(t *testing.T) {
	tests := []struct {
		name        string
		x0, y0      float32
		x1, y1      float32
		winding     int8
		wantNil     bool
		wantWinding int8
	}{
		{
			name: "downward with positive winding",
			x0:   0, y0: 0, x1: 10, y1: 10,
			winding: 1, wantNil: false, wantWinding: 1,
		},
		{
			name: "needs swap reverses winding",
			x0:   10, y0: 10, x1: 0, y1: 0,
			winding: 1, wantNil: false, wantWinding: -1,
		},
		{
			name: "horizontal returns nil",
			x0:   0, y0: 5, x1: 10, y1: 5,
			winding: 1, wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewEdgeWithWinding(tt.x0, tt.y0, tt.x1, tt.y1, tt.winding)
			if tt.wantNil {
				if edge != nil {
					t.Errorf("expected nil edge")
				}
				return
			}
			if edge == nil {
				t.Fatal("expected non-nil edge")
			}
			if edge.Winding != tt.wantWinding {
				t.Errorf("Winding = %d, want %d", edge.Winding, tt.wantWinding)
			}
		})
	}
}

// TestEdgeXAtY tests X coordinate interpolation along edge.
func TestEdgeXAtY(t *testing.T) {
	// 45-degree edge from (0,0) to (10,10), DXDY = 1
	edge := &Edge{
		YMin: 0, YMax: 10, XAtYMin: 0, DXDY: 1.0, Winding: 1,
	}

	tests := []struct {
		y    float32
		want float32
	}{
		{0, 0},
		{5, 5},
		{10, 10},
	}

	for _, tt := range tests {
		got := edge.XAtY(tt.y)
		if got != tt.want {
			t.Errorf("XAtY(%f) = %f, want %f", tt.y, got, tt.want)
		}
	}
}

// TestEdgeIsActiveAt tests active range check.
func TestEdgeIsActiveAt(t *testing.T) {
	edge := &Edge{YMin: 5, YMax: 15, XAtYMin: 0, DXDY: 0, Winding: 1}

	tests := []struct {
		y    float32
		want bool
	}{
		{4, false}, // below YMin
		{5, true},  // at YMin
		{10, true}, // middle
		{14.9, true},
		{15, false}, // at YMax (exclusive)
		{16, false}, // above YMax
	}

	for _, tt := range tests {
		got := edge.IsActiveAt(tt.y)
		if got != tt.want {
			t.Errorf("IsActiveAt(%f) = %v, want %v", tt.y, got, tt.want)
		}
	}
}

// TestEdgeContainsY tests inclusive Y range check.
func TestEdgeContainsY(t *testing.T) {
	edge := &Edge{YMin: 5, YMax: 15, XAtYMin: 0, DXDY: 0, Winding: 1}

	tests := []struct {
		y    float32
		want bool
	}{
		{4, false},
		{5, true},
		{10, true},
		{15, true}, // inclusive at YMax
		{16, false},
	}

	for _, tt := range tests {
		got := edge.ContainsY(tt.y)
		if got != tt.want {
			t.Errorf("ContainsY(%f) = %v, want %v", tt.y, got, tt.want)
		}
	}
}

// TestEdgeHeight tests vertical extent calculation.
func TestEdgeHeight(t *testing.T) {
	edge := &Edge{YMin: 3, YMax: 17}
	if h := edge.Height(); h != 14 {
		t.Errorf("Height() = %f, want 14", h)
	}
}

// TestEdgeList tests the edge collection operations.
func TestEdgeList(t *testing.T) {
	el := NewEdgeList()

	if el.Len() != 0 {
		t.Errorf("new EdgeList Len = %d, want 0", el.Len())
	}

	// Add edges
	el.AddLine(0, 0, 10, 10) // diagonal, winding +1
	el.AddLine(5, 20, 5, 0)  // upward vertical, winding -1

	if el.Len() != 2 {
		t.Errorf("EdgeList Len after 2 adds = %d, want 2", el.Len())
	}

	// Add nil edge (horizontal)
	el.Add(nil)
	if el.Len() != 2 {
		t.Errorf("EdgeList Len after adding nil = %d, want 2", el.Len())
	}

	// Add horizontal line (should not add)
	el.AddLine(0, 5, 10, 5)
	if el.Len() != 2 {
		t.Errorf("EdgeList Len after horizontal = %d, want 2", el.Len())
	}

	// Test SortByYMin
	el2 := NewEdgeList()
	el2.AddLine(0, 10, 5, 20)
	el2.AddLine(0, 0, 5, 5)
	el2.AddLine(0, 5, 5, 15)
	el2.SortByYMin()
	edges := el2.Edges()
	for i := 1; i < len(edges); i++ {
		if edges[i].YMin < edges[i-1].YMin {
			t.Errorf("SortByYMin: edge[%d].YMin=%f < edge[%d].YMin=%f",
				i, edges[i].YMin, i-1, edges[i-1].YMin)
		}
	}

	// Test Reset
	el.Reset()
	if el.Len() != 0 {
		t.Errorf("EdgeList Len after Reset = %d, want 0", el.Len())
	}
}

// TestEdgeListBounds tests bounding rectangle computation.
func TestEdgeListBounds(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		el := NewEdgeList()
		minX, minY, maxX, maxY := el.Bounds()
		if minX != 0 || minY != 0 || maxX != 0 || maxY != 0 {
			t.Errorf("empty bounds = (%f,%f,%f,%f), want (0,0,0,0)", minX, minY, maxX, maxY)
		}
	})

	t.Run("single edge", func(t *testing.T) {
		el := NewEdgeList()
		el.AddLine(5, 10, 15, 20)
		minX, minY, maxX, maxY := el.Bounds()
		if minX != 5 || minY != 10 || maxX != 15 || maxY != 20 {
			t.Errorf("bounds = (%f,%f,%f,%f), want (5,10,15,20)", minX, minY, maxX, maxY)
		}
	})

	t.Run("multiple edges", func(t *testing.T) {
		el := NewEdgeList()
		el.AddLine(0, 0, 5, 10)
		el.AddLine(-5, 5, 20, 30)
		minX, minY, maxX, maxY := el.Bounds()
		if minX > -5 || minY > 0 || maxX < 20 || maxY < 30 {
			t.Errorf("bounds = (%f,%f,%f,%f), want (-5,0,20,30) approximately", minX, minY, maxX, maxY)
		}
	})
}

// TestSimpleAET tests the simple active edge table.
func TestSimpleAET(t *testing.T) {
	aet := NewSimpleAET()

	if aet.Len() != 0 {
		t.Errorf("new AET Len = %d, want 0", aet.Len())
	}

	// Create edges
	e1 := &Edge{YMin: 0, YMax: 10, XAtYMin: 5, DXDY: 0, Winding: 1}
	e2 := &Edge{YMin: 0, YMax: 10, XAtYMin: 15, DXDY: 0, Winding: -1}
	e3 := &Edge{YMin: 0, YMax: 10, XAtYMin: 10, DXDY: 0, Winding: 1}

	// InsertEdge should maintain X order
	aet.InsertEdge(e1, 0)
	aet.InsertEdge(e2, 0)
	aet.InsertEdge(e3, 0)

	if aet.Len() != 3 {
		t.Fatalf("AET Len after 3 inserts = %d, want 3", aet.Len())
	}

	// Verify X-sorted order
	active := aet.Active()
	if active[0].X != 5 || active[1].X != 10 || active[2].X != 15 {
		t.Errorf("AET not sorted: X values = [%f, %f, %f]",
			active[0].X, active[1].X, active[2].X)
	}

	// UpdateX at y=5 (with 0 slope, X shouldn't change)
	aet.UpdateX(5)
	active = aet.Active()
	if active[0].X != 5 {
		t.Errorf("after UpdateX: X = %f, want 5", active[0].X)
	}

	// SortByX
	aet.SortByX()
	if aet.Len() != 3 {
		t.Errorf("AET Len after sort = %d, want 3", aet.Len())
	}

	// RemoveExpired
	aet.RemoveExpired(11)
	if aet.Len() != 0 {
		t.Errorf("AET Len after RemoveExpired(11) = %d, want 0", aet.Len())
	}

	// Reset
	aet.Reset()
	if aet.Len() != 0 {
		t.Errorf("AET Len after Reset = %d, want 0", aet.Len())
	}
}

// TestSimpleAET_RemoveExpired tests partial edge expiration.
func TestSimpleAET_RemoveExpired(t *testing.T) {
	aet := NewSimpleAET()

	e1 := &Edge{YMin: 0, YMax: 5, XAtYMin: 0, DXDY: 0, Winding: 1}
	e2 := &Edge{YMin: 0, YMax: 10, XAtYMin: 10, DXDY: 0, Winding: 1}
	e3 := &Edge{YMin: 0, YMax: 15, XAtYMin: 20, DXDY: 0, Winding: 1}

	aet.InsertEdge(e1, 0)
	aet.InsertEdge(e2, 0)
	aet.InsertEdge(e3, 0)

	aet.RemoveExpired(7) // e1 (YMax=5) should be removed
	if aet.Len() != 2 {
		t.Errorf("AET Len after RemoveExpired(7) = %d, want 2", aet.Len())
	}

	aet.RemoveExpired(12) // e2 (YMax=10) should be removed
	if aet.Len() != 1 {
		t.Errorf("AET Len after RemoveExpired(12) = %d, want 1", aet.Len())
	}
}
