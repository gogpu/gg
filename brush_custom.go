package gg

import "math"

// ColorFunc is a function that returns a color at a given position.
// Used by CustomBrush to define custom brush patterns.
type ColorFunc func(x, y float64) RGBA

// CustomBrush is a brush with a user-defined color function.
// It allows for arbitrary patterns, gradients, and procedural textures.
//
// CustomBrush implements the Brush interface, making it compatible
// with all brush-based operations.
//
// Example:
//
//	// Create a checkerboard pattern
//	checker := gg.NewCustomBrush(func(x, y float64) gg.RGBA {
//	    if (int(x/10)+int(y/10))%2 == 0 {
//	        return gg.Black
//	    }
//	    return gg.White
//	})
type CustomBrush struct {
	// Func is the color function that determines the color at each point.
	Func ColorFunc

	// Name is an optional identifier for debugging and logging.
	Name string
}

// brushMarker implements the sealed Brush interface.
func (CustomBrush) brushMarker() {}

// ColorAt implements Brush. Returns the color from the custom function.
func (b CustomBrush) ColorAt(x, y float64) RGBA {
	if b.Func == nil {
		return Transparent
	}
	return b.Func(x, y)
}

// NewCustomBrush creates a CustomBrush from a color function.
//
// Example:
//
//	// Horizontal gradient from red to blue
//	gradient := gg.NewCustomBrush(func(x, y float64) gg.RGBA {
//	    t := x / 100.0 // Assuming 100px width
//	    return gg.Red.Lerp(gg.Blue, t)
//	})
func NewCustomBrush(fn ColorFunc) CustomBrush {
	return CustomBrush{Func: fn}
}

// WithName returns a new CustomBrush with the specified name.
// Useful for debugging and logging.
//
// Example:
//
//	brush := gg.NewCustomBrush(myFunc).WithName("myPattern")
func (b CustomBrush) WithName(name string) CustomBrush {
	return CustomBrush{
		Func: b.Func,
		Name: name,
	}
}

// HorizontalGradient creates a linear gradient from left to right.
// x0 and x1 define the gradient range in pixel coordinates.
//
// Example:
//
//	gradient := gg.HorizontalGradient(gg.Red, gg.Blue, 0, 100)
func HorizontalGradient(c0, c1 RGBA, x0, x1 float64) CustomBrush {
	return CustomBrush{
		Func: func(x, _ float64) RGBA {
			t := (x - x0) / (x1 - x0)
			t = clampT(t)
			return c0.Lerp(c1, t)
		},
		Name: "horizontal_gradient",
	}
}

// VerticalGradient creates a linear gradient from top to bottom.
// y0 and y1 define the gradient range in pixel coordinates.
//
// Example:
//
//	gradient := gg.VerticalGradient(gg.White, gg.Black, 0, 100)
func VerticalGradient(c0, c1 RGBA, y0, y1 float64) CustomBrush {
	return CustomBrush{
		Func: func(_, y float64) RGBA {
			t := (y - y0) / (y1 - y0)
			t = clampT(t)
			return c0.Lerp(c1, t)
		},
		Name: "vertical_gradient",
	}
}

// LinearGradient creates a linear gradient along an arbitrary line.
// The gradient is defined from point (x0, y0) to point (x1, y1).
//
// Example:
//
//	// Diagonal gradient from top-left to bottom-right
//	gradient := gg.LinearGradient(gg.Red, gg.Blue, 0, 0, 100, 100)
func LinearGradient(c0, c1 RGBA, x0, y0, x1, y1 float64) CustomBrush {
	dx := x1 - x0
	dy := y1 - y0
	length := math.Sqrt(dx*dx + dy*dy)
	if length == 0 {
		return Solid(c0).toCustomBrush()
	}

	// Normalize direction
	nx := dx / length
	ny := dy / length

	return CustomBrush{
		Func: func(x, y float64) RGBA {
			// Project point onto gradient line
			px := x - x0
			py := y - y0
			t := (px*nx + py*ny) / length
			t = clampT(t)
			return c0.Lerp(c1, t)
		},
		Name: "linear_gradient",
	}
}

// RadialGradient creates a radial gradient from center outward.
// The gradient is defined from the center (cx, cy) with radius r.
// c0 is the center color, c1 is the edge color.
//
// Example:
//
//	// White center fading to black at radius 50
//	gradient := gg.RadialGradient(gg.White, gg.Black, 50, 50, 50)
func RadialGradient(c0, c1 RGBA, cx, cy, r float64) CustomBrush {
	if r <= 0 {
		return Solid(c0).toCustomBrush()
	}

	return CustomBrush{
		Func: func(x, y float64) RGBA {
			dx := x - cx
			dy := y - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			t := dist / r
			t = clampT(t)
			return c0.Lerp(c1, t)
		},
		Name: "radial_gradient",
	}
}

// Checkerboard creates a checkerboard pattern brush.
// size is the size of each square in pixels.
//
// Example:
//
//	checker := gg.Checkerboard(gg.Black, gg.White, 10)
func Checkerboard(c0, c1 RGBA, size float64) CustomBrush {
	if size <= 0 {
		size = 1
	}

	return CustomBrush{
		Func: func(x, y float64) RGBA {
			xi := int(math.Floor(x / size))
			yi := int(math.Floor(y / size))
			if (xi+yi)%2 == 0 {
				return c0
			}
			return c1
		},
		Name: "checkerboard",
	}
}

// Stripes creates a striped pattern brush.
// width is the stripe width, angle is the rotation in radians.
//
// Example:
//
//	// Vertical stripes
//	stripes := gg.Stripes(gg.Red, gg.White, 10, 0)
//
//	// Diagonal stripes (45 degrees)
//	diag := gg.Stripes(gg.Blue, gg.Yellow, 10, math.Pi/4)
func Stripes(c0, c1 RGBA, width, angle float64) CustomBrush {
	if width <= 0 {
		width = 1
	}

	cos := math.Cos(angle)
	sin := math.Sin(angle)

	return CustomBrush{
		Func: func(x, y float64) RGBA {
			// Rotate coordinate
			rx := x*cos + y*sin
			// Determine stripe
			stripe := int(math.Floor(rx / width))
			if stripe%2 == 0 {
				return c0
			}
			return c1
		},
		Name: "stripes",
	}
}

// clampT clamps a value to [0, 1] range.
func clampT(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// toCustomBrush converts a SolidBrush to CustomBrush.
// Used internally for edge cases in gradient constructors.
func (b SolidBrush) toCustomBrush() CustomBrush {
	c := b.Color
	return CustomBrush{
		Func: func(_, _ float64) RGBA { return c },
		Name: "solid",
	}
}
