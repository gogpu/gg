package gg

import (
	"github.com/gogpu/gg/internal/path"
	"github.com/gogpu/gg/internal/raster"
)

// RenderMode specifies which anti-aliasing algorithm to use.
type RenderMode int

const (
	// RenderModeSupersampled uses 4x supersampling for anti-aliasing (default).
	// This is the current stable implementation.
	RenderModeSupersampled RenderMode = iota

	// RenderModeAnalytic uses exact geometric coverage calculation.
	// This provides higher quality anti-aliasing without supersampling overhead.
	// Note: Analytic mode requires importing backend/native and calling
	// SetAnalyticFiller to configure the analytic rendering components.
	RenderModeAnalytic
)

// AnalyticFillerInterface defines the interface for analytic coverage calculation.
// This allows the analytic filler from backend/native to be injected without
// creating an import cycle.
type AnalyticFillerInterface interface {
	// Fill renders the path using analytic coverage calculation.
	// Parameters:
	//   - path: the gg.Path to render
	//   - fillRule: FillRuleNonZero or FillRuleEvenOdd
	//   - callback: called for each scanline with (y, x, alpha) values
	Fill(path *Path, fillRule FillRule, callback func(y int, iter func(yield func(x int, alpha uint8) bool)))
	// Reset clears the filler state for reuse.
	Reset()
}

// SoftwareRenderer is a CPU-based scanline rasterizer.
type SoftwareRenderer struct {
	rasterizer *raster.Rasterizer

	// Render mode selection
	mode RenderMode

	// Analytic AA components (optional, injected via SetAnalyticFiller)
	analyticFiller AnalyticFillerInterface

	// Dimensions for analytic filler
	width, height int
}

// NewSoftwareRenderer creates a new software renderer.
// The default render mode is RenderModeSupersampled (4x supersampling).
// For higher quality, call SetAnalyticFiller with an analytic filler instance.
func NewSoftwareRenderer(width, height int) *SoftwareRenderer {
	return &SoftwareRenderer{
		rasterizer: raster.NewRasterizer(width, height),
		mode:       RenderModeSupersampled,
		width:      width,
		height:     height,
	}
}

// SetRenderMode sets the anti-aliasing mode.
// RenderModeSupersampled (default) uses 4x supersampling.
// RenderModeAnalytic uses exact geometric coverage calculation (requires SetAnalyticFiller).
func (r *SoftwareRenderer) SetRenderMode(mode RenderMode) {
	r.mode = mode
}

// RenderMode returns the current anti-aliasing mode.
func (r *SoftwareRenderer) RenderMode() RenderMode {
	return r.mode
}

// SetAnalyticFiller configures the analytic filler for RenderModeAnalytic.
// This must be called before using RenderModeAnalytic.
// The filler is typically created from backend/native.NewAnalyticFillerAdapter.
func (r *SoftwareRenderer) SetAnalyticFiller(filler AnalyticFillerInterface) {
	r.analyticFiller = filler
	if filler != nil {
		r.mode = RenderModeAnalytic
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
// The rendering method is determined by the current RenderMode.
func (r *SoftwareRenderer) Fill(pixmap *Pixmap, p *Path, paint *Paint) error {
	switch r.mode {
	case RenderModeAnalytic:
		if r.analyticFiller != nil {
			return r.fillAnalytic(pixmap, p, paint)
		}
		// Fallback to supersampled if no analytic filler configured
		return r.fillSupersampled(pixmap, p, paint)
	case RenderModeSupersampled:
		return r.fillSupersampled(pixmap, p, paint)
	default:
		return r.fillSupersampled(pixmap, p, paint)
	}
}

// fillAnalytic renders the path using analytic coverage calculation.
// This provides high quality anti-aliasing without supersampling overhead.
func (r *SoftwareRenderer) fillAnalytic(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Get color from paint
	color := r.getColorFromPaint(paint)

	// Reset the filler for new path
	r.analyticFiller.Reset()

	// Fill using the analytic filler interface
	r.analyticFiller.Fill(p, paint.FillRule, func(y int, iter func(yield func(x int, alpha uint8) bool)) {
		// Blend alpha values to pixmap
		r.blendAlphaRunsFromIter(pixmap, y, iter, color)
	})

	return nil
}

// fillSupersampled renders the path using 4x supersampling (legacy method).
func (r *SoftwareRenderer) fillSupersampled(pixmap *Pixmap, p *Path, paint *Paint) error {
	// Convert path to internal format and flatten
	elements := convertPath(p)
	flattenedPath := path.Flatten(elements)
	rasterPoints := convertPoints(flattenedPath)

	// Get color from paint
	color := r.getColorFromPaint(paint)

	// Convert fill rule
	fillRule := raster.FillRuleNonZero
	if paint.FillRule == FillRuleEvenOdd {
		fillRule = raster.FillRuleEvenOdd
	}

	// Rasterize with anti-aliasing (4x supersampling)
	adapter := &pixmapAdapter{pixmap: pixmap}
	r.rasterizer.FillAA(adapter, rasterPoints, fillRule, raster.RGBA{
		R: color.R,
		G: color.G,
		B: color.B,
		A: color.A,
	})

	return nil
}

// getColorFromPaint extracts the solid color from the paint.
// Returns Black if no solid pattern is found.
func (r *SoftwareRenderer) getColorFromPaint(paint *Paint) RGBA {
	solidPattern, ok := paint.Pattern.(*SolidPattern)
	if !ok {
		return Black
	}
	return solidPattern.Color
}

// blendAlphaRunsFromIter blends alpha values to the pixmap for a given scanline.
// Uses source-over compositing for proper alpha blending.
func (r *SoftwareRenderer) blendAlphaRunsFromIter(pixmap *Pixmap, y int, iter func(yield func(x int, alpha uint8) bool), color RGBA) {
	// Skip if y is out of bounds
	if y < 0 || y >= pixmap.Height() {
		return
	}

	// Iterate over the alpha values
	iter(func(x int, alpha uint8) bool {
		if alpha == 0 {
			return true // continue
		}
		if x < 0 || x >= pixmap.Width() {
			return true // continue
		}

		// Full coverage - just set the pixel
		if alpha == 255 && color.A == 1.0 {
			pixmap.SetPixel(x, y, color)
			return true // continue
		}

		// Partial coverage - blend with existing pixel
		existing := pixmap.GetPixel(x, y)

		// Calculate effective source alpha (color alpha * coverage)
		srcAlpha := color.A * float64(alpha) / 255.0
		invSrcAlpha := 1.0 - srcAlpha

		// Source-over compositing
		outA := srcAlpha + existing.A*invSrcAlpha
		if outA > 0 {
			outR := (color.R*srcAlpha + existing.R*existing.A*invSrcAlpha) / outA
			outG := (color.G*srcAlpha + existing.G*existing.A*invSrcAlpha) / outA
			outB := (color.B*srcAlpha + existing.B*existing.A*invSrcAlpha) / outA
			pixmap.SetPixel(x, y, RGBA{R: outR, G: outG, B: outB, A: outA})
		}
		return true // continue
	})
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
