package scene

import (
	"math"
	"testing"
)

func TestNewPath(t *testing.T) {
	path := NewPath()
	if path == nil {
		t.Fatal("NewPath() returned nil")
	}
	if !path.IsEmpty() {
		t.Error("new path should be empty")
	}
	if path.VerbCount() != 0 {
		t.Errorf("VerbCount() = %d, want 0", path.VerbCount())
	}
	if path.PointCount() != 0 {
		t.Errorf("PointCount() = %d, want 0", path.PointCount())
	}
}

func TestPathMoveTo(t *testing.T) {
	path := NewPath()
	path.MoveTo(10, 20)

	if path.VerbCount() != 1 {
		t.Errorf("VerbCount() = %d, want 1", path.VerbCount())
	}
	if path.Verbs()[0] != VerbMoveTo {
		t.Errorf("verb = %v, want VerbMoveTo", path.Verbs()[0])
	}
	if path.PointCount() != 2 {
		t.Errorf("PointCount() = %d, want 2", path.PointCount())
	}

	points := path.Points()
	if points[0] != 10 || points[1] != 20 {
		t.Errorf("points = %v, want [10, 20]", points)
	}

	bounds := path.Bounds()
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 10 || bounds.MaxY != 20 {
		t.Errorf("bounds = %+v, want (10,20)-(10,20)", bounds)
	}
}

func TestPathLineTo(t *testing.T) {
	path := NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 50)

	if path.VerbCount() != 2 {
		t.Errorf("VerbCount() = %d, want 2", path.VerbCount())
	}
	if path.Verbs()[1] != VerbLineTo {
		t.Errorf("verb[1] = %v, want VerbLineTo", path.Verbs()[1])
	}
	if path.PointCount() != 4 {
		t.Errorf("PointCount() = %d, want 4", path.PointCount())
	}

	bounds := path.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 50 {
		t.Errorf("bounds = %+v, want (0,0)-(100,50)", bounds)
	}
}

func TestPathQuadTo(t *testing.T) {
	path := NewPath()
	path.MoveTo(0, 0)
	path.QuadTo(50, 100, 100, 0)

	if path.VerbCount() != 2 {
		t.Errorf("VerbCount() = %d, want 2", path.VerbCount())
	}
	if path.Verbs()[1] != VerbQuadTo {
		t.Errorf("verb[1] = %v, want VerbQuadTo", path.Verbs()[1])
	}
	// MoveTo: 2, QuadTo: 4 = 6
	if path.PointCount() != 6 {
		t.Errorf("PointCount() = %d, want 6", path.PointCount())
	}
}

func TestPathCubicTo(t *testing.T) {
	path := NewPath()
	path.MoveTo(0, 0)
	path.CubicTo(25, 100, 75, 100, 100, 0)

	if path.VerbCount() != 2 {
		t.Errorf("VerbCount() = %d, want 2", path.VerbCount())
	}
	if path.Verbs()[1] != VerbCubicTo {
		t.Errorf("verb[1] = %v, want VerbCubicTo", path.Verbs()[1])
	}
	// MoveTo: 2, CubicTo: 6 = 8
	if path.PointCount() != 8 {
		t.Errorf("PointCount() = %d, want 8", path.PointCount())
	}
}

func TestPathClose(t *testing.T) {
	path := NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 0)
	path.LineTo(50, 100)
	path.Close()

	if path.VerbCount() != 4 {
		t.Errorf("VerbCount() = %d, want 4", path.VerbCount())
	}
	if path.Verbs()[3] != VerbClose {
		t.Errorf("verb[3] = %v, want VerbClose", path.Verbs()[3])
	}
}

func TestPathRectangle(t *testing.T) {
	path := NewPath()
	path.Rectangle(10, 20, 100, 50)

	// MoveTo + 3 LineTo + Close = 5 verbs
	if path.VerbCount() != 5 {
		t.Errorf("VerbCount() = %d, want 5", path.VerbCount())
	}

	bounds := path.Bounds()
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 110 || bounds.MaxY != 70 {
		t.Errorf("bounds = %+v, want (10,20)-(110,70)", bounds)
	}
}

func TestPathRoundedRectangle(t *testing.T) {
	tests := []struct {
		name     string
		x, y     float32
		w, h     float32
		r        float32
		minVerbs int
	}{
		{"no radius", 0, 0, 100, 50, 0, 5},        // Falls back to rectangle
		{"small radius", 0, 0, 100, 50, 10, 9},    // 4 lines + 4 curves + close
		{"clamped radius", 0, 0, 100, 50, 100, 9}, // Radius clamped to 25
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := NewPath()
			path.RoundedRectangle(tt.x, tt.y, tt.w, tt.h, tt.r)

			if path.VerbCount() < tt.minVerbs {
				t.Errorf("VerbCount() = %d, want >= %d", path.VerbCount(), tt.minVerbs)
			}

			// Check bounds include the rectangle
			bounds := path.Bounds()
			if bounds.MinX > tt.x || bounds.MinY > tt.y {
				t.Errorf("bounds min (%f,%f) should be <= (%f,%f)", bounds.MinX, bounds.MinY, tt.x, tt.y)
			}
			if bounds.MaxX < tt.x+tt.w || bounds.MaxY < tt.y+tt.h {
				t.Errorf("bounds max (%f,%f) should be >= (%f,%f)", bounds.MaxX, bounds.MaxY, tt.x+tt.w, tt.y+tt.h)
			}
		})
	}
}

func TestPathCircle(t *testing.T) {
	path := NewPath()
	path.Circle(50, 50, 25)

	// Circle uses 4 cubic beziers + close + move = at least 6 verbs
	if path.VerbCount() < 5 {
		t.Errorf("VerbCount() = %d, want >= 5", path.VerbCount())
	}

	bounds := path.Bounds()
	// Bounds should approximately contain the circle
	if bounds.MinX > 25 || bounds.MinY > 25 || bounds.MaxX < 75 || bounds.MaxY < 75 {
		t.Errorf("bounds = %+v, expected to contain (25,25)-(75,75)", bounds)
	}
}

func TestPathEllipse(t *testing.T) {
	path := NewPath()
	path.Ellipse(100, 100, 50, 30)

	if path.VerbCount() < 5 {
		t.Errorf("VerbCount() = %d, want >= 5", path.VerbCount())
	}

	bounds := path.Bounds()
	// Bounds should approximately contain the ellipse
	if bounds.MinX > 50 || bounds.MinY > 70 || bounds.MaxX < 150 || bounds.MaxY < 130 {
		t.Errorf("bounds = %+v, expected to contain (50,70)-(150,130)", bounds)
	}
}

func TestPathReset(t *testing.T) {
	path := NewPath()
	path.Rectangle(0, 0, 100, 100)

	// Capture capacity before reset
	verbCap := cap(path.verbs)
	pointCap := cap(path.points)

	if path.IsEmpty() {
		t.Error("path should not be empty before reset")
	}

	path.Reset()

	if !path.IsEmpty() {
		t.Error("path should be empty after reset")
	}
	if path.VerbCount() != 0 {
		t.Errorf("VerbCount() = %d, want 0", path.VerbCount())
	}

	// Verify capacity is preserved
	if cap(path.verbs) != verbCap {
		t.Errorf("verbs capacity changed: got %d, had %d", cap(path.verbs), verbCap)
	}
	if cap(path.points) != pointCap {
		t.Errorf("points capacity changed: got %d, had %d", cap(path.points), pointCap)
	}
}

func TestPathTransform(t *testing.T) {
	path := NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 0)
	path.LineTo(100, 100)
	path.Close()

	// Translate by (50, 50)
	transform := TranslateAffine(50, 50)
	transformed := path.Transform(transform)

	// Original should be unchanged
	if path.Points()[0] != 0 || path.Points()[1] != 0 {
		t.Error("original path should be unchanged")
	}

	// Transformed path should have translated points
	points := transformed.Points()
	if points[0] != 50 || points[1] != 50 {
		t.Errorf("transformed point = (%f,%f), want (50,50)", points[0], points[1])
	}
	if points[2] != 150 || points[3] != 50 {
		t.Errorf("transformed point = (%f,%f), want (150,50)", points[2], points[3])
	}

	// Bounds should be updated
	bounds := transformed.Bounds()
	if bounds.MinX != 50 || bounds.MinY != 50 {
		t.Errorf("transformed bounds min = (%f,%f), want (50,50)", bounds.MinX, bounds.MinY)
	}
}

func TestPathClone(t *testing.T) {
	path := NewPath()
	path.Rectangle(10, 20, 100, 50)

	clone := path.Clone()

	// Clone should have same content
	if clone.VerbCount() != path.VerbCount() {
		t.Errorf("clone VerbCount() = %d, want %d", clone.VerbCount(), path.VerbCount())
	}
	if clone.PointCount() != path.PointCount() {
		t.Errorf("clone PointCount() = %d, want %d", clone.PointCount(), path.PointCount())
	}

	// Clone should be independent
	clone.Reset()
	if path.IsEmpty() {
		t.Error("resetting clone should not affect original")
	}
}

func TestPathChaining(t *testing.T) {
	path := NewPath().
		MoveTo(0, 0).
		LineTo(100, 0).
		LineTo(100, 100).
		LineTo(0, 100).
		Close()

	if path.VerbCount() != 5 {
		t.Errorf("VerbCount() = %d, want 5", path.VerbCount())
	}
}

func TestPathVerbString(t *testing.T) {
	tests := []struct {
		verb PathVerb
		want string
	}{
		{VerbMoveTo, "MoveTo"},
		{VerbLineTo, "LineTo"},
		{VerbQuadTo, "QuadTo"},
		{VerbCubicTo, "CubicTo"},
		{VerbClose, "Close"},
		{PathVerb(255), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.verb.String(); got != tt.want {
				t.Errorf("PathVerb(%d).String() = %q, want %q", tt.verb, got, tt.want)
			}
		})
	}
}

func TestPathVerbPointCount(t *testing.T) {
	tests := []struct {
		verb PathVerb
		want int
	}{
		{VerbMoveTo, 2},
		{VerbLineTo, 2},
		{VerbQuadTo, 4},
		{VerbCubicTo, 6},
		{VerbClose, 0},
	}

	for _, tt := range tests {
		t.Run(tt.verb.String(), func(t *testing.T) {
			if got := tt.verb.PointCount(); got != tt.want {
				t.Errorf("%v.PointCount() = %d, want %d", tt.verb, got, tt.want)
			}
		})
	}
}

func TestPathArc(t *testing.T) {
	// Quarter arc (90 degrees)
	path := NewPath()
	path.Arc(100, 100, 50, 50, 0, math.Pi/2, true)

	if path.IsEmpty() {
		t.Error("arc path should not be empty")
	}

	// Should have MoveTo and at least one CubicTo
	hasMoveTo := false
	hasCubic := false
	for _, verb := range path.Verbs() {
		if verb == VerbMoveTo {
			hasMoveTo = true
		}
		if verb == VerbCubicTo {
			hasCubic = true
		}
	}
	if !hasMoveTo {
		t.Error("arc should have MoveTo")
	}
	if !hasCubic {
		t.Error("arc should have CubicTo")
	}
}

func TestPathPool(t *testing.T) {
	pool := NewPathPool()

	// Get path from empty pool (should create new)
	p1 := pool.Get()
	if p1 == nil {
		t.Fatal("pool.Get() returned nil")
	}

	// Add some content
	p1.Rectangle(0, 0, 100, 100)

	// Return to pool
	pool.Put(p1)

	// Get should return the same path, reset
	p2 := pool.Get()
	if p2.IsEmpty() == false {
		t.Error("path from pool should be reset")
	}

	// Put nil should not panic
	pool.Put(nil)
}

// Benchmarks

func BenchmarkPathRectangle(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		path := NewPath()
		path.Rectangle(0, 0, 100, 100)
	}
}

func BenchmarkPathCircle(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		path := NewPath()
		path.Circle(50, 50, 25)
	}
}

func BenchmarkPathTransform(b *testing.B) {
	path := NewPath()
	path.Rectangle(0, 0, 100, 100)
	transform := TranslateAffine(50, 50).Multiply(ScaleAffine(2, 2))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = path.Transform(transform)
	}
}

func BenchmarkPathClone(b *testing.B) {
	path := NewPath()
	path.Circle(50, 50, 25)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = path.Clone()
	}
}

func BenchmarkPathPoolGetPut(b *testing.B) {
	pool := NewPathPool()
	pool.Put(NewPath()) // Pre-warm

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := pool.Get()
		p.Rectangle(0, 0, 100, 100)
		pool.Put(p)
	}
}

// Tests for iter.Seq-based Elements() iterator (Go 1.25+)

func TestPathElements(t *testing.T) {
	// Create a path with all verb types
	p := NewPath()
	p.MoveTo(10, 20)
	p.LineTo(30, 40)
	p.QuadTo(50, 60, 70, 80)
	p.CubicTo(90, 100, 110, 120, 130, 140)
	p.Close()

	// Collect elements (5 expected: MoveTo, LineTo, QuadTo, CubicTo, Close)
	elements := make([]PathElement, 0, 5)
	for elem := range p.Elements() {
		elements = append(elements, elem)
	}

	// Verify element count
	if len(elements) != 5 {
		t.Fatalf("expected 5 elements, got %d", len(elements))
	}

	// Verify MoveTo
	if elements[0].Verb != VerbMoveTo {
		t.Errorf("element 0: expected MoveTo, got %v", elements[0].Verb)
	}
	if len(elements[0].Points) != 1 {
		t.Errorf("element 0: expected 1 point, got %d", len(elements[0].Points))
	}
	if elements[0].Points[0].X != 10 || elements[0].Points[0].Y != 20 {
		t.Errorf("element 0: expected (10, 20), got (%v, %v)", elements[0].Points[0].X, elements[0].Points[0].Y)
	}

	// Verify LineTo
	if elements[1].Verb != VerbLineTo {
		t.Errorf("element 1: expected LineTo, got %v", elements[1].Verb)
	}
	if len(elements[1].Points) != 1 {
		t.Errorf("element 1: expected 1 point, got %d", len(elements[1].Points))
	}

	// Verify QuadTo
	if elements[2].Verb != VerbQuadTo {
		t.Errorf("element 2: expected QuadTo, got %v", elements[2].Verb)
	}
	if len(elements[2].Points) != 2 {
		t.Errorf("element 2: expected 2 points, got %d", len(elements[2].Points))
	}
	if elements[2].Points[0].X != 50 || elements[2].Points[0].Y != 60 {
		t.Errorf("element 2 control: expected (50, 60), got (%v, %v)", elements[2].Points[0].X, elements[2].Points[0].Y)
	}
	if elements[2].Points[1].X != 70 || elements[2].Points[1].Y != 80 {
		t.Errorf("element 2 end: expected (70, 80), got (%v, %v)", elements[2].Points[1].X, elements[2].Points[1].Y)
	}

	// Verify CubicTo
	if elements[3].Verb != VerbCubicTo {
		t.Errorf("element 3: expected CubicTo, got %v", elements[3].Verb)
	}
	if len(elements[3].Points) != 3 {
		t.Errorf("element 3: expected 3 points, got %d", len(elements[3].Points))
	}

	// Verify Close
	if elements[4].Verb != VerbClose {
		t.Errorf("element 4: expected Close, got %v", elements[4].Verb)
	}
	if len(elements[4].Points) != 0 {
		t.Errorf("element 4: expected 0 points, got %d", len(elements[4].Points))
	}
}

func TestPathElementsWithCursor(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)
	p.LineTo(30, 40)
	p.LineTo(50, 60)

	// Pre-allocate for 3 elements (MoveTo, LineTo, LineTo)
	cursors := make([]Point, 0, 3)
	elements := make([]PathElement, 0, 3)

	for cursor, elem := range p.ElementsWithCursor() {
		cursors = append(cursors, cursor)
		elements = append(elements, elem)
	}

	if len(cursors) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(cursors))
	}

	// First cursor is (0, 0) - initial position
	if cursors[0].X != 0 || cursors[0].Y != 0 {
		t.Errorf("cursor 0: expected (0, 0), got (%v, %v)", cursors[0].X, cursors[0].Y)
	}

	// After MoveTo(10, 20), next cursor is (10, 20)
	if cursors[1].X != 10 || cursors[1].Y != 20 {
		t.Errorf("cursor 1: expected (10, 20), got (%v, %v)", cursors[1].X, cursors[1].Y)
	}

	// After LineTo(30, 40), next cursor is (30, 40)
	if cursors[2].X != 30 || cursors[2].Y != 40 {
		t.Errorf("cursor 2: expected (30, 40), got (%v, %v)", cursors[2].X, cursors[2].Y)
	}
}

func TestPathElementsEmpty(t *testing.T) {
	p := NewPath()

	count := 0
	for range p.Elements() {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 elements for empty path, got %d", count)
	}
}

func TestPathElementsEarlyBreak(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 10)
	p.LineTo(20, 20)
	p.LineTo(30, 30)
	p.LineTo(40, 40)

	// Break after 2 elements
	count := 0
	for range p.Elements() {
		count++
		if count >= 2 {
			break
		}
	}

	if count != 2 {
		t.Errorf("expected 2 elements after early break, got %d", count)
	}
}

func BenchmarkPathElements(b *testing.B) {
	p := NewPath()
	for i := 0; i < 100; i++ {
		p.MoveTo(float32(i), float32(i))
		p.LineTo(float32(i+10), float32(i+10))
		p.QuadTo(float32(i+20), float32(i+20), float32(i+30), float32(i+30))
		p.CubicTo(float32(i+40), float32(i+40), float32(i+50), float32(i+50), float32(i+60), float32(i+60))
		p.Close()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		count := 0
		for range p.Elements() {
			count++
		}
		if count != 500 {
			b.Fatalf("unexpected count: %d", count)
		}
	}
}

func BenchmarkPathElementsWithCursor(b *testing.B) {
	p := NewPath()
	for i := 0; i < 100; i++ {
		p.MoveTo(float32(i), float32(i))
		p.LineTo(float32(i+10), float32(i+10))
		p.Close()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		count := 0
		for range p.ElementsWithCursor() {
			count++
		}
		if count != 300 {
			b.Fatalf("unexpected count: %d", count)
		}
	}
}
