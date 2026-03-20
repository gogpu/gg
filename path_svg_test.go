package gg

import (
	"math"
	"testing"
)

func TestParseSVGPath_Empty(t *testing.T) {
	p, err := ParseSVGPath("")
	if err != nil {
		t.Fatalf("empty string should not error: %v", err)
	}
	if len(p.Elements()) != 0 {
		t.Errorf("expected 0 elements, got %d", len(p.Elements()))
	}
}

func TestParseSVGPath_WhitespaceOnly(t *testing.T) {
	p, err := ParseSVGPath("   \t\n  ")
	if err != nil {
		t.Fatalf("whitespace-only should not error: %v", err)
	}
	if len(p.Elements()) != 0 {
		t.Errorf("expected 0 elements, got %d", len(p.Elements()))
	}
}

func TestParseSVGPath_MoveTo(t *testing.T) {
	tests := []struct {
		name         string
		d            string
		wantX, wantY float64
	}{
		{"absolute space", "M 10 20", 10, 20},
		{"absolute comma", "M10,20", 10, 20},
		{"absolute no sep", "M10 20", 10, 20},
		{"negative coords", "M-10,-20", -10, -20},
		{"float coords", "M1.5,2.7", 1.5, 2.7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseSVGPath(tt.d)
			if err != nil {
				t.Fatalf("ParseSVGPath(%q) error: %v", tt.d, err)
			}
			elems := p.Elements()
			if len(elems) != 1 {
				t.Fatalf("expected 1 element, got %d", len(elems))
			}
			m, ok := elems[0].(MoveTo)
			if !ok {
				t.Fatalf("expected MoveTo, got %T", elems[0])
			}
			if m.Point.X != tt.wantX || m.Point.Y != tt.wantY {
				t.Errorf("got MoveTo(%v, %v), want (%v, %v)", m.Point.X, m.Point.Y, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestParseSVGPath_RelativeMoveTo(t *testing.T) {
	// m starts at (0,0), so m10,20 = absolute (10,20).
	p, err := ParseSVGPath("M5,5 m10,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	m, ok := elems[1].(MoveTo)
	if !ok {
		t.Fatalf("expected MoveTo, got %T", elems[1])
	}
	// Relative to (5,5): (5+10, 5+20) = (15, 25).
	if m.Point.X != 15 || m.Point.Y != 25 {
		t.Errorf("got MoveTo(%v, %v), want (15, 25)", m.Point.X, m.Point.Y)
	}
}

func TestParseSVGPath_LineTo(t *testing.T) {
	p, err := ParseSVGPath("M0,0 L10,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	l, ok := elems[1].(LineTo)
	if !ok {
		t.Fatalf("expected LineTo, got %T", elems[1])
	}
	if l.Point.X != 10 || l.Point.Y != 20 {
		t.Errorf("got LineTo(%v, %v), want (10, 20)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_RelativeLineTo(t *testing.T) {
	p, err := ParseSVGPath("M5,5 l10,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	l, ok := elems[1].(LineTo)
	if !ok {
		t.Fatalf("expected LineTo, got %T", elems[1])
	}
	if l.Point.X != 15 || l.Point.Y != 25 {
		t.Errorf("got LineTo(%v, %v), want (15, 25)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_HorizontalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 H20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	l, ok := elems[1].(LineTo)
	if !ok {
		t.Fatalf("expected LineTo, got %T", elems[1])
	}
	// H20 = absolute x=20, y stays at 10.
	if l.Point.X != 20 || l.Point.Y != 10 {
		t.Errorf("got LineTo(%v, %v), want (20, 10)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_RelativeHorizontalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 h15")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	l := elems[1].(LineTo)
	if l.Point.X != 20 || l.Point.Y != 10 {
		t.Errorf("got LineTo(%v, %v), want (20, 10)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_VerticalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 V30")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	l := elems[1].(LineTo)
	// V30 = absolute y=30, x stays at 5.
	if l.Point.X != 5 || l.Point.Y != 30 {
		t.Errorf("got LineTo(%v, %v), want (5, 30)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_RelativeVerticalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 v20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	l := elems[1].(LineTo)
	if l.Point.X != 5 || l.Point.Y != 30 {
		t.Errorf("got LineTo(%v, %v), want (5, 30)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_CubicBezier(t *testing.T) {
	p, err := ParseSVGPath("M0,0 C10,20,30,40,50,60")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	c, ok := elems[1].(CubicTo)
	if !ok {
		t.Fatalf("expected CubicTo, got %T", elems[1])
	}
	if c.Control1.X != 10 || c.Control1.Y != 20 {
		t.Errorf("control1 = (%v, %v), want (10, 20)", c.Control1.X, c.Control1.Y)
	}
	if c.Control2.X != 30 || c.Control2.Y != 40 {
		t.Errorf("control2 = (%v, %v), want (30, 40)", c.Control2.X, c.Control2.Y)
	}
	if c.Point.X != 50 || c.Point.Y != 60 {
		t.Errorf("endpoint = (%v, %v), want (50, 60)", c.Point.X, c.Point.Y)
	}
}

func TestParseSVGPath_RelativeCubicBezier(t *testing.T) {
	p, err := ParseSVGPath("M10,10 c5,5,10,10,20,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	c := p.Elements()[1].(CubicTo)
	if c.Control1.X != 15 || c.Control1.Y != 15 {
		t.Errorf("control1 = (%v, %v), want (15, 15)", c.Control1.X, c.Control1.Y)
	}
	if c.Point.X != 30 || c.Point.Y != 30 {
		t.Errorf("endpoint = (%v, %v), want (30, 30)", c.Point.X, c.Point.Y)
	}
}

func TestParseSVGPath_SmoothCubic(t *testing.T) {
	// C sets control2 at (30,40), endpoint at (50,60).
	// S reflects control2 around (50,60) -> (70,80), then new control2 (80,90), end (100,100).
	p, err := ParseSVGPath("M0,0 C10,20,30,40,50,60 S80,90,100,100")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}
	s, ok := elems[2].(CubicTo)
	if !ok {
		t.Fatalf("expected CubicTo, got %T", elems[2])
	}
	// Reflected control1 = 2*(50,60) - (30,40) = (70, 80).
	if s.Control1.X != 70 || s.Control1.Y != 80 {
		t.Errorf("reflected control1 = (%v, %v), want (70, 80)", s.Control1.X, s.Control1.Y)
	}
	if s.Control2.X != 80 || s.Control2.Y != 90 {
		t.Errorf("control2 = (%v, %v), want (80, 90)", s.Control2.X, s.Control2.Y)
	}
	if s.Point.X != 100 || s.Point.Y != 100 {
		t.Errorf("endpoint = (%v, %v), want (100, 100)", s.Point.X, s.Point.Y)
	}
}

func TestParseSVGPath_SmoothCubicNoPrec(t *testing.T) {
	// S without preceding C: reflected control = current point.
	p, err := ParseSVGPath("M10,20 S30,40,50,60")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	s := p.Elements()[1].(CubicTo)
	// No previous C/S, so control1 = current point = (10, 20).
	if s.Control1.X != 10 || s.Control1.Y != 20 {
		t.Errorf("reflected control1 = (%v, %v), want (10, 20)", s.Control1.X, s.Control1.Y)
	}
}

func TestParseSVGPath_QuadBezier(t *testing.T) {
	p, err := ParseSVGPath("M0,0 Q10,20,30,40")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	q := p.Elements()[1].(QuadTo)
	if q.Control.X != 10 || q.Control.Y != 20 {
		t.Errorf("control = (%v, %v), want (10, 20)", q.Control.X, q.Control.Y)
	}
	if q.Point.X != 30 || q.Point.Y != 40 {
		t.Errorf("endpoint = (%v, %v), want (30, 40)", q.Point.X, q.Point.Y)
	}
}

func TestParseSVGPath_SmoothQuad(t *testing.T) {
	p, err := ParseSVGPath("M0,0 Q10,20,30,0 T60,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}
	tq := elems[2].(QuadTo)
	// Reflected control = 2*(30,0) - (10,20) = (50, -20).
	if tq.Control.X != 50 || tq.Control.Y != -20 {
		t.Errorf("reflected control = (%v, %v), want (50, -20)", tq.Control.X, tq.Control.Y)
	}
	if tq.Point.X != 60 || tq.Point.Y != 0 {
		t.Errorf("endpoint = (%v, %v), want (60, 0)", tq.Point.X, tq.Point.Y)
	}
}

func TestParseSVGPath_SmoothQuadNoPrec(t *testing.T) {
	// T without preceding Q: control = current point.
	p, err := ParseSVGPath("M10,20 T30,40")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	tq := p.Elements()[1].(QuadTo)
	if tq.Control.X != 10 || tq.Control.Y != 20 {
		t.Errorf("reflected control = (%v, %v), want (10, 20)", tq.Control.X, tq.Control.Y)
	}
}

func TestParseSVGPath_Close(t *testing.T) {
	p, err := ParseSVGPath("M0,0 L10,0 L10,10 Z")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(elems))
	}
	_, ok := elems[3].(Close)
	if !ok {
		t.Fatalf("expected Close, got %T", elems[3])
	}
	// After Z, current point returns to subpath start.
	if p.CurrentPoint().X != 0 || p.CurrentPoint().Y != 0 {
		t.Errorf("after Z, current point = (%v, %v), want (0, 0)",
			p.CurrentPoint().X, p.CurrentPoint().Y)
	}
}

func TestParseSVGPath_CloseMoveTo(t *testing.T) {
	// After Z, a new M starts a fresh subpath. Lowercase z also works.
	p, err := ParseSVGPath("M0,0 L10,0 z M20,20 L30,30")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 5 {
		t.Fatalf("expected 5 elements, got %d", len(elems))
	}
}

func TestParseSVGPath_ImplicitRepeat(t *testing.T) {
	// L with 4 numbers = L 10 20 L 30 40.
	p, err := ParseSVGPath("M0,0 L10,20,30,40")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements (M + 2L), got %d", len(elems))
	}
	l1 := elems[1].(LineTo)
	l2 := elems[2].(LineTo)
	if l1.Point.X != 10 || l1.Point.Y != 20 {
		t.Errorf("first L = (%v, %v), want (10, 20)", l1.Point.X, l1.Point.Y)
	}
	if l2.Point.X != 30 || l2.Point.Y != 40 {
		t.Errorf("second L = (%v, %v), want (30, 40)", l2.Point.X, l2.Point.Y)
	}
}

func TestParseSVGPath_ImplicitLineToAfterMoveTo(t *testing.T) {
	// M with extra pairs becomes implicit LineTo.
	p, err := ParseSVGPath("M10,20 30,40")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements (M + L), got %d", len(elems))
	}
	_, ok := elems[0].(MoveTo)
	if !ok {
		t.Fatalf("expected MoveTo, got %T", elems[0])
	}
	l, ok := elems[1].(LineTo)
	if !ok {
		t.Fatalf("expected LineTo, got %T", elems[1])
	}
	if l.Point.X != 30 || l.Point.Y != 40 {
		t.Errorf("implicit L = (%v, %v), want (30, 40)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_ImplicitHRepeat(t *testing.T) {
	p, err := ParseSVGPath("M0,0 H10 20 30")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	// M + 3 LineTo.
	if len(elems) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(elems))
	}
	l3 := elems[3].(LineTo)
	if l3.Point.X != 30 || l3.Point.Y != 0 {
		t.Errorf("third H = (%v, %v), want (30, 0)", l3.Point.X, l3.Point.Y)
	}
}

func TestParseSVGPath_NegativeAsSeparator(t *testing.T) {
	// "10-20" = "10, -20"
	p, err := ParseSVGPath("M10-20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := p.Elements()[0].(MoveTo)
	if m.Point.X != 10 || m.Point.Y != -20 {
		t.Errorf("got (%v, %v), want (10, -20)", m.Point.X, m.Point.Y)
	}
}

func TestParseSVGPath_ScientificNotation(t *testing.T) {
	p, err := ParseSVGPath("M1e2,2.5E1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := p.Elements()[0].(MoveTo)
	if m.Point.X != 100 || m.Point.Y != 25 {
		t.Errorf("got (%v, %v), want (100, 25)", m.Point.X, m.Point.Y)
	}
}

func TestParseSVGPath_ScientificNotationNegativeExponent(t *testing.T) {
	p, err := ParseSVGPath("M1e-2,.5e+1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := p.Elements()[0].(MoveTo)
	if math.Abs(m.Point.X-0.01) > 1e-12 {
		t.Errorf("X = %v, want 0.01", m.Point.X)
	}
	if math.Abs(m.Point.Y-5) > 1e-12 {
		t.Errorf("Y = %v, want 5", m.Point.Y)
	}
}

func TestParseSVGPath_LeadingDot(t *testing.T) {
	p, err := ParseSVGPath("M.5,.3")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := p.Elements()[0].(MoveTo)
	if m.Point.X != 0.5 || m.Point.Y != 0.3 {
		t.Errorf("got (%v, %v), want (0.5, 0.3)", m.Point.X, m.Point.Y)
	}
}

func TestParseSVGPath_Arc(t *testing.T) {
	// Simple semicircular arc from (0,0) to (100,0).
	p, err := ParseSVGPath("M0,0 A50,50,0,0,1,100,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	// MoveTo + 1-2 CubicTo segments.
	if len(elems) < 2 {
		t.Fatalf("expected >=2 elements, got %d", len(elems))
	}
	// The last element's endpoint should be (100, 0).
	last := elems[len(elems)-1].(CubicTo)
	if math.Abs(last.Point.X-100) > 0.01 || math.Abs(last.Point.Y) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (~100, ~0)", last.Point.X, last.Point.Y)
	}
}

func TestParseSVGPath_ArcZeroRadius(t *testing.T) {
	// Zero radius = straight line.
	p, err := ParseSVGPath("M0,0 A0,0,0,0,1,100,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements (M + L), got %d", len(elems))
	}
	l, ok := elems[1].(LineTo)
	if !ok {
		t.Fatalf("expected LineTo for zero-radius arc, got %T", elems[1])
	}
	if l.Point.X != 100 || l.Point.Y != 0 {
		t.Errorf("got (%v, %v), want (100, 0)", l.Point.X, l.Point.Y)
	}
}

func TestParseSVGPath_ArcSameEndpoints(t *testing.T) {
	// Same start and end = no arc drawn.
	p, err := ParseSVGPath("M10,20 A50,50,0,0,1,10,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	// Only MoveTo, arc is skipped.
	if len(elems) != 1 {
		t.Fatalf("expected 1 element (only M), got %d", len(elems))
	}
}

func TestParseSVGPath_ArcFlagsNoSeparator(t *testing.T) {
	// Flags can be adjacent: "1,0,0,50,25" or even "10050,25" is ambiguous
	// but "A25,26 -7 0,1 25,-25" tests the flag reading.
	p, err := ParseSVGPath("M10,80 A25,26,-7,0,1,25,-25")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	if len(elems) < 2 {
		t.Fatalf("expected >=2 elements, got %d", len(elems))
	}
	// Verify endpoint.
	last := elems[len(elems)-1].(CubicTo)
	if math.Abs(last.Point.X-25) > 0.01 || math.Abs(last.Point.Y-(-25)) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (25, -25)", last.Point.X, last.Point.Y)
	}
}

func TestParseSVGPath_ArcLargeArcSweep(t *testing.T) {
	// Large arc with sweep=1 — should produce more segments.
	p, err := ParseSVGPath("M0,0 A50,50,0,1,1,100,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	// Large arc = multiple cubic segments (typically 3-4).
	if len(elems) < 3 {
		t.Fatalf("expected >=3 elements for large arc, got %d", len(elems))
	}
	last := elems[len(elems)-1].(CubicTo)
	if math.Abs(last.Point.X-100) > 0.01 || math.Abs(last.Point.Y) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (~100, ~0)", last.Point.X, last.Point.Y)
	}
}

func TestParseSVGPath_RelativeArc(t *testing.T) {
	p, err := ParseSVGPath("M50,50 a25,25,0,0,1,50,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	last := elems[len(elems)-1].(CubicTo)
	// Relative: endpoint = (50+50, 50+0) = (100, 50).
	if math.Abs(last.Point.X-100) > 0.01 || math.Abs(last.Point.Y-50) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (~100, ~50)", last.Point.X, last.Point.Y)
	}
}

func TestParseSVGPath_CurrentPoint(t *testing.T) {
	p, err := ParseSVGPath("M10,20 L30,40 H50 V60")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	pt := p.CurrentPoint()
	if pt.X != 50 || pt.Y != 60 {
		t.Errorf("CurrentPoint = (%v, %v), want (50, 60)", pt.X, pt.Y)
	}
}

func TestParseSVGPath_CloseIcon(t *testing.T) {
	// JetBrains Close icon (X shape).
	d := "M2.64,1.27 L7.5,6.13 L12.36,1.27 L13.73,2.64 L8.87,7.5 L13.73,12.36 L12.36,13.73 L7.5,8.87 L2.64,13.73 L1.27,12.36 L6.13,7.5 L1.27,2.64 Z"
	p, err := ParseSVGPath(d)
	if err != nil {
		t.Fatalf("error parsing close icon: %v", err)
	}
	elems := p.Elements()
	// 1 MoveTo + 11 LineTo + 1 Close = 13.
	if len(elems) != 13 {
		t.Errorf("expected 13 elements, got %d", len(elems))
	}
	_, ok := elems[12].(Close)
	if !ok {
		t.Errorf("last element should be Close, got %T", elems[12])
	}
}

func TestParseSVGPath_RefreshIcon(t *testing.T) {
	d := "M8,2 C4.69,2 2,4.69 2,8 C2,11.31 4.69,14 8,14 C10.49,14 12.64,12.58 13.59,10.48"
	p, err := ParseSVGPath(d)
	if err != nil {
		t.Fatalf("error parsing refresh icon: %v", err)
	}
	elems := p.Elements()
	// M + 3 C = 4 elements.
	if len(elems) != 4 {
		t.Errorf("expected 4 elements, got %d", len(elems))
	}
}

func TestParseSVGPath_SearchIcon(t *testing.T) {
	d := "M10.5,0.5 C7.46,0.5 5,2.96 5,6 C5,7.49 5.55,8.85 6.46,9.89 L0.5,15.85"
	p, err := ParseSVGPath(d)
	if err != nil {
		t.Fatalf("error parsing search icon: %v", err)
	}
	elems := p.Elements()
	// M + 2C + 1L = 4.
	if len(elems) != 4 {
		t.Errorf("expected 4 elements, got %d", len(elems))
	}
}

func TestParseSVGPath_ComplexMultiCommand(t *testing.T) {
	// A complex path with multiple command types.
	d := "M10,10 L20,20 C30,30,40,40,50,50 Q60,60,70,70 Z M80,80 l-10,-10"
	p, err := ParseSVGPath(d)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := p.Elements()
	// M10,10 + L20,20 + C... + Q... + Z + M80,80 + l... = 7.
	if len(elems) != 7 {
		t.Errorf("expected 7 elements, got %d", len(elems))
	}
	// After Z + M80,80 + l-10,-10, current point = (70, 70).
	pt := p.CurrentPoint()
	if pt.X != 70 || pt.Y != 70 {
		t.Errorf("CurrentPoint = (%v, %v), want (70, 70)", pt.X, pt.Y)
	}
}

func TestParseSVGPath_WhitespaceVariants(t *testing.T) {
	tests := []struct {
		name string
		d    string
	}{
		{"spaces", "M 10 20 L 30 40"},
		{"commas", "M10,20L30,40"},
		{"tabs", "M\t10\t20\tL\t30\t40"},
		{"newlines", "M10\n20\nL30\n40"},
		{"mixed", "M10, 20 L 30,40"},
		{"extra spaces", "  M  10  20  L  30  40  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseSVGPath(tt.d)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			elems := p.Elements()
			if len(elems) != 2 {
				t.Fatalf("expected 2 elements, got %d", len(elems))
			}
			l := elems[1].(LineTo)
			if l.Point.X != 30 || l.Point.Y != 40 {
				t.Errorf("got (%v, %v), want (30, 40)", l.Point.X, l.Point.Y)
			}
		})
	}
}

// ---- Error cases ----

func TestParseSVGPath_ErrorInvalidCommand(t *testing.T) {
	_, err := ParseSVGPath("X10,20")
	if err == nil {
		t.Fatal("expected error for invalid command 'X'")
	}
}

func TestParseSVGPath_ErrorMissingNumber(t *testing.T) {
	_, err := ParseSVGPath("M10")
	if err == nil {
		t.Fatal("expected error for missing Y coordinate")
	}
}

func TestParseSVGPath_ErrorMissingArcParams(t *testing.T) {
	_, err := ParseSVGPath("M0,0 A50,50")
	if err == nil {
		t.Fatal("expected error for incomplete arc")
	}
}

func TestParseSVGPath_ErrorInvalidFlag(t *testing.T) {
	_, err := ParseSVGPath("M0,0 A50,50,0,2,1,100,0")
	if err == nil {
		t.Fatal("expected error for invalid flag value '2'")
	}
}

func TestParseSVGPath_ErrorInvalidNumber(t *testing.T) {
	_, err := ParseSVGPath("M10,abc")
	if err == nil {
		t.Fatal("expected error for non-numeric value")
	}
}

func TestParseSVGPath_ErrorTrailingCommand(t *testing.T) {
	// A command letter with no parameters (except Z which takes none).
	_, err := ParseSVGPath("M0,0 L")
	if err == nil {
		t.Fatal("expected error for L with no parameters")
	}
}

// ---- Benchmark ----

func BenchmarkParseSVGPath_SimpleIcon(b *testing.B) {
	d := "M2.64,1.27 L7.5,6.13 L12.36,1.27 L13.73,2.64 L8.87,7.5 L13.73,12.36 L12.36,13.73 L7.5,8.87 L2.64,13.73 L1.27,12.36 L6.13,7.5 L1.27,2.64 Z"
	for i := 0; i < b.N; i++ {
		_, _ = ParseSVGPath(d)
	}
}

func BenchmarkParseSVGPath_CurvesAndArcs(b *testing.B) {
	d := "M10,80 A25,25,0,0,1,50,80 A25,25,0,0,1,90,80 Q90,120,50,150 Q10,120,10,80 Z"
	for i := 0; i < b.N; i++ {
		_, _ = ParseSVGPath(d)
	}
}
