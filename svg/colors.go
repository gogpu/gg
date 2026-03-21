package svg

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

const (
	// colorNone is the SVG "none" color value, meaning no fill or stroke.
	colorNone = "none"
)

// parseColor parses an SVG color string into a Go color.Color.
// Supported formats:
//   - "#RGB" (short hex)
//   - "#RRGGBB" (hex)
//   - "#RRGGBBAA" (hex with alpha)
//   - "rgb(r, g, b)" with r,g,b as 0-255 integers
//   - "rgba(r, g, b, a)" with a as 0.0-1.0 float
//   - Named colors: black, white, red, green, blue, etc.
//   - "none" → returns (nil, nil) meaning no color (caller must check)
//   - "currentColor" → returns (nil, nil) meaning use parent color
//   - "" (empty) → returns (nil, nil)
func parseColor(s string) (color.Color, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == colorNone || s == "currentColor" {
		return nil, nil //nolint:nilnil // nil color is valid for "none"/""/currentColor
	}

	// Hex colors
	if s[0] == '#' {
		return parseHexColor(s[1:])
	}

	// rgb()/rgba()
	if strings.HasPrefix(s, "rgb") {
		return parseRGBFunc(s)
	}

	// Named colors
	if c, ok := namedColors[strings.ToLower(s)]; ok {
		return c, nil
	}

	return nil, fmt.Errorf("svg: unsupported color value %q", s)
}

// parseHexColor parses hex digits (without leading #) into color.NRGBA.
func parseHexColor(hex string) (color.Color, error) {
	switch len(hex) {
	case 3: // RGB
		r, err1 := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
		g, err2 := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
		b, err3 := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
		if err := firstErr(err1, err2, err3); err != nil {
			return nil, fmt.Errorf("svg: invalid hex color #%s: %w", hex, err)
		}
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil

	case 4: // RGBA
		r, err1 := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
		g, err2 := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
		b, err3 := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
		a, err4 := strconv.ParseUint(string(hex[3])+string(hex[3]), 16, 8)
		if err := firstErr(err1, err2, err3, err4); err != nil {
			return nil, fmt.Errorf("svg: invalid hex color #%s: %w", hex, err)
		}
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}, nil

	case 6: // RRGGBB
		r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
		g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
		b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
		if err := firstErr(err1, err2, err3); err != nil {
			return nil, fmt.Errorf("svg: invalid hex color #%s: %w", hex, err)
		}
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil

	case 8: // RRGGBBAA
		r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
		g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
		b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
		a, err4 := strconv.ParseUint(hex[6:8], 16, 8)
		if err := firstErr(err1, err2, err3, err4); err != nil {
			return nil, fmt.Errorf("svg: invalid hex color #%s: %w", hex, err)
		}
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}, nil

	default:
		return nil, fmt.Errorf("svg: invalid hex color length %d in #%s", len(hex), hex)
	}
}

// parseRGBFunc parses "rgb(r,g,b)" or "rgba(r,g,b,a)" format.
func parseRGBFunc(s string) (color.Color, error) {
	// Strip function name and parens.
	idx := strings.Index(s, "(")
	if idx < 0 || s[len(s)-1] != ')' {
		return nil, fmt.Errorf("svg: malformed color function %q", s)
	}
	inner := s[idx+1 : len(s)-1]
	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	switch len(parts) {
	case 3: // rgb(r, g, b)
		r, err1 := strconv.Atoi(parts[0])
		g, err2 := strconv.Atoi(parts[1])
		b, err3 := strconv.Atoi(parts[2])
		if err := firstErr(err1, err2, err3); err != nil {
			return nil, fmt.Errorf("svg: invalid rgb() values: %w", err)
		}
		return color.NRGBA{R: clampByte(r), G: clampByte(g), B: clampByte(b), A: 255}, nil

	case 4: // rgba(r, g, b, a)
		r, err1 := strconv.Atoi(parts[0])
		g, err2 := strconv.Atoi(parts[1])
		b, err3 := strconv.Atoi(parts[2])
		a, err4 := strconv.ParseFloat(parts[3], 64)
		if err := firstErr(err1, err2, err3, err4); err != nil {
			return nil, fmt.Errorf("svg: invalid rgba() values: %w", err)
		}
		return color.NRGBA{R: clampByte(r), G: clampByte(g), B: clampByte(b), A: clampByte(int(a * 255))}, nil

	default:
		return nil, fmt.Errorf("svg: expected 3 or 4 values in %q, got %d", s, len(parts))
	}
}

// clampByte clamps an integer to the 0-255 range.
func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// firstErr returns the first non-nil error from a list.
func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// namedColors maps common SVG/CSS named colors to Go color values.
// This is not the full CSS named color list — only common ones used in icon SVGs.
var namedColors = map[string]color.Color{
	"black":       color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	"white":       color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	"red":         color.NRGBA{R: 255, G: 0, B: 0, A: 255},
	"green":       color.NRGBA{R: 0, G: 128, B: 0, A: 255},
	"blue":        color.NRGBA{R: 0, G: 0, B: 255, A: 255},
	"yellow":      color.NRGBA{R: 255, G: 255, B: 0, A: 255},
	"cyan":        color.NRGBA{R: 0, G: 255, B: 255, A: 255},
	"magenta":     color.NRGBA{R: 255, G: 0, B: 255, A: 255},
	"gray":        color.NRGBA{R: 128, G: 128, B: 128, A: 255},
	"grey":        color.NRGBA{R: 128, G: 128, B: 128, A: 255},
	"orange":      color.NRGBA{R: 255, G: 165, B: 0, A: 255},
	"purple":      color.NRGBA{R: 128, G: 0, B: 128, A: 255},
	"transparent": color.NRGBA{R: 0, G: 0, B: 0, A: 0},
	"silver":      color.NRGBA{R: 192, G: 192, B: 192, A: 255},
	"maroon":      color.NRGBA{R: 128, G: 0, B: 0, A: 255},
	"navy":        color.NRGBA{R: 0, G: 0, B: 128, A: 255},
	"teal":        color.NRGBA{R: 0, G: 128, B: 128, A: 255},
	"olive":       color.NRGBA{R: 128, G: 128, B: 0, A: 255},
	"lime":        color.NRGBA{R: 0, G: 255, B: 0, A: 255},
	"aqua":        color.NRGBA{R: 0, G: 255, B: 255, A: 255},
	"fuchsia":     color.NRGBA{R: 255, G: 0, B: 255, A: 255},
}
