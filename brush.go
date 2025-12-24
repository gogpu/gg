package gg

// Brush represents what to paint with.
// This is a sealed interface - only types in this package implement it.
//
// The Brush pattern follows vello/peniko Rust conventions, providing a
// type-safe way to represent different brush types (solid colors, gradients,
// images) while maintaining extensibility through CustomBrush.
//
// Supported brush types:
//   - SolidBrush: A single solid color
//   - CustomBrush: User-defined color function (see brush_custom.go)
//
// Example usage:
//
//	// Using convenience constructors
//	ctx.SetFillBrush(gg.Solid(gg.Red))
//	ctx.SetStrokeBrush(gg.SolidRGB(0.5, 0.5, 0.5))
//
//	// Using hex colors
//	brush := gg.SolidHex("#FF5733")
type Brush interface {
	// brushMarker is an unexported method that seals this interface.
	// Only types in this package can implement Brush.
	brushMarker()

	// ColorAt returns the color at the given coordinates.
	// For solid brushes, this returns the same color regardless of position.
	// For pattern-based brushes, this samples the pattern at (x, y).
	ColorAt(x, y float64) RGBA
}

// SolidBrush is a single-color brush.
// It implements the Brush interface and always returns the same color.
type SolidBrush struct {
	// Color is the solid color of this brush.
	Color RGBA
}

// brushMarker implements the sealed Brush interface.
func (SolidBrush) brushMarker() {}

// ColorAt implements Brush. Returns the solid color regardless of position.
func (b SolidBrush) ColorAt(_, _ float64) RGBA {
	return b.Color
}

// Solid creates a SolidBrush from an RGBA color.
//
// Example:
//
//	brush := gg.Solid(gg.Red)
//	brush := gg.Solid(gg.RGBA{R: 1, G: 0, B: 0, A: 1})
func Solid(c RGBA) SolidBrush {
	return SolidBrush{Color: c}
}

// SolidRGB creates a SolidBrush from RGB components (0-1 range).
// Alpha is set to 1.0 (fully opaque).
//
// Example:
//
//	brush := gg.SolidRGB(1, 0, 0) // Red
//	brush := gg.SolidRGB(0.5, 0.5, 0.5) // Gray
func SolidRGB(r, g, b float64) SolidBrush {
	return SolidBrush{Color: RGB(r, g, b)}
}

// SolidRGBA creates a SolidBrush from RGBA components (0-1 range).
//
// Example:
//
//	brush := gg.SolidRGBA(1, 0, 0, 0.5) // Semi-transparent red
func SolidRGBA(r, g, b, a float64) SolidBrush {
	return SolidBrush{Color: RGBA2(r, g, b, a)}
}

// SolidHex creates a SolidBrush from a hex color string.
// Supports formats: "RGB", "RGBA", "RRGGBB", "RRGGBBAA", with optional '#' prefix.
//
// Example:
//
//	brush := gg.SolidHex("#FF5733")
//	brush := gg.SolidHex("FF5733")
//	brush := gg.SolidHex("#F53")
func SolidHex(hex string) SolidBrush {
	return SolidBrush{Color: Hex(hex)}
}

// WithAlpha returns a new SolidBrush with the specified alpha value.
// The RGB components are preserved.
//
// Example:
//
//	opaqueBrush := gg.Solid(gg.Red)
//	semiBrush := opaqueBrush.WithAlpha(0.5)
func (b SolidBrush) WithAlpha(alpha float64) SolidBrush {
	return SolidBrush{
		Color: RGBA{
			R: b.Color.R,
			G: b.Color.G,
			B: b.Color.B,
			A: alpha,
		},
	}
}

// Opaque returns a new SolidBrush with alpha set to 1.0.
func (b SolidBrush) Opaque() SolidBrush {
	return b.WithAlpha(1.0)
}

// Transparent returns a new SolidBrush with alpha set to 0.0.
func (b SolidBrush) Transparent() SolidBrush {
	return b.WithAlpha(0.0)
}

// Lerp performs linear interpolation between two solid brushes.
// Returns a new SolidBrush with the interpolated color.
//
// Example:
//
//	red := gg.Solid(gg.Red)
//	blue := gg.Solid(gg.Blue)
//	purple := red.Lerp(blue, 0.5)
func (b SolidBrush) Lerp(other SolidBrush, t float64) SolidBrush {
	return SolidBrush{Color: b.Color.Lerp(other.Color, t)}
}

// BrushFromPattern converts a legacy Pattern to a Brush.
// This is a compatibility helper for migrating from Pattern to Brush.
//
// If the pattern is a SolidPattern, it returns a SolidBrush.
// Otherwise, it wraps the pattern in a CustomBrush.
//
// Deprecated: Use Brush types directly instead of Pattern.
func BrushFromPattern(p Pattern) Brush {
	if sp, ok := p.(*SolidPattern); ok {
		return SolidBrush{Color: sp.Color}
	}
	// Wrap non-solid patterns in a CustomBrush
	return CustomBrush{
		Func: p.ColorAt,
		Name: "pattern",
	}
}

// PatternFromBrush converts a Brush to a legacy Pattern.
// This is a compatibility helper for code that still uses Pattern.
//
// Deprecated: Use Brush types directly instead of Pattern.
func PatternFromBrush(b Brush) Pattern {
	if sb, ok := b.(SolidBrush); ok {
		return NewSolidPattern(sb.Color)
	}
	// For other brush types, create a wrapper pattern
	return &brushPattern{brush: b}
}

// brushPattern wraps a Brush to implement the Pattern interface.
type brushPattern struct {
	brush Brush
}

// ColorAt implements Pattern.
func (p *brushPattern) ColorAt(x, y float64) RGBA {
	return p.brush.ColorAt(x, y)
}
