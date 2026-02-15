// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"testing"
)

// TestNewLineEdge tests line edge creation from curve points.
func TestNewLineEdge(t *testing.T) {
	tests := []struct {
		name        string
		p0, p1      CurvePoint
		shift       int
		wantNil     bool
		wantWinding int8
	}{
		{
			name:        "simple downward",
			p0:          CurvePoint{X: 0, Y: 0},
			p1:          CurvePoint{X: 10, Y: 10},
			shift:       0,
			wantNil:     false,
			wantWinding: 1,
		},
		{
			name:        "upward reverses winding",
			p0:          CurvePoint{X: 10, Y: 10},
			p1:          CurvePoint{X: 0, Y: 0},
			shift:       0,
			wantNil:     false,
			wantWinding: -1,
		},
		{
			name:    "horizontal returns nil",
			p0:      CurvePoint{X: 0, Y: 5},
			p1:      CurvePoint{X: 10, Y: 5},
			shift:   0,
			wantNil: true,
		},
		{
			name:        "with AA shift",
			p0:          CurvePoint{X: 5, Y: 0},
			p1:          CurvePoint{X: 5, Y: 20},
			shift:       2,
			wantNil:     false,
			wantWinding: 1,
		},
		{
			name:        "very short but not horizontal",
			p0:          CurvePoint{X: 0, Y: 0},
			p1:          CurvePoint{X: 0, Y: 1},
			shift:       0,
			wantNil:     false,
			wantWinding: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewLineEdge(tt.p0, tt.p1, tt.shift)
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
			// FirstY should be <= LastY
			if edge.FirstY > edge.LastY {
				t.Errorf("FirstY=%d > LastY=%d", edge.FirstY, edge.LastY)
			}
		})
	}
}

// TestLineEdge_IsVertical tests vertical edge detection.
func TestLineEdge_IsVertical(t *testing.T) {
	vertical := NewLineEdge(
		CurvePoint{X: 5, Y: 0},
		CurvePoint{X: 5, Y: 20},
		0,
	)
	if vertical == nil {
		t.Fatal("expected non-nil vertical edge")
	}
	if !vertical.IsVertical() {
		t.Error("edge with same X should be vertical")
	}

	diagonal := NewLineEdge(
		CurvePoint{X: 0, Y: 0},
		CurvePoint{X: 10, Y: 10},
		0,
	)
	if diagonal == nil {
		t.Fatal("expected non-nil diagonal edge")
	}
	if diagonal.IsVertical() {
		t.Error("diagonal edge should not be vertical")
	}
}

// TestNewQuadraticEdge tests quadratic edge creation.
func TestNewQuadraticEdge(t *testing.T) {
	tests := []struct {
		name    string
		p0      CurvePoint
		p1      CurvePoint
		p2      CurvePoint
		shift   int
		wantNil bool
	}{
		{
			// Y-monotonic curve: endpoints at different Y values
			name:    "monotonic down",
			p0:      CurvePoint{X: 0, Y: 0},
			p1:      CurvePoint{X: 50, Y: 50},
			p2:      CurvePoint{X: 100, Y: 100},
			shift:   0,
			wantNil: false,
		},
		{
			// Y-monotonic curve with AA shift
			name:    "with AA shift",
			p0:      CurvePoint{X: 10, Y: 10},
			p1:      CurvePoint{X: 50, Y: 50},
			p2:      CurvePoint{X: 90, Y: 90},
			shift:   2,
			wantNil: false,
		},
		{
			// Arch where endpoints have same Y: top == bottom, returns nil
			name:    "arch same endpoint Y returns nil",
			p0:      CurvePoint{X: 0, Y: 0},
			p1:      CurvePoint{X: 50, Y: 50},
			p2:      CurvePoint{X: 100, Y: 0},
			shift:   0,
			wantNil: true,
		},
		{
			name:    "horizontal curve returns nil",
			p0:      CurvePoint{X: 0, Y: 10},
			p1:      CurvePoint{X: 50, Y: 10},
			p2:      CurvePoint{X: 100, Y: 10},
			shift:   0,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewQuadraticEdge(tt.p0, tt.p1, tt.p2, tt.shift)
			if tt.wantNil {
				if edge != nil {
					t.Errorf("expected nil edge for %s", tt.name)
				}
				return
			}
			if edge == nil {
				t.Fatalf("expected non-nil edge for %s", tt.name)
			}

			// Verify accessors
			line := edge.Line()
			if line == nil {
				t.Error("Line() returned nil")
			}
			_ = edge.CurveCount()
			_ = edge.Winding()
		})
	}
}

// TestQuadraticEdge_Update tests forward differencing stepping.
func TestQuadraticEdge_Update(t *testing.T) {
	// Create a quadratic edge with enough segments to step
	edge := NewQuadraticEdge(
		CurvePoint{X: 0, Y: 0},
		CurvePoint{X: 50, Y: 50},
		CurvePoint{X: 100, Y: 100},
		0,
	)
	if edge == nil {
		t.Skip("NewQuadraticEdge returned nil")
	}

	// Step through all segments
	steps := 0
	for edge.CurveCount() > 0 {
		if !edge.Update() {
			break
		}
		steps++
		if steps > 100 {
			t.Fatal("too many steps, possible infinite loop")
		}
	}

	t.Logf("QuadraticEdge produced %d segments", steps)
}

// TestNewCubicEdge tests cubic edge creation.
func TestNewCubicEdge(t *testing.T) {
	tests := []struct {
		name    string
		p0      CurvePoint
		p1      CurvePoint
		p2      CurvePoint
		p3      CurvePoint
		shift   int
		wantNil bool
	}{
		{
			name:    "S-curve",
			p0:      CurvePoint{X: 0, Y: 0},
			p1:      CurvePoint{X: 30, Y: 60},
			p2:      CurvePoint{X: 70, Y: 40},
			p3:      CurvePoint{X: 100, Y: 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "with AA shift",
			p0:      CurvePoint{X: 10, Y: 10},
			p1:      CurvePoint{X: 30, Y: 80},
			p2:      CurvePoint{X: 70, Y: 20},
			p3:      CurvePoint{X: 90, Y: 90},
			shift:   2,
			wantNil: false,
		},
		{
			name:    "horizontal returns nil",
			p0:      CurvePoint{X: 0, Y: 5},
			p1:      CurvePoint{X: 30, Y: 5},
			p2:      CurvePoint{X: 70, Y: 5},
			p3:      CurvePoint{X: 100, Y: 5},
			shift:   0,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewCubicEdge(tt.p0, tt.p1, tt.p2, tt.p3, tt.shift)
			if tt.wantNil {
				if edge != nil {
					t.Errorf("expected nil edge")
				}
				return
			}
			if edge == nil {
				t.Fatalf("expected non-nil edge")
			}

			// Verify accessors
			line := edge.Line()
			if line == nil {
				t.Error("Line() returned nil")
			}
			_ = edge.CurveCount()
			_ = edge.Winding()
		})
	}
}

// TestCubicEdge_Update tests cubic forward differencing.
func TestCubicEdge_Update(t *testing.T) {
	edge := NewCubicEdge(
		CurvePoint{X: 0, Y: 0},
		CurvePoint{X: 30, Y: 60},
		CurvePoint{X: 70, Y: 40},
		CurvePoint{X: 100, Y: 100},
		0,
	)
	if edge == nil {
		t.Skip("NewCubicEdge returned nil")
	}

	steps := 0
	for edge.CurveCount() < 0 { // Cubic uses negative count
		if !edge.Update() {
			break
		}
		steps++
		if steps > 200 {
			t.Fatal("too many steps, possible infinite loop")
		}
	}

	t.Logf("CubicEdge produced %d segments", steps)
}

// TestCurveEdgeVariant_AsLine tests the polymorphic AsLine method.
func TestCurveEdgeVariant_AsLine(t *testing.T) {
	t.Run("line variant", func(t *testing.T) {
		v := NewLineEdgeVariant(CurvePoint{X: 0, Y: 0}, CurvePoint{X: 10, Y: 10}, 0)
		if v == nil {
			t.Skip("nil variant")
		}
		line := v.AsLine()
		if line == nil {
			t.Error("AsLine() should return non-nil for line variant")
		}
	})

	t.Run("quadratic variant", func(t *testing.T) {
		v := NewQuadraticEdgeVariant(
			CurvePoint{X: 0, Y: 0},
			CurvePoint{X: 50, Y: 50},
			CurvePoint{X: 100, Y: 100},
			0,
		)
		if v == nil {
			t.Skip("nil variant")
		}
		line := v.AsLine()
		if line == nil {
			t.Error("AsLine() should return non-nil for quadratic variant")
		}
	})

	t.Run("cubic variant", func(t *testing.T) {
		v := NewCubicEdgeVariant(
			CurvePoint{X: 0, Y: 0},
			CurvePoint{X: 30, Y: 60},
			CurvePoint{X: 70, Y: 40},
			CurvePoint{X: 100, Y: 100},
			0,
		)
		if v == nil {
			t.Skip("nil variant")
		}
		line := v.AsLine()
		if line == nil {
			t.Error("AsLine() should return non-nil for cubic variant")
		}
	})

	t.Run("unknown type", func(t *testing.T) {
		v := CurveEdgeVariant{Type: EdgeType(99)}
		line := v.AsLine()
		if line != nil {
			t.Error("AsLine() should return nil for unknown type")
		}
	})
}

// TestCurveEdgeVariant_TopY tests TopY for all edge types.
func TestCurveEdgeVariant_TopY(t *testing.T) {
	t.Run("line", func(t *testing.T) {
		v := NewLineEdgeVariant(CurvePoint{X: 0, Y: 5}, CurvePoint{X: 10, Y: 20}, 0)
		if v == nil {
			t.Skip("nil variant")
		}
		topY := v.TopY()
		if topY < 0 {
			t.Errorf("TopY = %d, should be >= 0", topY)
		}
	})

	t.Run("quadratic", func(t *testing.T) {
		v := NewQuadraticEdgeVariant(
			CurvePoint{X: 0, Y: 5},
			CurvePoint{X: 50, Y: 50},
			CurvePoint{X: 100, Y: 5},
			0,
		)
		if v == nil {
			t.Skip("nil variant")
		}
		topY := v.TopY()
		if topY < 0 {
			t.Errorf("TopY = %d, should be >= 0", topY)
		}
	})

	t.Run("cubic", func(t *testing.T) {
		v := NewCubicEdgeVariant(
			CurvePoint{X: 0, Y: 5},
			CurvePoint{X: 30, Y: 60},
			CurvePoint{X: 70, Y: 40},
			CurvePoint{X: 100, Y: 80},
			0,
		)
		if v == nil {
			t.Skip("nil variant")
		}
		topY := v.TopY()
		if topY < 0 {
			t.Errorf("TopY = %d, should be >= 0", topY)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		v := CurveEdgeVariant{Type: EdgeType(99)}
		if v.TopY() != 0 {
			t.Errorf("TopY for unknown type = %d, want 0", v.TopY())
		}
	})
}

// TestCurveEdgeVariant_BottomY tests BottomY for all edge types.
func TestCurveEdgeVariant_BottomY(t *testing.T) {
	t.Run("line", func(t *testing.T) {
		v := NewLineEdgeVariant(CurvePoint{X: 0, Y: 0}, CurvePoint{X: 10, Y: 20}, 0)
		if v == nil {
			t.Skip("nil variant")
		}
		bottomY := v.BottomY()
		topY := v.TopY()
		if bottomY <= topY {
			t.Errorf("BottomY=%d should be > TopY=%d", bottomY, topY)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		v := CurveEdgeVariant{Type: EdgeType(99)}
		if v.BottomY() != 0 {
			t.Errorf("BottomY for unknown type = %d, want 0", v.BottomY())
		}
	})
}

// TestCurveEdgeVariant_Update tests Update dispatch.
func TestCurveEdgeVariant_Update(t *testing.T) {
	t.Run("line always false", func(t *testing.T) {
		v := NewLineEdgeVariant(CurvePoint{X: 0, Y: 0}, CurvePoint{X: 10, Y: 10}, 0)
		if v == nil {
			t.Skip("nil variant")
		}
		if v.Update() {
			t.Error("Update() on line variant should return false")
		}
	})

	t.Run("unknown always false", func(t *testing.T) {
		v := CurveEdgeVariant{Type: EdgeType(99)}
		if v.Update() {
			t.Error("Update() on unknown variant should return false")
		}
	})
}

// TestComputeDY tests the DY computation for first scanline offset.
func TestComputeDY(t *testing.T) {
	// computeDY(top, y0) = top * 64 + 32 - y0
	tests := []struct {
		name string
		top  int32
		y0   FDot6
		want FDot6
	}{
		{"aligned", 1, 64, 32}, // 1*64 + 32 - 64 = 32
		{"zero", 0, 0, 32},     // 0 + 32 - 0 = 32
		{"offset", 2, 100, 60}, // 2*64 + 32 - 100 = 60
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDY(tt.top, tt.y0)
			if got != tt.want {
				t.Errorf("computeDY(%d, %d) = %d, want %d", tt.top, tt.y0, got, tt.want)
			}
		})
	}
}
