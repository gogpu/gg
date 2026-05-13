package clip

import (
	"testing"

	"github.com/gogpu/gg/internal/image"
)

// soaPath is a test helper for building SOA (verb+coords) paths for clip tests.
type soaPath struct {
	verbs  []PathVerb
	coords []float64
}

func newSOAPath() *soaPath { return &soaPath{} }

func (p *soaPath) moveTo(x, y float64) *soaPath {
	p.verbs = append(p.verbs, VerbMoveTo)
	p.coords = append(p.coords, x, y)
	return p
}

func (p *soaPath) lineTo(x, y float64) *soaPath {
	p.verbs = append(p.verbs, VerbLineTo)
	p.coords = append(p.coords, x, y)
	return p
}

func (p *soaPath) quadTo(cx, cy, x, y float64) *soaPath {
	p.verbs = append(p.verbs, VerbQuadTo)
	p.coords = append(p.coords, cx, cy, x, y)
	return p
}

func (p *soaPath) cubicTo(c1x, c1y, c2x, c2y, x, y float64) *soaPath {
	p.verbs = append(p.verbs, VerbCubicTo)
	p.coords = append(p.coords, c1x, c1y, c2x, c2y, x, y)
	return p
}

func (p *soaPath) close() *soaPath {
	p.verbs = append(p.verbs, VerbClose)
	return p
}

func TestNewMaskClipper(t *testing.T) {
	rect := newSOAPath().moveTo(10, 10).lineTo(20, 10).lineTo(20, 20).lineTo(10, 20).close()

	tests := []struct {
		name    string
		verbs   []PathVerb
		coords  []float64
		bounds  Rect
		wantNil bool
	}{
		{"simple rectangle", rect.verbs, rect.coords, NewRect(0, 0, 30, 30), false},
		{"empty bounds", nil, nil, NewRect(0, 0, 0, 0), true},
		{"negative dimensions", nil, nil, NewRect(0, 0, -10, -10), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc, err := NewMaskClipper(tt.verbs, tt.coords, tt.bounds, true)
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
				return
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
	p := newSOAPath().moveTo(5, 5).lineTo(15, 5).lineTo(15, 15).lineTo(5, 15).close()
	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	tests := []struct {
		name      string
		x, y      float64
		wantZero  bool
		wantFull  bool
		checkNonZ bool
	}{
		{"outside left", 2, 10, true, false, false},
		{"outside right", 18, 10, true, false, false},
		{"outside top", 10, 2, true, false, false},
		{"outside bottom", 10, 18, true, false, false},
		{"inside center", 10, 10, false, false, true},
		{"outside mask bounds", -5, -5, true, false, false},
		{"outside mask bounds high", 25, 25, true, false, false},
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
	p := newSOAPath().moveTo(0, 0).lineTo(10, 0).lineTo(10, 10).lineTo(0, 10).close()
	bounds := NewRect(0, 0, 10, 10)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}

	tests := []struct {
		name     string
		x, y     float64
		srcAlpha byte
		want     byte
	}{
		{"full coverage full alpha", 5, 5, 255, 255},
		{"zero coverage", 15, 15, 255, 0},
		{"full coverage half alpha", 5, 5, 128, 128},
		{"zero alpha", 5, 5, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mc.ApplyCoverage(tt.x, tt.y, tt.srcAlpha)
			if tt.name == "full coverage full alpha" || tt.name == "full coverage half alpha" {
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
	p := newSOAPath().moveTo(10, 20).lineTo(110, 220)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	if mc.Bounds() != bounds {
		t.Errorf("Bounds() = %v, want %v", mc.Bounds(), bounds)
	}
}

func TestMaskClipper_Mask(t *testing.T) {
	p := newSOAPath().moveTo(0, 0).lineTo(10, 10)
	bounds := NewRect(0, 0, 10, 10)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
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
	p := newSOAPath().moveTo(0, 10).quadTo(10, 0, 20, 10).lineTo(20, 20).lineTo(0, 20).close()
	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	if mc.Coverage(10, 15) == 0 {
		t.Error("Quadratic Bezier path should have coverage at center")
	}
}

func TestMaskClipper_CubicBezier(t *testing.T) {
	p := newSOAPath().moveTo(0, 10).cubicTo(5, 0, 15, 0, 20, 10).lineTo(20, 20).lineTo(0, 20).close()
	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	if mc.Coverage(10, 15) == 0 {
		t.Error("Cubic Bezier path should have coverage at center")
	}
}

func TestMaskClipper_EmptyPath(t *testing.T) {
	bounds := NewRect(0, 0, 10, 10)
	mc, err := NewMaskClipper(nil, nil, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if mc.Coverage(float64(x), float64(y)) != 0 {
				t.Errorf("Empty path should have zero coverage at (%d, %d)", x, y)
			}
		}
	}
}

func TestMaskClipper_Triangle(t *testing.T) {
	p := newSOAPath().moveTo(10, 5).lineTo(5, 15).lineTo(15, 15).close()
	bounds := NewRect(0, 0, 20, 20)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	if mc.Coverage(10, 10) == 0 {
		t.Error("Point inside triangle should have coverage")
	}
	if mc.Coverage(2, 2) != 0 {
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
		{"start point", 0, p0},
		{"end point", 1, p2},
		{"midpoint", 0.5, Pt(10, 5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evalQuadraticBezier(p0, p1, p2, tt.t)
			if !pointsEqual(got, tt.want, 0.01) {
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
		{"start point", 0, p0},
		{"end point", 1, p3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evalCubicBezier(p0, p1, p2, p3, tt.t)
			if !pointsEqual(got, tt.want, 0.01) {
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
		{"empty", []float64{}, []float64{}},
		{"single", []float64{5.0}, []float64{5.0}},
		{"sorted", []float64{1.0, 2.0, 3.0}, []float64{1.0, 2.0, 3.0}},
		{"reverse", []float64{3.0, 2.0, 1.0}, []float64{1.0, 2.0, 3.0}},
		{"unsorted", []float64{2.5, 1.0, 3.7, 0.5}, []float64{0.5, 1.0, 2.5, 3.7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make([]float64, len(tt.input))
			copy(got, tt.input)
			sortFloats(got)
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("sortFloats() = %v, want %v", got, tt.want)
					break
				}
			}
		})
	}
}

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

func TestMaskClipper_NonAA(t *testing.T) {
	p := newSOAPath().moveTo(5, 5).lineTo(45, 5).lineTo(45, 45).lineTo(5, 45).close()
	bounds := NewRect(0, 0, 50, 50)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, false)
	if err != nil {
		t.Fatalf("NewMaskClipper(non-AA) error = %v", err)
	}
	if mc.Coverage(25, 25) == 0 {
		t.Error("non-AA: inside point should have coverage")
	}
	if mc.Coverage(2, 2) != 0 {
		t.Errorf("non-AA: outside point coverage should be 0")
	}
}

func TestMaskClipper_LargeTriangle(t *testing.T) {
	p := newSOAPath().moveTo(50, 10).lineTo(10, 90).lineTo(90, 90).close()
	bounds := NewRect(0, 0, 100, 100)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	if mc.Coverage(50, 60) == 0 {
		t.Error("center of large triangle should have coverage")
	}
}

func TestMaskClipper_ApplyCoverageOutside(t *testing.T) {
	p := newSOAPath().moveTo(10, 10).lineTo(90, 10).lineTo(90, 90).lineTo(10, 90).close()
	bounds := NewRect(0, 0, 100, 100)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	if mc.ApplyCoverage(2, 2, 200) != 0 {
		t.Error("ApplyCoverage(outside) should be 0")
	}
	if mc.ApplyCoverage(50, 50, 255) == 0 {
		t.Error("ApplyCoverage(inside, 255) should be > 0")
	}
}

func TestMaskClipper_HorizontalEdge(t *testing.T) {
	p := newSOAPath().moveTo(0, 10).lineTo(20, 10).lineTo(20, 20).lineTo(0, 20).close()
	bounds := NewRect(0, 0, 20, 30)
	mc, err := NewMaskClipper(p.verbs, p.coords, bounds, true)
	if err != nil {
		t.Fatalf("NewMaskClipper() error = %v", err)
	}
	if mc.Coverage(10, 15) == 0 {
		t.Error("should have coverage in filled area despite horizontal edge")
	}
}

func BenchmarkNewMaskClipper_Rectangle(b *testing.B) {
	p := newSOAPath().moveTo(10, 10).lineTo(110, 10).lineTo(110, 110).lineTo(10, 110).close()
	bounds := NewRect(0, 0, 120, 120)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewMaskClipper(p.verbs, p.coords, bounds, true)
	}
}

func BenchmarkMaskClipper_Coverage(b *testing.B) {
	p := newSOAPath().moveTo(10, 10).lineTo(110, 10).lineTo(110, 110).lineTo(10, 110).close()
	bounds := NewRect(0, 0, 120, 120)
	mc, _ := NewMaskClipper(p.verbs, p.coords, bounds, true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mc.Coverage(50, 50)
	}
}

func BenchmarkMaskClipper_ApplyCoverage(b *testing.B) {
	p := newSOAPath().moveTo(10, 10).lineTo(110, 10).lineTo(110, 110).lineTo(10, 110).close()
	bounds := NewRect(0, 0, 120, 120)
	mc, _ := NewMaskClipper(p.verbs, p.coords, bounds, true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mc.ApplyCoverage(50, 50, 128)
	}
}
