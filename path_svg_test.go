package gg

import (
	"math"
	"testing"
)

// svgTestVerb holds verb + coords for path_svg_test assertions.
type svgTestVerb struct {
	Verb   PathVerb
	Coords []float64 // copy of coords for this verb
}

// collectVerbs returns all verbs with their coordinates.
// Test helper replacing the old Elements() pattern.
func collectVerbs(p *Path) []svgTestVerb {
	result := make([]svgTestVerb, 0, p.NumVerbs())
	p.Iterate(func(verb PathVerb, coords []float64) {
		sv := svgTestVerb{Verb: verb}
		if len(coords) > 0 {
			sv.Coords = make([]float64, len(coords))
			copy(sv.Coords, coords)
		}
		result = append(result, sv)
	})
	return result
}

func TestParseSVGPath_Empty(t *testing.T) {
	p, err := ParseSVGPath("")
	if err != nil {
		t.Fatalf("empty string should not error: %v", err)
	}
	if p.NumVerbs() != 0 {
		t.Errorf("expected 0 elements, got %d", p.NumVerbs())
	}
}

func TestParseSVGPath_WhitespaceOnly(t *testing.T) {
	p, err := ParseSVGPath("   \t\n  ")
	if err != nil {
		t.Fatalf("whitespace-only should not error: %v", err)
	}
	if p.NumVerbs() != 0 {
		t.Errorf("expected 0 elements, got %d", p.NumVerbs())
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
			elems := collectVerbs(p)
			if len(elems) != 1 {
				t.Fatalf("expected 1 element, got %d", len(elems))
			}
			if elems[0].Verb != MoveTo {
				t.Fatalf("expected MoveTo, got %v", elems[0].Verb)
			}
			if elems[0].Coords[0] != tt.wantX || elems[0].Coords[1] != tt.wantY {
				t.Errorf("got MoveTo(%v, %v), want (%v, %v)", elems[0].Coords[0], elems[0].Coords[1], tt.wantX, tt.wantY)
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
	elems := collectVerbs(p)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	if elems[1].Verb != MoveTo {
		t.Fatalf("expected MoveTo, got %v", elems[1].Verb)
	}
	// Relative to (5,5): (5+10, 5+20) = (15, 25).
	if elems[1].Coords[0] != 15 || elems[1].Coords[1] != 25 {
		t.Errorf("got MoveTo(%v, %v), want (15, 25)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_LineTo(t *testing.T) {
	p, err := ParseSVGPath("M0,0 L10,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	if elems[1].Verb != LineTo {
		t.Fatalf("expected LineTo, got %v", elems[1].Verb)
	}
	if elems[1].Coords[0] != 10 || elems[1].Coords[1] != 20 {
		t.Errorf("got LineTo(%v, %v), want (10, 20)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_RelativeLineTo(t *testing.T) {
	p, err := ParseSVGPath("M5,5 l10,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	if elems[1].Verb != LineTo {
		t.Fatalf("expected LineTo, got %v", elems[1].Verb)
	}
	if elems[1].Coords[0] != 15 || elems[1].Coords[1] != 25 {
		t.Errorf("got LineTo(%v, %v), want (15, 25)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_HorizontalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 H20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	if elems[1].Verb != LineTo {
		t.Fatalf("expected LineTo, got %v", elems[1].Verb)
	}
	// H20 = absolute x=20, y stays at 10.
	if elems[1].Coords[0] != 20 || elems[1].Coords[1] != 10 {
		t.Errorf("got LineTo(%v, %v), want (20, 10)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_RelativeHorizontalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 h15")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if elems[1].Coords[0] != 20 || elems[1].Coords[1] != 10 {
		t.Errorf("got LineTo(%v, %v), want (20, 10)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_VerticalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 V30")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	// V30 = absolute y=30, x stays at 5.
	if elems[1].Coords[0] != 5 || elems[1].Coords[1] != 30 {
		t.Errorf("got LineTo(%v, %v), want (5, 30)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_RelativeVerticalLine(t *testing.T) {
	p, err := ParseSVGPath("M5,10 v20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if elems[1].Coords[0] != 5 || elems[1].Coords[1] != 30 {
		t.Errorf("got LineTo(%v, %v), want (5, 30)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_CubicBezier(t *testing.T) {
	p, err := ParseSVGPath("M0,0 C10,20,30,40,50,60")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	if elems[1].Verb != CubicTo {
		t.Fatalf("expected CubicTo, got %v", elems[1].Verb)
	}
	if elems[1].Coords[0] != 10 || elems[1].Coords[1] != 20 {
		t.Errorf("control1 = (%v, %v), want (10, 20)", elems[1].Coords[0], elems[1].Coords[1])
	}
	if elems[1].Coords[2] != 30 || elems[1].Coords[3] != 40 {
		t.Errorf("control2 = (%v, %v), want (30, 40)", elems[1].Coords[2], elems[1].Coords[3])
	}
	if elems[1].Coords[4] != 50 || elems[1].Coords[5] != 60 {
		t.Errorf("endpoint = (%v, %v), want (50, 60)", elems[1].Coords[4], elems[1].Coords[5])
	}
}

func TestParseSVGPath_RelativeCubicBezier(t *testing.T) {
	p, err := ParseSVGPath("M10,10 c5,5,10,10,20,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	c := collectVerbs(p)[1]
	if c.Coords[0] != 15 || c.Coords[1] != 15 {
		t.Errorf("control1 = (%v, %v), want (15, 15)", c.Coords[0], c.Coords[1])
	}
	if c.Coords[4] != 30 || c.Coords[5] != 30 {
		t.Errorf("endpoint = (%v, %v), want (30, 30)", c.Coords[4], c.Coords[5])
	}
}

func TestParseSVGPath_SmoothCubic(t *testing.T) {
	// C sets control2 at (30,40), endpoint at (50,60).
	// S reflects control2 around (50,60) -> (70,80), then new control2 (80,90), end (100,100).
	p, err := ParseSVGPath("M0,0 C10,20,30,40,50,60 S80,90,100,100")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}
	if elems[2].Verb != CubicTo {
		t.Fatalf("expected CubicTo, got %v", elems[2].Verb)
	}
	// Reflected control1 = 2*(50,60) - (30,40) = (70, 80).
	if elems[2].Coords[0] != 70 || elems[2].Coords[1] != 80 {
		t.Errorf("reflected control1 = (%v, %v), want (70, 80)", elems[2].Coords[0], elems[2].Coords[1])
	}
	if elems[2].Coords[2] != 80 || elems[2].Coords[3] != 90 {
		t.Errorf("control2 = (%v, %v), want (80, 90)", elems[2].Coords[2], elems[2].Coords[3])
	}
	if elems[2].Coords[4] != 100 || elems[2].Coords[5] != 100 {
		t.Errorf("endpoint = (%v, %v), want (100, 100)", elems[2].Coords[4], elems[2].Coords[5])
	}
}

func TestParseSVGPath_SmoothCubicNoPrec(t *testing.T) {
	// S without preceding C: reflected control = current point.
	p, err := ParseSVGPath("M10,20 S30,40,50,60")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	s := collectVerbs(p)[1]
	// No previous C/S, so control1 = current point = (10, 20).
	if s.Coords[0] != 10 || s.Coords[1] != 20 {
		t.Errorf("reflected control1 = (%v, %v), want (10, 20)", s.Coords[0], s.Coords[1])
	}
}

func TestParseSVGPath_QuadBezier(t *testing.T) {
	p, err := ParseSVGPath("M0,0 Q10,20,30,40")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	q := collectVerbs(p)[1]
	if q.Coords[0] != 10 || q.Coords[1] != 20 {
		t.Errorf("control = (%v, %v), want (10, 20)", q.Coords[0], q.Coords[1])
	}
	if q.Coords[2] != 30 || q.Coords[3] != 40 {
		t.Errorf("endpoint = (%v, %v), want (30, 40)", q.Coords[2], q.Coords[3])
	}
}

func TestParseSVGPath_SmoothQuad(t *testing.T) {
	p, err := ParseSVGPath("M0,0 Q10,20,30,0 T60,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}
	// Reflected control = 2*(30,0) - (10,20) = (50, -20).
	if elems[2].Coords[0] != 50 || elems[2].Coords[1] != -20 {
		t.Errorf("reflected control = (%v, %v), want (50, -20)", elems[2].Coords[0], elems[2].Coords[1])
	}
	if elems[2].Coords[2] != 60 || elems[2].Coords[3] != 0 {
		t.Errorf("endpoint = (%v, %v), want (60, 0)", elems[2].Coords[2], elems[2].Coords[3])
	}
}

func TestParseSVGPath_SmoothQuadNoPrec(t *testing.T) {
	// T without preceding Q: control = current point.
	p, err := ParseSVGPath("M10,20 T30,40")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	tq := collectVerbs(p)[1]
	if tq.Coords[0] != 10 || tq.Coords[1] != 20 {
		t.Errorf("reflected control = (%v, %v), want (10, 20)", tq.Coords[0], tq.Coords[1])
	}
}

func TestParseSVGPath_Close(t *testing.T) {
	p, err := ParseSVGPath("M0,0 L10,0 L10,10 Z")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(elems))
	}
	if elems[3].Verb != Close {
		t.Fatalf("expected Close, got %v", elems[3].Verb)
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
	elems := collectVerbs(p)
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
	elems := collectVerbs(p)
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements (M + 2L), got %d", len(elems))
	}
	if elems[1].Coords[0] != 10 || elems[1].Coords[1] != 20 {
		t.Errorf("first L = (%v, %v), want (10, 20)", elems[1].Coords[0], elems[1].Coords[1])
	}
	if elems[2].Coords[0] != 30 || elems[2].Coords[1] != 40 {
		t.Errorf("second L = (%v, %v), want (30, 40)", elems[2].Coords[0], elems[2].Coords[1])
	}
}

func TestParseSVGPath_ImplicitLineToAfterMoveTo(t *testing.T) {
	// M with extra pairs becomes implicit LineTo.
	p, err := ParseSVGPath("M10,20 30,40")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements (M + L), got %d", len(elems))
	}
	if elems[0].Verb != MoveTo {
		t.Fatalf("expected MoveTo, got %v", elems[0].Verb)
	}
	if elems[1].Verb != LineTo {
		t.Fatalf("expected LineTo, got %v", elems[1].Verb)
	}
	if elems[1].Coords[0] != 30 || elems[1].Coords[1] != 40 {
		t.Errorf("implicit L = (%v, %v), want (30, 40)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_ImplicitHRepeat(t *testing.T) {
	p, err := ParseSVGPath("M0,0 H10 20 30")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	// M + 3 LineTo.
	if len(elems) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(elems))
	}
	if elems[3].Coords[0] != 30 || elems[3].Coords[1] != 0 {
		t.Errorf("third H = (%v, %v), want (30, 0)", elems[3].Coords[0], elems[3].Coords[1])
	}
}

func TestParseSVGPath_NegativeAsSeparator(t *testing.T) {
	// "10-20" = "10, -20"
	p, err := ParseSVGPath("M10-20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := collectVerbs(p)[0]
	if m.Coords[0] != 10 || m.Coords[1] != -20 {
		t.Errorf("got (%v, %v), want (10, -20)", m.Coords[0], m.Coords[1])
	}
}

func TestParseSVGPath_ScientificNotation(t *testing.T) {
	p, err := ParseSVGPath("M1e2,2.5E1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := collectVerbs(p)[0]
	if m.Coords[0] != 100 || m.Coords[1] != 25 {
		t.Errorf("got (%v, %v), want (100, 25)", m.Coords[0], m.Coords[1])
	}
}

func TestParseSVGPath_ScientificNotationNegativeExponent(t *testing.T) {
	p, err := ParseSVGPath("M1e-2,.5e+1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := collectVerbs(p)[0]
	if math.Abs(m.Coords[0]-0.01) > 1e-12 {
		t.Errorf("X = %v, want 0.01", m.Coords[0])
	}
	if math.Abs(m.Coords[1]-5) > 1e-12 {
		t.Errorf("Y = %v, want 5", m.Coords[1])
	}
}

func TestParseSVGPath_LeadingDot(t *testing.T) {
	p, err := ParseSVGPath("M.5,.3")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := collectVerbs(p)[0]
	if m.Coords[0] != 0.5 || m.Coords[1] != 0.3 {
		t.Errorf("got (%v, %v), want (0.5, 0.3)", m.Coords[0], m.Coords[1])
	}
}

func TestParseSVGPath_Arc(t *testing.T) {
	// Simple semicircular arc from (0,0) to (100,0).
	p, err := ParseSVGPath("M0,0 A50,50,0,0,1,100,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	// MoveTo + 1-2 CubicTo segments.
	if len(elems) < 2 {
		t.Fatalf("expected >=2 elements, got %d", len(elems))
	}
	// The last element's endpoint should be (100, 0).
	last := elems[len(elems)-1]
	if math.Abs(last.Coords[4]-100) > 0.01 || math.Abs(last.Coords[5]) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (~100, ~0)", last.Coords[4], last.Coords[5])
	}
}

func TestParseSVGPath_ArcZeroRadius(t *testing.T) {
	// Zero radius = straight line.
	p, err := ParseSVGPath("M0,0 A0,0,0,0,1,100,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements (M + L), got %d", len(elems))
	}
	if elems[1].Verb != LineTo {
		t.Fatalf("expected LineTo, got %v", elems[1].Verb)
	}
	if elems[1].Coords[0] != 100 || elems[1].Coords[1] != 0 {
		t.Errorf("got (%v, %v), want (100, 0)", elems[1].Coords[0], elems[1].Coords[1])
	}
}

func TestParseSVGPath_ArcSameEndpoints(t *testing.T) {
	// Same start and end = no arc drawn.
	p, err := ParseSVGPath("M10,20 A50,50,0,0,1,10,20")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
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
	elems := collectVerbs(p)
	if len(elems) < 2 {
		t.Fatalf("expected >=2 elements, got %d", len(elems))
	}
	// Verify endpoint.
	last := elems[len(elems)-1]
	if math.Abs(last.Coords[4]-25) > 0.01 || math.Abs(last.Coords[5]-(-25)) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (25, -25)", last.Coords[4], last.Coords[5])
	}
}

func TestParseSVGPath_ArcLargeArcSweep(t *testing.T) {
	// Large arc with sweep=1 — should produce more segments.
	p, err := ParseSVGPath("M0,0 A50,50,0,1,1,100,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	// Large arc = multiple cubic segments (typically 3-4).
	if len(elems) < 3 {
		t.Fatalf("expected >=3 elements for large arc, got %d", len(elems))
	}
	last := elems[len(elems)-1]
	if math.Abs(last.Coords[4]-100) > 0.01 || math.Abs(last.Coords[5]) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (~100, ~0)", last.Coords[4], last.Coords[5])
	}
}

func TestParseSVGPath_RelativeArc(t *testing.T) {
	p, err := ParseSVGPath("M50,50 a25,25,0,0,1,50,0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	elems := collectVerbs(p)
	last := elems[len(elems)-1]
	// Relative: endpoint = (50+50, 50+0) = (100, 50).
	if math.Abs(last.Coords[4]-100) > 0.01 || math.Abs(last.Coords[5]-50) > 0.01 {
		t.Errorf("arc endpoint = (%v, %v), want (~100, ~50)", last.Coords[4], last.Coords[5])
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
	elems := collectVerbs(p)
	// 1 MoveTo + 11 LineTo + 1 Close = 13.
	if len(elems) != 13 {
		t.Errorf("expected 13 elements, got %d", len(elems))
	}
	if elems[12].Verb != Close {
		t.Errorf("expected Close, got %v", elems[12].Verb)
	}
}

func TestParseSVGPath_RefreshIcon(t *testing.T) {
	d := "M8,2 C4.69,2 2,4.69 2,8 C2,11.31 4.69,14 8,14 C10.49,14 12.64,12.58 13.59,10.48"
	p, err := ParseSVGPath(d)
	if err != nil {
		t.Fatalf("error parsing refresh icon: %v", err)
	}
	elems := collectVerbs(p)
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
	elems := collectVerbs(p)
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
	elems := collectVerbs(p)
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
			elems := collectVerbs(p)
			if len(elems) != 2 {
				t.Fatalf("expected 2 elements, got %d", len(elems))
			}
			if elems[1].Coords[0] != 30 || elems[1].Coords[1] != 40 {
				t.Errorf("got (%v, %v), want (30, 40)", elems[1].Coords[0], elems[1].Coords[1])
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
