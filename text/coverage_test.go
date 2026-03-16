package text

import (
	"testing"

	"golang.org/x/image/font"
)

func TestMapHinting(t *testing.T) {
	tests := []struct {
		name string
		h    Hinting
		want font.Hinting
	}{
		{"none", HintingNone, font.HintingNone},
		{"vertical", HintingVertical, font.HintingVertical},
		{"full", HintingFull, font.HintingFull},
		{"unknown defaults to full", Hinting(99), font.HintingFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapHinting(tt.h)
			if got != tt.want {
				t.Errorf("mapHinting(%d) = %d, want %d", tt.h, got, tt.want)
			}
		})
	}
}

func TestRegisterParser(t *testing.T) {
	// Register a custom parser
	RegisterParser("test_parser", &ximageParser{})
	// Should be retrievable
	p := getParser("test_parser")
	if p == nil {
		t.Fatal("getParser('test_parser') returned nil")
	}
}

func TestGetParserDefault(t *testing.T) {
	// Unknown parser name should fall back to default
	p := getParser("nonexistent")
	if p == nil {
		t.Fatal("getParser('nonexistent') should return default parser")
	}
}

func TestGetParserXImage(t *testing.T) {
	p := getParser("ximage")
	if p == nil {
		t.Fatal("getParser('ximage') returned nil")
	}
}

func TestFontMetricsHeight(t *testing.T) {
	m := FontMetrics{
		Ascent:  10,
		Descent: -3,
		LineGap: 2,
	}
	// Height = Ascent - Descent + LineGap = 10 - (-3) + 2 = 15
	if h := m.Height(); h != 15 {
		t.Errorf("FontMetrics.Height() = %f, want 15", h)
	}
}

func TestRectEmpty(t *testing.T) {
	r := Rect{}
	if !r.Empty() {
		t.Error("zero Rect should be Empty")
	}

	r2 := Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 20}
	if r2.Empty() {
		t.Error("non-zero Rect should not be Empty")
	}
	if r2.Width() != 10 || r2.Height() != 20 {
		t.Errorf("Rect dimensions = %fx%f, want 10x20", r2.Width(), r2.Height())
	}
}

func TestDirectionMismatchError(t *testing.T) {
	err := &DirectionMismatchError{
		Index:    1,
		Got:      DirectionRTL,
		Expected: DirectionLTR,
	}
	s := err.Error()
	if s == "" {
		t.Error("Error() returned empty string")
	}
}
