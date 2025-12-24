package gg

import (
	"testing"
)

// TestSolidBrushColorAt tests that SolidBrush returns the same color for all coordinates.
func TestSolidBrushColorAt(t *testing.T) {
	tests := []struct {
		name  string
		brush SolidBrush
		x, y  float64
	}{
		{"red at origin", Solid(Red), 0, 0},
		{"blue at 100,100", Solid(Blue), 100, 100},
		{"green at negative", Solid(Green), -50, -50},
		{"custom color", SolidRGBA(0.5, 0.3, 0.7, 0.9), 1000, 2000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.brush.ColorAt(tt.x, tt.y)
			if got != tt.brush.Color {
				t.Errorf("ColorAt(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.brush.Color)
			}
		})
	}
}

// TestSolid tests the Solid constructor.
func TestSolid(t *testing.T) {
	tests := []struct {
		name  string
		color RGBA
	}{
		{"black", Black},
		{"white", White},
		{"red", Red},
		{"transparent", Transparent},
		{"custom", RGBA{R: 0.1, G: 0.2, B: 0.3, A: 0.4}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brush := Solid(tt.color)
			if brush.Color != tt.color {
				t.Errorf("Solid(%v).Color = %v, want %v", tt.color, brush.Color, tt.color)
			}
		})
	}
}

// TestSolidRGB tests the SolidRGB constructor.
func TestSolidRGB(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b float64
		wantR   float64
		wantG   float64
		wantB   float64
		wantA   float64
	}{
		{"black", 0, 0, 0, 0, 0, 0, 1},
		{"white", 1, 1, 1, 1, 1, 1, 1},
		{"red", 1, 0, 0, 1, 0, 0, 1},
		{"gray", 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brush := SolidRGB(tt.r, tt.g, tt.b)
			c := brush.Color
			if c.R != tt.wantR || c.G != tt.wantG || c.B != tt.wantB || c.A != tt.wantA {
				t.Errorf("SolidRGB(%v, %v, %v) = {R:%v, G:%v, B:%v, A:%v}, want {R:%v, G:%v, B:%v, A:%v}",
					tt.r, tt.g, tt.b, c.R, c.G, c.B, c.A, tt.wantR, tt.wantG, tt.wantB, tt.wantA)
			}
		})
	}
}

// TestSolidRGBA tests the SolidRGBA constructor.
func TestSolidRGBA(t *testing.T) {
	tests := []struct {
		name       string
		r, g, b, a float64
	}{
		{"opaque red", 1, 0, 0, 1},
		{"semi-transparent blue", 0, 0, 1, 0.5},
		{"transparent", 0, 0, 0, 0},
		{"custom", 0.2, 0.4, 0.6, 0.8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brush := SolidRGBA(tt.r, tt.g, tt.b, tt.a)
			c := brush.Color
			if c.R != tt.r || c.G != tt.g || c.B != tt.b || c.A != tt.a {
				t.Errorf("SolidRGBA(%v, %v, %v, %v) = {R:%v, G:%v, B:%v, A:%v}",
					tt.r, tt.g, tt.b, tt.a, c.R, c.G, c.B, c.A)
			}
		})
	}
}

// TestSolidHex tests the SolidHex constructor.
func TestSolidHex(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want RGBA
	}{
		{"red 6-digit", "FF0000", Red},
		{"red with hash", "#FF0000", Red},
		{"green 6-digit", "00FF00", Green},
		{"blue 6-digit", "0000FF", Blue},
		{"black 3-digit", "000", Black},
		{"white 3-digit", "FFF", White},
		{"semi-transparent", "FF000080", RGBA{R: 1, G: 0, B: 0, A: 128.0 / 255.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brush := SolidHex(tt.hex)
			c := brush.Color
			if !colorNear(c, tt.want, 0.01) {
				t.Errorf("SolidHex(%q) = %v, want %v", tt.hex, c, tt.want)
			}
		})
	}
}

// TestSolidBrushWithAlpha tests the WithAlpha method.
func TestSolidBrushWithAlpha(t *testing.T) {
	tests := []struct {
		name  string
		brush SolidBrush
		alpha float64
		wantA float64
	}{
		{"half transparent", Solid(Red), 0.5, 0.5},
		{"fully transparent", Solid(Blue), 0, 0},
		{"fully opaque", SolidRGBA(1, 0, 0, 0.3), 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.brush.WithAlpha(tt.alpha)
			if result.Color.A != tt.wantA {
				t.Errorf("WithAlpha(%v) = {A:%v}, want {A:%v}", tt.alpha, result.Color.A, tt.wantA)
			}
			// Check RGB is preserved
			if result.Color.R != tt.brush.Color.R ||
				result.Color.G != tt.brush.Color.G ||
				result.Color.B != tt.brush.Color.B {
				t.Error("WithAlpha modified RGB values")
			}
		})
	}
}

// TestSolidBrushOpaque tests the Opaque method.
func TestSolidBrushOpaque(t *testing.T) {
	brush := SolidRGBA(1, 0, 0, 0.3)
	opaque := brush.Opaque()

	if opaque.Color.A != 1.0 {
		t.Errorf("Opaque().A = %v, want 1.0", opaque.Color.A)
	}
	if opaque.Color.R != brush.Color.R {
		t.Error("Opaque modified R value")
	}
}

// TestSolidBrushTransparent tests the Transparent method.
func TestSolidBrushTransparent(t *testing.T) {
	brush := Solid(Red)
	transparent := brush.Transparent()

	if transparent.Color.A != 0.0 {
		t.Errorf("Transparent().A = %v, want 0.0", transparent.Color.A)
	}
	if transparent.Color.R != brush.Color.R {
		t.Error("Transparent modified R value")
	}
}

// TestSolidBrushLerp tests the Lerp method.
func TestSolidBrushLerp(t *testing.T) {
	tests := []struct {
		name   string
		b1, b2 SolidBrush
		t      float64
		wantR  float64
	}{
		{"red to blue at 0", Solid(Red), Solid(Blue), 0, 1},
		{"red to blue at 1", Solid(Red), Solid(Blue), 1, 0},
		{"red to blue at 0.5", Solid(Red), Solid(Blue), 0.5, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.b1.Lerp(tt.b2, tt.t)
			if !near(result.Color.R, tt.wantR) {
				t.Errorf("Lerp t=%v, R = %v, want %v", tt.t, result.Color.R, tt.wantR)
			}
		})
	}
}

// TestBrushInterface verifies SolidBrush implements Brush.
func TestBrushInterface(t *testing.T) {
	var _ Brush = SolidBrush{}
	var _ Brush = Solid(Red)
}

// TestBrushFromPattern tests conversion from Pattern to Brush.
func TestBrushFromPattern(t *testing.T) {
	t.Run("solid pattern", func(t *testing.T) {
		pattern := NewSolidPattern(Red)
		brush := BrushFromPattern(pattern)

		sb, ok := brush.(SolidBrush)
		if !ok {
			t.Fatal("Expected SolidBrush from SolidPattern")
		}
		if sb.Color != Red {
			t.Errorf("Color = %v, want %v", sb.Color, Red)
		}
	})

	t.Run("custom pattern", func(t *testing.T) {
		// Create a pattern that returns different colors based on position
		pattern := &testPattern{
			colorFn: func(x, y float64) RGBA {
				if x > 50 {
					return Red
				}
				return Blue
			},
		}
		brush := BrushFromPattern(pattern)

		cb, ok := brush.(CustomBrush)
		if !ok {
			t.Fatal("Expected CustomBrush from custom Pattern")
		}

		// Verify it samples correctly
		if cb.ColorAt(0, 0) != Blue {
			t.Error("Expected Blue at (0, 0)")
		}
		if cb.ColorAt(100, 0) != Red {
			t.Error("Expected Red at (100, 0)")
		}
	})
}

// TestPatternFromBrush tests conversion from Brush to Pattern.
func TestPatternFromBrush(t *testing.T) {
	t.Run("solid brush", func(t *testing.T) {
		brush := Solid(Green)
		pattern := PatternFromBrush(brush)

		sp, ok := pattern.(*SolidPattern)
		if !ok {
			t.Fatal("Expected SolidPattern from SolidBrush")
		}
		if sp.Color != Green {
			t.Errorf("Color = %v, want %v", sp.Color, Green)
		}
	})

	t.Run("custom brush", func(t *testing.T) {
		brush := NewCustomBrush(func(x, y float64) RGBA {
			return RGBA{R: x / 100, G: y / 100, B: 0, A: 1}
		})
		pattern := PatternFromBrush(brush)

		// Verify it samples correctly
		c := pattern.ColorAt(50, 50)
		if !near(c.R, 0.5) || !near(c.G, 0.5) {
			t.Errorf("ColorAt(50, 50) = %v, want R=0.5, G=0.5", c)
		}
	})
}

// testPattern is a test implementation of Pattern.
type testPattern struct {
	colorFn func(x, y float64) RGBA
}

func (p *testPattern) ColorAt(x, y float64) RGBA {
	return p.colorFn(x, y)
}

// colorNear checks if two colors are approximately equal.
func colorNear(a, b RGBA, epsilon float64) bool {
	return near2(a.R, b.R, epsilon) &&
		near2(a.G, b.G, epsilon) &&
		near2(a.B, b.B, epsilon) &&
		near2(a.A, b.A, epsilon)
}

// near checks if two values are approximately equal (default epsilon).
func near(a, b float64) bool {
	return near2(a, b, 0.001)
}

// near2 checks if two values are approximately equal with custom epsilon.
func near2(a, b, epsilon float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
