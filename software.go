package gg

import (
	"github.com/gogpu/gg/internal/path"
	"github.com/gogpu/gg/internal/raster"
)

// SoftwareRenderer is a CPU-based scanline rasterizer.
type SoftwareRenderer struct {
	rasterizer *raster.Rasterizer
}

// NewSoftwareRenderer creates a new software renderer.
func NewSoftwareRenderer(width, height int) *SoftwareRenderer {
	return &SoftwareRenderer{
		rasterizer: raster.NewRasterizer(width, height),
	}
}

// pixmapAdapter adapts gg.Pixmap to raster.Pixmap interface.
type pixmapAdapter struct {
	pixmap *Pixmap
}

func (p *pixmapAdapter) Width() int {
	return p.pixmap.Width()
}

func (p *pixmapAdapter) Height() int {
	return p.pixmap.Height()
}

func (p *pixmapAdapter) SetPixel(x, y int, c raster.RGBA) {
	p.pixmap.SetPixel(x, y, RGBA{R: c.R, G: c.G, B: c.B, A: c.A})
}

// convertPath converts gg.Path elements to path.PathElement for flattening.
func convertPath(p *Path) []path.PathElement {
	var elements []path.PathElement
	for _, elem := range p.Elements() {
		switch e := elem.(type) {
		case MoveTo:
			elements = append(elements, path.MoveTo{Point: path.Point{X: e.Point.X, Y: e.Point.Y}})
		case LineTo:
			elements = append(elements, path.LineTo{Point: path.Point{X: e.Point.X, Y: e.Point.Y}})
		case QuadTo:
			elements = append(elements, path.QuadTo{
				Control: path.Point{X: e.Control.X, Y: e.Control.Y},
				Point:   path.Point{X: e.Point.X, Y: e.Point.Y},
			})
		case CubicTo:
			elements = append(elements, path.CubicTo{
				Control1: path.Point{X: e.Control1.X, Y: e.Control1.Y},
				Control2: path.Point{X: e.Control2.X, Y: e.Control2.Y},
				Point:    path.Point{X: e.Point.X, Y: e.Point.Y},
			})
		case Close:
			elements = append(elements, path.Close{})
		}
	}
	return elements
}

// convertPoints converts path.Point to raster.Point.
func convertPoints(points []path.Point) []raster.Point {
	result := make([]raster.Point, len(points))
	for i, p := range points {
		result[i] = raster.Point{X: p.X, Y: p.Y}
	}
	return result
}

// Fill implements Renderer.Fill.
func (r *SoftwareRenderer) Fill(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Convert path to internal format and flatten
	elements := convertPath(p)
	flattenedPath := path.Flatten(elements)
	rasterPoints := convertPoints(flattenedPath)

	// Get color from paint
	solidPattern, ok := paint.Pattern.(*SolidPattern)
	if !ok {
		return nil // Only solid patterns supported in v0.1
	}
	color := solidPattern.Color

	// Convert fill rule
	fillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		fillRule = raster.FillRuleEvenOdd
	}

	// Rasterize
	adapter := &pixmapAdapter{pixmap: pixmap}
	r.rasterizer.Fill(adapter, rasterPoints, fillRule, raster.RGBA{
		R: color.R,
		G: color.G,
		B: color.B,
		A: color.A,
	})

	return nil
}

// Stroke implements Renderer.Stroke.
func (r *SoftwareRenderer) Stroke(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Convert path to internal format and flatten
	elements := convertPath(p)
	flattenedPath := path.Flatten(elements)
	rasterPoints := convertPoints(flattenedPath)

	// Get color from paint
	solidPattern, ok := paint.Pattern.(*SolidPattern)
	if !ok {
		return nil // Only solid patterns supported in v0.1
	}
	color := solidPattern.Color

	// Rasterize stroke
	adapter := &pixmapAdapter{pixmap: pixmap}
	r.rasterizer.Stroke(adapter, rasterPoints, paint.LineWidth, raster.RGBA{
		R: color.R,
		G: color.G,
		B: color.B,
		A: color.A,
	})

	return nil
}
