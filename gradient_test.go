package gg

import (
	"math"
	"testing"
)

// tolerance for floating point comparisons
const gradientEpsilon = 0.01

func colorsEqual(c1, c2 RGBA, epsilon float64) bool {
	return math.Abs(c1.R-c2.R) < epsilon &&
		math.Abs(c1.G-c2.G) < epsilon &&
		math.Abs(c1.B-c2.B) < epsilon &&
		math.Abs(c1.A-c2.A) < epsilon
}

// --- ExtendMode Tests ---

func TestApplyExtendMode(t *testing.T) {
	tests := []struct {
		name string
		t    float64
		mode ExtendMode
		want float64
	}{
		// ExtendPad (clamp to [0,1])
		{"pad negative", -0.5, ExtendPad, 0},
		{"pad zero", 0, ExtendPad, 0},
		{"pad middle", 0.5, ExtendPad, 0.5},
		{"pad one", 1, ExtendPad, 1},
		{"pad over", 1.5, ExtendPad, 1},

		// ExtendRepeat
		{"repeat negative", -0.25, ExtendRepeat, 0.75},
		{"repeat zero", 0, ExtendRepeat, 0},
		{"repeat middle", 0.5, ExtendRepeat, 0.5},
		{"repeat one", 1, ExtendRepeat, 0},
		{"repeat 1.25", 1.25, ExtendRepeat, 0.25},
		{"repeat 2.5", 2.5, ExtendRepeat, 0.5},

		// ExtendReflect
		// For reflect: t in [0,1] -> [0,1], t in [1,2] -> [1,0], t in [2,3] -> [0,1], etc.
		// At boundaries: t=1 is end of first period, t=2 is end of second (reflected) period
		{"reflect negative", -0.25, ExtendReflect, 0.25},
		{"reflect zero", 0, ExtendReflect, 0},
		{"reflect middle", 0.5, ExtendReflect, 0.5},
		{"reflect one", 1, ExtendReflect, 1},        // End of first period
		{"reflect 1.25", 1.25, ExtendReflect, 0.75}, // Reflecting: 1 - 0.25 = 0.75
		{"reflect 1.5", 1.5, ExtendReflect, 0.5},    // Reflecting: 1 - 0.5 = 0.5
		{"reflect 2.0", 2.0, ExtendReflect, 0},      // End of reflected period
		{"reflect 2.25", 2.25, ExtendReflect, 0.25}, // Back to normal direction
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

// --- ColorStop Tests ---

func TestSortStops(t *testing.T) {
	tests := []struct {
		name  string
		stops []ColorStop
		wantN int
		first float64
		last  float64
	}{
		{
			name:  "empty",
			stops: nil,
			wantN: 0,
		},
		{
			name: "already sorted",
			stops: []ColorStop{
				{Offset: 0, Color: Red},
				{Offset: 0.5, Color: Green},
				{Offset: 1, Color: Blue},
			},
			wantN: 3,
			first: 0,
			last:  1,
		},
		{
			name: "reverse order",
			stops: []ColorStop{
				{Offset: 1, Color: Blue},
				{Offset: 0, Color: Red},
				{Offset: 0.5, Color: Green},
			},
			wantN: 3,
			first: 0,
			last:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortStops(tt.stops)
			if len(got) != tt.wantN {
				t.Errorf("sortStops() len = %v, want %v", len(got), tt.wantN)
			}
			if tt.wantN > 0 {
				if got[0].Offset != tt.first {
					t.Errorf("sortStops() first = %v, want %v", got[0].Offset, tt.first)
				}
				if got[len(got)-1].Offset != tt.last {
					t.Errorf("sortStops() last = %v, want %v", got[len(got)-1].Offset, tt.last)
				}
			}
		})
	}
}

// --- Color Interpolation Tests ---

func TestInterpolateColorLinear(t *testing.T) {
	tests := []struct {
		name string
		c1   RGBA
		c2   RGBA
		t    float64
		want RGBA
	}{
		{
			name: "t=0 returns first color",
			c1:   Red,
			c2:   Blue,
			t:    0,
			want: Red,
		},
		{
			name: "t=1 returns second color",
			c1:   Red,
			c2:   Blue,
			t:    1,
			want: Blue,
		},
		{
			// In linear sRGB interpolation, midpoint of black (0,0,0) and white (1,1,1)
			// is NOT 0.5 in sRGB space. Linear midpoint (0.5 in linear space)
			// converts to approximately 0.735 in sRGB due to gamma correction.
			// This is the expected and correct behavior for perceptually uniform blending.
			name: "black to white midpoint",
			c1:   Black,
			c2:   White,
			t:    0.5,
			want: RGB(0.735, 0.735, 0.735), // Linear midpoint in sRGB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpolateColorLinear(tt.c1, tt.c2, tt.t)
			// For endpoints, we expect exact match
			if tt.t == 0 || tt.t == 1 {
				if !colorsEqual(got, tt.want, 0.001) {
					t.Errorf("interpolateColorLinear() = %+v, want %+v", got, tt.want)
				}
			}
			// For midpoint, check it's close to expected gamma-corrected value
			if tt.t == 0.5 {
				if !colorsEqual(got, tt.want, 0.01) {
					t.Errorf("interpolateColorLinear() = %+v, want approximately %+v", got, tt.want)
				}
			}
		})
	}
}

// --- LinearGradientBrush Tests ---

func TestLinearGradientBrush_New(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0)
	if g.Start.X != 0 || g.Start.Y != 0 {
		t.Errorf("Start = %+v, want (0, 0)", g.Start)
	}
	if g.End.X != 100 || g.End.Y != 0 {
		t.Errorf("End = %+v, want (100, 0)", g.End)
	}
	if g.Extend != ExtendPad {
		t.Errorf("Extend = %v, want ExtendPad", g.Extend)
	}
}

func TestLinearGradientBrush_AddColorStop(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	if len(g.Stops) != 2 {
		t.Errorf("len(Stops) = %v, want 2", len(g.Stops))
	}
}

func TestLinearGradientBrush_ColorAt(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	tests := []struct {
		name string
		x, y float64
		want RGBA
	}{
		{"at start", 0, 0, Red},
		{"at end", 100, 0, Blue},
		{"before start (pad)", -50, 0, Red},
		{"after end (pad)", 150, 0, Blue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.ColorAt(tt.x, tt.y)
			if !colorsEqual(got, tt.want, gradientEpsilon) {
				t.Errorf("ColorAt(%v, %v) = %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestLinearGradientBrush_ZeroLength(t *testing.T) {
	g := NewLinearGradientBrush(50, 50, 50, 50).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// Zero-length gradient should return first stop color
	got := g.ColorAt(0, 0)
	if !colorsEqual(got, Red, gradientEpsilon) {
		t.Errorf("ColorAt for zero-length gradient = %+v, want Red", got)
	}
}

func TestLinearGradientBrush_EmptyStops(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0)
	got := g.ColorAt(50, 0)
	if !colorsEqual(got, Transparent, gradientEpsilon) {
		t.Errorf("ColorAt with no stops = %+v, want Transparent", got)
	}
}

func TestLinearGradientBrush_SingleStop(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0.5, Green)

	got := g.ColorAt(0, 0)
	if !colorsEqual(got, Green, gradientEpsilon) {
		t.Errorf("ColorAt with single stop = %+v, want Green", got)
	}
}

func TestLinearGradientBrush_Vertical(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 0, 100).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// Vertical gradient should work with y coordinate
	startColor := g.ColorAt(0, 0)
	endColor := g.ColorAt(0, 100)

	if !colorsEqual(startColor, Red, gradientEpsilon) {
		t.Errorf("Vertical start = %+v, want Red", startColor)
	}
	if !colorsEqual(endColor, Blue, gradientEpsilon) {
		t.Errorf("Vertical end = %+v, want Blue", endColor)
	}
}

func TestLinearGradientBrush_ExtendRepeat(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(1, Blue).
		SetExtend(ExtendRepeat)

	// At 150, should repeat (t=0.5 of second cycle)
	// This would be approximately halfway between Red and Blue
	got := g.ColorAt(150, 0)
	// Just verify it's not Red or Blue (which would be pad behavior)
	if colorsEqual(got, Red, 0.1) || colorsEqual(got, Blue, 0.1) {
		t.Errorf("ExtendRepeat at 150 should not be at endpoints, got %+v", got)
	}
}

// --- RadialGradientBrush Tests ---

func TestRadialGradientBrush_New(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 100)
	if g.Center.X != 50 || g.Center.Y != 50 {
		t.Errorf("Center = %+v, want (50, 50)", g.Center)
	}
	if g.Focus.X != 50 || g.Focus.Y != 50 {
		t.Errorf("Focus = %+v, want (50, 50) (should default to center)", g.Focus)
	}
	if g.StartRadius != 0 {
		t.Errorf("StartRadius = %v, want 0", g.StartRadius)
	}
	if g.EndRadius != 100 {
		t.Errorf("EndRadius = %v, want 100", g.EndRadius)
	}
}

func TestRadialGradientBrush_SetFocus(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 100).
		SetFocus(30, 30)

	if g.Focus.X != 30 || g.Focus.Y != 30 {
		t.Errorf("Focus = %+v, want (30, 30)", g.Focus)
	}
}

func TestRadialGradientBrush_ColorAt(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 50).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	tests := []struct {
		name string
		x, y float64
		want RGBA
	}{
		{"at center", 50, 50, Red},
		{"at edge", 100, 50, Blue},
		{"at edge top", 50, 0, Blue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.ColorAt(tt.x, tt.y)
			if !colorsEqual(got, tt.want, gradientEpsilon) {
				t.Errorf("ColorAt(%v, %v) = %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestRadialGradientBrush_EmptyStops(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 50)
	got := g.ColorAt(50, 50)
	if !colorsEqual(got, Transparent, gradientEpsilon) {
		t.Errorf("ColorAt with no stops = %+v, want Transparent", got)
	}
}

func TestRadialGradientBrush_ZeroRadius(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 0).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	got := g.ColorAt(50, 50)
	if !colorsEqual(got, Red, gradientEpsilon) {
		t.Errorf("ColorAt for zero-radius gradient = %+v, want Red", got)
	}
}

func TestRadialGradientBrush_StartRadius(t *testing.T) {
	// Gradient with hole in center (donut gradient)
	g := NewRadialGradientBrush(50, 50, 25, 50).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// At center, should be Red (t < 0, clamped)
	centerColor := g.ColorAt(50, 50)
	if !colorsEqual(centerColor, Red, gradientEpsilon) {
		t.Errorf("Center color = %+v, want Red", centerColor)
	}

	// At inner radius, should be Red
	innerColor := g.ColorAt(75, 50) // 25 units from center
	if !colorsEqual(innerColor, Red, gradientEpsilon) {
		t.Errorf("Inner radius color = %+v, want Red", innerColor)
	}

	// At outer radius, should be Blue
	outerColor := g.ColorAt(100, 50) // 50 units from center
	if !colorsEqual(outerColor, Blue, gradientEpsilon) {
		t.Errorf("Outer radius color = %+v, want Blue", outerColor)
	}
}

// --- SweepGradientBrush Tests ---

func TestSweepGradientBrush_New(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0)
	if g.Center.X != 50 || g.Center.Y != 50 {
		t.Errorf("Center = %+v, want (50, 50)", g.Center)
	}
	if g.StartAngle != 0 {
		t.Errorf("StartAngle = %v, want 0", g.StartAngle)
	}
	if math.Abs(g.EndAngle-2*math.Pi) > 0.001 {
		t.Errorf("EndAngle = %v, want 2*Pi", g.EndAngle)
	}
}

func TestSweepGradientBrush_SetEndAngle(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0).
		SetEndAngle(math.Pi)

	if g.EndAngle != math.Pi {
		t.Errorf("EndAngle = %v, want Pi", g.EndAngle)
	}
}

func TestSweepGradientBrush_ColorAt(t *testing.T) {
	// Sweep gradient: 0 radians = right, Pi/2 = bottom, Pi = left, 3*Pi/2 = top
	// With stops at 0, 0.5, 1 and full rotation (2*Pi):
	// - offset 0 = 0 radians (right) = Red
	// - offset 0.25 = Pi/2 (bottom) = halfway between Red and Green
	// - offset 0.5 = Pi (left) = Green
	// - offset 0.75 = 3*Pi/2 (top) = halfway between Green and Red
	// - offset 1.0 = 2*Pi (right again) = Red
	g := NewSweepGradientBrush(50, 50, 0).
		AddColorStop(0, Red).
		AddColorStop(0.5, Green).
		AddColorStop(1, Red) // Wrap back to red

	tests := []struct {
		name string
		x, y float64
		want RGBA
	}{
		{"right (0 degrees)", 100, 50, Red},
		{"left (180 degrees)", 0, 50, Green},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.ColorAt(tt.x, tt.y)
			if !colorsEqual(got, tt.want, gradientEpsilon) {
				t.Errorf("ColorAt(%v, %v) = %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestSweepGradientBrush_AtCenter(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// At center, angle is undefined, should return first stop
	got := g.ColorAt(50, 50)
	if !colorsEqual(got, Red, gradientEpsilon) {
		t.Errorf("ColorAt center = %+v, want Red", got)
	}
}

func TestSweepGradientBrush_EmptyStops(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0)
	got := g.ColorAt(100, 50)
	if !colorsEqual(got, Transparent, gradientEpsilon) {
		t.Errorf("ColorAt with no stops = %+v, want Transparent", got)
	}
}

func TestSweepGradientBrush_WithOffset(t *testing.T) {
	// Start from top (negative y direction)
	g := NewSweepGradientBrush(50, 50, -math.Pi/2).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// At top, should be Red (start angle)
	topColor := g.ColorAt(50, 0)
	if !colorsEqual(topColor, Red, gradientEpsilon) {
		t.Errorf("Top color (start angle) = %+v, want Red", topColor)
	}
}

// --- Brush and Pattern Interface Tests ---

func TestGradient_BrushInterface(t *testing.T) {
	// Verify all gradients implement Brush interface
	var _ Brush = (*LinearGradientBrush)(nil)
	var _ Brush = (*RadialGradientBrush)(nil)
	var _ Brush = (*SweepGradientBrush)(nil)
}

func TestGradient_PatternInterface(t *testing.T) {
	// Verify all gradients implement Pattern interface
	var _ Pattern = (*LinearGradientBrush)(nil)
	var _ Pattern = (*RadialGradientBrush)(nil)
	var _ Pattern = (*SweepGradientBrush)(nil)
}

// --- Multi-stop Gradient Tests ---

func TestLinearGradientBrush_MultipleStops(t *testing.T) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(0.25, Yellow).
		AddColorStop(0.5, Green).
		AddColorStop(0.75, Cyan).
		AddColorStop(1, Blue)

	// At each stop position, should return that color
	tests := []struct {
		x    float64
		want RGBA
	}{
		{0, Red},
		{25, Yellow},
		{50, Green},
		{75, Cyan},
		{100, Blue},
	}

	for _, tt := range tests {
		got := g.ColorAt(tt.x, 0)
		if !colorsEqual(got, tt.want, gradientEpsilon) {
			t.Errorf("ColorAt(%v, 0) = %+v, want %+v", tt.x, got, tt.want)
		}
	}
}

// --- Focal Gradient Tests ---

func TestRadialGradientBrush_Focal(t *testing.T) {
	// Create a radial gradient with focus offset from center
	g := NewRadialGradientBrush(50, 50, 0, 50).
		SetFocus(40, 40).
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// At focus point, should be Red
	focusColor := g.ColorAt(40, 40)
	if !colorsEqual(focusColor, Red, gradientEpsilon) {
		t.Errorf("Focus color = %+v, want Red", focusColor)
	}

	// At center, should be somewhere between Red and Blue
	centerColor := g.ColorAt(50, 50)
	// Just verify it's not exactly Red (would be if focal wasn't working)
	if colorsEqual(centerColor, Red, 0.01) {
		t.Errorf("Center with focal should not be exactly Red, got %+v", centerColor)
	}
}

func TestRadialGradientBrush_SetExtend(t *testing.T) {
	g := NewRadialGradientBrush(50, 50, 0, 25).
		AddColorStop(0, Red).
		AddColorStop(1, Blue).
		SetExtend(ExtendRepeat)

	if g.Extend != ExtendRepeat {
		t.Errorf("Extend = %v, want ExtendRepeat", g.Extend)
	}

	// At radius 50 (2x end radius), should repeat
	got := g.ColorAt(100, 50)
	// With repeat, t=2 should be same as t=0
	if !colorsEqual(got, Red, gradientEpsilon) {
		t.Errorf("Repeat at 2x radius = %+v, want Red", got)
	}
}

func TestSweepGradientBrush_SetExtend(t *testing.T) {
	g := NewSweepGradientBrush(50, 50, 0).
		SetEndAngle(math.Pi). // Half rotation
		AddColorStop(0, Red).
		AddColorStop(1, Blue).
		SetExtend(ExtendRepeat)

	if g.Extend != ExtendRepeat {
		t.Errorf("Extend = %v, want ExtendRepeat", g.Extend)
	}
}

func TestSweepGradientBrush_NegativeSweep(t *testing.T) {
	// Sweep in negative direction (clockwise in standard coords)
	g := NewSweepGradientBrush(50, 50, 0).
		SetEndAngle(-math.Pi). // Negative half rotation
		AddColorStop(0, Red).
		AddColorStop(1, Blue)

	// At 0 degrees (right), should be Red (start)
	rightColor := g.ColorAt(100, 50)
	if !colorsEqual(rightColor, Red, gradientEpsilon) {
		t.Errorf("Right color = %+v, want Red", rightColor)
	}
}

// --- Benchmark Tests ---

func BenchmarkLinearGradientBrush_ColorAt(b *testing.B) {
	g := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, Red).
		AddColorStop(0.5, Green).
		AddColorStop(1, Blue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.ColorAt(50, 25)
	}
}

func BenchmarkRadialGradientBrush_ColorAt(b *testing.B) {
	g := NewRadialGradientBrush(50, 50, 0, 50).
		AddColorStop(0, Red).
		AddColorStop(0.5, Green).
		AddColorStop(1, Blue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.ColorAt(75, 75)
	}
}

func BenchmarkSweepGradientBrush_ColorAt(b *testing.B) {
	g := NewSweepGradientBrush(50, 50, 0).
		AddColorStop(0, Red).
		AddColorStop(0.5, Green).
		AddColorStop(1, Blue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.ColorAt(75, 75)
	}
}

func BenchmarkInterpolateColor(b *testing.B) {
	c1 := Red
	c2 := Blue

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = interpolateColorLinear(c1, c2, 0.5)
	}
}
