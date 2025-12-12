package gg

import "math"

// DrawRegularPolygon draws a regular polygon with n sides.
func (c *Context) DrawRegularPolygon(n int, x, y, r, rotation float64) {
	angle := 2.0 * math.Pi / float64(n)
	for i := 0; i < n; i++ {
		a := rotation + angle*float64(i)
		px := x + r*math.Cos(a)
		py := y + r*math.Sin(a)
		if i == 0 {
			c.MoveTo(px, py)
		} else {
			c.LineTo(px, py)
		}
	}
	c.ClosePath()
}
