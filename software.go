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

// BlendPixelAlpha blends a color with the existing pixel using given alpha.
// This implements the raster.AAPixmap interface for anti-aliased rendering.
func (p *pixmapAdapter) BlendPixelAlpha(x, y int, c raster.RGBA, alpha uint8) {
	if alpha == 0 {
		return
	}

	// Bounds check
	if x < 0 || x >= p.pixmap.Width() || y < 0 || y >= p.pixmap.Height() {
		return
	}

	if alpha == 255 {
		p.pixmap.SetPixel(x, y, RGBA{R: c.R, G: c.G, B: c.B, A: c.A})
		return
	}

	// Get existing pixel
	existing := p.pixmap.GetPixel(x, y)

	// Calculate blend factor
	srcAlpha := c.A * float64(alpha) / 255.0
	invSrcAlpha := 1.0 - srcAlpha

	// Source-over compositing
	outA := srcAlpha + existing.A*invSrcAlpha
	if outA > 0 {
		outR := (c.R*srcAlpha + existing.R*existing.A*invSrcAlpha) / outA
		outG := (c.G*srcAlpha + existing.G*existing.A*invSrcAlpha) / outA
		outB := (c.B*srcAlpha + existing.B*existing.A*invSrcAlpha) / outA
		p.pixmap.SetPixel(x, y, RGBA{R: outR, G: outG, B: outB, A: outA})
	}
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

// Fill implements Renderer.Fill with anti-aliasing enabled by default.
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

	// Rasterize with anti-aliasing
	adapter := &pixmapAdapter{pixmap: pixmap}
	r.rasterizer.FillAA(adapter, rasterPoints, fillRule, raster.RGBA{
		R: color.R,
		G: color.G,
		B: color.B,
		A: color.A,
	})

	return nil
}

// FillNoAA fills without anti-aliasing (faster but aliased).
func (r *SoftwareRenderer) FillNoAA(pixmap *Pixmap, p *Path, paint *Paint) error {
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

	// Rasterize without AA
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
