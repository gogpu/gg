package text

import "testing"

func TestDirectionString(t *testing.T) {
	tests := []struct {
		dir  Direction
		want string
	}{
		{DirectionLTR, "LTR"},
		{DirectionRTL, "RTL"},
		{DirectionTTB, "TTB"},
		{DirectionBTT, "BTT"},
		{Direction(99), "Unknown"},
	}

	for _, tt := range tests {
		got := tt.dir.String()
		if got != tt.want {
			t.Errorf("Direction(%d).String() = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

func TestDirectionIsHorizontal(t *testing.T) {
	tests := []struct {
		dir  Direction
		want bool
	}{
		{DirectionLTR, true},
		{DirectionRTL, true},
		{DirectionTTB, false},
		{DirectionBTT, false},
	}

	for _, tt := range tests {
		got := tt.dir.IsHorizontal()
		if got != tt.want {
			t.Errorf("%s.IsHorizontal() = %v, want %v", tt.dir, got, tt.want)
		}
	}
}

func TestDirectionIsVertical(t *testing.T) {
	tests := []struct {
		dir  Direction
		want bool
	}{
		{DirectionLTR, false},
		{DirectionRTL, false},
		{DirectionTTB, true},
		{DirectionBTT, true},
	}

	for _, tt := range tests {
		got := tt.dir.IsVertical()
		if got != tt.want {
			t.Errorf("%s.IsVertical() = %v, want %v", tt.dir, got, tt.want)
		}
	}
}

func TestGlyphTypeString(t *testing.T) {
	tests := []struct {
		gt   GlyphType
		want string
	}{
		{GlyphTypeOutline, "Outline"},
		{GlyphTypeBitmap, "Bitmap"},
		{GlyphTypeCOLR, "COLR"},
		{GlyphTypeSVG, "SVG"},
		{GlyphType(99), "Unknown"},
	}

	for _, tt := range tests {
		got := tt.gt.String()
		if got != tt.want {
			t.Errorf("GlyphType(%d).String() = %q, want %q", tt.gt, got, tt.want)
		}
	}
}

func TestGlyphFlagsHas(t *testing.T) {
	flags := GlyphFlagLigature | GlyphFlagClusterStart

	if !flags.Has(GlyphFlagLigature) {
		t.Error("flags should have GlyphFlagLigature")
	}
	if !flags.Has(GlyphFlagClusterStart) {
		t.Error("flags should have GlyphFlagClusterStart")
	}
	if flags.Has(GlyphFlagMark) {
		t.Error("flags should not have GlyphFlagMark")
	}
	if flags.Has(GlyphFlagSafeToBreak) {
		t.Error("flags should not have GlyphFlagSafeToBreak")
	}
}

func TestGlyphFlagsString(t *testing.T) {
	tests := []struct {
		flags GlyphFlags
		want  string
	}{
		{0, "None"},
		{GlyphFlagLigature, "Ligature"},
		{GlyphFlagMark, "Mark"},
		{GlyphFlagSafeToBreak, "SafeToBreak"},
		{GlyphFlagClusterStart, "ClusterStart"},
		{GlyphFlagLigature | GlyphFlagMark, "Ligature|Mark"},
		{GlyphFlagLigature | GlyphFlagMark | GlyphFlagSafeToBreak | GlyphFlagClusterStart,
			"Ligature|Mark|SafeToBreak|ClusterStart"},
	}

	for _, tt := range tests {
		got := tt.flags.String()
		if got != tt.want {
			t.Errorf("GlyphFlags(%d).String() = %q, want %q", tt.flags, got, tt.want)
		}
	}
}

func TestShapedRunDimensions(t *testing.T) {
	run := &ShapedRun{
		Advance:   100,
		Ascent:    12,
		Descent:   4,
		Direction: DirectionLTR,
	}

	// Horizontal text
	if got := run.Width(); got != 100 {
		t.Errorf("Width() = %v, want 100", got)
	}
	if got := run.Height(); got != 16 {
		t.Errorf("Height() = %v, want 16", got)
	}
	if got := run.LineHeight(); got != 16 {
		t.Errorf("LineHeight() = %v, want 16", got)
	}

	// Vertical text
	run.Direction = DirectionTTB
	if got := run.Width(); got != 16 { // ascent + descent
		t.Errorf("Width() vertical = %v, want 16", got)
	}
	if got := run.Height(); got != 100 { // advance
		t.Errorf("Height() vertical = %v, want 100", got)
	}
}

func TestShapedRunBounds(t *testing.T) {
	run := &ShapedRun{
		Advance:   100,
		Ascent:    12,
		Descent:   4,
		Direction: DirectionLTR,
	}

	// Horizontal bounds
	x, y, w, h := run.Bounds()
	if x != 0 {
		t.Errorf("Bounds() x = %v, want 0", x)
	}
	if y != -12 { // -ascent
		t.Errorf("Bounds() y = %v, want -12", y)
	}
	if w != 100 {
		t.Errorf("Bounds() w = %v, want 100", w)
	}
	if h != 16 {
		t.Errorf("Bounds() h = %v, want 16", h)
	}

	// Vertical bounds
	run.Direction = DirectionTTB
	x, y, w, h = run.Bounds()
	if x != -12 { // -ascent
		t.Errorf("Bounds() vertical x = %v, want -12", x)
	}
	if y != 0 {
		t.Errorf("Bounds() vertical y = %v, want 0", y)
	}
	if w != 16 {
		t.Errorf("Bounds() vertical w = %v, want 16", w)
	}
	if h != 100 {
		t.Errorf("Bounds() vertical h = %v, want 100", h)
	}
}
