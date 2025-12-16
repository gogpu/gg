// Package image provides image buffer management for gogpu/gg.
package image

import "math"

// SpreadMode determines how to handle coordinates outside [0,1].
type SpreadMode uint8

const (
	// SpreadPad clamps coordinates to the edge (default).
	// Values outside [0,1] are clamped to the nearest edge pixel.
	SpreadPad SpreadMode = iota

	// SpreadRepeat tiles/repeats the image.
	// Coordinates wrap around at the boundaries.
	SpreadRepeat

	// SpreadReflect mirrors the image at boundaries.
	// Creates a mirrored tiling effect.
	SpreadReflect
)

const unknownMode = "Unknown"

// String returns a string representation of the spread mode.
func (s SpreadMode) String() string {
	switch s {
	case SpreadPad:
		return "Pad"
	case SpreadRepeat:
		return "Repeat"
	case SpreadReflect:
		return "Reflect"
	default:
		return unknownMode
	}
}

// ImagePattern represents an image-based fill pattern.
//
// ImagePattern applies transformations, spread modes, and interpolation
// to sample colors from an image. It supports optional mipmap chains
// for quality downscaling at different scales.
//
// The pattern operates in pattern space where coordinates are transformed
// via the inverse transform before sampling from the image.
type ImagePattern struct {
	image      *ImageBuf
	transform  Affine
	inverse    Affine // cached inverse transform
	spreadMode SpreadMode
	interp     InterpolationMode
	opacity    float64
	mipmaps    *MipmapChain // optional, for quality downscaling
}

// NewImagePattern creates a pattern from an image.
//
// Default settings:
//   - Identity transform
//   - SpreadPad mode
//   - InterpBilinear interpolation
//   - Opacity 1.0 (fully opaque)
//   - No mipmaps
//
// Returns nil if img is nil.
func NewImagePattern(img *ImageBuf) *ImagePattern {
	if img == nil {
		return nil
	}

	identity := Identity()
	return &ImagePattern{
		image:      img,
		transform:  identity,
		inverse:    identity, // identity inverse is itself
		spreadMode: SpreadPad,
		interp:     InterpBilinear,
		opacity:    1.0,
		mipmaps:    nil,
	}
}

// WithTransform sets the transformation matrix for the pattern.
// Returns the pattern for method chaining.
//
// The transform converts from pattern space to image space.
// The inverse is cached for efficient sampling.
func (p *ImagePattern) WithTransform(t Affine) *ImagePattern {
	p.transform = t
	inv, ok := t.Invert()
	if ok {
		p.inverse = inv
	} else {
		// Singular matrix - keep identity
		p.inverse = Identity()
	}
	return p
}

// WithSpreadMode sets the spread mode for coordinates outside [0,1].
// Returns the pattern for method chaining.
func (p *ImagePattern) WithSpreadMode(mode SpreadMode) *ImagePattern {
	p.spreadMode = mode
	return p
}

// WithInterpolation sets the interpolation mode for sampling.
// Returns the pattern for method chaining.
func (p *ImagePattern) WithInterpolation(mode InterpolationMode) *ImagePattern {
	p.interp = mode
	return p
}

// WithOpacity sets the opacity multiplier (0.0 = transparent, 1.0 = opaque).
// Returns the pattern for method chaining.
//
// Opacity is clamped to [0.0, 1.0].
func (p *ImagePattern) WithOpacity(opacity float64) *ImagePattern {
	// Clamp opacity to [0, 1]
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}
	p.opacity = opacity
	return p
}

// WithMipmaps sets the mipmap chain for quality downscaling.
// Returns the pattern for method chaining.
//
// The mipmap chain is optional. If provided, SampleWithScale will
// select the appropriate mipmap level based on the scale factor.
func (p *ImagePattern) WithMipmaps(chain *MipmapChain) *ImagePattern {
	p.mipmaps = chain
	return p
}

// Sample returns the color at the given pattern-space coordinates.
//
// The sampling process:
//  1. Apply inverse transform to convert pattern coords to image coords
//  2. Apply spread mode to handle coordinates outside [0,1]
//  3. Apply interpolation to sample the image
//  4. Apply opacity
//
// Returns (0,0,0,0) if the pattern or image is nil.
func (p *ImagePattern) Sample(x, y float64) (r, g, b, a byte) {
	if p == nil || p.image == nil {
		return 0, 0, 0, 0
	}

	// Apply inverse transform to get image coordinates
	u, v := p.inverse.TransformPoint(x, y)

	// Apply spread mode
	u, v = p.applySpreadMode(u, v)

	// Sample the image with interpolation
	r, g, b, a = Sample(p.image, u, v, p.interp)

	// Apply opacity
	if p.opacity < 1.0 {
		a = byte(float64(a) * p.opacity)
	}

	return r, g, b, a
}

// SampleWithScale selects mipmap level based on scale if mipmaps available.
//
// The scale parameter represents the ratio of displayed size to original size:
//   - scale = 1.0: original size
//   - scale = 0.5: half size (select mipmap level 1)
//   - scale = 0.25: quarter size (select mipmap level 2)
//
// Falls back to Sample if no mipmaps are available or scale >= 1.0.
func (p *ImagePattern) SampleWithScale(x, y, scale float64) (r, g, b, a byte) {
	if p == nil || p.image == nil {
		return 0, 0, 0, 0
	}

	// Use mipmaps if available and downscaling
	var img *ImageBuf
	if p.mipmaps != nil && scale < 1.0 {
		img = p.mipmaps.LevelForScale(scale)
	}

	// Fall back to original image if no suitable mipmap
	if img == nil {
		img = p.image
	}

	// Apply inverse transform to get image coordinates
	u, v := p.inverse.TransformPoint(x, y)

	// Apply spread mode
	u, v = p.applySpreadMode(u, v)

	// Sample the selected image with interpolation
	r, g, b, a = Sample(img, u, v, p.interp)

	// Apply opacity
	if p.opacity < 1.0 {
		a = byte(float64(a) * p.opacity)
	}

	return r, g, b, a
}

// applySpreadMode applies the spread mode to normalized coordinates.
func (p *ImagePattern) applySpreadMode(u, v float64) (float64, float64) {
	switch p.spreadMode {
	case SpreadPad:
		// Clamp to [0, 1]
		u = clampFloat(u, 0, 1)
		v = clampFloat(v, 0, 1)

	case SpreadRepeat:
		// Fractional part (wrap around)
		u -= math.Floor(u)
		v -= math.Floor(v)

	case SpreadReflect:
		// Mirror at boundaries
		u = reflectCoord(u)
		v = reflectCoord(v)
	}

	return u, v
}

// reflectCoord applies reflection mode to a coordinate.
// Creates a mirrored tiling effect by reflecting at each integer boundary.
func reflectCoord(t float64) float64 {
	// Get integer and fractional parts
	ti := math.Floor(t)
	tf := t - ti

	// Check if we're in an odd or even period
	period := int(ti)
	if period%2 != 0 {
		// Odd period: reflect (1 - fractional part)
		return 1.0 - tf
	}

	// Even period: use fractional part as-is
	return tf
}

// Image returns the underlying image buffer.
func (p *ImagePattern) Image() *ImageBuf {
	if p == nil {
		return nil
	}
	return p.image
}

// Transform returns the current transformation matrix.
func (p *ImagePattern) Transform() Affine {
	if p == nil {
		return Identity()
	}
	return p.transform
}

// SpreadMode returns the current spread mode.
func (p *ImagePattern) SpreadMode() SpreadMode {
	if p == nil {
		return SpreadPad
	}
	return p.spreadMode
}

// Interpolation returns the current interpolation mode.
func (p *ImagePattern) Interpolation() InterpolationMode {
	if p == nil {
		return InterpBilinear
	}
	return p.interp
}

// Opacity returns the current opacity value.
func (p *ImagePattern) Opacity() float64 {
	if p == nil {
		return 1.0
	}
	return p.opacity
}

// Mipmaps returns the mipmap chain, or nil if not set.
func (p *ImagePattern) Mipmaps() *MipmapChain {
	if p == nil {
		return nil
	}
	return p.mipmaps
}
