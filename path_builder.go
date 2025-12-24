// path_builder.go

package gg

import "math"

// PathBuilder provides a fluent interface for path construction.
// All methods return the builder for chaining.
type PathBuilder struct {
	path *Path
}

// BuildPath starts a new path builder.
func BuildPath() *PathBuilder {
	return &PathBuilder{path: NewPath()}
}

// MoveTo moves to a new position.
func (b *PathBuilder) MoveTo(x, y float64) *PathBuilder {
	b.path.MoveTo(x, y)
	return b
}

// LineTo draws a line to a position.
func (b *PathBuilder) LineTo(x, y float64) *PathBuilder {
	b.path.LineTo(x, y)
	return b
}

// QuadTo draws a quadratic Bezier curve.
func (b *PathBuilder) QuadTo(cx, cy, x, y float64) *PathBuilder {
	b.path.QuadraticTo(cx, cy, x, y)
	return b
}

// CubicTo draws a cubic Bezier curve.
func (b *PathBuilder) CubicTo(c1x, c1y, c2x, c2y, x, y float64) *PathBuilder {
	b.path.CubicTo(c1x, c1y, c2x, c2y, x, y)
	return b
}

// Close closes the current subpath.
func (b *PathBuilder) Close() *PathBuilder {
	b.path.Close()
	return b
}

// Rect adds a rectangle to the path.
func (b *PathBuilder) Rect(x, y, w, h float64) *PathBuilder {
	b.path.MoveTo(x, y)
	b.path.LineTo(x+w, y)
	b.path.LineTo(x+w, y+h)
	b.path.LineTo(x, y+h)
	b.path.Close()
	return b
}

// RoundRect adds a rounded rectangle to the path.
func (b *PathBuilder) RoundRect(x, y, w, h, r float64) *PathBuilder {
	// Clamp radius
	r = min(r, min(w, h)/2)
	k := 0.5522847498 * r // Control point distance for circle approximation

	b.path.MoveTo(x+r, y)
	b.path.LineTo(x+w-r, y)
	b.path.CubicTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r)
	b.path.LineTo(x+w, y+h-r)
	b.path.CubicTo(x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h)
	b.path.LineTo(x+r, y+h)
	b.path.CubicTo(x+r-k, y+h, x, y+h-r+k, x, y+h-r)
	b.path.LineTo(x, y+r)
	b.path.CubicTo(x, y+r-k, x+r-k, y, x+r, y)
	b.path.Close()
	return b
}

// Circle adds a circle to the path.
func (b *PathBuilder) Circle(cx, cy, r float64) *PathBuilder {
	return b.Ellipse(cx, cy, r, r)
}

// Ellipse adds an ellipse to the path.
func (b *PathBuilder) Ellipse(cx, cy, rx, ry float64) *PathBuilder {
	kx := 0.5522847498 * rx
	ky := 0.5522847498 * ry

	b.path.MoveTo(cx+rx, cy)
	b.path.CubicTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
	b.path.CubicTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
	b.path.CubicTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
	b.path.CubicTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	b.path.Close()
	return b
}

// Polygon adds a regular polygon to the path.
func (b *PathBuilder) Polygon(cx, cy, radius float64, sides int) *PathBuilder {
	if sides < 3 {
		return b
	}

	angleStep := 2 * math.Pi / float64(sides)
	startAngle := -math.Pi / 2 // Start at top

	for i := 0; i < sides; i++ {
		angle := startAngle + float64(i)*angleStep
		x := cx + radius*math.Cos(angle)
		y := cy + radius*math.Sin(angle)
		if i == 0 {
			b.path.MoveTo(x, y)
		} else {
			b.path.LineTo(x, y)
		}
	}
	b.path.Close()
	return b
}

// Star adds a star shape to the path.
func (b *PathBuilder) Star(cx, cy, outerRadius, innerRadius float64, points int) *PathBuilder {
	if points < 3 {
		return b
	}

	angleStep := math.Pi / float64(points)
	startAngle := -math.Pi / 2

	for i := 0; i < points*2; i++ {
		angle := startAngle + float64(i)*angleStep
		r := outerRadius
		if i%2 == 1 {
			r = innerRadius
		}
		x := cx + r*math.Cos(angle)
		y := cy + r*math.Sin(angle)
		if i == 0 {
			b.path.MoveTo(x, y)
		} else {
			b.path.LineTo(x, y)
		}
	}
	b.path.Close()
	return b
}

// Build returns the constructed path.
func (b *PathBuilder) Build() *Path {
	return b.path
}

// Path returns the constructed path (alias for Build).
func (b *PathBuilder) Path() *Path {
	return b.path
}
