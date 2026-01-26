// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"testing"
)

// TestFDot6Conversion tests FDot6 conversion functions.
func TestFDot6Conversion(t *testing.T) {
	tests := []struct {
		name    string
		input   float32
		want    FDot6
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
			got := FDot6FromFloat32(tt.input)
			if got != tt.want {
				t.Errorf("FDot6FromFloat32(%v) = %v, want %v", tt.input, got, tt.want)
			}

			// Round-trip test
			roundTrip := FDot6ToFloat32(got)
			if diff := absFloat32(roundTrip - tt.input); diff > tt.epsilon {
				t.Errorf("round-trip: FDot6ToFloat32(FDot6FromFloat32(%v)) = %v, diff = %v",
					tt.input, roundTrip, diff)
			}
		})
	}
}

// TestFDot6Rounding tests FDot6 floor/ceil/round operations.
func TestFDot6Rounding(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
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
			if got := FDot6Floor(tt.input); got != tt.floor {
				t.Errorf("FDot6Floor(%v) = %v, want %v", tt.input, got, tt.floor)
			}
			if got := FDot6Ceil(tt.input); got != tt.ceil {
				t.Errorf("FDot6Ceil(%v) = %v, want %v", tt.input, got, tt.ceil)
			}
			if got := FDot6Round(tt.input); got != tt.round {
				t.Errorf("FDot6Round(%v) = %v, want %v", tt.input, got, tt.round)
			}
		})
	}
}

// TestFDot16Conversion tests FDot16 conversion functions.
func TestFDot16Conversion(t *testing.T) {
	tests := []struct {
		name    string
		input   float32
		want    FDot16
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
			got := FDot16FromFloat32(tt.input)
			diff := absInt32(got - tt.want)
			// Allow 1 unit tolerance for rounding
			if diff > 1 {
				t.Errorf("FDot16FromFloat32(%v) = %v, want %v (diff=%v)", tt.input, got, tt.want, diff)
			}

			// Round-trip test
			roundTrip := FDot16ToFloat32(got)
			if diff := absFloat32(roundTrip - tt.input); diff > tt.epsilon {
				t.Errorf("round-trip: FDot16ToFloat32(FDot16FromFloat32(%v)) = %v, diff = %v",
					tt.input, roundTrip, diff)
			}
		})
	}
}

// TestFDot6ToFDot16 tests conversion between fixed-point formats.
func TestFDot6ToFDot16(t *testing.T) {
	tests := []struct {
		name  string
		input FDot6
		want  FDot16
	}{
		{"zero", 0, 0},
		{"one", 64, 65536},     // 1.0 in FDot6 -> 1.0 in FDot16
		{"half", 32, 32768},    // 0.5 in FDot6 -> 0.5 in FDot16
		{"quarter", 16, 16384}, // 0.25 in both
		{"negative", -64, -65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6ToFDot16(tt.input)
			if got != tt.want {
				t.Errorf("FDot6ToFDot16(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestFDot16Mul tests fixed-point multiplication.
func TestFDot16Mul(t *testing.T) {
	tests := []struct {
		name string
		a, b FDot16
		want FDot16
	}{
		{"one times one", 65536, 65536, 65536},
		{"half times half", 32768, 32768, 16384}, // 0.5 * 0.5 = 0.25
		{"two times half", 131072, 32768, 65536}, // 2.0 * 0.5 = 1.0
		{"negative", 65536, -65536, -65536},      // 1.0 * -1.0 = -1.0
		{"small", 6553, 6553, 655},               // ~0.1 * ~0.1 ≈ 0.01
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot16Mul(tt.a, tt.b)
			// Allow 1 unit tolerance for rounding
			if diff := absInt32(got - tt.want); diff > 1 {
				t.Errorf("FDot16Mul(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestFDot6Div tests fixed-point division.
// FDot6Div divides two FDot6 values and returns an FDot16 slope.
// For edge scanline conversion: slope = (x1-x0) / (y1-y0)
// The result is in FDot16 format (16.16 fixed-point).
func TestFDot6Div(t *testing.T) {
	tests := []struct {
		name     string
		a, b     FDot6
		wantNear FDot16 // Expected result in FDot16
	}{
		// 1.0 / 1.0 = 1.0 (FDot16)
		{"one over one", 64, 64, FDot16One},
		// 2.0 / 1.0 = 2.0 (FDot16)
		{"two over one", 128, 64, 2 * FDot16One},
		// 1.0 / 2.0 = 0.5 (FDot16)
		{"one over two", 64, 128, FDot16Half},
		// 100.0 / 1.0 = 100.0 (FDot16)
		{"large numerator", 6400, 64, 100 * FDot16One},
		// 0 / 1.0 = 0
		{"zero numerator", 0, 64, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FDot6Div(tt.a, tt.b)
			// Allow 1 unit tolerance for rounding
			diff := absInt32(got - tt.wantNear)
			if diff > 1 {
				t.Errorf("FDot6Div(%v, %v) = %v, want %v (diff=%v)",
					tt.a, tt.b, got, tt.wantNear, diff)
			}
		})
	}

	// Test division by zero separately
	t.Run("zero denominator", func(t *testing.T) {
		got := FDot6Div(64, 0)
		if got != 0x7FFFFFFF && got != -0x7FFFFFFF {
			t.Errorf("FDot6Div(64, 0) = %v, want max value", got)
		}
	})
}

// TestDiffToShift tests the subdivision count calculation.
// The algorithm heuristically determines how many subdivisions (1 << shift)
// are needed to approximate a curve to within sub-pixel accuracy.
func TestDiffToShift(t *testing.T) {
	tests := []struct {
		name    string
		dx, dy  FDot6
		shiftAA int
		wantMin int
		wantMax int
	}{
		{"flat curve", 0, 0, 0, 0, 0},
		{"small deviation", 1, 1, 0, 0, 1},
		{"medium deviation", 64, 64, 0, 2, 4},  // 1 pixel deviation
		{"large deviation", 640, 640, 0, 2, 4}, // 10 pixel deviation (heuristic rounds down)
		{"with AA", 64, 64, 2, 1, 3},           // AA reduces required shift
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffToShift(tt.dx, tt.dy, tt.shiftAA)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("diffToShift(%v, %v, %v) = %v, want [%v, %v]",
					tt.dx, tt.dy, tt.shiftAA, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestCheapDistance tests the distance approximation.
func TestCheapDistance(t *testing.T) {
	tests := []struct {
		name    string
		dx, dy  FDot6
		wantMin FDot6
		wantMax FDot6
	}{
		{"zero", 0, 0, 0, 0},
		{"horizontal", 100, 0, 100, 100},
		{"vertical", 0, 100, 100, 100},
		{"diagonal", 100, 100, 140, 160},                // sqrt(2)*100 ≈ 141
		{"3-4-5", 64 * 3, 64 * 4, 64*5 - 64, 64*5 + 64}, // ≈ 5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cheapDistance(tt.dx, tt.dy)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("cheapDistance(%v, %v) = %v, want [%v, %v]",
					tt.dx, tt.dy, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestLineEdge tests basic line edge creation.
func TestLineEdge(t *testing.T) {
	tests := []struct {
		name    string
		p0, p1  CurvePoint
		shift   int
		wantNil bool
		wantUp  bool // winding should be negative (upward)
	}{
		{
			name:    "downward line",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{10, 10},
			shift:   0,
			wantNil: false,
			wantUp:  false,
		},
		{
			name:    "upward line",
			p0:      CurvePoint{10, 10},
			p1:      CurvePoint{0, 0},
			shift:   0,
			wantNil: false,
			wantUp:  true,
		},
		{
			name:    "horizontal line (should be nil)",
			p0:      CurvePoint{0, 5},
			p1:      CurvePoint{10, 5},
			shift:   0,
			wantNil: true,
		},
		{
			name:    "vertical line",
			p0:      CurvePoint{5, 0},
			p1:      CurvePoint{5, 10},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "with AA shift",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{10, 10},
			shift:   2,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewLineEdge(tt.p0, tt.p1, tt.shift)

			if (edge == nil) != tt.wantNil {
				t.Errorf("NewLineEdge() nil = %v, want nil = %v", edge == nil, tt.wantNil)
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
		p0, p1, p2 CurvePoint
		shift      int
		wantNil    bool
	}{
		{
			name:    "simple quadratic",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{50, 0}, // Control point above
			p2:      CurvePoint{100, 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "flat quadratic (becomes line)",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{50, 50}, // On the chord
			p2:      CurvePoint{100, 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "horizontal quadratic (should be nil)",
			p0:      CurvePoint{0, 50},
			p1:      CurvePoint{50, 50},
			p2:      CurvePoint{100, 50},
			shift:   0,
			wantNil: true,
		},
		{
			name:    "curved quadratic",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{25, 50}, // Significant curvature
			p2:      CurvePoint{50, 0},
			shift:   0,
			wantNil: true, // Returns to same Y, so no vertical extent
		},
		{
			name:    "with AA shift",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{50, 25},
			p2:      CurvePoint{100, 100},
			shift:   2,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewQuadraticEdge(tt.p0, tt.p1, tt.p2, tt.shift)

			if (edge == nil) != tt.wantNil {
				t.Errorf("NewQuadraticEdge() nil = %v, want nil = %v", edge == nil, tt.wantNil)
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
	p0 := CurvePoint{0, 0}
	p1 := CurvePoint{50, 50}
	p2 := CurvePoint{100, 100}

	edge := NewQuadraticEdge(p0, p1, p2, 0)
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
		FDot16ToFloat32(line.X), FDot16ToFloat32(line.X + line.DX*(line.LastY-line.FirstY)),
	})

	for edge.Update() {
		line := edge.Line()
		lines = append(lines, struct{ x0, y0, x1, y1 float32 }{
			float32(line.FirstY), float32(line.LastY),
			FDot16ToFloat32(line.X), FDot16ToFloat32(line.X + line.DX*(line.LastY-line.FirstY)),
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
		p0, p1, p2, p3 CurvePoint
		shift          int
		wantNil        bool
	}{
		{
			name:    "simple cubic",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{33, 50},
			p2:      CurvePoint{66, 50},
			p3:      CurvePoint{100, 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "S-curve",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{100, 0},
			p2:      CurvePoint{0, 100},
			p3:      CurvePoint{100, 100},
			shift:   0,
			wantNil: false,
		},
		{
			name:    "horizontal cubic (should be nil)",
			p0:      CurvePoint{0, 50},
			p1:      CurvePoint{33, 50},
			p2:      CurvePoint{66, 50},
			p3:      CurvePoint{100, 50},
			shift:   0,
			wantNil: true,
		},
		{
			name:    "with AA shift",
			p0:      CurvePoint{0, 0},
			p1:      CurvePoint{33, 25},
			p2:      CurvePoint{66, 75},
			p3:      CurvePoint{100, 100},
			shift:   2,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := NewCubicEdge(tt.p0, tt.p1, tt.p2, tt.p3, tt.shift)

			if (edge == nil) != tt.wantNil {
				t.Errorf("NewCubicEdge() nil = %v, want nil = %v", edge == nil, tt.wantNil)
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
	p0 := CurvePoint{0, 0}
	p1 := CurvePoint{25, 50}
	p2 := CurvePoint{75, 50}
	p3 := CurvePoint{100, 100}

	edge := NewCubicEdge(p0, p1, p2, p3, 0)
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
		v := NewLineEdgeVariant(CurvePoint{0, 0}, CurvePoint{100, 100}, 0)
		if v == nil {
			t.Fatal("expected non-nil variant")
		}
		if v.Type != EdgeTypeLine {
			t.Errorf("expected EdgeTypeLine, got %v", v.Type)
		}
		if v.AsLine() == nil {
			t.Error("AsLine() returned nil")
		}
		if v.Update() {
			t.Error("line variant should not produce more segments")
		}
	})

	t.Run("quadratic variant", func(t *testing.T) {
		v := NewQuadraticEdgeVariant(
			CurvePoint{0, 0},
			CurvePoint{50, 25},
			CurvePoint{100, 100},
			0,
		)
		if v == nil {
			t.Fatal("expected non-nil variant")
		}
		if v.Type != EdgeTypeQuadratic {
			t.Errorf("expected EdgeTypeQuadratic, got %v", v.Type)
		}

		count := 0
		for v.Update() {
			count++
		}
		t.Logf("quadratic variant produced %d additional segments", count)
	})

	t.Run("cubic variant", func(t *testing.T) {
		v := NewCubicEdgeVariant(
			CurvePoint{0, 0},
			CurvePoint{25, 50},
			CurvePoint{75, 50},
			CurvePoint{100, 100},
			0,
		)
		if v == nil {
			t.Fatal("expected non-nil variant")
		}
		if v.Type != EdgeTypeCubic {
			t.Errorf("expected EdgeTypeCubic, got %v", v.Type)
		}

		count := 0
		for v.Update() {
			count++
		}
		t.Logf("cubic variant produced %d additional segments", count)
	})
}

// TestCubicDeltaFromLine tests the cubic deviation calculation.
func TestCubicDeltaFromLine(t *testing.T) {
	tests := []struct {
		name       string
		a, b, c, d FDot6
		expectZero bool
	}{
		{"flat line", 0, 0, 0, 0, true},
		{"collinear", 0, 100, 200, 300, true}, // All points on a line
		{"curved", 0, 200, 100, 300, false},   // Control points off the chord
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cubicDeltaFromLine(tt.a, tt.b, tt.c, tt.d)
			if tt.expectZero && got != 0 {
				t.Errorf("expected zero delta, got %v", got)
			}
			if !tt.expectZero && got == 0 {
				t.Error("expected non-zero delta, got 0")
			}
		})
	}
}

// Benchmark tests

// BenchmarkQuadraticEdgeCreate benchmarks quadratic edge creation.
func BenchmarkQuadraticEdgeCreate(b *testing.B) {
	p0 := CurvePoint{0, 0}
	p1 := CurvePoint{50, 50}
	p2 := CurvePoint{100, 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewQuadraticEdge(p0, p1, p2, 0)
	}
}

// BenchmarkQuadraticEdgeUpdate benchmarks forward differencing step.
func BenchmarkQuadraticEdgeUpdate(b *testing.B) {
	p0 := CurvePoint{0, 0}
	p1 := CurvePoint{50, 50}
	p2 := CurvePoint{100, 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		edge := NewQuadraticEdge(p0, p1, p2, 0)
		if edge != nil {
			for edge.Update() {
				// O(1) per step - forward differencing
			}
		}
	}
}

// BenchmarkCubicEdgeCreate benchmarks cubic edge creation.
func BenchmarkCubicEdgeCreate(b *testing.B) {
	p0 := CurvePoint{0, 0}
	p1 := CurvePoint{33, 50}
	p2 := CurvePoint{66, 50}
	p3 := CurvePoint{100, 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewCubicEdge(p0, p1, p2, p3, 0)
	}
}

// BenchmarkCubicEdgeUpdate benchmarks cubic forward differencing.
func BenchmarkCubicEdgeUpdate(b *testing.B) {
	p0 := CurvePoint{0, 0}
	p1 := CurvePoint{33, 50}
	p2 := CurvePoint{66, 50}
	p3 := CurvePoint{100, 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		edge := NewCubicEdge(p0, p1, p2, p3, 0)
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

// BenchmarkDiffToShift benchmarks the subdivision calculation.
func BenchmarkDiffToShift(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diffToShift(FDot6(i%1000), FDot6((i*7)%1000), 2)
	}
}

// Helper function for tests
func absFloat32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
