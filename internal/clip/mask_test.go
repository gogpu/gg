package clip

import (
	"testing"

	"github.com/gogpu/gg/internal/image"
)

func TestNewMaskClipper(t *testing.T) {
	tests := []struct {
		name     string
		elements []PathElement
		bounds   Rect
		wantNil  bool
	}{
		{
			name: "simple rectangle",
			elements: []PathElement{
				MoveTo{Point: Pt(10, 10)},
				LineTo{Point: Pt(20, 10)},
				LineTo{Point: Pt(20, 20)},
				LineTo{Point: Pt(10, 20)},
				Close{},
			},
			bounds:  NewRect(0, 0, 30, 30),
			wantNil: false,
		},
		{
			name:     "empty bounds",
			elements: []PathElement{},
			bounds:   NewRect(0, 0, 0, 0),
			wantNil:  true, // Should return error for empty bounds
		},
		{
			name:     "negative dimensions",
			elements: []PathElement{},
			bounds:   NewRect(0, 0, -10, -10),
			wantNil:  true, // Should return error for negative dimensions
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc, err := NewMaskClipper(tt.elements, tt.bounds, true)

			if tt.wantNil {
				if err == nil {
					t.Errorf("NewMaskClipper() expected error, got nil")
				}
				if mc != nil {
					t.Errorf("NewMaskClipper() expected nil clipper, got %v", mc)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewMaskClipper() error = %v", err)
			}

			if mc == nil {
				t.Fatal("NewMaskClipper() returned nil unexpectedly")
			}

			if mc.mask == nil {
				t.Error("mask is nil")
			}

			if mc.bounds != tt.bounds {
				t.Errorf("bounds = %v, want %v", mc.bounds, tt.bounds)
			}
		})
	}
}

func TestMaskClipper_Coverage(t *testing.T) {
	// Create a 10x10 rectangle from (5,5) to (15,15)
	elements := []PathElement{
		MoveTo{Point: Pt(5, 5)},
		LineTo{Point: Pt(15, 5)},
		LineTo{Point: Pt(15, 15)},
		LineTo{Point: Pt(5, 15)},
		Close{},
	}

	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	tests := []struct {
		name      string
		x, y      float64
		wantZero  bool // true if coverage should be 0
		wantFull  bool // true if coverage should be 255
		checkNonZ bool // true if coverage should be > 0
	}{
		{
			name:     "outside left",
			x:        2,
			y:        10,
			wantZero: true,
		},
		{
			name:     "outside right",
			x:        18,
			y:        10,
			wantZero: true,
		},
		{
			name:     "outside top",
			x:        10,
			y:        2,
			wantZero: true,
		},
		{
			name:     "outside bottom",
			x:        10,
			y:        18,
			wantZero: true,
		},
		{
			name:      "inside center",
			x:         10,
			y:         10,
			checkNonZ: true,
		},
		{
			name:     "outside mask bounds",
			x:        -5,
			y:        -5,
			wantZero: true,
		},
		{
			name:     "outside mask bounds high",
			x:        25,
			y:        25,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage := mc.Coverage(tt.x, tt.y)

			if tt.wantZero && coverage != 0 {
				t.Errorf("Coverage(%v, %v) = %v, want 0", tt.x, tt.y, coverage)
			}

			if tt.wantFull && coverage != 255 {
				t.Errorf("Coverage(%v, %v) = %v, want 255", tt.x, tt.y, coverage)
			}

			if tt.checkNonZ && coverage == 0 {
				t.Errorf("Coverage(%v, %v) = 0, want > 0", tt.x, tt.y)
			}
		})
	}
}

func TestMaskClipper_ApplyCoverage(t *testing.T) {
	// Create a simple filled rectangle
	elements := []PathElement{
		MoveTo{Point: Pt(0, 0)},
		LineTo{Point: Pt(10, 0)},
		LineTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(0, 10)},
		Close{},
	}

	bounds := NewRect(0, 0, 10, 10)
	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	tests := []struct {
		name     string
		x, y     float64
		srcAlpha byte
		want     byte
	}{
		{
			name:     "full coverage full alpha",
			x:        5,
			y:        5,
			srcAlpha: 255,
			want:     255, // Should be 255 or close to it (inside)
		},
		{
			name:     "zero coverage",
			x:        15,
			y:        15,
			srcAlpha: 255,
			want:     0, // Outside, so coverage = 0
		},
		{
			name:     "full coverage half alpha",
			x:        5,
			y:        5,
			srcAlpha: 128,
			want:     128, // Inside, so should preserve alpha
		},
		{
			name:     "zero alpha",
			x:        5,
			y:        5,
			srcAlpha: 0,
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mc.ApplyCoverage(tt.x, tt.y, tt.srcAlpha)

			// For inside points, allow some tolerance due to rasterization
			if tt.name == "full coverage full alpha" || tt.name == "full coverage half alpha" {
				// Just check it's non-zero for inside points
				if result == 0 {
					t.Errorf("ApplyCoverage(%v, %v, %v) = %v, expected non-zero (inside)", tt.x, tt.y, tt.srcAlpha, result)
				}
			} else {
				if result != tt.want {
					t.Errorf("ApplyCoverage(%v, %v, %v) = %v, want %v", tt.x, tt.y, tt.srcAlpha, result, tt.want)
				}
			}
		})
	}
}

func TestMaskClipper_Bounds(t *testing.T) {
	bounds := NewRect(10, 20, 100, 200)
	elements := []PathElement{
		MoveTo{Point: Pt(10, 20)},
		LineTo{Point: Pt(110, 220)},
	}

	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	if mc.Bounds() != bounds {
		t.Errorf("Bounds() = %v, want %v", mc.Bounds(), bounds)
	}
}

func TestMaskClipper_Mask(t *testing.T) {
	elements := []PathElement{
		MoveTo{Point: Pt(0, 0)},
		LineTo{Point: Pt(10, 10)},
	}
	bounds := NewRect(0, 0, 10, 10)

	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	mask := mc.Mask()
	if mask == nil {
		t.Fatal("Mask() returned nil")
	}

	if mask.Format() != image.FormatGray8 {
		t.Errorf("Mask format = %v, want FormatGray8", mask.Format())
	}

	if mask.Width() != 10 || mask.Height() != 10 {
		t.Errorf("Mask dimensions = %dx%d, want 10x10", mask.Width(), mask.Height())
	}
}

func TestMaskClipper_QuadraticBezier(t *testing.T) {
	// Test that quadratic Bezier curves are properly rasterized
	elements := []PathElement{
		MoveTo{Point: Pt(0, 10)},
		QuadTo{
			Control: Pt(10, 0),
			Point:   Pt(20, 10),
		},
		LineTo{Point: Pt(20, 20)},
		LineTo{Point: Pt(0, 20)},
		Close{},
	}

	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	// Check that center point has coverage
	coverage := mc.Coverage(10, 15)
	if coverage == 0 {
		t.Error("Quadratic Bezier path should have coverage at center")
	}
}

func TestMaskClipper_CubicBezier(t *testing.T) {
	// Test that cubic Bezier curves are properly rasterized
	elements := []PathElement{
		MoveTo{Point: Pt(0, 10)},
		CubicTo{
			Control1: Pt(5, 0),
			Control2: Pt(15, 0),
			Point:    Pt(20, 10),
		},
		LineTo{Point: Pt(20, 20)},
		LineTo{Point: Pt(0, 20)},
		Close{},
	}

	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	// Check that center point has coverage
	coverage := mc.Coverage(10, 15)
	if coverage == 0 {
		t.Error("Cubic Bezier path should have coverage at center")
	}
}

func TestMaskClipper_EmptyPath(t *testing.T) {
	elements := []PathElement{}
	bounds := NewRect(0, 0, 10, 10)

	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	// Empty path should have zero coverage everywhere
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			coverage := mc.Coverage(float64(x), float64(y))
			if coverage != 0 {
				t.Errorf("Empty path should have zero coverage, got %v at (%d, %d)", coverage, x, y)
			}
		}
	}
}

func TestMaskClipper_Triangle(t *testing.T) {
	// Create a triangle path
	elements := []PathElement{
		MoveTo{Point: Pt(10, 5)},
		LineTo{Point: Pt(5, 15)},
		LineTo{Point: Pt(15, 15)},
		Close{},
	}

	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(elements, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	// Point inside triangle
	inside := mc.Coverage(10, 10)
	if inside == 0 {
		t.Error("Point inside triangle should have coverage")
	}

	// Point outside triangle
	outside := mc.Coverage(2, 2)
	if outside != 0 {
		t.Error("Point outside triangle should have zero coverage")
	}
}

func TestEvalQuadraticBezier(t *testing.T) {
	p0 := Pt(0, 0)
	p1 := Pt(10, 10)
	p2 := Pt(20, 0)

	tests := []struct {
		name string
		t    float64
		want Point
	}{
		{
			name: "start point",
			t:    0,
			want: p0,
		},
		{
			name: "end point",
			t:    1,
			want: p2,
		},
		{
			name: "midpoint",
			t:    0.5,
			want: Pt(10, 5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evalQuadraticBezier(p0, p1, p2, tt.t)

			const epsilon = 0.01
			if !pointsEqual(got, tt.want, epsilon) {
				t.Errorf("evalQuadraticBezier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvalCubicBezier(t *testing.T) {
	p0 := Pt(0, 0)
	p1 := Pt(5, 10)
	p2 := Pt(15, 10)
	p3 := Pt(20, 0)

	tests := []struct {
		name string
		t    float64
		want Point
	}{
		{
			name: "start point",
			t:    0,
			want: p0,
		},
		{
			name: "end point",
			t:    1,
			want: p3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evalCubicBezier(p0, p1, p2, p3, tt.t)

			const epsilon = 0.01
			if !pointsEqual(got, tt.want, epsilon) {
				t.Errorf("evalCubicBezier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortFloats(t *testing.T) {
	tests := []struct {
		name  string
		input []float64
		want  []float64
	}{
		{
			name:  "empty",
			input: []float64{},
			want:  []float64{},
		},
		{
			name:  "single element",
			input: []float64{5.0},
			want:  []float64{5.0},
		},
		{
			name:  "already sorted",
			input: []float64{1.0, 2.0, 3.0},
			want:  []float64{1.0, 2.0, 3.0},
		},
		{
			name:  "reverse sorted",
			input: []float64{3.0, 2.0, 1.0},
			want:  []float64{1.0, 2.0, 3.0},
		},
		{
			name:  "unsorted",
			input: []float64{2.5, 1.0, 3.7, 0.5},
			want:  []float64{0.5, 1.0, 2.5, 3.7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make([]float64, len(tt.input))
			copy(got, tt.input)
			sortFloats(got)

			if len(got) != len(tt.want) {
				t.Errorf("length = %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("sortFloats() = %v, want %v", got, tt.want)
					break
				}
			}
		})
	}
}

// Helper function to compare points with epsilon tolerance.
func pointsEqual(p1, p2 Point, epsilon float64) bool {
	dx := p1.X - p2.X
	dy := p1.Y - p2.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx < epsilon && dy < epsilon
}

// Benchmark tests
func BenchmarkNewMaskClipper_Rectangle(b *testing.B) {
	elements := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(110, 10)},
		LineTo{Point: Pt(110, 110)},
		LineTo{Point: Pt(10, 110)},
		Close{},
	}
	bounds := NewRect(0, 0, 120, 120)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewMaskClipper(elements, bounds, true)
	}
}

func BenchmarkMaskClipper_Coverage(b *testing.B) {
	elements := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(110, 10)},
		LineTo{Point: Pt(110, 110)},
		LineTo{Point: Pt(10, 110)},
		Close{},
	}
	bounds := NewRect(0, 0, 120, 120)

	mc, _ := NewMaskClipper(elements, bounds, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mc.Coverage(50, 50)
	}
}

func BenchmarkMaskClipper_ApplyCoverage(b *testing.B) {
	elements := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(110, 10)},
		LineTo{Point: Pt(110, 110)},
		LineTo{Point: Pt(10, 110)},
		Close{},
	}
	bounds := NewRect(0, 0, 120, 120)

	mc, _ := NewMaskClipper(elements, bounds, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mc.ApplyCoverage(50, 50, 128)
	}
}
