package gg

import (
	"image/color"
	"math"
)

// RGBA represents a color with red, green, blue, and alpha components.
// Each component is in the range [0, 1].
type RGBA struct {
	R, G, B, A float64
}

// Color converts RGBA to the standard color.Color interface.
func (c RGBA) Color() color.Color {
	return color.NRGBA{
		R: uint8(clamp255(c.R * 255)),
		G: uint8(clamp255(c.G * 255)),
		B: uint8(clamp255(c.B * 255)),
		A: uint8(clamp255(c.A * 255)),
	}
}

// FromColor converts a standard color.Color to RGBA.
func FromColor(c color.Color) RGBA {
	r, g, b, a := c.RGBA()
	return RGBA{
		R: float64(r) / 65535,
		G: float64(g) / 65535,
		B: float64(b) / 65535,
		A: float64(a) / 65535,
	}
}

// RGB creates an opaque color from RGB components.
func RGB(r, g, b float64) RGBA {
	return RGBA{R: r, G: g, B: b, A: 1.0}
}

// RGBA2 creates a color from RGBA components.
func RGBA2(r, g, b, a float64) RGBA {
	return RGBA{R: r, G: g, B: b, A: a}
}

// Hex creates a color from a hex string.
// Supports formats: "RGB", "RGBA", "RRGGBB", "RRGGBBAA".
func Hex(hex string) RGBA {
	if hex != "" && hex[0] == '#' {
		hex = hex[1:]
	}

	var r, g, b, a uint32
	a = 255

	switch len(hex) {
	case 3: // RGB
		parseHex(hex[0:1], &r)
		parseHex(hex[1:2], &g)
		parseHex(hex[2:3], &b)
		r, g, b = r*17, g*17, b*17
	case 4: // RGBA
		parseHex(hex[0:1], &r)
		parseHex(hex[1:2], &g)
		parseHex(hex[2:3], &b)
		parseHex(hex[3:4], &a)
		r, g, b, a = r*17, g*17, b*17, a*17
	case 6: // RRGGBB
		parseHex(hex[0:2], &r)
		parseHex(hex[2:4], &g)
		parseHex(hex[4:6], &b)
	case 8: // RRGGBBAA
		parseHex(hex[0:2], &r)
		parseHex(hex[2:4], &g)
		parseHex(hex[4:6], &b)
		parseHex(hex[6:8], &a)
	default:
		return RGBA{R: 0, G: 0, B: 0, A: 1}
	}

	return RGBA{
		R: float64(r) / 255,
		G: float64(g) / 255,
		B: float64(b) / 255,
		A: float64(a) / 255,
	}
}

// parseHex is a helper for hex parsing
func parseHex(s string, val *uint32) {
	*val = 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		*val *= 16
		switch {
		case '0' <= c && c <= '9':
			*val += uint32(c - '0')
		case 'a' <= c && c <= 'f':
			*val += uint32(c - 'a' + 10)
		case 'A' <= c && c <= 'F':
			*val += uint32(c - 'A' + 10)
		default:
			return
		}
	}
}

// Premultiply returns a premultiplied color.
func (c RGBA) Premultiply() RGBA {
	return RGBA{
		R: c.R * c.A,
		G: c.G * c.A,
		B: c.B * c.A,
		A: c.A,
	}
}

// Unpremultiply returns an unpremultiplied color.
func (c RGBA) Unpremultiply() RGBA {
	if c.A == 0 {
		return RGBA{R: 0, G: 0, B: 0, A: 0}
	}
	return RGBA{
		R: c.R / c.A,
		G: c.G / c.A,
		B: c.B / c.A,
		A: c.A,
	}
}

// Lerp performs linear interpolation between two colors.
func (c RGBA) Lerp(other RGBA, t float64) RGBA {
	return RGBA{
		R: c.R + (other.R-c.R)*t,
		G: c.G + (other.G-c.G)*t,
		B: c.B + (other.B-c.B)*t,
		A: c.A + (other.A-c.A)*t,
	}
}

// clamp255 restricts a value to [0, 255] range.
func clamp255(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return x
}

// Common colors
var (
	Black       = RGB(0, 0, 0)
	White       = RGB(1, 1, 1)
	Red         = RGB(1, 0, 0)
	Green       = RGB(0, 1, 0)
	Blue        = RGB(0, 0, 1)
	Yellow      = RGB(1, 1, 0)
	Cyan        = RGB(0, 1, 1)
	Magenta     = RGB(1, 0, 1)
	Transparent = RGBA2(0, 0, 0, 0)
)

// HSL creates a color from HSL values.
// h is hue [0, 360), s is saturation [0, 1], l is lightness [0, 1].
func HSL(h, s, l float64) RGBA {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	h /= 360

	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h*6, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 1.0/6:
		r, g, b = c, x, 0
	case h < 2.0/6:
		r, g, b = x, c, 0
	case h < 3.0/6:
		r, g, b = 0, c, x
	case h < 4.0/6:
		r, g, b = 0, x, c
	case h < 5.0/6:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	return RGB(r+m, g+m, b+m)
}
