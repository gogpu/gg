package gogpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

func TestTessellateFill_NilInputs(t *testing.T) {
	path := gg.NewPath()
	path.Rectangle(10, 10, 100, 100)
	paint := gg.NewPaint()

	tests := []struct {
		name   string
		path   *gg.Path
		paint  *gg.Paint
		width  int
		height int
	}{
		{"nil path", nil, paint, 800, 600},
		{"nil paint", path, nil, 800, 600},
		{"zero width", path, paint, 0, 600},
		{"zero height", path, paint, 800, 0},
		{"negative width", path, paint, -1, 600},
		{"negative height", path, paint, 800, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TessellateFill(tt.path, tt.paint, tt.width, tt.height)
			if result != nil {
				t.Errorf("expected nil, got %v", result)
			}
		})
	}
}

func TestTessellateFill_EmptyPath(t *testing.T) {
	path := gg.NewPath()
	paint := gg.NewPaint()

	result := TessellateFill(path, paint, 800, 600)
	if result != nil {
		t.Errorf("expected nil for empty path, got %d vertices", len(result))
	}
}

func TestTessellateFill_TwoPoints(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 100)

	paint := gg.NewPaint()

	result := TessellateFill(path, paint, 800, 600)
	if result != nil {
		t.Errorf("expected nil for path with <3 points, got %d vertices", len(result))
	}
}

func TestTessellateFill_Triangle(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 0)
	path.LineTo(50, 100)
	path.Close()

	paint := gg.NewPaint()

	result := TessellateFill(path, paint, 800, 600)
	if result == nil {
		t.Fatal("expected vertices, got nil")
	}

	// Triangle: 3 points -> 1 triangle -> 3 vertices
	// But with Close, we have 4 points (including return to start)
	// So we get: 4 points -> 2 triangles -> 6 vertices
	if len(result) != 6 {
		t.Errorf("expected 6 vertices for closed triangle, got %d", len(result))
	}
}

func TestTessellateFill_Rectangle(t *testing.T) {
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)

	paint := gg.NewPaint()

	result := TessellateFill(path, paint, 800, 600)
	if result == nil {
		t.Fatal("expected vertices, got nil")
	}

	// Rectangle: 5 points (4 corners + close back to first)
	// Fan triangulation: 5 points -> 3 triangles -> 9 vertices
	if len(result) != 9 {
		t.Errorf("expected 9 vertices for rectangle, got %d", len(result))
	}
}

func TestTessellateFill_ColorFromPaint(t *testing.T) {
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)

	// Create paint with red color
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	result := TessellateFill(path, paint, 800, 600)
	if result == nil {
		t.Fatal("expected vertices, got nil")
	}

	// Check that vertices have red color (R=1, G=0, B=0, A=1)
	for i, v := range result {
		if v.R != 1.0 || v.G != 0.0 || v.B != 0.0 || v.A != 1.0 {
			t.Errorf("vertex %d: expected red (1,0,0,1), got (%v,%v,%v,%v)",
				i, v.R, v.G, v.B, v.A)
			break
		}
	}
}

func TestTessellateFill_NDCConversion(t *testing.T) {
	path := gg.NewPath()
	// Create path at known pixel coordinates
	path.MoveTo(0, 0)     // Top-left in pixels -> (-1, 1) in NDC
	path.LineTo(800, 0)   // Top-right -> (1, 1)
	path.LineTo(800, 600) // Bottom-right -> (1, -1)
	path.LineTo(0, 600)   // Bottom-left -> (-1, -1)
	path.Close()

	paint := gg.NewPaint()

	result := TessellateFill(path, paint, 800, 600)
	if result == nil {
		t.Fatal("expected vertices, got nil")
	}

	// Check first vertex (should be at NDC -1, 1 for pixel 0, 0)
	eps := float32(0.001)
	if !approxEqual32(result[0].X, -1.0, eps) || !approxEqual32(result[0].Y, 1.0, eps) {
		t.Errorf("vertex 0: expected NDC (-1, 1), got (%v, %v)", result[0].X, result[0].Y)
	}
}

func TestToNDC(t *testing.T) {
	tests := []struct {
		name          string
		p             point2D
		width, height int
		wantX, wantY  float32
	}{
		{"top-left", point2D{0, 0}, 800, 600, -1.0, 1.0},
		{"top-right", point2D{800, 0}, 800, 600, 1.0, 1.0},
		{"bottom-left", point2D{0, 600}, 800, 600, -1.0, -1.0},
		{"bottom-right", point2D{800, 600}, 800, 600, 1.0, -1.0},
		{"center", point2D{400, 300}, 800, 600, 0.0, 0.0},
	}

	eps := float32(0.001)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toNDC(tt.p, tt.width, tt.height)
			if !approxEqual32(result.X, tt.wantX, eps) || !approxEqual32(result.Y, tt.wantY, eps) {
				t.Errorf("got (%v, %v), want (%v, %v)", result.X, result.Y, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestFlattenPath_Empty(t *testing.T) {
	path := gg.NewPath()
	result := flattenPath(path)
	if result != nil {
		t.Errorf("expected nil for empty path, got %d points", len(result))
	}
}

func TestFlattenPath_LineOnly(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 100)
	path.LineTo(200, 0)

	result := flattenPath(path)
	if len(result) != 3 {
		t.Errorf("expected 3 points, got %d", len(result))
	}
}

func TestFlattenPath_QuadraticCurve(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.QuadraticTo(50, 100, 100, 0) // Control point at (50, 100)

	result := flattenPath(path)
	if len(result) < 3 {
		t.Errorf("expected at least 3 points for quadratic curve, got %d", len(result))
	}

	// First point should be start
	if result[0].X != 0 || result[0].Y != 0 {
		t.Errorf("first point should be (0, 0), got (%v, %v)", result[0].X, result[0].Y)
	}

	// Last point should be end
	last := result[len(result)-1]
	if last.X != 100 || last.Y != 0 {
		t.Errorf("last point should be (100, 0), got (%v, %v)", last.X, last.Y)
	}
}

func TestFlattenPath_CubicCurve(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.CubicTo(33, 100, 66, 100, 100, 0)

	result := flattenPath(path)
	if len(result) < 3 {
		t.Errorf("expected at least 3 points for cubic curve, got %d", len(result))
	}

	// First point should be start
	if result[0].X != 0 || result[0].Y != 0 {
		t.Errorf("first point should be (0, 0), got (%v, %v)", result[0].X, result[0].Y)
	}

	// Last point should be end
	last := result[len(result)-1]
	if last.X != 100 || last.Y != 0 {
		t.Errorf("last point should be (100, 0), got (%v, %v)", last.X, last.Y)
	}
}

func TestFlattenPath_Circle(t *testing.T) {
	path := gg.NewPath()
	path.Circle(100, 100, 50)

	result := flattenPath(path)
	// Circle is made of 4 cubic Bezier curves, should produce many points
	if len(result) < 10 {
		t.Errorf("expected many points for circle, got %d", len(result))
	}
}

func TestPointToLineDistance(t *testing.T) {
	tests := []struct {
		name string
		p    point2D
		a    point2D
		b    point2D
		want float32
	}{
		{"point on line", point2D{5, 5}, point2D{0, 0}, point2D{10, 10}, 0.0},
		{"point above horizontal line", point2D{5, 5}, point2D{0, 0}, point2D{10, 0}, 5.0},
		{"point right of vertical line", point2D{5, 5}, point2D{0, 0}, point2D{0, 10}, 5.0},
	}

	eps := float32(0.001)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pointToLineDistance(tt.p, tt.a, tt.b)
			if !approxEqual32(result, tt.want, eps) {
				t.Errorf("got %v, want %v", result, tt.want)
			}
		})
	}
}

func TestMidpoint(t *testing.T) {
	a := point2D{0, 0}
	b := point2D{10, 20}
	result := midpoint(a, b)
	if result.X != 5 || result.Y != 10 {
		t.Errorf("expected (5, 10), got (%v, %v)", result.X, result.Y)
	}
}

func TestFlattenQuadratic_Flat(t *testing.T) {
	// Very flat curve should produce just 2 points
	p0 := point2D{0, 0}
	p1 := point2D{50, 0.01} // Control point almost on the line
	p2 := point2D{100, 0}

	result := flattenQuadratic(p0, p1, p2, 0.25)
	if len(result) != 2 {
		t.Errorf("expected 2 points for flat curve, got %d", len(result))
	}
}

func TestFlattenCubic_Flat(t *testing.T) {
	// Very flat curve should produce just 2 points
	p0 := point2D{0, 0}
	p1 := point2D{33, 0.01}
	p2 := point2D{66, 0.01}
	p3 := point2D{100, 0}

	result := flattenCubic(p0, p1, p2, p3, 0.25)
	if len(result) != 2 {
		t.Errorf("expected 2 points for flat curve, got %d", len(result))
	}
}

// approxEqual32 checks if two float32 values are approximately equal.
func approxEqual32(a, b, eps float32) bool {
	return float32(math.Abs(float64(a-b))) <= eps
}
