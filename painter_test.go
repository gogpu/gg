package gg

import (
	"testing"
)

func TestPainterFromPaint_Solid(t *testing.T) {
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	painter := PainterFromPaint(paint)
	sp, ok := painter.(*SolidPainter)
	if !ok {
		t.Fatalf("expected *SolidPainter, got %T", painter)
	}
	if sp.Color != Red {
		t.Errorf("SolidPainter.Color = %v, want Red", sp.Color)
	}
}

func TestPainterFromPaint_SolidPattern(t *testing.T) {
	paint := &Paint{
		Pattern: NewSolidPattern(Blue),
	}

	painter := PainterFromPaint(paint)
	sp, ok := painter.(*SolidPainter)
	if !ok {
		t.Fatalf("expected *SolidPainter, got %T", painter)
	}
	if sp.Color != Blue {
		t.Errorf("SolidPainter.Color = %v, want Blue", sp.Color)
	}
}

func TestPainterFromPaint_CustomBrush(t *testing.T) {
	paint := NewPaint()
	paint.SetBrush(NewCustomBrush(func(x, y float64) RGBA {
		return Green
	}))

	painter := PainterFromPaint(paint)
	_, ok := painter.(*FuncPainter)
	if !ok {
		t.Fatalf("expected *FuncPainter, got %T", painter)
	}
}

func TestPainterFromPaint_CustomPattern(t *testing.T) {
	paint := &Paint{
		Pattern: &testPattern{colorFn: func(_, _ float64) RGBA { return Green }},
	}

	painter := PainterFromPaint(paint)
	fp, ok := painter.(*FuncPainter)
	if !ok {
		t.Fatalf("expected *FuncPainter, got %T", painter)
	}
	// Verify it samples the pattern correctly
	c := fp.Fn(0, 0)
	if c != Green {
		t.Errorf("FuncPainter.Fn returned %v, want Green", c)
	}
}

func TestPainterFromPaint_Empty(t *testing.T) {
	paint := &Paint{}

	painter := PainterFromPaint(paint)
	sp, ok := painter.(*SolidPainter)
	if !ok {
		t.Fatalf("expected *SolidPainter, got %T", painter)
	}
	if sp.Color != Black {
		t.Errorf("SolidPainter.Color = %v, want Black", sp.Color)
	}
}

func TestSolidPainter_PaintSpan(t *testing.T) {
	sp := &SolidPainter{Color: Red}
	dest := make([]RGBA, 5)
	sp.PaintSpan(dest, 10, 20, 5)

	for i, c := range dest {
		if c != Red {
			t.Errorf("dest[%d] = %v, want Red", i, c)
		}
	}
}

func TestFuncPainter_PaintSpan(t *testing.T) {
	// Pattern that returns different colors based on x
	fp := &FuncPainter{
		Fn: func(x, _ float64) RGBA {
			if int(x)%2 == 0 {
				return Red
			}
			return Blue
		},
	}

	dest := make([]RGBA, 4)
	fp.PaintSpan(dest, 0, 0, 4)

	// x=0 -> Red (center 0.5, int(0.5)=0, even)
	// x=1 -> Blue (center 1.5, int(1.5)=1, odd)
	// x=2 -> Red (center 2.5, int(2.5)=2, even)
	// x=3 -> Blue (center 3.5, int(3.5)=3, odd)
	if dest[0] != Red {
		t.Errorf("dest[0] = %v, want Red", dest[0])
	}
	if dest[1] != Blue {
		t.Errorf("dest[1] = %v, want Blue", dest[1])
	}
	if dest[2] != Red {
		t.Errorf("dest[2] = %v, want Red", dest[2])
	}
	if dest[3] != Blue {
		t.Errorf("dest[3] = %v, want Blue", dest[3])
	}
}
