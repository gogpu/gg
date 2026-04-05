package gg

import (
	"math"
	"testing"
)

const pathEpsilon = 1e-10

// --- NewPath Tests ---

func TestNewPath(t *testing.T) {
	p := NewPath()
	if p == nil {
		t.Fatal("NewPath() returned nil")
	}
	if len(p.verbs) != 0 {
		t.Errorf("NewPath() has %d verbs, want 0", len(p.verbs))
	}
	if len(p.coords) != 0 {
		t.Errorf("NewPath() has %d coords, want 0", len(p.coords))
	}
	if p.isEmpty() != true {
		t.Error("NewPath() isEmpty should be true")
	}
	if p.HasCurrentPoint() {
		t.Error("NewPath() HasCurrentPoint should be false")
	}
}

// --- MoveTo Tests ---

func TestPath_MoveTo(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)

	if len(p.verbs) != 1 {
		t.Fatalf("After MoveTo: %d verbs, want 1", len(p.verbs))
	}
	if p.verbs[0] != VerbMoveTo {
		t.Fatalf("First verb is %v, want VerbMoveTo", p.verbs[0])
	}
	if p.coords[0] != 10 || p.coords[1] != 20 {
		t.Errorf("MoveTo coords = (%v, %v), want (10, 20)", p.coords[0], p.coords[1])
	}

	if p.current.X != 10 || p.current.Y != 20 {
		t.Errorf("current = %v, want (10, 20)", p.current)
	}
	if p.start.X != 10 || p.start.Y != 20 {
		t.Errorf("start = %v, want (10, 20)", p.start)
	}
	if !p.HasCurrentPoint() {
		t.Error("After MoveTo, HasCurrentPoint should be true")
	}
}

func TestPath_MoveTo_Multiple(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)
	p.MoveTo(30, 40)

	if len(p.verbs) != 2 {
		t.Fatalf("After two MoveTo: %d verbs, want 2", len(p.verbs))
	}

	// Current point should be the last MoveTo
	if p.current.X != 30 || p.current.Y != 40 {
		t.Errorf("current = %v, want (30, 40)", p.current)
	}
	if p.start.X != 30 || p.start.Y != 40 {
		t.Errorf("start = %v, want (30, 40)", p.start)
	}
}

// --- LineTo Tests ---

func TestPath_LineTo(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 20)

	if len(p.verbs) != 2 {
		t.Fatalf("After MoveTo+LineTo: %d verbs, want 2", len(p.verbs))
	}
	if p.verbs[1] != VerbLineTo {
		t.Fatalf("Second verb is %v, want VerbLineTo", p.verbs[1])
	}
	// LineTo coords start at offset 2 (after MoveTo's 2 coords)
	if p.coords[2] != 10 || p.coords[3] != 20 {
		t.Errorf("LineTo coords = (%v, %v), want (10, 20)", p.coords[2], p.coords[3])
	}
	if p.current.X != 10 || p.current.Y != 20 {
		t.Errorf("current = %v, want (10, 20)", p.current)
	}
}

func TestPath_LineTo_Chain(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.LineTo(0, 10)

	if len(p.verbs) != 4 {
		t.Fatalf("After MoveTo+3*LineTo: %d verbs, want 4", len(p.verbs))
	}
	if p.current.X != 0 || p.current.Y != 10 {
		t.Errorf("current = %v, want (0, 10)", p.current)
	}
}

// --- QuadraticTo Tests ---

func TestPath_QuadraticTo(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(5, 10, 10, 0)

	if len(p.verbs) != 2 {
		t.Fatalf("After MoveTo+QuadraticTo: %d verbs, want 2", len(p.verbs))
	}
	if p.verbs[1] != VerbQuadTo {
		t.Fatalf("Second verb is %v, want VerbQuadTo", p.verbs[1])
	}
	// QuadTo coords at offset 2: cx=5, cy=10, x=10, y=0
	if p.coords[2] != 5 || p.coords[3] != 10 {
		t.Errorf("QuadTo control = (%v, %v), want (5, 10)", p.coords[2], p.coords[3])
	}
	if p.coords[4] != 10 || p.coords[5] != 0 {
		t.Errorf("QuadTo point = (%v, %v), want (10, 0)", p.coords[4], p.coords[5])
	}
	if p.current.X != 10 || p.current.Y != 0 {
		t.Errorf("current = %v, want (10, 0)", p.current)
	}
}

// --- CubicTo Tests ---

func TestPath_CubicTo(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.CubicTo(3, 10, 7, 10, 10, 0)

	if len(p.verbs) != 2 {
		t.Fatalf("After MoveTo+CubicTo: %d verbs, want 2", len(p.verbs))
	}
	if p.verbs[1] != VerbCubicTo {
		t.Fatalf("Second verb is %v, want VerbCubicTo", p.verbs[1])
	}
	// CubicTo coords at offset 2: c1x=3,c1y=10, c2x=7,c2y=10, x=10,y=0
	if p.coords[2] != 3 || p.coords[3] != 10 {
		t.Errorf("CubicTo control1 = (%v, %v), want (3, 10)", p.coords[2], p.coords[3])
	}
	if p.coords[4] != 7 || p.coords[5] != 10 {
		t.Errorf("CubicTo control2 = (%v, %v), want (7, 10)", p.coords[4], p.coords[5])
	}
	if p.coords[6] != 10 || p.coords[7] != 0 {
		t.Errorf("CubicTo point = (%v, %v), want (10, 0)", p.coords[6], p.coords[7])
	}
	if p.current.X != 10 || p.current.Y != 0 {
		t.Errorf("current = %v, want (10, 0)", p.current)
	}
}

// --- Close Tests ---

func TestPath_Close(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.Close()

	if len(p.verbs) != 4 {
		t.Fatalf("After triangle+Close: %d verbs, want 4", len(p.verbs))
	}
	if p.verbs[3] != VerbClose {
		t.Fatalf("Last verb is %v, want VerbClose", p.verbs[3])
	}

	// After close, current should return to start
	if p.current.X != 0 || p.current.Y != 0 {
		t.Errorf("After Close, current = %v, want (0, 0)", p.current)
	}
}

func TestPath_Close_ResetsToStart(t *testing.T) {
	p := NewPath()
	p.MoveTo(5, 10)
	p.LineTo(20, 30)
	p.Close()

	if p.current.X != 5 || p.current.Y != 10 {
		t.Errorf("After Close, current = %v, want start (5, 10)", p.current)
	}
}

func TestPath_Close_ThenContinue(t *testing.T) {
	p := NewPath()
	// First subpath
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.Close()

	// Second subpath
	p.MoveTo(20, 20)
	p.LineTo(30, 20)

	if len(p.verbs) != 5 {
		t.Fatalf("Two subpaths: %d verbs, want 5", len(p.verbs))
	}
	if p.current.X != 30 || p.current.Y != 20 {
		t.Errorf("current = %v, want (30, 20)", p.current)
	}
}

// --- Clear Tests ---

func TestPath_Clear(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)
	p.LineTo(30, 40)
	p.LineTo(50, 60)
	p.Close()

	p.Clear()

	if len(p.verbs) != 0 {
		t.Errorf("After Clear: %d verbs, want 0", len(p.verbs))
	}
	if len(p.coords) != 0 {
		t.Errorf("After Clear: %d coords, want 0", len(p.coords))
	}
	if p.current.X != 0 || p.current.Y != 0 {
		t.Errorf("After Clear, current = %v, want (0, 0)", p.current)
	}
	if p.start.X != 0 || p.start.Y != 0 {
		t.Errorf("After Clear, start = %v, want (0, 0)", p.start)
	}
	if p.isEmpty() != true {
		t.Error("After Clear, isEmpty should be true")
	}
}

// --- Reset Tests ---

func TestPath_Reset(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)
	p.LineTo(30, 40)

	verbCap := cap(p.verbs)
	coordCap := cap(p.coords)

	p.Reset()

	if len(p.verbs) != 0 {
		t.Errorf("After Reset: %d verbs, want 0", len(p.verbs))
	}
	if cap(p.verbs) != verbCap {
		t.Errorf("After Reset: verb capacity changed from %d to %d", verbCap, cap(p.verbs))
	}
	if cap(p.coords) != coordCap {
		t.Errorf("After Reset: coord capacity changed from %d to %d", coordCap, cap(p.coords))
	}
}

// --- Elements Tests (backward compatibility) ---

func TestPath_Elements(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 10)

	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("Elements() = %d, want 2", len(elems))
	}

	if _, ok := elems[0].(MoveTo); !ok {
		t.Errorf("Elements()[0] = %T, want MoveTo", elems[0])
	}
	if _, ok := elems[1].(LineTo); !ok {
		t.Errorf("Elements()[1] = %T, want LineTo", elems[1])
	}
}

func TestPath_Elements_AllTypes(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 10)
	p.QuadraticTo(15, 5, 20, 0)
	p.CubicTo(25, 5, 30, 5, 35, 0)
	p.Close()

	elems := p.Elements()
	if len(elems) != 5 {
		t.Fatalf("Elements() = %d, want 5", len(elems))
	}

	// Verify types and values
	m, ok := elems[0].(MoveTo)
	if !ok {
		t.Fatalf("elems[0] = %T, want MoveTo", elems[0])
	}
	if m.Point.X != 0 || m.Point.Y != 0 {
		t.Errorf("MoveTo point = %v, want (0, 0)", m.Point)
	}

	l, ok := elems[1].(LineTo)
	if !ok {
		t.Fatalf("elems[1] = %T, want LineTo", elems[1])
	}
	if l.Point.X != 10 || l.Point.Y != 10 {
		t.Errorf("LineTo point = %v, want (10, 10)", l.Point)
	}

	q, ok := elems[2].(QuadTo)
	if !ok {
		t.Fatalf("elems[2] = %T, want QuadTo", elems[2])
	}
	if q.Control.X != 15 || q.Control.Y != 5 {
		t.Errorf("QuadTo control = %v, want (15, 5)", q.Control)
	}
	if q.Point.X != 20 || q.Point.Y != 0 {
		t.Errorf("QuadTo point = %v, want (20, 0)", q.Point)
	}

	c, ok := elems[3].(CubicTo)
	if !ok {
		t.Fatalf("elems[3] = %T, want CubicTo", elems[3])
	}
	if c.Control1.X != 25 || c.Control1.Y != 5 {
		t.Errorf("CubicTo control1 = %v, want (25, 5)", c.Control1)
	}
	if c.Control2.X != 30 || c.Control2.Y != 5 {
		t.Errorf("CubicTo control2 = %v, want (30, 5)", c.Control2)
	}
	if c.Point.X != 35 || c.Point.Y != 0 {
		t.Errorf("CubicTo point = %v, want (35, 0)", c.Point)
	}

	if _, ok := elems[4].(Close); !ok {
		t.Errorf("elems[4] = %T, want Close", elems[4])
	}
}

// --- Iterate Tests ---

func TestPath_Iterate(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 20)
	p.QuadraticTo(5, 10, 15, 0)
	p.CubicTo(1, 2, 3, 4, 5, 6)
	p.Close()

	var verbs []PathVerb
	var coordLens []int
	p.Iterate(func(verb PathVerb, coords []float64) {
		verbs = append(verbs, verb)
		coordLens = append(coordLens, len(coords))
	})

	wantVerbs := []PathVerb{VerbMoveTo, VerbLineTo, VerbQuadTo, VerbCubicTo, VerbClose}
	wantLens := []int{2, 2, 4, 6, 0}

	if len(verbs) != len(wantVerbs) {
		t.Fatalf("Iterate: got %d verbs, want %d", len(verbs), len(wantVerbs))
	}
	for i := range verbs {
		if verbs[i] != wantVerbs[i] {
			t.Errorf("verb[%d] = %v, want %v", i, verbs[i], wantVerbs[i])
		}
		if coordLens[i] != wantLens[i] {
			t.Errorf("coord len[%d] = %d, want %d", i, coordLens[i], wantLens[i])
		}
	}
}

// --- CurrentPoint Tests ---

func TestPath_CurrentPoint(t *testing.T) {
	p := NewPath()
	p.MoveTo(5, 10)
	if cp := p.CurrentPoint(); cp.X != 5 || cp.Y != 10 {
		t.Errorf("CurrentPoint() = %v, want (5, 10)", cp)
	}

	p.LineTo(20, 30)
	if cp := p.CurrentPoint(); cp.X != 20 || cp.Y != 30 {
		t.Errorf("After LineTo, CurrentPoint() = %v, want (20, 30)", cp)
	}

	p.QuadraticTo(25, 35, 30, 40)
	if cp := p.CurrentPoint(); cp.X != 30 || cp.Y != 40 {
		t.Errorf("After QuadraticTo, CurrentPoint() = %v, want (30, 40)", cp)
	}

	p.CubicTo(35, 45, 40, 50, 50, 60)
	if cp := p.CurrentPoint(); cp.X != 50 || cp.Y != 60 {
		t.Errorf("After CubicTo, CurrentPoint() = %v, want (50, 60)", cp)
	}
}

// --- isEmpty Tests ---

func TestPath_IsEmpty(t *testing.T) {
	p := NewPath()
	if !p.isEmpty() {
		t.Error("New path should be empty")
	}

	p.MoveTo(0, 0)
	if p.isEmpty() {
		t.Error("Path with MoveTo should not be empty")
	}

	p.Clear()
	if !p.isEmpty() {
		t.Error("Cleared path should be empty")
	}
}

// --- Rectangle Tests ---

func TestPath_Rectangle(t *testing.T) {
	p := NewPath()
	p.Rectangle(10, 20, 100, 50)

	elems := p.Elements()
	// Rectangle: MoveTo + 3 LineTo + Close = 5 elements
	if len(elems) != 5 {
		t.Fatalf("Rectangle: %d elements, want 5", len(elems))
	}

	// First element is MoveTo to (10, 20)
	m, ok := elems[0].(MoveTo)
	if !ok {
		t.Fatalf("elems[0] = %T, want MoveTo", elems[0])
	}
	if m.Point.X != 10 || m.Point.Y != 20 {
		t.Errorf("Rectangle start = %v, want (10, 20)", m.Point)
	}

	// Last element is Close
	if _, ok := elems[4].(Close); !ok {
		t.Errorf("Last element = %T, want Close", elems[4])
	}
}

// --- Circle Tests ---

func TestPath_Circle(t *testing.T) {
	p := NewPath()
	p.Circle(50, 50, 25)

	elems := p.Elements()
	// Circle: MoveTo + 4 CubicTo + Close = 6 elements
	if len(elems) != 6 {
		t.Fatalf("Circle: %d elements, want 6", len(elems))
	}

	// Verify first element is MoveTo at (75, 50) = center + radius on x-axis
	m, ok := elems[0].(MoveTo)
	if !ok {
		t.Fatalf("elems[0] = %T, want MoveTo", elems[0])
	}
	if math.Abs(m.Point.X-75) > pathEpsilon || math.Abs(m.Point.Y-50) > pathEpsilon {
		t.Errorf("Circle start = %v, want (75, 50)", m.Point)
	}

	// Verify all middle elements are CubicTo
	for i := 1; i <= 4; i++ {
		if _, ok := elems[i].(CubicTo); !ok {
			t.Errorf("elems[%d] = %T, want CubicTo", i, elems[i])
		}
	}

	// Verify last element is Close
	if _, ok := elems[5].(Close); !ok {
		t.Errorf("Last element = %T, want Close", elems[5])
	}
}

// --- Ellipse Tests ---

func TestPath_Ellipse(t *testing.T) {
	p := NewPath()
	p.Ellipse(50, 50, 30, 20)

	elems := p.Elements()
	// Ellipse: MoveTo + 4 CubicTo + Close = 6 elements
	if len(elems) != 6 {
		t.Fatalf("Ellipse: %d elements, want 6", len(elems))
	}

	// Start at (80, 50) = center + rx
	m, ok := elems[0].(MoveTo)
	if !ok {
		t.Fatalf("elems[0] = %T, want MoveTo", elems[0])
	}
	if math.Abs(m.Point.X-80) > pathEpsilon || math.Abs(m.Point.Y-50) > pathEpsilon {
		t.Errorf("Ellipse start = %v, want (80, 50)", m.Point)
	}
}

// --- Arc Tests ---

func TestPath_Arc(t *testing.T) {
	p := NewPath()
	p.Arc(0, 0, 10, 0, math.Pi/2) // Quarter circle

	elems := p.Elements()
	// Arc: MoveTo (implicit from arcSegment) + 1 CubicTo = at least 2 elements
	if len(elems) < 2 {
		t.Fatalf("Arc: %d elements, want at least 2", len(elems))
	}
}

func TestPath_Arc_FullCircle(t *testing.T) {
	p := NewPath()
	p.Arc(0, 0, 10, 0, 2*math.Pi)

	elems := p.Elements()
	// Full circle: 4 segments (max 90 degrees each) + 1 MoveTo = at least 5
	if len(elems) < 5 {
		t.Fatalf("Full arc: %d elements, want at least 5", len(elems))
	}
}

func TestPath_Arc_LargeAngle(t *testing.T) {
	p := NewPath()
	p.Arc(0, 0, 10, 0, 3*math.Pi) // > 360 degrees

	elems := p.Elements()
	if len(elems) < 6 {
		t.Fatalf("Large arc: %d elements, want at least 6", len(elems))
	}
}

func TestPath_Arc_NegativeAngle(t *testing.T) {
	p := NewPath()
	// angle2 < angle1 should wrap
	p.Arc(0, 0, 10, math.Pi, 0)

	elems := p.Elements()
	if len(elems) < 2 {
		t.Fatalf("Negative direction arc: %d elements, want at least 2", len(elems))
	}
}

// --- RoundedRectangle Tests ---

func TestPath_RoundedRectangle(t *testing.T) {
	p := NewPath()
	p.RoundedRectangle(0, 0, 100, 50, 10)

	elems := p.Elements()
	// RoundedRectangle has MoveTo + LineTo/Arc segments + Close
	if len(elems) < 9 {
		t.Fatalf("RoundedRectangle: %d elements, want at least 9", len(elems))
	}

	// Last element should be Close
	if _, ok := elems[len(elems)-1].(Close); !ok {
		t.Errorf("Last element = %T, want Close", elems[len(elems)-1])
	}
}

func TestPath_RoundedRectangle_RadiusClamping(t *testing.T) {
	// Radius larger than half the smaller dimension
	p1 := NewPath()
	p1.RoundedRectangle(0, 0, 100, 50, 100) // r > h/2

	// Should not panic, should produce a valid path
	if len(p1.Elements()) < 5 {
		t.Errorf("Clamped radius path: %d elements, want at least 5", len(p1.Elements()))
	}
}

func TestPath_RoundedRectangle_ZeroRadius(t *testing.T) {
	p := NewPath()
	p.RoundedRectangle(0, 0, 100, 50, 0)

	// Zero radius should still produce a valid path (essentially a rectangle)
	if len(p.Elements()) < 5 {
		t.Errorf("Zero radius path: %d elements, want at least 5", len(p.Elements()))
	}
}

// --- Clone Tests ---

func TestPath_Clone(t *testing.T) {
	original := NewPath()
	original.MoveTo(10, 20)
	original.LineTo(30, 40)
	original.Close()

	cloned := original.Clone()

	// Verify same structure
	if len(cloned.verbs) != len(original.verbs) {
		t.Fatalf("Clone verbs: %d, want %d", len(cloned.verbs), len(original.verbs))
	}
	if len(cloned.coords) != len(original.coords) {
		t.Fatalf("Clone coords: %d, want %d", len(cloned.coords), len(original.coords))
	}

	// Verify current and start are preserved
	if cloned.current != original.current {
		t.Errorf("Clone current = %v, want %v", cloned.current, original.current)
	}
	if cloned.start != original.start {
		t.Errorf("Clone start = %v, want %v", cloned.start, original.start)
	}

	// Verify independence: modifying clone does not affect original
	cloned.LineTo(50, 60)
	if len(original.verbs) != 3 {
		t.Errorf("Modifying clone affected original: %d verbs", len(original.verbs))
	}
}

func TestPath_Clone_Empty(t *testing.T) {
	original := NewPath()
	cloned := original.Clone()

	if len(cloned.verbs) != 0 {
		t.Errorf("Clone of empty path: %d verbs, want 0", len(cloned.verbs))
	}
}

// --- Transform Tests ---

func TestPath_Transform_Identity(t *testing.T) {
	p := NewPath()
	p.MoveTo(10, 20)
	p.LineTo(30, 40)

	identity := Identity()
	transformed := p.Transform(identity)

	if len(transformed.Elements()) != 2 {
		t.Fatalf("Transform identity: %d elements, want 2", len(transformed.Elements()))
	}

	m, ok := transformed.Elements()[0].(MoveTo)
	if !ok {
		t.Fatalf("Transform identity elems[0] = %T, want MoveTo", transformed.Elements()[0])
	}
	if math.Abs(m.Point.X-10) > pathEpsilon || math.Abs(m.Point.Y-20) > pathEpsilon {
		t.Errorf("Identity transform changed point: %v", m.Point)
	}
}

func TestPath_Transform_Translate(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 10)

	tr := Translate(5, 3)
	transformed := p.Transform(tr)

	m, _ := transformed.Elements()[0].(MoveTo)
	if math.Abs(m.Point.X-5) > pathEpsilon || math.Abs(m.Point.Y-3) > pathEpsilon {
		t.Errorf("Translated MoveTo = %v, want (5, 3)", m.Point)
	}

	l, _ := transformed.Elements()[1].(LineTo)
	if math.Abs(l.Point.X-15) > pathEpsilon || math.Abs(l.Point.Y-13) > pathEpsilon {
		t.Errorf("Translated LineTo = %v, want (15, 13)", l.Point)
	}
}

func TestPath_Transform_Scale(t *testing.T) {
	p := NewPath()
	p.MoveTo(1, 2)
	p.LineTo(3, 4)

	sc := Scale(2, 3)
	transformed := p.Transform(sc)

	m, _ := transformed.Elements()[0].(MoveTo)
	if math.Abs(m.Point.X-2) > pathEpsilon || math.Abs(m.Point.Y-6) > pathEpsilon {
		t.Errorf("Scaled MoveTo = %v, want (2, 6)", m.Point)
	}

	l, _ := transformed.Elements()[1].(LineTo)
	if math.Abs(l.Point.X-6) > pathEpsilon || math.Abs(l.Point.Y-12) > pathEpsilon {
		t.Errorf("Scaled LineTo = %v, want (6, 12)", l.Point)
	}
}

func TestPath_Transform_WithQuadCubic(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(5, 10, 10, 0)
	p.CubicTo(15, 10, 20, 10, 25, 0)
	p.Close()

	tr := Translate(100, 200)
	transformed := p.Transform(tr)

	elems := transformed.Elements()
	if len(elems) != 4 {
		t.Fatalf("Transformed elements: %d, want 4", len(elems))
	}

	// Check QuadTo was transformed
	q, ok := elems[1].(QuadTo)
	if !ok {
		t.Fatalf("elems[1] = %T, want QuadTo", elems[1])
	}
	if math.Abs(q.Control.X-105) > pathEpsilon || math.Abs(q.Control.Y-210) > pathEpsilon {
		t.Errorf("Transformed QuadTo control = %v, want (105, 210)", q.Control)
	}

	// Check CubicTo was transformed
	c, ok := elems[2].(CubicTo)
	if !ok {
		t.Fatalf("elems[2] = %T, want CubicTo", elems[2])
	}
	if math.Abs(c.Control1.X-115) > pathEpsilon || math.Abs(c.Control1.Y-210) > pathEpsilon {
		t.Errorf("Transformed CubicTo control1 = %v, want (115, 210)", c.Control1)
	}

	// Check Close is still Close
	if _, ok := elems[3].(Close); !ok {
		t.Errorf("elems[3] = %T, want Close", elems[3])
	}
}

// --- PathElement Type Tests ---

func TestPathElement_Types(t *testing.T) {
	// Verify all types implement PathElement interface
	var _ PathElement = MoveTo{}
	var _ PathElement = LineTo{}
	var _ PathElement = QuadTo{}
	var _ PathElement = CubicTo{}
	var _ PathElement = Close{}
}

// --- PathVerb Tests ---

func TestPathVerb_String(t *testing.T) {
	tests := []struct {
		verb PathVerb
		want string
	}{
		{VerbMoveTo, "MoveTo"},
		{VerbLineTo, "LineTo"},
		{VerbQuadTo, "QuadTo"},
		{VerbCubicTo, "CubicTo"},
		{VerbClose, "Close"},
		{PathVerb(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.verb.String(); got != tt.want {
			t.Errorf("PathVerb(%d).String() = %q, want %q", tt.verb, got, tt.want)
		}
	}
}

func TestVerbCoordCount(t *testing.T) {
	tests := []struct {
		verb PathVerb
		want int
	}{
		{VerbMoveTo, 2},
		{VerbLineTo, 2},
		{VerbQuadTo, 4},
		{VerbCubicTo, 6},
		{VerbClose, 0},
		{PathVerb(99), 0},
	}
	for _, tt := range tests {
		if got := verbCoordCount(tt.verb); got != tt.want {
			t.Errorf("verbCoordCount(%v) = %d, want %d", tt.verb, got, tt.want)
		}
	}
}

// --- Verbs/Coords Direct Access Tests ---

func TestPath_VerbsCoords(t *testing.T) {
	p := NewPath()
	p.MoveTo(1, 2)
	p.LineTo(3, 4)
	p.Close()

	verbs := p.Verbs()
	if len(verbs) != 3 {
		t.Fatalf("Verbs() len = %d, want 3", len(verbs))
	}
	if verbs[0] != VerbMoveTo || verbs[1] != VerbLineTo || verbs[2] != VerbClose {
		t.Errorf("Verbs() = %v, want [MoveTo LineTo Close]", verbs)
	}

	coords := p.Coords()
	// MoveTo(1,2) + LineTo(3,4) = 4 coords, Close = 0
	if len(coords) != 4 {
		t.Fatalf("Coords() len = %d, want 4", len(coords))
	}
	if coords[0] != 1 || coords[1] != 2 || coords[2] != 3 || coords[3] != 4 {
		t.Errorf("Coords() = %v, want [1 2 3 4]", coords)
	}

	if p.NumVerbs() != 3 {
		t.Errorf("NumVerbs() = %d, want 3", p.NumVerbs())
	}
}

// --- Complex Path Tests ---

func TestPath_MultipleSubpaths(t *testing.T) {
	p := NewPath()

	// First subpath: triangle
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(5, 10)
	p.Close()

	// Second subpath: square
	p.MoveTo(20, 0)
	p.LineTo(30, 0)
	p.LineTo(30, 10)
	p.LineTo(20, 10)
	p.Close()

	if len(p.Elements()) != 9 {
		t.Errorf("Two subpaths: %d elements, want 9", len(p.Elements()))
	}
}

func TestPath_MixedCurves(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.QuadraticTo(15, 5, 20, 0)
	p.CubicTo(25, 5, 30, 5, 35, 0)
	p.Close()

	elems := p.Elements()
	if len(elems) != 5 {
		t.Fatalf("Mixed curves: %d elements, want 5", len(elems))
	}

	// Verify types in order
	types := []string{"MoveTo", "LineTo", "QuadTo", "CubicTo", "Close"}
	for i, elem := range elems {
		var gotType string
		switch elem.(type) {
		case MoveTo:
			gotType = "MoveTo"
		case LineTo:
			gotType = "LineTo"
		case QuadTo:
			gotType = "QuadTo"
		case CubicTo:
			gotType = "CubicTo"
		case Close:
			gotType = "Close"
		}
		if gotType != types[i] {
			t.Errorf("elems[%d] = %s, want %s", i, gotType, types[i])
		}
	}
}

// --- HasCurrentPoint / isEmpty State Machine Tests ---

func TestPath_StateTransitions(t *testing.T) {
	p := NewPath()

	// Empty path
	if p.HasCurrentPoint() {
		t.Error("Empty: HasCurrentPoint should be false")
	}
	if !p.isEmpty() {
		t.Error("Empty: isEmpty should be true")
	}

	// After MoveTo
	p.MoveTo(0, 0)
	if !p.HasCurrentPoint() {
		t.Error("After MoveTo: HasCurrentPoint should be true")
	}
	if p.isEmpty() {
		t.Error("After MoveTo: isEmpty should be false")
	}

	// After LineTo
	p.LineTo(10, 10)
	if !p.HasCurrentPoint() {
		t.Error("After LineTo: HasCurrentPoint should be true")
	}

	// After Close
	p.Close()
	if !p.HasCurrentPoint() {
		t.Error("After Close: HasCurrentPoint should be true (elements still exist)")
	}

	// After Clear
	p.Clear()
	if p.HasCurrentPoint() {
		t.Error("After Clear: HasCurrentPoint should be false")
	}
	if !p.isEmpty() {
		t.Error("After Clear: isEmpty should be true")
	}
}

// --- Circle/Ellipse Mathematical Accuracy ---

func TestPath_Circle_BoundingBox(t *testing.T) {
	p := NewPath()
	p.Circle(100, 100, 50)

	bbox := p.BoundingBox()
	tolerance := 0.5

	if math.Abs(bbox.Min.X-50) > tolerance {
		t.Errorf("Circle bbox min X = %v, want ~50", bbox.Min.X)
	}
	if math.Abs(bbox.Min.Y-50) > tolerance {
		t.Errorf("Circle bbox min Y = %v, want ~50", bbox.Min.Y)
	}
	if math.Abs(bbox.Max.X-150) > tolerance {
		t.Errorf("Circle bbox max X = %v, want ~150", bbox.Max.X)
	}
	if math.Abs(bbox.Max.Y-150) > tolerance {
		t.Errorf("Circle bbox max Y = %v, want ~150", bbox.Max.Y)
	}
}

func TestPath_Ellipse_BoundingBox(t *testing.T) {
	p := NewPath()
	p.Ellipse(0, 0, 30, 20)

	bbox := p.BoundingBox()
	tolerance := 0.5

	if math.Abs(bbox.Min.X-(-30)) > tolerance {
		t.Errorf("Ellipse bbox min X = %v, want ~-30", bbox.Min.X)
	}
	if math.Abs(bbox.Min.Y-(-20)) > tolerance {
		t.Errorf("Ellipse bbox min Y = %v, want ~-20", bbox.Min.Y)
	}
	if math.Abs(bbox.Max.X-30) > tolerance {
		t.Errorf("Ellipse bbox max X = %v, want ~30", bbox.Max.X)
	}
	if math.Abs(bbox.Max.Y-20) > tolerance {
		t.Errorf("Ellipse bbox max Y = %v, want ~20", bbox.Max.Y)
	}
}

// --- SOA Memory Layout Tests ---

func TestPath_SOA_ZeroAllocPerVerb(t *testing.T) {
	// Verify coord counts match expectations
	p := NewPath()
	p.MoveTo(1, 2)                   // 2 coords
	p.LineTo(3, 4)                   // 2 coords
	p.QuadraticTo(5, 6, 7, 8)        // 4 coords
	p.CubicTo(9, 10, 11, 12, 13, 14) // 6 coords
	p.Close()                        // 0 coords

	if len(p.verbs) != 5 {
		t.Errorf("verbs count = %d, want 5", len(p.verbs))
	}
	// 2 + 2 + 4 + 6 + 0 = 14 coords
	if len(p.coords) != 14 {
		t.Errorf("coords count = %d, want 14", len(p.coords))
	}
}

func TestPath_Append(t *testing.T) {
	p1 := NewPath()
	p1.MoveTo(0, 0)
	p1.LineTo(10, 10)

	p2 := NewPath()
	p2.MoveTo(20, 20)
	p2.LineTo(30, 30)

	p1.Append(p2)

	if len(p1.verbs) != 4 {
		t.Errorf("After Append: %d verbs, want 4", len(p1.verbs))
	}
	if len(p1.coords) != 8 {
		t.Errorf("After Append: %d coords, want 8", len(p1.coords))
	}
	if p1.current != p2.current {
		t.Errorf("After Append: current = %v, want %v", p1.current, p2.current)
	}
}

func TestPath_Append_Nil(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.Append(nil) // Should not panic
	if len(p.verbs) != 1 {
		t.Errorf("After Append(nil): %d verbs, want 1", len(p.verbs))
	}
}

func TestPath_Append_Empty(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.Append(NewPath()) // Append empty path
	if len(p.verbs) != 1 {
		t.Errorf("After Append(empty): %d verbs, want 1", len(p.verbs))
	}
}

// --- Benchmarks ---

func BenchmarkPathBuild_Rectangle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		p := NewPath()
		p.Rectangle(10, 20, 100, 50)
	}
}

func BenchmarkPathBuild_Circle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		p := NewPath()
		p.Circle(50, 50, 25)
	}
}

func BenchmarkPathBuild_ComplexPath(b *testing.B) {
	// Simulates a typical icon-sized path with mixed verbs
	for i := 0; i < b.N; i++ {
		p := NewPath()
		p.MoveTo(0, 0)
		p.LineTo(10, 0)
		p.LineTo(10, 10)
		p.QuadraticTo(5, 15, 0, 10)
		p.CubicTo(-2, 8, -2, 2, 0, 0)
		p.Close()
		p.MoveTo(2, 2)
		p.LineTo(8, 2)
		p.LineTo(8, 8)
		p.LineTo(2, 8)
		p.Close()
	}
}

func BenchmarkPathIterate(b *testing.B) {
	p := NewPath()
	p.Circle(50, 50, 25)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Iterate(func(verb PathVerb, coords []float64) {
			_ = verb
			_ = coords
		})
	}
}

func BenchmarkPathElements(b *testing.B) {
	p := NewPath()
	p.Circle(50, 50, 25)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Elements()
	}
}
