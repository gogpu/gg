// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"image"
	"image/color"
)

// FillRule specifies how to determine which areas are inside a path.
type FillRule uint8

const (
	// FillRuleNonZero uses the non-zero winding rule.
	// A point is inside if the winding number is non-zero.
	FillRuleNonZero FillRule = iota

	// FillRuleEvenOdd uses the even-odd rule.
	// A point is inside if the winding number is odd.
	FillRuleEvenOdd
)

// LineCap specifies the shape of line endpoints.
type LineCap uint8

const (
	// LineCapButt specifies a flat line cap (no extension).
	LineCapButt LineCap = iota

	// LineCapRound specifies a semicircular line cap.
	LineCapRound

	// LineCapSquare specifies a square line cap (extends by half width).
	LineCapSquare
)

// LineJoin specifies the shape of line joins.
type LineJoin uint8

const (
	// LineJoinMiter specifies a sharp (mitered) join.
	LineJoinMiter LineJoin = iota

	// LineJoinRound specifies a rounded join.
	LineJoinRound

	// LineJoinBevel specifies a beveled join.
	LineJoinBevel
)

// FillStyle defines how to fill a path.
type FillStyle struct {
	// Color is the fill color.
	Color color.Color

	// Rule is the fill rule (NonZero or EvenOdd).
	Rule FillRule

	// Pattern is an optional pattern brush for complex fills.
	// When set, takes precedence over Color.
	Pattern Pattern
}

// DefaultFillStyle returns a FillStyle with default values.
// Uses black color and non-zero fill rule.
func DefaultFillStyle() FillStyle {
	return FillStyle{
		Color: color.Black,
		Rule:  FillRuleNonZero,
	}
}

// WithColor returns a copy with the specified color.
func (f FillStyle) WithColor(c color.Color) FillStyle {
	f.Color = c
	return f
}

// WithRule returns a copy with the specified fill rule.
func (f FillStyle) WithRule(r FillRule) FillStyle {
	f.Rule = r
	return f
}

// WithPattern returns a copy with the specified pattern.
func (f FillStyle) WithPattern(p Pattern) FillStyle {
	f.Pattern = p
	return f
}

// StrokeStyle defines how to stroke a path.
type StrokeStyle struct {
	// Color is the stroke color.
	Color color.Color

	// Width is the line width in pixels.
	Width float64

	// Cap is the line cap style.
	Cap LineCap

	// Join is the line join style.
	Join LineJoin

	// MiterLimit is the limit for miter joins.
	// When the miter length exceeds this, a bevel join is used instead.
	MiterLimit float64

	// DashPattern defines the dash/gap pattern.
	// nil or empty means solid line.
	DashPattern []float64

	// DashOffset is the starting offset into the dash pattern.
	DashOffset float64

	// Pattern is an optional pattern brush for complex strokes.
	// When set, takes precedence over Color.
	Pattern Pattern
}

// DefaultStrokeStyle returns a StrokeStyle with default values.
// Uses black color, 1px width, butt caps, miter joins.
func DefaultStrokeStyle() StrokeStyle {
	return StrokeStyle{
		Color:      color.Black,
		Width:      1.0,
		Cap:        LineCapButt,
		Join:       LineJoinMiter,
		MiterLimit: 4.0,
	}
}

// WithColor returns a copy with the specified color.
func (s StrokeStyle) WithColor(c color.Color) StrokeStyle {
	s.Color = c
	return s
}

// WithWidth returns a copy with the specified width.
func (s StrokeStyle) WithWidth(w float64) StrokeStyle {
	s.Width = w
	return s
}

// WithCap returns a copy with the specified cap style.
func (s StrokeStyle) WithCap(lineCap LineCap) StrokeStyle {
	s.Cap = lineCap
	return s
}

// WithJoin returns a copy with the specified join style.
func (s StrokeStyle) WithJoin(join LineJoin) StrokeStyle {
	s.Join = join
	return s
}

// WithMiterLimit returns a copy with the specified miter limit.
func (s StrokeStyle) WithMiterLimit(limit float64) StrokeStyle {
	s.MiterLimit = limit
	return s
}

// WithDash returns a copy with the specified dash pattern.
func (s StrokeStyle) WithDash(pattern []float64, offset float64) StrokeStyle {
	s.DashPattern = pattern
	s.DashOffset = offset
	return s
}

// WithPattern returns a copy with the specified pattern.
func (s StrokeStyle) WithPattern(p Pattern) StrokeStyle {
	s.Pattern = p
	return s
}

// IsDashed returns true if this style has a dash pattern.
func (s StrokeStyle) IsDashed() bool {
	return len(s.DashPattern) > 0
}

// DrawImageOptions defines options for drawing images.
type DrawImageOptions struct {
	// SrcRect is the source rectangle within the image.
	// If nil, the entire image is used.
	SrcRect *image.Rectangle

	// DstRect is the destination rectangle on the surface.
	// If nil, the image is drawn at At with its original size.
	DstRect *image.Rectangle

	// Alpha is the opacity (0.0 = transparent, 1.0 = opaque).
	// Default: 1.0
	Alpha float64

	// Filter is the interpolation mode for scaling.
	Filter Filter
}

// DefaultDrawImageOptions returns DrawImageOptions with default values.
func DefaultDrawImageOptions() *DrawImageOptions {
	return &DrawImageOptions{
		Alpha:  1.0,
		Filter: FilterNearest,
	}
}

// Filter specifies the interpolation mode for image scaling.
type Filter uint8

const (
	// FilterNearest uses nearest-neighbor interpolation.
	FilterNearest Filter = iota

	// FilterBilinear uses bilinear interpolation.
	FilterBilinear
)

// Pattern is a color source that can vary across the surface.
// Used for gradients and image patterns.
type Pattern interface {
	// ColorAt returns the color at the given coordinates.
	ColorAt(x, y float64) color.Color
}

// SolidPattern is a pattern that returns a single color.
type SolidPattern struct {
	Color color.Color
}

// ColorAt implements Pattern.
func (p SolidPattern) ColorAt(_, _ float64) color.Color {
	return p.Color
}

// Point represents a 2D point with float64 coordinates.
type Point struct {
	X, Y float64
}

// Pt creates a Point from x, y coordinates.
func Pt(x, y float64) Point {
	return Point{X: x, Y: y}
}

// Options configures surface creation.
type Options struct {
	// Width is the surface width in pixels.
	Width int

	// Height is the surface height in pixels.
	Height int

	// Antialias enables anti-aliased rendering.
	// Default: true
	Antialias bool

	// BackgroundColor is the initial background color.
	// Default: transparent
	BackgroundColor color.Color

	// Custom options for specific backends.
	Custom map[string]any
}

// DefaultOptions returns Options with default values.
func DefaultOptions(width, height int) Options {
	return Options{
		Width:     width,
		Height:    height,
		Antialias: true,
	}
}
