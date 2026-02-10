//go:build !nogpu

// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package gpu

import (
	"github.com/gogpu/gg/internal/raster"
	"testing"
)

// TestFDot6Conversion tests raster.FDot6 conversion functions.
func TestFDot6Conversion(t *testing.T) {
	tests := []struct {
		name    string
		input   float32
		want    raster.FDot6
		epsilon float32
	}{
		{"zero", 0.0, 0, 0.016},
		{"one", 1.0, 64, 0.016},
		{"half", 0.5, 32, 0.016},
		{"negative", -1.0, -64, 0.016},
		{"fractional", 0.25, 16, 0.016},
		{"large", 100.0, 6400, 0.016},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raster.FDot6FromFloat32(tt.input)
			if got != tt.want {
				t.Errorf("raster.FDot6FromFloat32(%v) = %v, want %v", tt.input, got, tt.want)
			}

			// Round-trip test
			roundTrip := raster.FDot6ToFloat32(got)
			if diff := absFloat32(roundTrip - tt.input); diff > tt.epsilon {
				t.Errorf("round-trip: raster.FDot6ToFloat32(raster.FDot6FromFloat32(%v)) = %v, diff = %v",
					tt.input, roundTrip, diff)
			}
		})
	}
}

// TestFDot6Rounding tests raster.FDot6 floor/ceil/round operations.
func TestFDot6Rounding(t *testing.T) {
	tests := []struct {
		name  string
		input raster.FDot6
		floor int32
		ceil  int32
		round int32
	}{
		{"zero", 0, 0, 0, 0},
		{"one", 64, 1, 1, 1},
		{"half", 32, 0, 1, 1},
		{"quarter", 16, 0, 1, 0},
		{"three-quarters", 48, 0, 1, 1},
		{"negative-half", -32, -1, 0, 0},
		{"negative-one", -64, -1, -1, -1},
		{"one-point-five", 96, 1, 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := raster.FDot6Floor(tt.input); got != tt.floor {
				t.Errorf("raster.FDot6Floor(%v) = %v, want %v", tt.input, got, tt.floor)
			}
			if got := raster.FDot6Ceil(tt.input); got != tt.ceil {
				t.Errorf("raster.FDot6Ceil(%v) = %v, want %v", tt.input, got, tt.ceil)
			}
			if got := raster.FDot6Round(tt.input); got != tt.round {
				t.Errorf("raster.FDot6Round(%v) = %v, want %v", tt.input, got, tt.round)
			}
		})
	}
}

// TestFDot16Conversion tests raster.FDot16 conversion functions.
func TestFDot16Conversion(t *testing.T) {
	tests := []struct {
		name    string
		input   float32
		want    raster.FDot16
		epsilon float32
	}{
		{"zero", 0.0, 0, 0.0001},
		{"one", 1.0, 65536, 0.0001},
		{"half", 0.5, 32768, 0.0001},
		{"negative", -1.0, -65536, 0.0001},
		{"fractional", 0.25, 16384, 0.0001},
		{"small", 0.001, 65, 0.0001}, // 0.001 * 65536 ≈ 65.5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raster.FDot16FromFloat32(tt.input)
			diff := absInt32(got - tt.want)
			// Allow 1 unit tolerance for rounding
			if diff > 1 {
				t.Errorf("raster.FDot16FromFloat32(%v) = %v, want %v (diff=%v)", tt.input, got, tt.want, diff)
			}

			// Round-trip test
			roundTrip := raster.FDot16ToFloat32(got)
			if diff := absFloat32(roundTrip - tt.input); diff > tt.epsilon {
				t.Errorf("round-trip: raster.FDot16ToFloat32(raster.FDot16FromFloat32(%v)) = %v, diff = %v",
					tt.input, roundTrip, diff)
			}
		})
	}
}

// TestFDot6ToFDot16 tests conversion between fixed-point formats.
func TestFDot6ToFDot16(t *testing.T) {
	tests := []struct {
		name  string
		input raster.FDot6
		want  raster.FDot16
	}{
		{"zero", 0, 0},
		{"one", 64, 65536},     // 1.0 in raster.FDot6 -> 1.0 in raster.FDot16
		{"half", 32, 32768},    // 0.5 in raster.FDot6 -> 0.5 in raster.FDot16
		{"quarter", 16, 16384}, // 0.25 in both
		{"negative", -64, -65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raster.FDot6ToFDot16(tt.input)
			if got != tt.want {
				t.Errorf("raster.FDot6ToFDot16(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot16Mul tests fixed-point multiplication.
func TestFDot16Mul(t *testing.T) {
	tests := []struct {
		name string
		a, b raster.FDot16
		want raster.FDot16
	}{
		{"one times one", 65536, 65536, 65536},
		{"half times half", 32768, 32768, 16384}, // 0.5 * 0.5 = 0.25
		{"two times half", 131072, 32768, 65536}, // 2.0 * 0.5 = 1.0
		{"negative", 65536, -65536, -65536},      // 1.0 * -1.0 = -1.0
		{"small", 6553, 6553, 655},               // ~0.1 * ~0.1 ≈ 0.01
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raster.FDot16Mul(tt.a, tt.b)
			// Allow 1 unit tolerance for rounding
			if diff := absInt32(got - tt.want); diff > 1 {
				t.Errorf("raster.FDot16Mul(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestFDot6Div tests fixed-point division.
// raster.FDot6Div divides two raster.FDot6 values and returns an raster.FDot16 slope.
// For edge scanline conversion: slope = (x1-x0) / (y1-y0)
// The result is in raster.FDot16 format (16.16 fixed-point).
func TestFDot6Div(t *testing.T) {
	tests := []struct {
		name     string
		a, b     raster.FDot6
		wantNear raster.FDot16 // Expected result in raster.FDot16
	}{
		// 1.0 / 1.0 = 1.0 (raster.FDot16)
		{"one over one", 64, 64, raster.FDot16One},
		// 2.0 / 1.0 = 2.0 (raster.FDot16)
		{"two over one", 128, 64, 2 * raster.FDot16One},
		// 1.0 / 2.0 = 0.5 (raster.FDot16)
		{"one over two", 64, 128, raster.FDot16Half},
		// 100.0 / 1.0 = 100.0 (raster.FDot16)
		{"large numerator", 6400, 64, 100 * raster.FDot16One},
		// 0 / 1.0 = 0
		{"zero numerator", 0, 64, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := raster.FDot6Div(tt.a, tt.b)
			// Allow 1 unit tolerance for rounding
			diff := absInt32(got - tt.wantNear)
			if diff > 1 {
				t.Errorf("raster.FDot6Div(%v, %v) = %v, want %v (diff=%v)",
					tt.a, tt.b, got, tt.wantNear, diff)
			}
		})
	}

	// Test division by zero separately
	t.Run("zero denominator", func(t *testing.T) {
		got := raster.FDot6Div(64, 0)
		if got != 0x7FFFFFFF && got != -0x7FFFFFFF {
			t.Errorf("raster.FDot6Div(64, 0) = %v, want max value", got)
		}
	})
}

// Tests for diffToShift, cheapDistance, and cubicDeltaFromLine have been moved
// to raster/curve_edge_test.go since they test unexported raster functions.

// TestLineEdge tests basic line edge creation.
func TestLineEdge(t *testing.T) {
	tests := []struct {
		name    string
		p0, p1  raster.CurvePoint
		shift   int
		wantNil bool
		wantUp  bool // winding should be negative (upward)
	}{
		{
			name:    "downward line",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 10, Y: 10},
			shift:   0,
			wantNil: false,
			wantUp:  false,
		},
		{
			name:    "upward line",
			p0:      raster.CurvePoint{X: 10, Y: 10},
			p1:      raster.CurvePoint{X: 0, Y: 0},
			shift:   0,
			wantNil: false,
			wantUp:  true,
		},
		{
			name:    "horizontal line (should be nil)",
			p0:      raster.CurvePoint{X: 0, Y: 5},
			p1:      raster.CurvePoint{X: 10, Y: 5},
			shift:   0,
			wantNil: true,
		},
		{
			name:    "vertical line",
			p0:      raster.CurvePoint{X: 5, Y: 0},
			p1:      raster.CurvePoint{X: 5, Y: 10},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "with AA shift",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 10, Y: 10},
			shift:   2,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := raster.NewLineEdge(tt.p0, tt.p1, tt.shift)

			if (edge == nil) != tt.wantNil {
				t.Errorf("raster.NewLineEdge() nil = %v, want nil = %v", edge == nil, tt.wantNil)
				return
			}

			if edge == nil {
				return
			}

			if tt.wantUp && edge.Winding != -1 {
				t.Errorf("expected upward winding (-1), got %v", edge.Winding)
			}
			if !tt.wantUp && edge.Winding != 1 {
				t.Errorf("expected downward winding (1), got %v", edge.Winding)
			}

			// FirstY should always be <= LastY
			if edge.FirstY > edge.LastY {
				t.Errorf("FirstY (%v) > LastY (%v)", edge.FirstY, edge.LastY)
			}
		})
	}
}

// TestQuadraticEdge tests quadratic Bezier edge creation and iteration.
func TestQuadraticEdge(t *testing.T) {
	tests := []struct {
		name       string
		p0, p1, p2 raster.CurvePoint
		shift      int
		wantNil    bool
	}{
		{
			name:    "simple quadratic",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 50, Y: 0}, // Control point above
			p2:      raster.CurvePoint{X: 100, Y: 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "flat quadratic (becomes line)",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 50, Y: 50}, // On the chord
			p2:      raster.CurvePoint{X: 100, Y: 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "horizontal quadratic (should be nil)",
			p0:      raster.CurvePoint{X: 0, Y: 50},
			p1:      raster.CurvePoint{X: 50, Y: 50},
			p2:      raster.CurvePoint{X: 100, Y: 50},
			shift:   0,
			wantNil: true,
		},
		{
			name:    "curved quadratic",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 25, Y: 50}, // Significant curvature
			p2:      raster.CurvePoint{X: 50, Y: 0},
			shift:   0,
			wantNil: true, // Returns to same Y, so no vertical extent
		},
		{
			name:    "with AA shift",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 50, Y: 25},
			p2:      raster.CurvePoint{X: 100, Y: 100},
			shift:   2,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := raster.NewQuadraticEdge(tt.p0, tt.p1, tt.p2, tt.shift)

			if (edge == nil) != tt.wantNil {
				t.Errorf("raster.NewQuadraticEdge() nil = %v, want nil = %v", edge == nil, tt.wantNil)
				return
			}

			if edge == nil {
				return
			}

			// Test iteration
			segmentCount := 1 // First segment already processed
			for edge.Update() {
				segmentCount++
				if segmentCount > 100 {
					t.Fatal("too many segments, possible infinite loop")
				}
			}

			// Should produce at least one segment
			if segmentCount < 1 {
				t.Errorf("expected at least 1 segment, got %v", segmentCount)
			}

			t.Logf("quadratic produced %d segments", segmentCount)
		})
	}
}

// TestQuadraticEdgeForwardDifferencing verifies the forward differencing algorithm.
func TestQuadraticEdgeForwardDifferencing(t *testing.T) {
	// Create a quadratic with known control points
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 50, Y: 50}
	p2 := raster.CurvePoint{X: 100, Y: 100}

	edge := raster.NewQuadraticEdge(p0, p1, p2, 0)
	if edge == nil {
		t.Fatal("expected non-nil edge")
	}

	// The quadratic p(t) = (1-t)^2*p0 + 2t(1-t)*p1 + t^2*p2
	// For this case, it should be approximately a straight line

	// Collect all line segments
	var lines []struct{ x0, y0, x1, y1 float32 }

	line := edge.Line()
	lines = append(lines, struct{ x0, y0, x1, y1 float32 }{
		float32(line.FirstY), float32(line.LastY),
		raster.FDot16ToFloat32(line.X), raster.FDot16ToFloat32(line.X + line.DX*(line.LastY-line.FirstY)),
	})

	for edge.Update() {
		line := edge.Line()
		lines = append(lines, struct{ x0, y0, x1, y1 float32 }{
			float32(line.FirstY), float32(line.LastY),
			raster.FDot16ToFloat32(line.X), raster.FDot16ToFloat32(line.X + line.DX*(line.LastY-line.FirstY)),
		})
	}

	// Verify we got some lines
	if len(lines) == 0 {
		t.Error("expected at least one line segment")
	}

	t.Logf("forward differencing produced %d line segments", len(lines))
}

// TestCubicEdge tests cubic Bezier edge creation and iteration.
func TestCubicEdge(t *testing.T) {
	tests := []struct {
		name           string
		p0, p1, p2, p3 raster.CurvePoint
		shift          int
		wantNil        bool
	}{
		{
			name:    "simple cubic",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 33, Y: 50},
			p2:      raster.CurvePoint{X: 66, Y: 50},
			p3:      raster.CurvePoint{X: 100, Y: 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "S-curve",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 100, Y: 0},
			p2:      raster.CurvePoint{X: 0, Y: 100},
			p3:      raster.CurvePoint{X: 100, Y: 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "horizontal cubic (should be nil)",
			p0:      raster.CurvePoint{X: 0, Y: 50},
			p1:      raster.CurvePoint{X: 33, Y: 50},
			p2:      raster.CurvePoint{X: 66, Y: 50},
			p3:      raster.CurvePoint{X: 100, Y: 50},
			shift:   0,
			wantNil: true,
		},
		{
			name:    "with AA shift",
			p0:      raster.CurvePoint{X: 0, Y: 0},
			p1:      raster.CurvePoint{X: 33, Y: 25},
			p2:      raster.CurvePoint{X: 66, Y: 75},
			p3:      raster.CurvePoint{X: 100, Y: 100},
			shift:   2,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := raster.NewCubicEdge(tt.p0, tt.p1, tt.p2, tt.p3, tt.shift)

			if (edge == nil) != tt.wantNil {
				t.Errorf("raster.NewCubicEdge() nil = %v, want nil = %v", edge == nil, tt.wantNil)
				return
			}

			if edge == nil {
				return
			}

			// Test iteration
			segmentCount := 1 // First segment already processed
			for edge.Update() {
				segmentCount++
				if segmentCount > 200 {
					t.Fatal("too many segments, possible infinite loop")
				}
			}

			// Should produce at least one segment
			if segmentCount < 1 {
				t.Errorf("expected at least 1 segment, got %v", segmentCount)
			}

			t.Logf("cubic produced %d segments", segmentCount)
		})
	}
}

// TestCubicEdgeForwardDifferencing verifies cubic forward differencing algorithm.
func TestCubicEdgeForwardDifferencing(t *testing.T) {
	// Create a cubic with known control points
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 25, Y: 50}
	p2 := raster.CurvePoint{X: 75, Y: 50}
	p3 := raster.CurvePoint{X: 100, Y: 100}

	edge := raster.NewCubicEdge(p0, p1, p2, p3, 0)
	if edge == nil {
		t.Fatal("expected non-nil edge")
	}

	// Collect all Y values to verify monotonicity
	var yValues []int32
	yValues = append(yValues, edge.Line().FirstY)

	for edge.Update() {
		yValues = append(yValues, edge.Line().FirstY)
	}

	// Verify Y values are non-decreasing (monotonic)
	for i := 1; i < len(yValues); i++ {
		if yValues[i] < yValues[i-1] {
			t.Errorf("Y values not monotonic: Y[%d]=%v < Y[%d]=%v",
				i, yValues[i], i-1, yValues[i-1])
		}
	}

	t.Logf("cubic forward differencing: %d Y values, range [%d, %d]",
		len(yValues), yValues[0], yValues[len(yValues)-1])
}

// TestEdgeVariant tests the polymorphic edge wrapper.
func TestEdgeVariant(t *testing.T) {
	t.Run("line variant", func(t *testing.T) {
		v := raster.NewLineEdgeVariant(raster.CurvePoint{X: 0, Y: 0}, raster.CurvePoint{X: 100, Y: 100}, 0)
		if v == nil {
			t.Fatal("expected non-nil variant")
		}
		if v.Type != raster.EdgeTypeLine {
			t.Errorf("expected raster.EdgeTypeLine, got %v", v.Type)
		}
		if v.AsLine() == nil {
			t.Error("AsLine() returned nil")
		}
		if v.Update() {
			t.Error("line variant should not produce more segments")
		}
	})

	t.Run("quadratic variant", func(t *testing.T) {
		v := raster.NewQuadraticEdgeVariant(
			raster.CurvePoint{X: 0, Y: 0},
			raster.CurvePoint{X: 50, Y: 25},
			raster.CurvePoint{X: 100, Y: 100},
			0,
		)
		if v == nil {
			t.Fatal("expected non-nil variant")
		}
		if v.Type != raster.EdgeTypeQuadratic {
			t.Errorf("expected raster.EdgeTypeQuadratic, got %v", v.Type)
		}

		count := 0
		for v.Update() {
			count++
		}
		t.Logf("quadratic variant produced %d additional segments", count)
	})

	t.Run("cubic variant", func(t *testing.T) {
		v := raster.NewCubicEdgeVariant(
			raster.CurvePoint{X: 0, Y: 0},
			raster.CurvePoint{X: 25, Y: 50},
			raster.CurvePoint{X: 75, Y: 50},
			raster.CurvePoint{X: 100, Y: 100},
			0,
		)
		if v == nil {
			t.Fatal("expected non-nil variant")
		}
		if v.Type != raster.EdgeTypeCubic {
			t.Errorf("expected raster.EdgeTypeCubic, got %v", v.Type)
		}

		count := 0
		for v.Update() {
			count++
		}
		t.Logf("cubic variant produced %d additional segments", count)
	})
}

// TestCubicDeltaFromLine moved to raster/curve_edge_test.go.

// Benchmark tests

// BenchmarkQuadraticEdgeCreate benchmarks quadratic edge creation.
func BenchmarkQuadraticEdgeCreate(b *testing.B) {
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 50, Y: 50}
	p2 := raster.CurvePoint{X: 100, Y: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = raster.NewQuadraticEdge(p0, p1, p2, 0)
	}
}

// BenchmarkQuadraticEdgeUpdate benchmarks forward differencing step.
func BenchmarkQuadraticEdgeUpdate(b *testing.B) {
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 50, Y: 50}
	p2 := raster.CurvePoint{X: 100, Y: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		edge := raster.NewQuadraticEdge(p0, p1, p2, 0)
		if edge != nil {
			for edge.Update() {
				// O(1) per step - forward differencing
			}
		}
	}
}

// BenchmarkCubicEdgeCreate benchmarks cubic edge creation.
func BenchmarkCubicEdgeCreate(b *testing.B) {
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 33, Y: 50}
	p2 := raster.CurvePoint{X: 66, Y: 50}
	p3 := raster.CurvePoint{X: 100, Y: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = raster.NewCubicEdge(p0, p1, p2, p3, 0)
	}
}

// BenchmarkCubicEdgeUpdate benchmarks cubic forward differencing.
func BenchmarkCubicEdgeUpdate(b *testing.B) {
	p0 := raster.CurvePoint{X: 0, Y: 0}
	p1 := raster.CurvePoint{X: 33, Y: 50}
	p2 := raster.CurvePoint{X: 66, Y: 50}
	p3 := raster.CurvePoint{X: 100, Y: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		edge := raster.NewCubicEdge(p0, p1, p2, p3, 0)
		if edge != nil {
			for edge.Update() {
				// O(1) per step - forward differencing
			}
		}
	}
}

// BenchmarkNaiveQuadraticEval benchmarks naive Bezier evaluation for comparison.
func BenchmarkNaiveQuadraticEval(b *testing.B) {
	// Naive approach: evaluate Bezier at each step using full formula
	p0x, p0y := float32(0), float32(0)
	p1x, p1y := float32(50), float32(50)
	p2x, p2y := float32(100), float32(100)
	steps := 16

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for s := 0; s <= steps; s++ {
			t := float32(s) / float32(steps)
			oneMinusT := 1 - t
			// Naive evaluation: O(n) multiplications per step
			_ = oneMinusT*oneMinusT*p0x + 2*t*oneMinusT*p1x + t*t*p2x
			_ = oneMinusT*oneMinusT*p0y + 2*t*oneMinusT*p1y + t*t*p2y
		}
	}
}

// BenchmarkDiffToShift moved to raster/curve_edge_test.go.

// absInt32 returns the absolute value of an int32 (test helper).
func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// absFloat32 returns the absolute value of a float32 (test helper).
func absFloat32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
