package gg

import (
	"math"
	"testing"
)

// --- Extended ExtendMode Tests ---

func TestApplyExtendMode_LargeValues(t *testing.T) {
	tests := []struct {
		name string
		t    float64
		mode ExtendMode
		want float64
	}{
		{"repeat large positive", 5.75, ExtendRepeat, 0.75},
		{"repeat large negative", -3.25, ExtendRepeat, 0.75},
		{"reflect 3.25", 3.25, ExtendReflect, 0.75},
		{"reflect 4.5", 4.5, ExtendReflect, 0.5},
		{"pad large positive", 100, ExtendPad, 1},
		{"pad large negative", -100, ExtendPad, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyExtendMode(tt.t, tt.mode)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("applyExtendMode(%v, %v) = %v, want %v", tt.t, tt.mode, got, tt.want)
			}
		})
	}
}

// --- clamp01 Tests ---

func TestClamp01(t *testing.T) {
	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"negative", -0.5, 0},
		{"zero", 0, 0},
		{"middle", 0.5, 0.5},
		{"one", 1.0, 1.0},
		{"over", 1.5, 1.0},
		{"large negative", -100, 0},
		{"large positive", 100, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clamp01(tt.x)
			if got != tt.want {
				t.Errorf("clamp01(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

// --- colorAtOffset Tests ---

func TestColorAtOffset_EdgeCases(t *testing.T) {
	t.Run("empty stops returns transparent", func(t *testing.T) {
		got := colorAtOffset(nil, 0.5, ExtendPad)
		if !colorsEqual(got, Transparent, 0.001) {
			t.Errorf("colorAtOffset(nil) = %+v, want Transparent", got)
		}
	})

	t.Run("single stop returns that color", func(t *testing.T) {
		stops := []ColorStop{{Offset: 0.5, Color: Red}}
		got := colorAtOffset(stops, 0.5, ExtendPad)
		if !colorsEqual(got, Red, 0.001) {
			t.Errorf("colorAtOffset(single) = %+v, want Red", got)
		}
	})

	t.Run("t before first stop", func(t *testing.T) {
		stops := []ColorStop{
			{Offset: 0.25, Color: Red},
			{Offset: 0.75, Color: Blue},
		}
		got := colorAtOffset(stops, 0.1, ExtendPad)
		if !colorsEqual(got, Red, 0.001) {
			t.Errorf("colorAtOffset(before first) = %+v, want Red", got)
		}
	})

	t.Run("t after last stop", func(t *testing.T) {
		stops := []ColorStop{
			{Offset: 0.25, Color: Red},
			{Offset: 0.75, Color: Blue},
		}
		got := colorAtOffset(stops, 0.9, ExtendPad)
		if !colorsEqual(got, Blue, 0.001) {
			t.Errorf("colorAtOffset(after last) = %+v, want Blue", got)
		}
	})

	t.Run("coincident stops", func(t *testing.T) {
		stops := []ColorStop{
			{Offset: 0.5, Color: Red},
			{Offset: 0.5, Color: Blue},
		}
		got := colorAtOffset(stops, 0.5, ExtendPad)
		// Should return the first stop's color (no division by zero)
		if !colorsEqual(got, Red, 0.01) && !colorsEqual(got, Blue, 0.01) {
			t.Errorf("colorAtOffset(coincident) should return one of the stop colors, got %+v", got)
		}
	})

	t.Run("unsorted stops are sorted", func(t *testing.T) {
		stops := []ColorStop{
			{Offset: 1.0, Color: Blue},
			{Offset: 0.0, Color: Red},
		}
		got := colorAtOffset(stops, 0, ExtendPad)
		if !colorsEqual(got, Red, 0.001) {
			t.Errorf("colorAtOffset(unsorted, t=0) = %+v, want Red", got)
		}
	})
}

func TestColorAtOffset_Interpolation(t *testing.T) {
	stops := []ColorStop{
		{Offset: 0, Color: Red},
		{Offset: 1, Color: Blue},
	}

	// At exact start offset
	got := colorAtOffset(stops, 0, ExtendPad)
	if !colorsEqual(got, Red, 0.01) {
		t.Errorf("t=0 should be Red, got %+v", got)
	}

	// At exact end offset
	got = colorAtOffset(stops, 1, ExtendPad)
	if !colorsEqual(got, Blue, 0.01) {
		t.Errorf("t=1 should be Blue, got %+v", got)
	}

	// Midpoint should be neither Red nor Blue
	got = colorAtOffset(stops, 0.5, ExtendPad)
	if colorsEqual(got, Red, 0.1) || colorsEqual(got, Blue, 0.1) {
		t.Errorf("t=0.5 should be midpoint, not endpoint, got %+v", got)
	}
}

func TestColorAtOffset_ThreeStops(t *testing.T) {
	stops := []ColorStop{
		{Offset: 0, Color: Red},
		{Offset: 0.5, Color: Green},
		{Offset: 1, Color: Blue},
	}

	// At stop positions should return exact colors
	tests := []struct {
		t    float64
		want RGBA
	}{
		{0, Red},
		{0.5, Green},
		{1, Blue},
	}
	for _, tt := range tests {
		got := colorAtOffset(stops, tt.t, ExtendPad)
		if !colorsEqual(got, tt.want, 0.01) {
			t.Errorf("colorAtOffset(3 stops, t=%v) = %+v, want %+v", tt.t, got, tt.want)
		}
	}
}

// --- sortStops Tests ---

func TestSortStops_SingleStop(t *testing.T) {
	stops := []ColorStop{{Offset: 0.5, Color: Red}}
	got := sortStops(stops)
	if len(got) != 1 {
		t.Errorf("sortStops(single) len = %d, want 1", len(got))
	}
	if got[0].Offset != 0.5 {
		t.Errorf("sortStops(single) offset = %v, want 0.5", got[0].Offset)
	}
}

func TestSortStops_DoesNotModifyOriginal(t *testing.T) {
	stops := []ColorStop{
		{Offset: 1, Color: Blue},
		{Offset: 0, Color: Red},
	}
	original := make([]ColorStop, len(stops))
	copy(original, stops)

	_ = sortStops(stops)

	// Original should be unchanged
	for i, s := range stops {
		if s.Offset != original[i].Offset {
			t.Errorf("sortStops modified original: stops[%d].Offset = %v, want %v",
				i, s.Offset, original[i].Offset)
		}
	}
}

// --- Linear Gradient Extended Tests ---

func TestLinearGradientBrush_Diagonal(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 100).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// Point on the gradient line at start
	startColor := g.ColorAt(0, 0)
	if !colorsEqual(startColor, Red, gradientEpsilon) {
		t.Errorf("Diagonal start = %+v, want Red", startColor)
	}

	// Point on the gradient line at end
	endColor := g.ColorAt(100, 100)
	if !colorsEqual(endColor, Blue, gradientEpsilon) {
		t.Errorf("Diagonal end = %+v, want Blue", endColor)
	}
}

func TestLinearGradientBrush_SetExtend(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0)
	if g.Extend != ExtendPad {
		t.Errorf("Default extend = %v, want ExtendPad", g.Extend)
	}

	g.SetExtend(ExtendRepeat)
	if g.Extend != ExtendRepeat {
		t.Errorf("After SetExtend(Repeat) = %v, want ExtendRepeat", g.Extend)
	}

	g.SetExtend(ExtendReflect)
	if g.Extend != ExtendReflect {
		t.Errorf("After SetExtend(Reflect) = %v, want ExtendReflect", g.Extend)
	}
}

func TestLinearGradientBrush_ExtendReflect(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(1, Blue).
		SetExtend(ExtendReflect)

	// At 200 (2x range), should reflect back to start
	got := g.ColorAt(200, 0)
	if !colorsEqual(got, Red, 0.05) {
		t.Errorf("ExtendReflect at 200 should be near Red, got %+v", got)
	}
}

func TestLinearGradientBrush_PerpendicularPoint(t *testing.T) {
	// Horizontal gradient (0,0) -> (100,0)
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// Points at same X but different Y should get same color
	c1 := g.ColorAt(50, 0)
	c2 := g.ColorAt(50, 100)
	c3 := g.ColorAt(50, -50)

	if !colorsEqual(c1, c2, 0.001) {
		t.Errorf("Perpendicular should give same color: %+v vs %+v", c1, c2)
	}
	if !colorsEqual(c1, c3, 0.001) {
		t.Errorf("Perpendicular should give same color: %+v vs %+v", c1, c3)
	}
}

// --- Radial Gradient Extended Tests ---

func TestRadialGradientBrush_SimpleSymmetry(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 50).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// Points equidistant from center should get same color
	c1 := g.ColorAt(75, 50) // 25 units right
	c2 := g.ColorAt(25, 50) // 25 units left
	c3 := g.ColorAt(50, 75) // 25 units down
	c4 := g.ColorAt(50, 25) // 25 units up

	if !colorsEqual(c1, c2, 0.01) {
		t.Errorf("Radial symmetry: right %+v vs left %+v", c1, c2)
	}
	if !colorsEqual(c1, c3, 0.01) {
		t.Errorf("Radial symmetry: right %+v vs down %+v", c1, c3)
	}
	if !colorsEqual(c1, c4, 0.01) {
		t.Errorf("Radial symmetry: right %+v vs up %+v", c1, c4)
	}
}

func TestRadialGradientBrush_FocalAtCenter(t *testing.T) {
	// When focus equals center, SetFocus should not change behavior
	g := NewRadialGradientBrush(50, 50, 0, 50).
		SetFocus(50, 50). // Same as center
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	centerColor := g.ColorAt(50, 50)
	if !colorsEqual(centerColor, Red, gradientEpsilon) {
		t.Errorf("Focus at center, center color = %+v, want Red", centerColor)
	}
}

func TestRadialGradientBrush_FocalCompute(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 50).
		SetFocus(30, 30).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// computeTFocal should be called when focus != center
	t1 := g.computeT(30, 30) // At focus
	if math.Abs(t1) > 0.001 {
		t.Errorf("computeT at focus should be ~0, got %v", t1)
	}
}

func TestRadialGradientBrush_SingleStop(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 50).
		AddColorStop(0.5, Green)

	got := g.ColorAt(50, 50)
	if !colorsEqual(got, Green, gradientEpsilon) {
		t.Errorf("Single stop radial = %+v, want Green", got)
	}
}

func TestRadialGradientBrush_ExtendReflect(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 25).
		AddColorStop(0, Red).
		AddColorStop(1, Blue).
		SetExtend(ExtendReflect)

	if g.Extend != ExtendReflect {
		t.Errorf("Extend = %v, want ExtendReflect", g.Extend)
	}
}

// --- Sweep Gradient Extended Tests ---

func TestSweepGradientBrush_SingleStop(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0).
		AddColorStop(0.5, Green)

	got := g.ColorAt(100, 50)
	if !colorsEqual(got, Green, gradientEpsilon) {
		t.Errorf("Single stop sweep = %+v, want Green", got)
	}
}

func TestSweepGradientBrush_ZeroSweep(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0).
		SetEndAngle(0). // Zero sweep range
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// With zero sweep, angleToT should return 0
	got := g.ColorAt(100, 50)
	if !colorsEqual(got, Red, gradientEpsilon) {
		t.Errorf("Zero sweep should return first stop, got %+v", got)
	}
}

func TestSweepGradientBrush_PartialSweep(t *testing.T) {
	// Half rotation sweep
	g := NewSweepGradientBrush(50, 50, 0).
		SetEndAngle(math.Pi).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// At start angle (0 radians = right)
	rightColor := g.ColorAt(100, 50)
	if !colorsEqual(rightColor, Red, gradientEpsilon) {
		t.Errorf("Partial sweep at start = %+v, want Red", rightColor)
	}

	// At end angle (Pi = left)
	leftColor := g.ColorAt(0, 50)
	if !colorsEqual(leftColor, Blue, gradientEpsilon) {
		t.Errorf("Partial sweep at end = %+v, want Blue", leftColor)
	}
}

func TestNormalizeAngle(t *testing.T) {
	tests := []struct {
		name       string
		angle      float64
		sweepRange float64
		wantSign   int // 1 = positive, -1 = negative, 0 = zero
	}{
		{"positive sweep, positive angle", math.Pi, 2 * math.Pi, 1},
		{"positive sweep, negative angle", -math.Pi, 2 * math.Pi, 1},
		{"negative sweep, positive angle", math.Pi, -2 * math.Pi, -1},
		{"negative sweep, negative angle", -math.Pi, -2 * math.Pi, -1},
		{"zero angle, positive sweep", 0, 2 * math.Pi, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAngle(tt.angle, tt.sweepRange)
			switch tt.wantSign {
			case 1:
				if got < 0 {
					t.Errorf("normalizeAngle(%v, %v) = %v, expected >= 0", tt.angle, tt.sweepRange, got)
				}
			case -1:
				if got > 0 {
					t.Errorf("normalizeAngle(%v, %v) = %v, expected <= 0", tt.angle, tt.sweepRange, got)
				}
			case 0:
				if math.Abs(got) > 0.001 {
					t.Errorf("normalizeAngle(%v, %v) = %v, expected ~0", tt.angle, tt.sweepRange, got)
				}
			}
		})
	}
}

// --- Interpolation Extended Tests ---

func TestInterpolateColorLinear_SameColor(t *testing.T) {
	got := interpolateColorLinear(Red, Red, 0.5)
	if !colorsEqual(got, Red, 0.001) {
		t.Errorf("interpolateColorLinear(Red, Red, 0.5) = %+v, want Red", got)
	}
}

func TestInterpolateColorLinear_WithAlpha(t *testing.T) {
	transparent := RGBA2(1, 0, 0, 0) // Red but fully transparent
	opaque := RGBA2(1, 0, 0, 1)      // Red fully opaque

	// At t=0 should be transparent
	got := interpolateColorLinear(transparent, opaque, 0)
	if math.Abs(got.A-0) > 0.01 {
		t.Errorf("t=0 alpha = %v, want ~0", got.A)
	}

	// At t=1 should be opaque
	got = interpolateColorLinear(transparent, opaque, 1)
	if math.Abs(got.A-1) > 0.01 {
		t.Errorf("t=1 alpha = %v, want ~1", got.A)
	}

	// At t=0.5 should be semi-transparent
	got = interpolateColorLinear(transparent, opaque, 0.5)
	if got.A < 0.1 || got.A > 0.9 {
		t.Errorf("t=0.5 alpha = %v, expected between 0.1 and 0.9", got.A)
	}
}

// --- firstStopColor Tests ---

func TestFirstStopColor(t *testing.T) {
	t.Run("empty returns transparent", func(t *testing.T) {
		got := firstStopColor(nil)
		if !colorsEqual(got, Transparent, 0.001) {
			t.Errorf("firstStopColor(nil) = %+v, want Transparent", got)
		}
	})

	t.Run("single stop", func(t *testing.T) {
		stops := []ColorStop{{Offset: 0.5, Color: Green}}
		got := firstStopColor(stops)
		if !colorsEqual(got, Green, 0.001) {
			t.Errorf("firstStopColor(single) = %+v, want Green", got)
		}
	})

	t.Run("unsorted returns minimum offset", func(t *testing.T) {
		stops := []ColorStop{
			{Offset: 0.7, Color: Blue},
			{Offset: 0.3, Color: Red},
		}
		got := firstStopColor(stops)
		if !colorsEqual(got, Red, 0.001) {
			t.Errorf("firstStopColor(unsorted) = %+v, want Red (min offset)", got)
		}
	})
}

// --- Method Chaining Tests ---

func TestLinearGradientBrush_Chaining(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(0.5, Green).
		AddColorStop(1, Blue).
		SetExtend(ExtendRepeat)

	if len(g.Stops) != 3 {
		t.Errorf("Chaining: stops = %d, want 3", len(g.Stops))
	}
	if g.Extend != ExtendRepeat {
		t.Errorf("Chaining: extend = %v, want ExtendRepeat", g.Extend)
	}
}

func TestRadialGradientBrush_Chaining(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 50).
		SetFocus(30, 30).
		AddColorStop(0, Red).
		AddColorStop(1, Blue).
		SetExtend(ExtendReflect)

	if len(g.Stops) != 2 {
		t.Errorf("Chaining: stops = %d, want 2", len(g.Stops))
	}
	if g.Focus.X != 30 || g.Focus.Y != 30 {
		t.Errorf("Chaining: focus = %v, want (30,30)", g.Focus)
	}
	if g.Extend != ExtendReflect {
		t.Errorf("Chaining: extend = %v, want ExtendReflect", g.Extend)
	}
}

func TestSweepGradientBrush_Chaining(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0).
		SetEndAngle(math.Pi).
		AddColorStop(0, Red).
		AddColorStop(1, Blue).
		SetExtend(ExtendRepeat)

	if len(g.Stops) != 2 {
		t.Errorf("Chaining: stops = %d, want 2", len(g.Stops))
	}
	if g.EndAngle != math.Pi {
		t.Errorf("Chaining: endAngle = %v, want Pi", g.EndAngle)
	}
	if g.Extend != ExtendRepeat {
		t.Errorf("Chaining: extend = %v, want ExtendRepeat", g.Extend)
	}
}
