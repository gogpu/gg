package gg

import (
	"image/color"
	"math"
	"testing"
)

func TestPremultiply(t *testing.T) {
	tests := []struct {
		name string
		c    RGBA
		want RGBA
	}{
		{"opaque red", RGBA{1, 0, 0, 1}, RGBA{1, 0, 0, 1}},
		{"50% alpha red", RGBA{1, 0, 0, 0.5}, RGBA{0.5, 0, 0, 0.5}},
		{"transparent", RGBA{1, 1, 1, 0}, RGBA{0, 0, 0, 0}},
		{"25% alpha white", RGBA{1, 1, 1, 0.25}, RGBA{0.25, 0.25, 0.25, 0.25}},
		{"full color half alpha", RGBA{0.8, 0.6, 0.4, 0.5}, RGBA{0.4, 0.3, 0.2, 0.5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.Premultiply()
			if !colorsNear(got, tt.want, 1e-10) {
				t.Errorf("Premultiply() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnpremultiply(t *testing.T) {
	tests := []struct {
		name string
		c    RGBA
		want RGBA
	}{
		{"opaque red", RGBA{1, 0, 0, 1}, RGBA{1, 0, 0, 1}},
		{"premul 50% alpha red", RGBA{0.5, 0, 0, 0.5}, RGBA{1, 0, 0, 0.5}},
		{"transparent", RGBA{0, 0, 0, 0}, RGBA{0, 0, 0, 0}},
		{"premul 25% alpha white", RGBA{0.25, 0.25, 0.25, 0.25}, RGBA{1, 1, 1, 0.25}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.Unpremultiply()
			if !colorsNear(got, tt.want, 1e-10) {
				t.Errorf("Unpremultiply() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPremultiply_Roundtrip(t *testing.T) {
	original := RGBA{0.8, 0.3, 0.5, 0.7}
	roundtripped := original.Premultiply().Unpremultiply()
	if !colorsNear(roundtripped, original, 1e-10) {
		t.Errorf("Premultiply→Unpremultiply roundtrip: %v → %v, want %v", original, roundtripped, original)
	}
}

func TestLerp(t *testing.T) {
	tests := []struct {
		name string
		c1   RGBA
		c2   RGBA
		t    float64
		want RGBA
	}{
		{"t=0 returns first", Red, Blue, 0, Red},
		{"t=1 returns second", Red, Blue, 1, Blue},
		{"t=0.5 midpoint", RGBA{0, 0, 0, 1}, RGBA{1, 1, 1, 1}, 0.5, RGBA{0.5, 0.5, 0.5, 1}},
		{"alpha interpolation", RGBA{1, 0, 0, 0}, RGBA{1, 0, 0, 1}, 0.5, RGBA{1, 0, 0, 0.5}},
		{"extrapolate t=2", RGBA{0, 0, 0, 1}, RGBA{0.5, 0.5, 0.5, 1}, 2, RGBA{1, 1, 1, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c1.Lerp(tt.c2, tt.t)
			if !colorsNear(got, tt.want, 1e-10) {
				t.Errorf("Lerp(%v, %v) = %v, want %v", tt.c2, tt.t, got, tt.want)
			}
		})
	}
}

func TestHSL_ExtendedCases(t *testing.T) {
	tests := []struct {
		name string
		h    float64
		s    float64
		l    float64
		want RGBA
	}{
		{"red", 0, 1, 0.5, RGB(1, 0, 0)},
		{"green", 120, 1, 0.5, RGB(0, 1, 0)},
		{"blue", 240, 1, 0.5, RGB(0, 0, 1)},
		{"white", 0, 0, 1, RGB(1, 1, 1)},
		{"black", 0, 0, 0, RGB(0, 0, 0)},
		{"50% gray", 0, 0, 0.5, RGB(0.5, 0.5, 0.5)},
		{"yellow", 60, 1, 0.5, RGB(1, 1, 0)},
		{"cyan", 180, 1, 0.5, RGB(0, 1, 1)},
		{"magenta", 300, 1, 0.5, RGB(1, 0, 1)},
		{"negative hue wraps", -60, 1, 0.5, RGB(1, 0, 1)},
		{"hue > 360 wraps", 420, 1, 0.5, RGB(1, 1, 0)},
		{"zero saturation", 120, 0, 0.5, RGB(0.5, 0.5, 0.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HSL(tt.h, tt.s, tt.l)
			if !colorsNear(got, tt.want, 0.01) {
				t.Errorf("HSL(%v, %v, %v) = %v, want %v", tt.h, tt.s, tt.l, got, tt.want)
			}
		})
	}
}

func TestHex(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want RGBA
	}{
		{"6-char with hash", "#FF0000", RGB(1, 0, 0)},
		{"6-char no hash", "00FF00", RGB(0, 1, 0)},
		{"3-char", "#F00", RGB(1, 0, 0)},
		{"3-char green", "#0F0", RGB(0, 1, 0)},
		{"8-char with alpha", "#FF000080", RGBA{1, 0, 0, 128.0 / 255.0}},
		{"4-char with alpha", "#F008", RGBA{1, 0, 0, 136.0 / 255.0}},
		{"white", "#FFFFFF", RGB(1, 1, 1)},
		{"black", "#000000", RGB(0, 0, 0)},
		{"invalid length returns black opaque", "#12345", RGB(0, 0, 0)},
		{"empty string returns black opaque", "", RGB(0, 0, 0)},
		{"lowercase hex", "#ff8800", RGB(1, 136.0/255.0, 0)},
		{"mixed case", "#Ff8800", RGB(1, 136.0/255.0, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Hex(tt.hex)
			if !colorsNear(got, tt.want, 0.01) {
				t.Errorf("Hex(%q) = %v, want %v", tt.hex, got, tt.want)
			}
		})
	}
}

func TestFromColor(t *testing.T) {
	tests := []struct {
		name string
		c    color.Color
		want RGBA
	}{
		{"opaque white NRGBA", color.NRGBA{255, 255, 255, 255}, RGB(1, 1, 1)},
		{"opaque red NRGBA", color.NRGBA{255, 0, 0, 255}, RGB(1, 0, 0)},
		{"transparent", color.NRGBA{0, 0, 0, 0}, RGBA{0, 0, 0, 0}},
		{"half alpha", color.NRGBA{255, 0, 0, 128}, RGBA{1, 0, 0, 128.0 * 257.0 / 65535.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromColor(tt.c)
			if !colorsNear(got, tt.want, 0.02) {
				t.Errorf("FromColor(%v) = %v, want %v", tt.c, got, tt.want)
			}
		})
	}
}

func TestRGB(t *testing.T) {
	c := RGB(0.5, 0.3, 0.1)
	if c.R != 0.5 || c.G != 0.3 || c.B != 0.1 || c.A != 1.0 {
		t.Errorf("RGB(0.5, 0.3, 0.1) = %v, want {0.5, 0.3, 0.1, 1.0}", c)
	}
}

func TestRGBA2(t *testing.T) {
	c := RGBA2(0.5, 0.3, 0.1, 0.7)
	if c.R != 0.5 || c.G != 0.3 || c.B != 0.1 || c.A != 0.7 {
		t.Errorf("RGBA2(0.5, 0.3, 0.1, 0.7) = %v, want {0.5, 0.3, 0.1, 0.7}", c)
	}
}

func TestColor_Method(t *testing.T) {
	c := RGB(0.5, 0.3, 0.1)
	stdColor := c.Color()
	nrgba, ok := stdColor.(color.NRGBA)
	if !ok {
		t.Fatalf("Color() returned %T, want color.NRGBA", stdColor)
	}
	if nrgba.A != 255 {
		t.Errorf("Color().A = %d, want 255 for opaque", nrgba.A)
	}
	if nrgba.R != 127 { // 0.5 * 255 = 127.5 → uint8 truncates to 127
		t.Errorf("Color().R = %d, want 127", nrgba.R)
	}
}

func TestClamp65535(t *testing.T) {
	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"negative", -100, 0},
		{"zero", 0, 0},
		{"middle", 32768, 32768},
		{"max", 65535, 65535},
		{"above max", 70000, 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clamp65535(tt.x)
			if got != tt.want {
				t.Errorf("clamp65535(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func TestClamp255(t *testing.T) {
	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"negative", -50, 0},
		{"zero", 0, 0},
		{"middle", 128, 128},
		{"max", 255, 255},
		{"above max", 300, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clamp255(tt.x)
			if got != tt.want {
				t.Errorf("clamp255(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func TestRGBA_Interface_Clamping(t *testing.T) {
	// Test that RGBA() properly clamps out-of-range values
	c := RGBA{2.0, -0.5, 0.5, 1.0}
	r, g, b, a := c.RGBA()
	if a != 65535 {
		t.Errorf("alpha = %d, want 65535", a)
	}
	if r != 65535 { // 2.0 * 1.0 * 65535 clamped to 65535
		t.Errorf("red = %d, want 65535 (clamped)", r)
	}
	if g != 0 { // -0.5 * 1.0 * 65535 clamped to 0
		t.Errorf("green = %d, want 0 (clamped)", g)
	}
	if b < 32000 || b > 33000 { // ~32767
		t.Errorf("blue = %d, want ~32767", b)
	}
}

func TestCommonColors(t *testing.T) {
	tests := []struct {
		name       string
		c          RGBA
		r, g, b, a float64
	}{
		{"Black", Black, 0, 0, 0, 1},
		{"White", White, 1, 1, 1, 1},
		{"Red", Red, 1, 0, 0, 1},
		{"Green", Green, 0, 1, 0, 1},
		{"Blue", Blue, 0, 0, 1, 1},
		{"Yellow", Yellow, 1, 1, 0, 1},
		{"Cyan", Cyan, 0, 1, 1, 1},
		{"Magenta", Magenta, 1, 0, 1, 1},
		{"Transparent", Transparent, 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.c.R != tt.r || tt.c.G != tt.g || tt.c.B != tt.b || tt.c.A != tt.a {
				t.Errorf("%s = %v, want {%v, %v, %v, %v}", tt.name, tt.c, tt.r, tt.g, tt.b, tt.a)
			}
		})
	}
}

func TestParseHex(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want uint32
	}{
		{"single digit", "F", 15},
		{"two digits", "FF", 255},
		{"lowercase", "ff", 255},
		{"mixed", "Ab", 171},
		{"zero", "00", 0},
		{"invalid char", "GG", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got uint32
			parseHex(tt.s, &got)
			if got != tt.want {
				t.Errorf("parseHex(%q) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}

// colorsNear checks if two RGBA colors are approximately equal.
func colorsNear(a, b RGBA, eps float64) bool {
	return math.Abs(a.R-b.R) < eps &&
		math.Abs(a.G-b.G) < eps &&
		math.Abs(a.B-b.B) < eps &&
		math.Abs(a.A-b.A) < eps
}
