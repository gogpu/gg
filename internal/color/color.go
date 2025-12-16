// Package color provides color space types and conversions for gg.
package color

// ColorSpace represents a color space.
type ColorSpace uint8

const (
	// ColorSpaceSRGB represents the standard sRGB color space.
	ColorSpaceSRGB ColorSpace = iota
	// ColorSpaceLinear represents the linear RGB color space.
	ColorSpaceLinear
)

// ColorF32 represents a color with float32 components in [0,1].
// RGB components are in the color space indicated by context.
// Alpha is always linear (never gamma-encoded).
type ColorF32 struct {
	R, G, B, A float32
}

// ColorU8 represents a color with uint8 components in [0,255].
// RGB components are in the color space indicated by context.
// Alpha is always linear (never gamma-encoded).
type ColorU8 struct {
	R, G, B, A uint8
}
